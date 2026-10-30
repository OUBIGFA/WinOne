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
	"strings"
	"sync"
	"testing"
	"time"

	"winone/internal/ai"
)

// A companion reply quote: the model receives the host note before the owner's
// words, while the transcript, preview and draft keep only what the owner wrote.
func TestPresentedPromptKeepsOwnerTextAndReplyQuote(t *testing.T) {
	bun, worker := os.Getenv("WINONE_CHAT_BUN_EXE"), os.Getenv("WINONE_CHAT_WORKER")
	if bun == "" || worker == "" {
		t.Skip("explicit Pi runtime fixture")
	}
	root := t.TempDir()
	store, err := Open(filepath.Join(root, "profile"))
	if err != nil {
		t.Fatal(err)
	}
	c, err := store.NewConversation("", "", "Reply quote", ai.Selection{})
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var bodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(body))
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Noted\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
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
	display := "And the costs?"
	quote := json.RawMessage(`{"clientId":"reply-entry","role":"assistant","excerpt":"The report shows growth."}`)
	note := "[引用说明] 以下为系统自动附带的背景"
	if _, err = service.PromptPresented(ctx, c.ID, "quoted-input", note+"\n\n"+display, "queue", NativeInputPresentation{Text: &display, ReplyQuote: quote}, nil); err != nil {
		t.Fatal(err)
	}
	for {
		page, err := service.Poll(c.ID, 0)
		if err != nil {
			t.Fatal(err)
		}
		if !page.Running && page.Status == "completed" {
			break
		}
		select {
		case <-time.After(10 * time.Millisecond):
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	mu.Lock()
	sent := strings.Join(bodies, "\n")
	mu.Unlock()
	if !strings.Contains(sent, "[引用说明]") || !strings.Contains(sent, display) {
		t.Fatal("model request lacks the quote note or the owner's text", sent)
	}
	raw, err := service.Command(ctx, c.ID, "state", nil)
	if err != nil {
		t.Fatal(err)
	}
	var state struct {
		Messages []struct {
			Role, RequestID string
			Content         json.RawMessage
			ReplyQuote      json.RawMessage
		}
	}
	if err = json.Unmarshal(raw, &state); err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, message := range state.Messages {
		if message.Role != "user" || message.RequestID != "quoted-input" {
			continue
		}
		found = true
		var content string
		if err = json.Unmarshal(message.Content, &content); err != nil || content != display {
			t.Fatal("transcript shows the model note instead of the owner's text", string(message.Content))
		}
		var saved, expected map[string]any
		if json.Unmarshal(message.ReplyQuote, &saved) != nil || json.Unmarshal(quote, &expected) != nil || fmt.Sprint(saved) != fmt.Sprint(expected) {
			t.Fatal("reply quote was not kept with the owner's message", string(message.ReplyQuote))
		}
	}
	if !found {
		t.Fatal("missing saved quoted input")
	}
	draft, err := store.ReadDraft(c.ID)
	if err != nil || draft.Text != "" || draft.PendingID != "" {
		t.Fatal("accepted quoted input remained as a draft", draft, err)
	}
	// IM's restart uses the same Worker command: stop/reset model context,
	// retain the owner-visible branch, and do not fabricate another conversation.
	if _, err = service.Command(ctx, c.ID, "context.reset", nil); err != nil {
		t.Fatal(err)
	}
	if _, err = service.PromptPresented(ctx, c.ID, "after-reset", "A fresh question", "queue", NativeInputPresentation{}, nil); err != nil {
		t.Fatal(err)
	}
	for {
		page, err := service.Poll(c.ID, 0)
		if err != nil {
			t.Fatal(err)
		}
		if !page.Running && page.Status == "completed" {
			break
		}
		select {
		case <-time.After(10 * time.Millisecond):
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	mu.Lock()
	lastBody := bodies[len(bodies)-1]
	mu.Unlock()
	if strings.Contains(lastBody, note) || strings.Contains(lastBody, display) || !strings.Contains(lastBody, "A fresh question") {
		t.Fatal("reset did not isolate the new model context", lastBody)
	}
	view, err := service.Command(ctx, c.ID, "state", nil)
	if err != nil || !strings.Contains(string(view), display) || !strings.Contains(string(view), "A fresh question") {
		t.Fatal("reset discarded visible conversation history", string(view), err)
	}
}
