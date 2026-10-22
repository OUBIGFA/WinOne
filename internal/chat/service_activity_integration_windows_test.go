//go:build windows

package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"winone/internal/ai"
)

// Only the HTTP model is a fixture: cancellation, worker exit, saved history
// and reopening all use the real Pi runtime. No external API or user config.
func TestNativeInterruptedActivitySurvivesWorkerExitAndReopen(t *testing.T) {
	bun, worker := os.Getenv("WINONE_CHAT_BUN_EXE"), os.Getenv("WINONE_CHAT_WORKER")
	if bun == "" || worker == "" {
		t.Skip("explicit Pi runtime fixture")
	}
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.Mkdir(workspace, 0700); err != nil {
		t.Fatal(err)
	}
	store, err := Open(filepath.Join(root, "profile"))
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := store.NewConversation("", workspace, "Interrupted", ai.Selection{Provider: "test", Model: "model"})
	if err != nil {
		t.Fatal(err)
	}
	started, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	var requests atomic.Int32
	bridge, err := NewPrivateBridge(context.Background(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		if requests.Add(1) == 1 {
			fmt.Fprint(w, ": waiting for explicit user stop\n\n")
			w.(http.Flusher).Flush()
			close(started)
			select {
			case <-r.Context().Done():
			case <-release:
			}
			return
		}
		for _, choice := range []map[string]any{{"index": 0, "delta": map[string]string{"content": "Next turn complete"}}, {"index": 0, "delta": map[string]string{}, "finish_reason": "stop"}} {
			raw, _ := json.Marshal(map[string]any{"id": "proof", "object": "chat.completion.chunk", "choices": []any{choice}})
			fmt.Fprintf(w, "data: %s\n\n", raw)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer bridge.Close()
	service := NewService(context.Background(), store, ServiceOptions{Bun: bun, Worker: worker, Configure: func(Conversation) (map[string]any, func(), error) {
		return map[string]any{"baseUrl": bridge.BaseURL, "key": bridge.Key, "agentDir": filepath.Join(root, "agent"), "model": map[string]string{"id": "test/model", "name": "Test"}, "language": "zh-CN"}, func() {}, nil
	}})
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	defer func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer stopCancel()
		if err := service.Stop(stopCtx); err != nil {
			t.Error(err)
		}
	}()
	if _, err = service.Open(ctx, conversation.ID, 42); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Prompt(ctx, conversation.ID, "stop-request", "Wait until I stop this turn"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("model request did not start", ctx.Err())
	}
	if _, err = service.Command(ctx, conversation.ID, "abort", nil); err != nil {
		t.Fatal(err)
	}
	if got := service.Activity()[conversation.ID]; got.Phase != "interrupted" {
		t.Fatal("stop was not projected as interruption", got)
	}
	live, err := service.session(conversation.ID)
	if err != nil {
		t.Fatal(err)
	}
	runner := live.runner
	service.Release(conversation.ID, 42)
	select {
	case <-runner.Done():
	case <-ctx.Done():
		t.Fatal("stopped worker was retained", ctx.Err())
	}
	for service.Active() {
		select {
		case <-time.After(10 * time.Millisecond):
		case <-ctx.Done():
			t.Fatal("worker exit did not retire session", ctx.Err())
		}
	}
	if got := service.Activity()[conversation.ID]; got.Phase != "interrupted" {
		t.Fatal("normal worker shutdown replaced interruption", got)
	}
	state, err := service.Open(ctx, conversation.ID, 43)
	if err != nil || !containsJSON(state, "Wait until I stop this turn") {
		t.Fatal("reopened history lost stopped turn", string(state), err)
	}
	if got := service.Activity()[conversation.ID]; got.Phase != "interrupted" {
		t.Fatal("reopening erased interruption", got)
	}
	if requests.Load() != 1 {
		t.Fatal("reopening replayed a stopped request", requests.Load())
	}
	if _, err = service.Prompt(ctx, conversation.ID, "next-request", "Start a separate new turn"); err != nil {
		t.Fatal(err)
	}
	for {
		page, err := service.Poll(conversation.ID, 0)
		if err != nil {
			t.Fatal(err)
		}
		if !page.Running {
			break
		}
		select {
		case <-time.After(10 * time.Millisecond):
		case <-ctx.Done():
			t.Fatal("new turn did not complete", ctx.Err())
		}
	}
	if got := service.Activity()[conversation.ID]; got.Phase != "completed" || requests.Load() != 2 {
		t.Fatal("old interruption hid or replayed the next turn", got, requests.Load())
	}
}
