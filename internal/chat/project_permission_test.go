package chat

import (
	"path/filepath"
	"testing"
	"winone/internal/ai"
)

func TestProjectPermissionUpdatesExistingAndFutureConversationsAndPersists(t *testing.T) {
	root := t.TempDir()
	s, err := Open(filepath.Join(root, "profile"))
	if err != nil {
		t.Fatal(err)
	}
	project, err := s.CreateProject(root, "project")
	if err != nil {
		t.Fatal(err)
	}
	first, _ := s.NewConversation(project.ID, "", "first", ai.Selection{})
	second, _ := s.NewConversation(project.ID, "", "second", ai.Selection{})
	other, _ := s.NewConversation("", "", "other", ai.Selection{})
	if _, err = s.UpdateProjectPermission(project.ID, "auto"); err != nil {
		t.Fatal(err)
	}
	third, _ := s.NewConversation(project.ID, "", "third", ai.Selection{})
	again, err := Open(filepath.Join(root, "profile"))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{first.ID, second.ID, third.ID} {
		c, err := again.Conversation(id)
		if err != nil || c.Permission != "auto" {
			t.Fatal(c, err)
		}
	}
	c, _ := again.Conversation(other.ID)
	if c.Permission != "ask" {
		t.Fatal("projectless permission changed")
	}
	if _, err = again.UpdateProjectPermission(project.ID, "invalid"); err == nil {
		t.Fatal("invalid permission accepted")
	}
}
