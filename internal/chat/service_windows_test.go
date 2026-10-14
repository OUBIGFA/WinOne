//go:build windows

package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"winone/internal/ai"
)

func TestNativeConversationRelocationRetainsHistoryAndSendsMigratedAttachmentDraft(t *testing.T) {
	bun, script := os.Getenv("WINONE_CHAT_BUN_EXE"), os.Getenv("WINONE_CHAT_WORKER")
	if bun == "" || script == "" {
		t.Skip("explicit Pi runtime fixture")
	}
	root := t.TempDir()
	store, err := Open(filepath.Join(root, "profile"))
	if err != nil {
		t.Fatal(err)
	}
	c, err := store.NewConversation("", "", "Move history", ai.Selection{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	var received atomic.Bool
	service := NewService(ctx, store, ServiceOptions{Bun: bun, Worker: script, Configure: func(c Conversation) (map[string]any, func(), error) {
		bridge, err := NewPrivateBridge(ctx, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			if bytes.Contains(body, []byte("ATTACHMENT_TRANSFER_PROOF")) {
				received.Store(true)
			}
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"id\":\"test\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Read successfully\"},\"finish_reason\":null}]}\n\ndata: {\"id\":\"test\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
		}))
		if err != nil {
			return nil, nil, err
		}
		return map[string]any{"baseUrl": bridge.BaseURL, "key": bridge.Key, "agentDir": filepath.Join(root, "agent"), "model": map[string]string{"id": "test/model", "name": "Fixture"}}, bridge.Close, nil
	}})
	defer func() {
		ending, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		_ = service.Stop(ending)
	}()
	if _, err = service.Open(ctx, c.ID, 0); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Open(ctx, c.ID, 0); err != nil {
		t.Fatal(err)
	}
	live, _ := service.session(c.ID)
	service.Release(c.ID, 0)
	service.closeIdle(c.ID, live)
	if _, err = service.Command(ctx, c.ID, "state", nil); err != nil {
		t.Fatal("one execution release removed another live lease", err)
	}
	waitIdle := func() {
		t.Helper()
		cursor := uint64(0)
		for {
			page, err := service.WaitEvents(ctx, c.ID, cursor)
			if err != nil {
				t.Fatal(err)
			}
			if !page.Running {
				return
			}
			cursor = page.Cursor
		}
	}
	if _, err = service.Prompt(ctx, c.ID, "before-transfer", "First persisted message"); err != nil {
		t.Fatal(err)
	}
	waitIdle()
	c, err = store.Conversation(c.ID)
	if err != nil {
		t.Fatal(err)
	}
	oldFile := c.SessionFile
	history, err := os.ReadFile(oldFile)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "proof.txt")
	os.WriteFile(file, []byte("ATTACHMENT_TRANSFER_PROOF"), 0600)
	attachments, err := store.AddAttachments(c.ID, []string{file})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.SaveDraft(c.ID, Draft{Text: "Read the imported attachment", Attachments: attachments}); err != nil {
		t.Fatal(err)
	}
	next := filepath.Join(root, "next")
	os.Mkdir(next, 0700)
	c, err = service.ConfigureConversation(ctx, c.ID, func(value *Conversation) error { return RelocateFiles(value, next) })
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{oldFile, c.SessionFile} {
		raw, err := os.ReadFile(file)
		if err != nil || !bytes.Equal(raw, history) {
			t.Fatal("real Pi history was changed by relocation", err)
		}
	}
	draft, err := store.ReadDraft(c.ID)
	if err != nil || len(draft.Attachments) != 1 || draft.Attachments[0].Path == attachments[0].Path {
		t.Fatal("attachment draft was not migrated", draft, err)
	}
	state, err := service.Open(ctx, c.ID, 0)
	if err != nil || !bytes.Contains(state, []byte("First persisted message")) {
		t.Fatal("relocated native history failed to reopen", err)
	}
	if _, err = service.Prompt(ctx, c.ID, "after-transfer", draft.Text, []string{draft.Attachments[0].Path}); err != nil {
		t.Fatal(err)
	}
	waitIdle()
	if !received.Load() {
		t.Fatal("migrated attachment did not reach the real model request")
	}
	service.Release(c.ID, 0)
}

// Exercise the real Pi process from prompt acceptance through persisted history,
// approval, release and reopening. A fake HTTP model is the only external seam.
func TestServicePersistsApprovedWorkAndReleasesIdleRunner(t *testing.T) {
	bun, script := os.Getenv("WINONE_CHAT_BUN_EXE"), os.Getenv("WINONE_CHAT_WORKER")
	if bun == "" || script == "" {
		t.Skip("explicit Pi runtime fixture")
	}
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	os.Mkdir(workspace, 0700)
	store, err := Open(filepath.Join(root, "chat"))
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := store.NewConversation("", workspace, "Test", ai.Selection{Provider: "test", Model: "model"})
	if err != nil {
		t.Fatal(err)
	}
	conversation, err = store.UpdateConversation(conversation.ID, func(c *Conversation) error { c.Permission = "ask"; return nil })
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	service := NewService(context.Background(), store, ServiceOptions{Bun: bun, Worker: script, Configure: func(c Conversation) (map[string]any, func(), error) {
		bridge, err := NewPrivateBridge(context.Background(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			io.Copy(io.Discard, r.Body)
			call := calls.Add(1)
			delta := map[string]any{"content": "Done"}
			finish := "stop"
			if call == 1 {
				delta = map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "proof", "type": "function", "function": map[string]any{"name": "write", "arguments": `{"path":"proof.txt","content":"saved by Pi"}`}}}}
				finish = "tool_calls"
			}
			w.Header().Set("Content-Type", "text/event-stream")
			for _, part := range []map[string]any{delta, {}} {
				var reason any
				if len(part) == 0 {
					reason = finish
				}
				raw, _ := json.Marshal(map[string]any{"id": "test", "object": "chat.completion.chunk", "choices": []any{map[string]any{"index": 0, "delta": part, "finish_reason": reason}}})
				fmt.Fprintf(w, "data: %s\n\n", raw)
			}
			fmt.Fprint(w, "data: [DONE]\n\n")
		}))
		if err != nil {
			return nil, nil, err
		}
		return map[string]any{"baseUrl": bridge.BaseURL, "key": bridge.Key, "agentDir": filepath.Join(root, "agent"), "model": map[string]string{"id": "test/model", "name": "Test"}, "language": "zh-CN"}, bridge.Close, nil
	}})
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	defer service.Stop(ctx)
	if _, err = service.Open(ctx, conversation.ID, 42); err != nil {
		t.Fatal(err)
	}
	if err = store.SaveDraft(conversation.ID, Draft{Text: "create proof"}); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Prompt(ctx, conversation.ID, "request-1", "create proof"); err != nil {
		t.Fatal(err)
	}
	for {
		page, err := service.Poll(conversation.ID, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Approvals) > 0 {
			var request struct {
				ID string `json:"id"`
			}
			json.Unmarshal(page.Approvals[0], &request)
			if _, err = service.Command(ctx, conversation.ID, "permission", map[string]any{"id": request.ID, "allow": true}); err != nil {
				t.Fatal(err)
			}
		}
		if !page.Running {
			break
		}
		select {
		case <-time.After(20 * time.Millisecond):
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	content, err := os.ReadFile(filepath.Join(workspace, "proof.txt"))
	if err != nil || string(content) != "saved by Pi" {
		t.Fatal(string(content), err)
	}
	draft, err := store.ReadDraft(conversation.ID)
	if err != nil || draft.Text != "" {
		t.Fatal("input not committed", draft, err)
	}
	if _, err = service.Prompt(ctx, conversation.ID, "request-1", "create proof"); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatal("replayed prompt ran twice", calls.Load())
	}
	if _, err = service.Prompt(ctx, conversation.ID, "request-1", "different operation"); err == nil {
		t.Fatal("a persisted request identifier accepted different content")
	}
	live, _ := service.session(conversation.ID)
	runner := live.runner
	service.Detach(42)
	for service.Active() {
		select {
		case <-time.After(20 * time.Millisecond):
		case <-ctx.Done():
			t.Fatal("idle Pi leaked", ctx.Err())
		}
	}
	select {
	case <-runner.Done():
	default:
		t.Fatal("removed a live Pi process from the service")
	}
	originalConfigure := service.options.Configure
	service.options.Configure = func(Conversation) (map[string]any, func(), error) { return nil, nil, errors.New("model removed") }
	state, err := service.Open(ctx, conversation.ID, 43)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(state) || !containsJSON(state, "saved by Pi") {
		t.Fatal("persisted history lost", string(state))
	}
	if !containsJSON(state, "model removed") {
		t.Fatal("unavailable model was hidden")
	}
	if _, err = service.Prompt(ctx, conversation.ID, "new-request", "run without a model"); err == nil {
		t.Fatal("missing model accepted a run")
	}
	service.options.Configure = originalConfigure
	updated, err := service.ConfigureConversation(ctx, conversation.ID, func(c *Conversation) error { c.Permission = "bypassPermissions"; return nil })
	if err != nil || updated.Permission != "bypassPermissions" {
		t.Fatal(updated, err)
	}
	if _, err = service.Open(ctx, conversation.ID, 43); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Prompt(ctx, conversation.ID, "request-1", "create proof"); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatal("reopening replayed saved work", calls.Load())
	}
	original, err := store.Conversation(conversation.ID)
	if err != nil {
		t.Fatal(err)
	}
	beforeBranch, err := os.ReadFile(original.SessionFile)
	if err != nil {
		t.Fatal(err)
	}
	branch, err := service.Fork(ctx, conversation.ID, "", "Branch")
	if err != nil {
		t.Fatal(err)
	}
	branchBytes, err := os.ReadFile(branch.SessionFile)
	if err != nil || !containsJSON(branchBytes, "saved by Pi") {
		t.Fatal("branch missing history", err)
	}
	afterBranch, err := os.ReadFile(original.SessionFile)
	if err != nil || string(beforeBranch) != string(afterBranch) {
		t.Fatal("branch changed source history", err)
	}
	if err = service.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if service.Active() {
		t.Fatal("service left a process")
	}
}

func containsJSON(data []byte, text string) bool { return strings.Contains(string(data), text) }
