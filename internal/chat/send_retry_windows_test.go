//go:build windows

package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
	"winone/internal/ai"
)

// These tests exercise actual Worker receipts, rather than a fake Runner that
// would hide cached rejections or completed retries being marked running again.
func TestNativeRetryAcknowledgementDoesNotReopenACompletedTurn(t *testing.T) {
	bun, worker := os.Getenv("WINONE_CHAT_BUN_EXE"), os.Getenv("WINONE_CHAT_WORKER")
	if bun == "" || worker == "" {
		t.Skip("explicit Pi runtime fixture")
	}
	root := t.TempDir()
	store, err := Open(filepath.Join(root, "profile"))
	if err != nil {
		t.Fatal(err)
	}
	c, err := store.NewConversation("", "", "Retry receipts", ai.Selection{})
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Done\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	service := NewService(ctx, store, ServiceOptions{Bun: bun, Worker: worker, Configure: func(Conversation) (map[string]any, func(), error) {
		return map[string]any{"baseUrl": server.URL + "/v1", "key": "fixture-session-token", "agentDir": filepath.Join(root, "agent"), "model": map[string]string{"id": "fixture", "name": "Fixture"}}, func() {}, nil
	}})
	defer func() {
		end, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if err := service.Stop(end); err != nil {
			t.Error(err)
		}
	}()
	if _, err = service.Open(ctx, c.ID, 1); err != nil {
		t.Fatal(err)
	}
	wait := func() {
		t.Helper()
		for {
			page, err := service.Poll(c.ID, 0)
			if err != nil {
				t.Fatal(err)
			}
			if !page.Running && page.Status == "completed" {
				return
			}
			select {
			case <-time.After(10 * time.Millisecond):
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
		}
	}
	if _, err = service.Prompt(ctx, c.ID, "original", "Original input"); err != nil {
		t.Fatal(err)
	}
	wait()
	raw, err := service.Command(ctx, c.ID, "state", nil)
	if err != nil {
		t.Fatal(err)
	}
	var state struct {
		Messages []struct{ ID, Role, RequestID string }
	}
	if err = json.Unmarshal(raw, &state); err != nil {
		t.Fatal(err)
	}
	var entry string
	for _, message := range state.Messages {
		if message.Role == "user" {
			entry = message.ID
		}
	}
	if entry == "" {
		t.Fatal("missing saved input")
	}
	params := map[string]any{"entry": entry, "requestId": "retry-receipt"}
	if _, err = service.Command(ctx, c.ID, "retry", params); err != nil {
		t.Fatal(err)
	}
	wait()
	if _, err = service.Command(ctx, c.ID, "retry", params); err != nil {
		t.Fatal("replayed acknowledgement", err)
	}
	page, err := service.Poll(c.ID, 0)
	if err != nil || page.Running {
		t.Fatal("completed retry became a new running task", page.Status, page.Running, err)
	}
	if calls.Load() != 2 {
		t.Fatal("retry receipt executed more than once", calls.Load())
	}
}

func TestNativeAcceptedQueueIsNotRestoredAsAnUnsentDraft(t *testing.T) {
	bun, worker := os.Getenv("WINONE_CHAT_BUN_EXE"), os.Getenv("WINONE_CHAT_WORKER")
	if bun == "" || worker == "" {
		t.Skip("explicit Pi runtime fixture")
	}
	root := t.TempDir()
	store, err := Open(filepath.Join(root, "profile"))
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := store.NewConversation("", "", "Queued receipt", ai.Selection{})
	if err != nil {
		t.Fatal(err)
	}
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		if calls.Add(1) == 1 {
			close(started)
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Done\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	service := NewService(ctx, store, ServiceOptions{Bun: bun, Worker: worker, Configure: func(Conversation) (map[string]any, func(), error) {
		return map[string]any{"baseUrl": server.URL + "/v1", "key": "fixture-session-token", "agentDir": filepath.Join(root, "agent"), "model": map[string]string{"id": "fixture", "name": "Fixture"}}, func() {}, nil
	}})
	defer func() {
		close(release)
		end, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if err := service.Stop(end); err != nil {
			t.Error(err)
		}
		server.Close()
	}()
	if _, err := service.Open(ctx, conversation.ID, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Prompt(ctx, conversation.ID, "first", "Keep this turn running"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if _, err := service.Prompt(ctx, conversation.ID, "queued", "Accepted next input"); err != nil {
		t.Fatal(err)
	}
	// Do not read state first: that would reconcile the saved queue and hide
	// the bug where draft.get races the history read when reopening a chat.
	draft, err := store.ReadDraft(conversation.ID)
	if err != nil || draft.Text != "" || draft.PendingID != "" {
		t.Fatalf("accepted queue remained in the composer draft: %#v, %v", draft, err)
	}
	if err := store.SaveDraft(conversation.ID, Draft{Text: "Newer unsent draft"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Prompt(ctx, conversation.ID, "queued", "Accepted next input"); err != nil {
		t.Fatal(err)
	}
	draft, err = store.ReadDraft(conversation.ID)
	if err != nil || draft.Text != "Newer unsent draft" || calls.Load() != 1 {
		t.Fatal("replayed receipt changed a newer draft or dispatched the queue early", draft, calls.Load(), err)
	}
}

func TestNativeRunnerCanRetryARejectedAttachmentWithoutChangingItsReceipt(t *testing.T) {
	bun, worker := os.Getenv("WINONE_CHAT_BUN_EXE"), os.Getenv("WINONE_CHAT_WORKER")
	if bun == "" || worker == "" {
		t.Skip("explicit Pi runtime fixture")
	}
	root := t.TempDir()
	sessions := filepath.Join(root, "sessions")
	file := filepath.Join(sessions, "attachments", "one", "note.txt")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Done\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	runner, err := StartRunner(ctx, bun, worker, root, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		end, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if err := runner.Close(end); err != nil {
			t.Error(err)
		}
	}()
	if _, err = runner.Call(ctx, "init", map[string]any{"cwd": root, "sessionDirectory": sessions, "agentDir": filepath.Join(root, "agent"), "baseUrl": server.URL + "/v1", "key": "fixture-session-token", "model": map[string]string{"id": "fixture", "name": "Fixture"}}); err != nil {
		t.Fatal(err)
	}
	params := map[string]any{"text": "Read the attachment", "attachments": []string{file}}
	if _, err = runner.CallOnce(ctx, "rejected-input", "prompt", params); err == nil {
		t.Fatal("missing attachment was accepted")
	}
	if err = os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(file, []byte("Now available"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = runner.CallOnce(ctx, "rejected-input", "prompt", params); err != nil {
		t.Fatal("an explicit rejection was cached as a permanent failure", err)
	}
}
