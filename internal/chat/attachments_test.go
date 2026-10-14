package chat

import (
	"os"
	"path/filepath"
	"testing"
	"winone/internal/ai"
)

func TestAttachmentsCopyIntoConversationAndRoundTripDraft(t *testing.T) {
	root := t.TempDir()
	s, _ := Open(filepath.Join(root, "profile"))
	c, err := s.NewConversation("", "", "Attachment", ai.Selection{})
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "notes.txt")
	if err = os.WriteFile(source, []byte("attachment evidence"), 0600); err != nil {
		t.Fatal(err)
	}
	attachments, err := s.AddAttachments(c.ID, []string{source})
	if err != nil {
		t.Fatal(err)
	}
	if len(attachments) != 1 || attachments[0].Path == source || !within(c.SessionDirectory, attachments[0].Path) {
		t.Fatal("attachment was not copied", attachments)
	}
	if raw, err := os.ReadFile(source); err != nil || string(raw) != "attachment evidence" {
		t.Fatal("source changed")
	}
	if err = s.SaveDraft(c.ID, Draft{Text: "Read this", Attachments: attachments}); err != nil {
		t.Fatal(err)
	}
	draft, err := s.ReadDraft(c.ID)
	if err != nil || len(draft.Attachments) != 1 || draft.Attachments[0].Name != "notes.txt" {
		t.Fatal("draft lost attachment", draft, err)
	}
}

func TestAttachmentsRejectDirectoriesAndUnregisteredPromptPaths(t *testing.T) {
	root := t.TempDir()
	s, _ := Open(filepath.Join(root, "profile"))
	c, _ := s.NewConversation("", "", "Attachment", ai.Selection{})
	if _, err := s.AddAttachments(c.ID, []string{root}); err == nil {
		t.Fatal("directory imported as file")
	}
	if err := s.ValidateAttachments(c.ID, []string{filepath.Join(root, "outside.txt")}); err == nil {
		t.Fatal("arbitrary path accepted as imported attachment")
	}
}
