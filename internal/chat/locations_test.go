package chat

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"winone/internal/ai"
)

func TestConversationLocationIsIndependentOfProjectAndRelocationPreservesData(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	saved := filepath.Join(root, "saved")
	next := filepath.Join(root, "next")
	for _, dir := range []string{project, saved, next} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	store, _ := Open(filepath.Join(root, "profile"))
	p, err := store.AddProject(project, "")
	if err != nil {
		t.Fatal(err)
	}
	c, err := store.NewConversation(p.ID, "", "Location test", ai.Selection{}, saved)
	if err != nil {
		t.Fatal(err)
	}
	if c.Directory != project || !within(saved, c.SessionDirectory) {
		t.Fatal("history location replaced working directory", c)
	}
	history := filepath.Join(c.SessionDirectory, "session.jsonl")
	if err = os.WriteFile(history, []byte("persisted Pi history\n"), 0600); err != nil {
		t.Fatal(err)
	}
	c, err = store.BindSession(c.ID, history)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.SaveDraft(c.ID, Draft{Text: "draft survives"}); err != nil {
		t.Fatal(err)
	}
	before := c.SessionDirectory
	c, err = store.UpdateConversation(c.ID, func(c *Conversation) error { return RelocateFiles(c, next) })
	if err != nil {
		t.Fatal(err)
	}
	if c.Directory != project || !within(next, c.SessionDirectory) {
		t.Fatal(c)
	}
	for _, name := range []string{"session.jsonl", "draft.json"} {
		old, err := os.ReadFile(filepath.Join(before, name))
		if err != nil {
			t.Fatal(err)
		}
		moved, err := os.ReadFile(filepath.Join(c.SessionDirectory, name))
		if err != nil || string(old) != string(moved) {
			t.Fatal("relocation lost data", name, err)
		}
	}
	reopened, err := Open(filepath.Join(root, "profile"))
	if err != nil {
		t.Fatal(err)
	}
	restored, err := reopened.Conversation(c.ID)
	if err != nil || restored.SessionDirectory != c.SessionDirectory {
		t.Fatal(restored, err)
	}
}

func TestRelocationCopiesImportedAttachmentTreeAndMigratesOnlyDraftPaths(t *testing.T) {
	root := t.TempDir()
	store, err := Open(filepath.Join(root, "profile"))
	if err != nil {
		t.Fatal(err)
	}
	c, err := store.NewConversation("", "", "Media", ai.Selection{})
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "note.txt")
	if err = os.WriteFile(source, []byte("Imported file"), 0600); err != nil {
		t.Fatal(err)
	}
	attachments, err := store.AddAttachments(c.ID, []string{source})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.SaveDraft(c.ID, Draft{Text: "Read attached file", Attachments: attachments}); err != nil {
		t.Fatal(err)
	}
	history := []byte("{\"type\":\"session\"}\n{\"historicalPath\":\"preserve exactly\"}\n")
	historyFile := filepath.Join(c.SessionDirectory, "history.jsonl")
	if err = os.WriteFile(historyFile, history, 0600); err != nil {
		t.Fatal(err)
	}
	c, err = store.BindSession(c.ID, historyFile)
	if err != nil {
		t.Fatal(err)
	}
	oldDirectory := c.SessionDirectory
	destination := filepath.Join(root, "destination")
	os.Mkdir(destination, 0700)
	c, err = store.UpdateConversation(c.ID, func(value *Conversation) error { return RelocateFiles(value, destination) })
	if err != nil {
		t.Fatal(err)
	}
	draft, err := store.ReadDraft(c.ID)
	if err != nil || len(draft.Attachments) != 1 {
		t.Fatal(draft, err)
	}
	if draft.Attachments[0].Path == attachments[0].Path || !within(c.SessionDirectory, draft.Attachments[0].Path) {
		t.Fatal("draft still points at old attachment", draft)
	}
	if err = store.ValidateAttachments(c.ID, []string{draft.Attachments[0].Path}); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{filepath.Join(oldDirectory, "history.jsonl"), c.SessionFile} {
		data, err := os.ReadFile(file)
		if err != nil || !bytes.Equal(data, history) {
			t.Fatal("original Pi history was rewritten", file, err)
		}
	}
	for _, file := range []string{attachments[0].Path, draft.Attachments[0].Path} {
		data, err := os.ReadFile(file)
		if err != nil || string(data) != "Imported file" {
			t.Fatal("attachment or recovery copy lost", file, err)
		}
	}
}

func TestRelocationRefusesConflictingDestinationAndKeepsOriginalIndex(t *testing.T) {
	root := t.TempDir()
	store, _ := Open(filepath.Join(root, "profile"))
	c, err := store.NewConversation("", "", "Conflict", ai.Selection{})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.SaveDraft(c.ID, Draft{Text: "original"}); err != nil {
		t.Fatal(err)
	}
	parent := filepath.Join(root, "destination")
	destination := filepath.Join(parent, ".winone-chat", c.ID)
	if err = os.MkdirAll(destination, 0700); err != nil {
		t.Fatal(err)
	}
	collision := filepath.Join(destination, "draft.json")
	if err = os.WriteFile(collision, []byte("other data"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = store.UpdateConversation(c.ID, func(next *Conversation) error { return RelocateFiles(next, parent) }); err == nil {
		t.Fatal("destination was overwritten")
	}
	actual, _ := store.Conversation(c.ID)
	if actual.SessionDirectory != c.SessionDirectory {
		t.Fatal("failed copy changed index")
	}
	content, err := os.ReadFile(collision)
	if err != nil || string(content) != "other data" {
		t.Fatal("destination was changed", err)
	}
	draft, err := store.ReadDraft(c.ID)
	if err != nil || draft.Text != "original" {
		t.Fatal("original draft lost", err)
	}
}
