//go:build windows

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"winone/internal/ai"
	"winone/internal/chat"
)

// oneBotBuiltinRoutinePrefix is Cindy's BUILTIN_ROUTINE_SCHEDULE_PREFIX: the
// schedule id of a built-in rule (`routine-builtin-follow-up-<bot>` and so on).
const oneBotBuiltinRoutinePrefix = "routine-builtin-"

// ownsSession reports whether a conversation lives in the teammates' store.
func (b *chatOneBot) ownsSession(id string) bool {
	b.startMu.Lock()
	sessions := b.sessions
	b.startMu.Unlock()
	if sessions == nil {
		return false
	}
	_, err := sessions.store.Conversation(id)
	return err == nil
}

// routinePolicy is schedule_notify / schedule_silence for the run currently
// executing in a teammate's chat: the Worker's routine scheduler checks that
// this accepted request is that run before changing its report policy.
func (b *chatOneBot) routinePolicy(ctx context.Context, id, action string) (json.RawMessage, int, error) {
	requestID := b.sessions.engine.ActiveRequest(id)
	if requestID == "" {
		return nil, 409, errors.New("No active automation turn")
	}
	runner, err := b.ensure(ctx)
	if err != nil {
		return nil, 500, err
	}
	value, err := runner.Call(ctx, "routines.policy", map[string]string{"sessionId": id, "requestId": requestID, "action": action})
	if err != nil {
		return nil, 409, err
	}
	return value, 200, nil
}

const oneBotRoutineRoutePrefix = "routine:"

// routineLane is Cindy ensureRoutineSession: a hidden task for a routine that
// asks for isolated context or its own model. It inherits the teammate's
// permissions; the model override changes only this lane.
func (b *chatOneBot) routineLane(ctx context.Context, raw json.RawMessage) (map[string]any, error) {
	var input struct {
		BotID, RoutineID, Title string
		ModelOverride           *struct {
			Model      string `json:"model"`
			ProviderID string `json:"providerId"`
			Effort     string `json:"effort"`
			FastMode   *bool  `json:"fastMode"`
		}
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	if input.BotID == "" || input.RoutineID == "" {
		return nil, errors.New("例行任务专线无效")
	}
	conversation, err := b.openRoute(ctx, input.BotID, oneBotRoutineRoutePrefix+input.RoutineID, "thread", input.Title)
	if err != nil {
		return map[string]any{"ok": false, "message": err.Error()}, nil
	}
	if override := input.ModelOverride; override != nil && override.Model != "" {
		conversation, err = b.sessions.store.UpdateConversation(conversation.ID, func(c *chat.Conversation) error {
			c.Model = ai.Selection{Provider: override.ProviderID, Model: override.Model}
			if override.Effort != "" {
				c.ThinkingLevel = override.Effort
			}
			if override.FastMode != nil {
				c.FastMode = *override.FastMode
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return map[string]any{"ok": true, "sessionId": conversation.ID}, nil
}

// discardRoutineLane archives a fresh lane no model turn ever ran in, so skipped
// or deferred runs leave no empty tasks behind.
func (b *chatOneBot) discardRoutineLane(raw json.RawMessage) (map[string]bool, error) {
	var input struct{ SessionID string }
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	if _, err := b.sessions.store.UpdateConversation(input.SessionID, func(c *chat.Conversation) error {
		c.Archived = true
		return nil
	}); err != nil {
		return nil, err
	}
	return map[string]bool{"ok": true}, nil
}

// authorizeRoutine is a private Worker callback, not a desktop or model method.
// A saved standing instruction can run only in its current canonical chat, with
// that chat's existing model and permission mode. The exact prompt is receipted.
func (b *chatOneBot) authorizeRoutine(ctx context.Context, raw json.RawMessage) (map[string]bool, error) {
	var input struct {
		SessionID  string `json:"sessionId"`
		RunID      string `json:"runId"`
		Prompt     string `json:"prompt"`
		ScheduleID string `json:"scheduleId"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	if input.SessionID == "" || input.RunID == "" || input.Prompt == "" || len(input.Prompt) > 256<<10 {
		return nil, errors.New("例行任务输入无效")
	}
	value, err := b.contextFor(ctx, map[string]string{"sessionId": input.SessionID})
	if err != nil {
		return nil, err
	}
	// A routine runs in the teammate's main chat, or in the hidden lane its own
	// settings asked for; never in another route.
	if (value.Role != "canonical" || value.CanonicalSessionID != input.SessionID) && !strings.HasPrefix(value.RouteKey, oneBotRoutineRoutePrefix) {
		return nil, errors.New("例行任务只能使用伙伴当前的主对话或自己的专线")
	}
	if _, err := b.sessions.store.Conversation(input.SessionID); err != nil {
		return nil, err
	}
	digest := sha256.Sum256([]byte(input.Prompt))
	// Cindy teammateTurnPrincipal: an owner-created routine is a standing
	// grant; built-in rules (follow-up, todo check) are Host templates and act
	// on the teammate's own initiative.
	grant := oneBotInputGrant{SessionID: input.SessionID, Owner: true, Channel: "routine", Principal: "entrusted", Digest: hex.EncodeToString(digest[:])}
	if strings.HasPrefix(input.ScheduleID, oneBotBuiltinRoutinePrefix) {
		grant.Owner, grant.Principal = false, "autonomous"
	}
	if err := b.grantInput(input.RunID, grant); err != nil {
		return nil, err
	}
	return map[string]bool{"ok": true}, nil
}
