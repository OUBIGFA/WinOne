package chat

import (
	"os"
	"path/filepath"
	"testing"
	"winone/internal/ai"
)

func TestDetachHistoryPersistsDestinationBeforeRemovingSource(t *testing.T) {
	root := t.TempDir()
	source, _ := Open(filepath.Join(root, "onebot"))
	destination, _ := Open(filepath.Join(root, "chat"))
	c, err := source.NewConversation("", root, "History", ai.Selection{})
	if err != nil {
		t.Fatal(err)
	}
	if err := source.SaveDraft(c.ID, Draft{Text: "unsent"}); err != nil {
		t.Fatal(err)
	}
	// An unavailable destination must leave the source navigation and draft intact.
	if err := os.MkdirAll(destination.path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := source.DetachHistory(destination, []string{c.ID}); err == nil {
		t.Fatal("destination write unexpectedly succeeded")
	}
	if _, err := source.Conversation(c.ID); err != nil {
		t.Fatal("failed transfer removed source", err)
	}
	destination.path = filepath.Join(root, "chat", "recovered.json")
	if err := source.DetachHistory(destination, []string{c.ID}); err != nil {
		t.Fatal(err)
	}
	retained, err := destination.Conversation(c.ID)
	if err != nil || !retained.Archived || retained.SessionDirectory != c.SessionDirectory || retained.Title != c.Title {
		t.Fatalf("archive: %+v %v", retained, err)
	}
	if draft, err := destination.ReadDraft(c.ID); err != nil || draft.Text != "unsent" {
		t.Fatal("draft lost", err)
	}
	if _, err := source.Conversation(c.ID); err == nil {
		t.Fatal("source still owns history")
	}
	// A retry after committing both catalogs is safe and does not add another row.
	if err := source.DetachHistory(destination, []string{c.ID}); err != nil {
		t.Fatal(err)
	}
	if len(destination.Snapshot().Conversations) != 1 {
		t.Fatal("retry duplicated archive")
	}
}
