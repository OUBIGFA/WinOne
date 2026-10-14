package chat

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"winone/internal/ai"
)

func TestConversationProjectMoveRetainsEveryStorageFieldAndReopens(t *testing.T) {
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
	conversation, err := store.NewConversation(first.ID, "", "Preserve history", ai.Selection{Provider: "fixture", Model: "model"})
	if err != nil {
		t.Fatal(err)
	}
	history := []byte("original Pi JSONL bytes\n")
	file := filepath.Join(conversation.SessionDirectory, "original.jsonl")
	if err = os.WriteFile(file, history, 0600); err != nil {
		t.Fatal(err)
	}
	conversation, err = store.BindSession(conversation.ID, file)
	if err != nil {
		t.Fatal(err)
	}
	attachment := filepath.Join(root, "attachment.txt")
	if err = os.WriteFile(attachment, []byte("original attachment"), 0600); err != nil {
		t.Fatal(err)
	}
	attachments, err := store.AddAttachments(conversation.ID, []string{attachment})
	if err != nil {
		t.Fatal(err)
	}
	draft := Draft{Text: "unsent draft", PendingID: "pending", Attachments: attachments}
	if err = store.SaveDraft(conversation.ID, draft); err != nil {
		t.Fatal(err)
	}
	moved, err := store.MoveConversation(conversation.ID, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if moved.ProjectID != second.ID || moved.Directory != second.Path || moved.SessionDirectory != conversation.SessionDirectory || moved.SessionFile != file || moved.ID != conversation.ID || moved.Model != conversation.Model {
		t.Fatal("move changed storage or identity", moved)
	}
	reopened, err := Open(filepath.Join(root, "profile"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := reopened.ReadDraft(conversation.ID)
	if err != nil {
		t.Fatal(err)
	}
	oldDraft, _ := json.Marshal(draft)
	newDraft, _ := json.Marshal(got)
	if !bytes.Equal(oldDraft, newDraft) {
		t.Fatal("draft changed")
	}
	raw, err := os.ReadFile(file)
	if err != nil || !bytes.Equal(raw, history) {
		t.Fatal("original transcript changed", err)
	}
	free, err := reopened.MoveConversation(conversation.ID, "")
	if err != nil || free.ProjectID != "" || free.Directory != second.Path || free.SessionFile != file {
		t.Fatal("projectless move changed cwd or history", free, err)
	}
}

func TestConversationProjectMoveRejectsMissingDestinationWithoutChangingCatalog(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := store.NewConversation("", "", "Keep me", ai.Selection{})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(store.Snapshot())
	if _, err = store.MoveConversation(conversation.ID, "missing-project"); err == nil {
		t.Fatal("missing destination accepted")
	}
	after, _ := json.Marshal(store.Snapshot())
	if !bytes.Equal(before, after) {
		t.Fatal("failed move rewrote the catalog")
	}
}
