package chat

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"winone/internal/ai"
)

func TestWorkspacePreferenceOnlyAffectsNewConversations(t *testing.T) {
	root := t.TempDir()
	s, err := Open(filepath.Join(root, "profile"))
	if err != nil {
		t.Fatal(err)
	}
	old, err := s.NewConversation("", "", "Old", ai.Selection{})
	if err != nil {
		t.Fatal(err)
	}
	custom := filepath.Join(root, "new-workspaces")
	if err = os.Mkdir(custom, 0700); err != nil {
		t.Fatal(err)
	}
	prefs := s.Preferences()
	prefs.WorkspaceDirectory = custom
	if _, err = s.SavePreferences(prefs); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(filepath.Join(root, "profile"))
	if err != nil {
		t.Fatal(err)
	}
	next, err := reopened.NewConversation("", "", "New", ai.Selection{})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(next.Directory) != custom {
		t.Fatalf("new conversation ignored preference: %s", next.Directory)
	}
	kept, _ := reopened.Conversation(old.ID)
	if kept.Directory != old.Directory {
		t.Fatal("preference relocated existing work")
	}
}

func TestInvalidPreferencesLeaveSavedConfigurationUntouched(t *testing.T) {
	s, _ := Open(t.TempDir())
	prefs := s.Preferences()
	prefs.SkillDirectories = []string{"relative/path"}
	if _, err := s.SavePreferences(prefs); err == nil {
		t.Fatal("relative skill path accepted")
	}
	if len(s.Preferences().SkillDirectories) != 0 {
		t.Fatal("invalid preferences persisted")
	}
}

func TestDefaultWorkspaceCanBeSavedBeforeTheFirstConversation(t *testing.T) {
	root := t.TempDir()
	s, _ := Open(filepath.Join(root, "chat"))
	prefs := s.Preferences()
	if _, err := s.SavePreferences(prefs); err != nil {
		t.Fatal("first settings save requires a pre-existing conversation", err)
	}
}

func TestConversationDeletionRecyclesOnlyHistoryAndPreservesOnFailure(t *testing.T) {
	root := t.TempDir()
	s, _ := Open(filepath.Join(root, "profile"))
	c, err := s.NewConversation("", root, "Delete", ai.Selection{})
	if err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(root, "work.txt")
	if err = os.WriteFile(work, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveDraft(c.ID, Draft{Text: "draft"}); err != nil {
		t.Fatal(err)
	}
	refused := errors.New("recycle unavailable")
	if err = s.DeleteConversation(c.ID, func(string) error { return refused }); !errors.Is(err, refused) {
		t.Fatal(err)
	}
	if _, err = s.Conversation(c.ID); err != nil {
		t.Fatal("failed recycling removed catalog entry")
	}
	var recycled string
	if err = s.DeleteConversation(c.ID, func(source string) error {
		recycled = source
		return os.Rename(source, filepath.Join(root, "recycled-history"))
	}); err != nil {
		t.Fatal(err)
	}
	if recycled != c.SessionDirectory {
		t.Fatal("wrong recycle target", recycled)
	}
	reopened, err := Open(filepath.Join(root, "profile"))
	if err != nil {
		t.Fatal(err)
	}
	if len(reopened.Snapshot().Conversations) != 0 {
		t.Fatal("deleted conversation returned after restart")
	}
	if raw, err := os.ReadFile(work); err != nil || string(raw) != "keep" {
		t.Fatal("work file affected", err)
	}
	if raw, err := os.ReadFile(filepath.Join(root, "recycled-history", "draft.json")); err != nil || len(raw) == 0 {
		t.Fatal("draft not recoverable", err)
	}
}
