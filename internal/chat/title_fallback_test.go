package chat

import (
	"path/filepath"
	"strings"
	"testing"
	"winone/internal/ai"
)

func TestFallbackTitlePreservesManualEditsAndTruncatesUnicode(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "profile"))
	if err != nil {
		t.Fatal(err)
	}
	before, err := s.NewConversation("", "", "Chat", ai.Selection{})
	if err != nil {
		t.Fatal(err)
	}
	saved, err := s.ApplyFallbackTitle(before, strings.Repeat("测😀", 30), false)
	if err != nil || len([]rune(saved.Title)) != 40 {
		t.Fatal(saved.Title, err)
	}
	before = saved
	if _, err = s.RenameConversation(before.ID, "我的标题"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ApplyFallbackTitle(before, "过期内容", false); err == nil {
		t.Fatal("late fallback overwrote manual title")
	}
	saved, _ = s.Conversation(before.ID)
	if saved.Title != "我的标题" {
		t.Fatal(saved.Title)
	}
}
