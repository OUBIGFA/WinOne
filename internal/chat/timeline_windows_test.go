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
	"strings"
	"testing"
	"time"
	"winone/internal/ai"
)

// The coordinator's visible anchor is a native history entry, not a user
// prompt. Creating or replaying it must not spend a model call or lose a draft.
func TestNativeTimelineAppendIsDurableWithoutPromptingOrReplacingDraft(t *testing.T) {
	bun, worker := os.Getenv("WINONE_CHAT_BUN_EXE"), os.Getenv("WINONE_CHAT_WORKER")
	if bun == "" || worker == "" {
		t.Skip("explicit isolated Pi runtime paths are required")
	}
	root := t.TempDir()
	store, err := Open(filepath.Join(root, "chat"))
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := store.NewConversation("", root, "Timeline", ai.Selection{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	calls := 0
	service := NewService(ctx, store, ServiceOptions{Bun: bun, Worker: worker, Configure: func(c Conversation) (map[string]any, func(), error) {
		bridge, err := NewPrivateBridge(ctx, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, _ := io.ReadAll(r.Body)
			if strings.Contains(string(raw), "TASK_ANCHOR") || strings.Contains(string(raw), "VOICE_PRIVATE_TEXT") {
				t.Error("presentation anchor entered model context")
			}
			calls++
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"DONE\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
		}))
		if err != nil {
			return nil, nil, err
		}
		return map[string]any{"baseUrl": bridge.BaseURL, "key": bridge.Key, "agentDir": filepath.Join(root, "agent"), "model": map[string]string{"id": "fixture", "name": "Fixture"}}, bridge.Close, nil
	}})
	defer func() {
		end, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		if err := service.Stop(end); err != nil {
			t.Error(err)
		}
	}()
	if _, err = service.Open(ctx, conversation.ID, 0); err != nil {
		t.Fatal(err)
	}
	if err = store.SaveDraft(conversation.ID, Draft{Text: "keep draft"}); err != nil {
		t.Fatal(err)
	}
	entry := map[string]any{"clientId": "task-anchor", "role": "assistant", "content": "TASK_ANCHOR", "createdAt": 1000, "agentMeta": map[string]any{"taskId": "task-one"}}
	for range 2 {
		if _, err = service.Command(ctx, conversation.ID, "timeline.append", entry); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 0 {
		t.Fatal("timeline started a model request")
	}
	voiceInitial := map[string]any{"callId": "call-one", "startedAt": 1000, "turns": []any{}, "handoffs": []any{}}
	if _, err = service.Command(ctx, conversation.ID, "timeline.append", map[string]any{"clientId": "voice-call:call-one", "role": "assistant", "content": "", "createdAt": 1000, "agentMeta": map[string]any{"voiceCall": voiceInitial}}); err != nil {
		t.Fatal(err)
	}
	voiceRecord := map[string]any{"callId": "call-one", "startedAt": 1000, "endedAt": 2000, "turns": []any{map[string]any{"id": "utterance", "role": "user", "text": "VOICE_PRIVATE_TEXT", "createdAt": 1100, "complete": true}}, "handoffs": []any{}}
	for range 2 {
		if _, err = service.Command(ctx, conversation.ID, "timeline.voice", map[string]any{"clientId": "voice-call:call-one", "record": voiceRecord}); err != nil {
			t.Fatal(err)
		}
	}
	entry["content"] = "REPLACED"
	if _, err = service.Command(ctx, conversation.ID, "timeline.append", entry); err == nil {
		t.Fatal("conflicting receipt overwrote the task anchor")
	}
	draft, err := store.ReadDraft(conversation.ID)
	if err != nil || draft.Text != "keep draft" {
		t.Fatalf("draft changed: %+v %v", draft, err)
	}
	if err = service.CloseSession(ctx, conversation.ID); err != nil {
		t.Fatal(err)
	}
	raw, err := service.Open(ctx, conversation.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	var state struct {
		Messages []struct {
			Content   any
			Role      string
			AgentMeta map[string]any
		}
	}
	if err = json.Unmarshal(raw, &state); err != nil {
		t.Fatal(err)
	}
	if len(state.Messages) != 2 || state.Messages[0].Content != "TASK_ANCHOR" || state.Messages[0].AgentMeta["taskId"] != "task-one" {
		t.Fatalf("timeline lost or duplicated: %s", raw)
	}
	voiceSaved, _ := json.Marshal(state.Messages[1].AgentMeta["voiceCall"])
	if !strings.Contains(string(voiceSaved), "VOICE_PRIVATE_TEXT") {
		t.Fatal("voice transcript was not retained in native history", string(voiceSaved))
	}
	if _, err = service.Prompt(ctx, conversation.ID, "after-anchor", "Hello"); err != nil {
		t.Fatal(err)
	}
	cursor := uint64(0)
	for {
		page, err := service.WaitEvents(ctx, conversation.ID, cursor)
		if err != nil {
			t.Fatal(err)
		}
		cursor = page.Cursor
		if !page.Running {
			break
		}
	}
	if calls != 1 {
		t.Fatalf("model requests=%d", calls)
	}
}
