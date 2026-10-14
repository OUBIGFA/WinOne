//go:build windows

package chat

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"winone/internal/ai"
)

func TestSessionReviewUsesSuccessfulActiveBranchToolsWithoutGit(t *testing.T) {
	root := t.TempDir()
	store, err := Open(filepath.Join(root, "profile"))
	if err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(root, "work")
	if err = os.Mkdir(workspace, 0700); err != nil {
		t.Fatal(err)
	}
	c, err := store.NewConversation("", workspace, "Review", ai.Selection{})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(c.SessionDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	session := filepath.Join(c.SessionDirectory, "session.jsonl")
	patch := "--- a/中文.txt\n+++ b/中文.txt\n@@ -1 +1 @@\n-旧内容\n+新内容\n"
	entries := []map[string]any{
		{"type": "session", "id": "session"},
		{"type": "message", "id": "call", "message": map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "toolCall", "id": "edit1", "name": "edit", "arguments": map[string]string{"path": "中文.txt"}}}}},
		{"type": "message", "id": "result", "parentId": "call", "message": map[string]any{"role": "toolResult", "toolCallId": "edit1", "toolName": "edit", "details": map[string]string{"patch": patch}}},
		{"type": "message", "id": "discarded", "parentId": "result", "message": map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "toolCall", "id": "write1", "name": "write", "arguments": map[string]string{"path": "discarded.txt", "content": "discarded"}}}}},
		{"type": "message", "id": "discarded-result", "parentId": "discarded", "message": map[string]any{"role": "toolResult", "toolCallId": "write1", "toolName": "write"}},
		{"type": "message", "id": "active", "parentId": "result", "message": map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "toolCall", "id": "failed", "name": "write", "arguments": map[string]string{"path": "failed.txt", "content": "failed"}}}}},
		{"type": "message", "id": "failed-result", "parentId": "active", "message": map[string]any{"role": "toolResult", "toolCallId": "failed", "toolName": "write", "isError": true}},
	}
	var body strings.Builder
	for _, entry := range entries {
		raw, _ := json.Marshal(entry)
		body.Write(raw)
		body.WriteByte('\n')
	}
	if err = os.WriteFile(session, []byte(body.String()), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = store.UpdateConversation(c.ID, func(next *Conversation) error { next.SessionFile = session; return nil }); err != nil {
		t.Fatal(err)
	}
	reader, ok := any(store).(interface {
		SessionReview(string) (ReviewSnapshot, map[string]string, error)
	})
	if !ok {
		t.Fatal("ordinary conversations still require a Git repository")
	}
	snapshot, patches, err := reader.SessionReview(c.ID)
	if err != nil || len(snapshot.Files) != 1 || snapshot.Files[0].Path != "中文.txt" || patches["中文.txt"] != patch {
		t.Fatalf("snapshot=%+v patches=%+v err=%v", snapshot, patches, err)
	}
	if _, err = os.Stat(filepath.Join(workspace, ".git")); !os.IsNotExist(err) {
		t.Fatal("review created a repository")
	}
}
