//go:build windows

package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"winone/internal/ai"
	"winone/internal/chat"
)

func TestOrdinaryConversationReviewOpensRecordedFilesThroughHost(t *testing.T) {
	root := t.TempDir()
	store, err := chat.Open(filepath.Join(root, "profile"))
	if err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(root, "work")
	if err = os.Mkdir(workspace, 0700); err != nil {
		t.Fatal(err)
	}
	conversation, err := store.NewConversation("", workspace, "Chat", ai.Selection{})
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "report.md")
	if err = os.WriteFile(output, []byte("# 中文报告\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(conversation.SessionDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	entries := []any{
		map[string]any{"type": "session", "id": "session"},
		map[string]any{"type": "message", "id": "call", "message": map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "toolCall", "id": "write", "name": "powershell", "arguments": map[string]string{"command": "write report"}}}}},
		map[string]any{"type": "message", "id": "result", "parentId": "call", "message": map[string]any{"role": "toolResult", "toolCallId": "write", "toolName": "powershell", "details": map[string]any{"files": []any{map[string]string{"path": output, "status": "A", "patch": "--- /dev/null\n+++ b/report.md\n@@ -0,0 +1 @@\n+# 中文报告\n"}}}}},
	}
	var body strings.Builder
	for _, entry := range entries {
		raw, _ := json.Marshal(entry)
		body.Write(raw)
		body.WriteByte('\n')
	}
	session := filepath.Join(conversation.SessionDirectory, "session.jsonl")
	if err = os.WriteFile(session, []byte(body.String()), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = store.UpdateConversation(conversation.ID, func(c *chat.Conversation) error { c.SessionFile = session; return nil }); err != nil {
		t.Fatal(err)
	}
	control := &chatControl{store: store, ctx: context.Background()}
	t.Cleanup(func() {
		if control.previews != nil {
			control.previews.Close()
		}
	})
	// Native open/reveal/copy must discover the exact successful-tool grant
	// themselves, even before a review or preview has populated that cache.
	resolveParams, _ := json.Marshal(map[string]string{"id": conversation.ID, "path": output})
	resolved, err := control.fileAction("file.resolve", resolveParams)
	if err != nil || resolved.(map[string]string)["path"] != output {
		t.Fatal("native file action lost the historical tool grant", resolved, err)
	}
	externalDirectory, _ := json.Marshal(map[string]string{"id": conversation.ID, "path": root})
	if _, err = control.fileAction("file.resolve", externalDirectory); err == nil {
		t.Fatal("exact tool file grant authorized an external directory")
	}
	params, _ := json.Marshal(map[string]string{"sessionId": conversation.ID, "scope": "working"})
	value, err := control.reviewAction(context.Background(), "review.snapshot", params)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := value.(chat.ReviewSnapshot)
	if snapshot.Source != "session" || len(snapshot.Files) != 1 {
		t.Fatalf("source=%s root=%s files=%d", snapshot.Source, snapshot.Root, len(snapshot.Files))
	}
	if _, err = control.reviewAction(context.Background(), "review.branches", params); err != nil {
		t.Fatal(err)
	}
	if _, err = control.reviewAction(context.Background(), "review.mutate", params); err == nil {
		t.Fatal("non-Git mutation accepted")
	}
	fileParams, _ := json.Marshal(map[string]string{"id": conversation.ID, "path": output})
	previewValue, err := control.fileAction("file.preview", fileParams)
	if err != nil {
		t.Fatal(err)
	}
	preview := previewValue.(chat.FilePreview)
	if !preview.ReadOnly {
		t.Fatal("external output must remain read-only")
	}
	response, err := http.Get(preview.URL)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != 200 || string(data) != "# 中文报告\n" {
		t.Fatalf("preview: %d %q %v", response.StatusCode, data, err)
	}
}
