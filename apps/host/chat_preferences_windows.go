//go:build windows

package main

import (
	"context"
	"errors"
	"strings"
	"time"
	"winone/internal/ai"
	"winone/internal/chat"
)

// Retire idle project runtimes before changing the shared permission. A busy
// member blocks the change; no active run retains a silently stale policy.
func (c *chatControl) configureProjectPermission(ctx context.Context, id, permission string) (chat.Catalog, error) {
	snapshot := c.store.Snapshot()
	for _, conversation := range snapshot.Conversations {
		if conversation.ProjectID != id {
			continue
		}
		page, err := c.engine.Poll(conversation.ID, 0)
		if err == nil && page.Running {
			return snapshot, errors.New("请先停止此项目中正在运行的任务")
		}
	}
	for _, conversation := range snapshot.Conversations {
		if conversation.ProjectID != id {
			continue
		}
		if _, err := c.engine.ConfigureConversation(ctx, conversation.ID, func(*chat.Conversation) error { return nil }); err != nil {
			return snapshot, err
		}
	}
	if _, err := c.store.UpdateProjectPermission(id, permission); err != nil {
		return snapshot, err
	}
	return c.store.Snapshot(), nil
}

// Title generation uses the same saved provider, credential rotation and
// account gate as normal chat. It never gains tool or filesystem access.
func (c *chatControl) generateTitle(ctx context.Context, id, text string, requested ...bool) (result chat.Conversation, err error) {
	conversation, err := c.store.Conversation(id)
	if err != nil {
		return conversation, err
	}
	explicit := len(requested) > 0 && requested[0]
	if !explicit && (conversation.TitleManual || conversation.Title != "Chat") {
		return conversation, nil
	}
	// Keep the original provider error observable while persisting the requested
	// text fallback. The store checks the original title before writing it.
	defer func() {
		if err != nil {
			fallback, fallbackErr := c.store.ApplyFallbackTitle(conversation, text, explicit)
			if fallbackErr == nil {
				result = fallback
			} else {
				err = errors.Join(err, fallbackErr)
			}
		}
	}()
	selection := c.store.Preferences().TitleModel
	if selection.Provider == "" {
		selection = conversation.Model
	}
	if _, err = c.aiConfiguration(); err != nil {
		return conversation, err
	}
	c.ai.mu.Lock()
	preferences, plan, err := c.ai.store.ForTaskKeys("chat", &selection)
	store := c.ai.store
	runtime := c.ai.translator
	c.ai.mu.Unlock()
	if err != nil {
		return conversation, err
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	prompt, err := c.store.TitlePrompt(id, text, c.language(), explicit)
	if err != nil {
		return conversation, err
	}
	// Cindy's complete one-shot prompt goes in the request itself. The quoted
	// task can never become an unadorned user turn in the dialogue worker.
	input := ai.Generation{Text: prompt}
	var title string
	run := func(callCtx context.Context, key string) (bool, error) {
		var failure error
		title, failure = generateTitleText(callCtx, func(attemptCtx context.Context) (string, error) {
			var output strings.Builder
			// The stream boundary preserves typed HTTP status/Retry-After and
			// whether output began. The title remains atomic to the desktop.
			_, err := runtime.ForProvider(preferences.ProviderID).GenerateStream(attemptCtx, preferences, key, input, func(delta ai.Delta) error {
				output.WriteString(delta.Text)
				return nil
			})
			return output.String(), err
		})
		return title != "", failure
	}
	if preferences.AuthMode == "account" {
		gate := c.ai.accountGateFor(preferences.ProviderID)
		select {
		case gate <- struct{}{}:
			_, err = run(ctx, "")
			<-gate
		case <-ctx.Done():
			err = ctx.Err()
		}
	} else {
		err = store.WithKeys(ctx, plan, run)
	}
	if err != nil {
		return conversation, err
	}
	title, err = chat.ValidateTitleOutput(title)
	if err != nil {
		return conversation, err
	}
	if explicit {
		return c.store.ApplyRequestedTitle(conversation, title)
	}
	return c.store.ApplyGeneratedTitle(id, title)
}

// Retry one transient, unanswered provider request within the original title
// deadline. Authentication, partial results and invalid titles never replay.
// Credential rotation remains owned by WithKeys outside this per-key attempt.
func generateTitleText(ctx context.Context, generate func(context.Context) (string, error)) (string, error) {
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		text, err := generate(ctx)
		if ctx.Err() != nil {
			return text, ctx.Err()
		}
		var failure *ai.AttemptFailure
		if err == nil || text != "" || attempt >= 1 || !errors.As(err, &failure) || !(failure.Status == 408 || failure.Status == 429 || failure.Status >= 500 && failure.Status <= 599) {
			return text, err
		}
		delay := max(500*time.Millisecond, failure.RetryAfter)
		if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) <= delay {
			return text, err
		}
		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return text, ctx.Err()
		}
	}
}
