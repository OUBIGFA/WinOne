//go:build windows

package chat

import (
	"archive/zip"
	"bytes"
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

// The HTTP model is the only external seam. Real Pi opens and appends its
// transcript, executes its write tool, and packages through the bundled worker.
func TestNativeConversationProjectMoveAndChatArchive(t *testing.T) {
	bun, worker := os.Getenv("WINONE_CHAT_BUN_EXE"), os.Getenv("WINONE_CHAT_WORKER")
	if bun == "" || worker == "" {
		t.Skip("explicit Pi runtime fixture")
	}
	root := t.TempDir()
	store, err := Open(filepath.Join(root, "profile"))
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.CreateProject(root, "first")
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.CreateProject(root, "second")
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := store.NewConversation(first.ID, "", "Complete task", ai.Selection{Provider: "fixture", Model: "model"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	releaseFirst := make(chan struct{})
	var requests atomic.Int32
	service := NewService(ctx, store, ServiceOptions{Bun: bun, Worker: worker, Configure: func(c Conversation) (map[string]any, func(), error) {
		bridge, err := NewPrivateBridge(ctx, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.Copy(io.Discard, r.Body)
			number := requests.Add(1)
			if number == 1 {
				select {
				case <-releaseFirst:
				case <-r.Context().Done():
					return
				}
			}
			w.Header().Set("Content-Type", "text/event-stream")
			emit := func(delta any, finish any) {
				raw, _ := json.Marshal(map[string]any{"id": "fixture", "object": "chat.completion.chunk", "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}}})
				fmt.Fprintf(w, "data: %s\n\n", raw)
			}
			switch number {
			case 1:
				emit(map[string]any{"content": "HISTORY_BEFORE_MOVE"}, nil)
				emit(map[string]any{}, "stop")
			case 2:
				emit(map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "write-after-move", "type": "function", "function": map[string]any{"name": "write", "arguments": `{"path":"moved-output.txt","content":"ONLY_SECOND_PROJECT"}`}}}}, nil)
				emit(map[string]any{}, "tool_calls")
			default:
				emit(map[string]any{"content": "MOVED_AND_WRITTEN"}, nil)
				emit(map[string]any{}, "stop")
			}
			fmt.Fprint(w, "data: [DONE]\n\n")
		}))
		if err != nil {
			return nil, nil, err
		}
		return map[string]any{"cwd": c.Directory, "baseUrl": bridge.BaseURL, "key": bridge.Key, "agentDir": filepath.Join(root, "agent"), "model": map[string]string{"id": "fixture/model", "name": "Fixture"}}, bridge.Close, nil
	}})
	defer func() {
		ending, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if err := service.Stop(ending); err != nil {
			t.Error(err)
		}
	}()
	_, err = store.UpdateConversation(conversation.ID, func(c *Conversation) error { c.Permission = "bypassPermissions"; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Open(ctx, conversation.ID, 1); err != nil {
		t.Fatal(err)
	}
	if _, err = service.Prompt(ctx, conversation.ID, "first-turn", "Remember our earlier work."); err != nil {
		t.Fatal(err)
	}
	if _, err = service.MoveConversation(ctx, conversation.ID, second.ID); err == nil {
		t.Fatal("a running task was moved")
	}
	retained, _ := store.Conversation(conversation.ID)
	if retained.Directory != first.Path {
		t.Fatal("busy move changed cwd")
	}
	close(releaseFirst)
	waitIdle := func() {
		t.Helper()
		cursor := uint64(0)
		for {
			page, err := service.WaitEvents(ctx, conversation.ID, cursor)
			if err != nil {
				t.Fatal(err)
			}
			if !page.Running {
				return
			}
			cursor = page.Cursor
		}
	}
	waitIdle()
	conversation, err = store.Conversation(conversation.ID)
	if err != nil {
		t.Fatal(err)
	}
	history, err := os.ReadFile(conversation.SessionFile)
	if err != nil {
		t.Fatal(err)
	}
	draft := Draft{Text: "UNSENT_DRAFT", PendingID: "unsubmitted"}
	if err = store.SaveDraft(conversation.ID, draft); err != nil {
		t.Fatal(err)
	}
	moved, err := service.MoveConversation(ctx, conversation.ID, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if moved.Directory != second.Path || moved.SessionFile != conversation.SessionFile || moved.SessionDirectory != conversation.SessionDirectory {
		t.Fatal("move changed storage", moved)
	}
	state, err := service.Open(ctx, conversation.ID, 1)
	if err != nil || !bytes.Contains(state, []byte("HISTORY_BEFORE_MOVE")) {
		t.Fatal("history failed to reopen", err)
	}
	savedDraft, err := store.ReadDraft(conversation.ID)
	if err != nil || savedDraft.Text != draft.Text || savedDraft.PendingID != draft.PendingID {
		t.Fatal("move discarded the draft", savedDraft, err)
	}
	if _, err = service.Prompt(ctx, conversation.ID, "after-move", "Write moved-output.txt in this project."); err != nil {
		t.Fatal(err)
	}
	waitIdle()
	written, err := os.ReadFile(filepath.Join(second.Path, "moved-output.txt"))
	if err != nil || string(written) != "ONLY_SECOND_PROJECT" {
		t.Fatal("Pi did not use the destination cwd", err)
	}
	if _, err = os.Stat(filepath.Join(first.Path, "moved-output.txt")); !os.IsNotExist(err) {
		t.Fatal("Pi wrote into the former project")
	}
	finalHistory, err := os.ReadFile(conversation.SessionFile)
	if err != nil || !bytes.HasPrefix(finalHistory, history) {
		t.Fatal("native history was replaced by move", err)
	}
	// Sending the next prompt correctly replaces its composer draft. A new
	// unsent draft after that turn must survive the independent export.
	if err = store.SaveDraft(conversation.ID, draft); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "task.cshare")
	outcome, err := service.ExportArchive(ctx, ArchiveOptions{ID: conversation.ID, TargetPath: target})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Status != "ok" || outcome.Fidelity != "full" || outcome.MessageCount < 5 {
		t.Fatal("unexpected export receipt", outcome)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(data[12:]), int64(len(data)-12))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, file := range archive.File {
		if file.Name == "messages.jsonl" {
			reader, err := file.Open()
			if err != nil {
				t.Fatal(err)
			}
			contents, err := io.ReadAll(reader)
			reader.Close()
			if err != nil {
				t.Fatal(err)
			}
			found = bytes.Contains(contents, []byte("HISTORY_BEFORE_MOVE")) && bytes.Contains(contents, []byte("MOVED_AND_WRITTEN"))
		}
	}
	if !found {
		t.Fatal("complete message history was not exported")
	}
	savedDraft, err = store.ReadDraft(conversation.ID)
	if err != nil || savedDraft.Text != draft.Text || savedDraft.PendingID != draft.PendingID {
		t.Fatal("export discarded the draft", savedDraft, err)
	}
	if _, err = service.Command(ctx, conversation.ID, "state", nil); err != nil {
		t.Fatal("export retired the live conversation", err)
	}
}

func TestChatArchiveRejectsInvalidDestinations(t *testing.T) {
	root := t.TempDir()
	for _, target := range []string{"relative.cshare", filepath.Join(root, "wrong.txt"), filepath.Join(root, "missing", "task.cshare")} {
		if ValidateArchiveTarget(target) == nil {
			t.Fatal("invalid export target accepted", target)
		}
	}
	if err := ValidateArchiveTarget(filepath.Join(root, "task.cshare")); err != nil {
		t.Fatal(err)
	}
}
