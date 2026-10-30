package chat

import (
	"path/filepath"
	"testing"
	"winone/internal/ai"
)

func TestTaskConversationAdmissionIsIdempotentAndPreservesLaterUserChanges(t *testing.T) {
	root := t.TempDir()
	store, err := Open(filepath.Join(root, "chat"))
	if err != nil {
		t.Fatal(err)
	}
	prepared := Conversation{ID: "ABCDEFGHIJKLMNOPQRSTUVWXYZ", Title: "Prepared task", Directory: root,
		Model: ai.Selection{Provider: "fixture", Model: "selected"}, ThinkingLevel: "balanced", FastMode: true, Permission: "ask",
		OneBotTaskID: "task-receipt", OneBotOwnerID: "owner", OneBotParentID: "BCDEFGHIJKLMNOPQRSTUVWXYZ2"}
	created, err := store.AdmitTaskConversation(prepared)
	if err != nil {
		t.Fatal(err)
	}
	if created.Model != prepared.Model || created.ThinkingLevel != "balanced" || !created.FastMode || created.Permission != "ask" {
		t.Fatalf("initial route was not persisted atomically: %+v", created)
	}
	if _, err = store.UpdateConversation(created.ID, func(c *Conversation) error { c.Title = "User title"; c.Archived = true; return nil }); err != nil {
		t.Fatal(err)
	}
	retried, err := store.AdmitTaskConversation(prepared)
	if err != nil || retried.Title != "User title" || !retried.Archived {
		t.Fatalf("retry overwrote user state: %+v %v", retried, err)
	}
	prepared.OneBotTaskID = "another-task"
	if _, err = store.AdmitTaskConversation(prepared); err == nil {
		t.Fatal("another task claimed an existing conversation")
	}
	reopened, err := Open(filepath.Join(root, "chat"))
	if err != nil {
		t.Fatal(err)
	}
	saved, err := reopened.Conversation(created.ID)
	if err != nil || saved.OneBotTaskID != "task-receipt" || !saved.Archived {
		t.Fatalf("task ownership did not survive reopening: %+v %v", saved, err)
	}
}
