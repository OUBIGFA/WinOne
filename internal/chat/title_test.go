package chat

import (
	"path/filepath"
	"testing"
	"winone/internal/ai"
)

// AI 请求期间正常会话更新不应被误判成用户修改了标题。
func TestRequestedTitleSurvivesUnrelatedConversationUpdate(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "profile"))
	if err != nil { t.Fatal(err) }
	before, err := s.NewConversation("", "", "Original", ai.Selection{})
	if err != nil { t.Fatal(err) }
	_, err = s.UpdateConversation(before.ID, func(c *Conversation) error {
		c.HasMessages = true
		c.UpdatedAt = "2000-01-01T00:00:00Z"
		return nil
	})
	if err != nil { t.Fatal(err) }
	current, err := s.ApplyRequestedTitle(before, "AI title")
	if err != nil || current.Title != "AI title" || !current.HasMessages {
		t.Fatalf("普通会话更新不应阻止 AI 重命名: %+v, %v", current, err)
	}
}

func TestExplicitGeneratedTitleReplacesExistingTitleButPreservesLaterEdits(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "profile"))
	if err != nil {
		t.Fatal(err)
	}
	c, _ := s.NewConversation("", "", "Old title", ai.Selection{})
	c, _ = s.RenameConversation(c.ID, "Manual title")
	renamed, err := s.ApplyRequestedTitle(c, "AI title")
	if err != nil || renamed.Title != "AI title" {
		t.Fatal(renamed, err)
	}
	snapshot := renamed
	if _, err = s.RenameConversation(c.ID, "New manual title"); err != nil {
		t.Fatal(err)
	}
	renamed, err = s.ApplyRequestedTitle(snapshot, "Late AI title")
	if err == nil || renamed.Title == "Late AI title" {
		t.Fatal("late title must report conflict", renamed, err)
	}
	current, _ := s.Conversation(c.ID)
	if current.Title != "New manual title" {
		t.Fatal(current)
	}
}

func TestTitleModelPersistsAndManualTitleWinsOverLateGeneration(t *testing.T) {
	root := t.TempDir()
	s, err := Open(filepath.Join(root, "profile"))
	if err != nil {
		t.Fatal(err)
	}
	prefs := s.Preferences()
	prefs.TitleModel = ai.Selection{Provider: "local", Model: "title"}
	if _, err = s.SavePreferences(prefs); err != nil {
		t.Fatal(err)
	}
	again, err := Open(filepath.Join(root, "profile"))
	if err != nil {
		t.Fatal(err)
	}
	if again.Preferences().TitleModel != prefs.TitleModel {
		t.Fatal("title model was lost")
	}
	c, _ := s.NewConversation("", "", "Chat", ai.Selection{})
	if _, err = s.RenameConversation(c.ID, "My title"); err != nil {
		t.Fatal(err)
	}
	current, err := s.ApplyGeneratedTitle(c.ID, "Late generated title")
	if err != nil || current.Title != "My title" {
		t.Fatal(current, err)
	}
	c, _ = s.NewConversation("", "", "Chat", ai.Selection{})
	current, err = s.ApplyGeneratedTitle(c.ID, "Generated title")
	if err != nil || current.Title != "Generated title" {
		t.Fatal(current, err)
	}
}
