package chat

import (
	"os"
	"path/filepath"
	"testing"
	"winone/internal/ai"
)

func TestProjectsAndProjectlessConversationLocationsPersist(t *testing.T) {
	root := t.TempDir()
	store, err := Open(filepath.Join(root, "profile"))
	if err != nil {
		t.Fatal(err)
	}
	project, err := store.CreateProject(root, "project")
	if err != nil {
		t.Fatal(err)
	}
	again, err := store.AddProject(project.Path, "ignored duplicate")
	if err != nil || again.ID != project.ID {
		t.Fatal("duplicate project", again, err)
	}
	c, err := store.NewConversation(project.ID, "", "Project chat", ai.Selection{})
	if err != nil {
		t.Fatal(err)
	}
	custom := filepath.Join(root, "chosen")
	if err = os.Mkdir(custom, 0700); err != nil {
		t.Fatal(err)
	}
	free, err := store.NewConversation("", custom, "Free chat", ai.Selection{})
	if err != nil {
		t.Fatal(err)
	}
	if c.Directory != project.Path || free.Directory != custom || !within(custom, free.SessionDirectory) {
		t.Fatal("workspace or chosen storage was changed", c, free)
	}
	reopened, err := Open(filepath.Join(root, "profile"))
	if err != nil || len(reopened.Snapshot().Conversations) != 2 {
		t.Fatal("chat catalog was not persisted", err)
	}
	if err = reopened.RemoveProject(project.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(project.Path); err != nil {
		t.Fatal("removing project metadata removed user files", err)
	}
	kept, _ := reopened.Conversation(c.ID)
	if kept.ProjectID != "" || kept.Directory != project.Path {
		t.Fatal("detaching project lost conversation", kept)
	}
}
func TestDraftAcknowledgementCannotEraseNewerTyping(t *testing.T) {
	store, _ := Open(t.TempDir())
	c, err := store.NewConversation("", "", "Chat", ai.Selection{})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.SaveDraft(c.ID, Draft{Text: "first input", PendingID: "request-one"}); err != nil {
		t.Fatal(err)
	}
	if err = store.SaveDraft(c.ID, Draft{Text: "next input"}); err != nil {
		t.Fatal(err)
	}
	if err = store.ConfirmInput(c.ID, "request-one"); err != nil {
		t.Fatal(err)
	}
	draft, err := store.ReadDraft(c.ID)
	if err != nil || draft.Text != "next input" {
		t.Fatal("late acknowledgement erased newer text", draft, err)
	}
}
func TestChatRejectsInvalidProjectNamesAndForeignSessionFiles(t *testing.T) {
	root := t.TempDir()
	store, _ := Open(filepath.Join(root, "data"))
	for _, name := range []string{"../escape", "CON", "trailing.", "a/b"} {
		if _, err := store.CreateProject(root, name); err == nil {
			t.Fatal("invalid directory name accepted", name)
		}
	}
	c, err := store.NewConversation("", "", "Chat", ai.Selection{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.BindSession(c.ID, filepath.Join(root, "outside.jsonl")); err == nil {
		t.Fatal("foreign Pi session file accepted")
	}
}
