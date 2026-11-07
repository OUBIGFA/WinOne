package chat

import (
	"os"
	"path/filepath"
	"testing"
	"winone/internal/ai"
)

func TestLegacyPlanIsPersistedWithTheInitialConversationInsteadOfBroadAsk(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, "work")
	if err := os.MkdirAll(work, 0700); err != nil {
		t.Fatal(err)
	}
	store, err := Open(filepath.Join(root, "profile"))
	if err != nil {
		t.Fatal(err)
	}
	row, err := store.NewConfiguredConversation("", work, "Read-only plan", ai.Selection{Provider: "fixture", Model: "model"}, "plan", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if row.Permission != "ask" || !row.LegacyPlan {
		t.Fatalf("legacy plan widened: %+v", row)
	}
	reopened, err := Open(filepath.Join(root, "profile"))
	if err != nil {
		t.Fatal(err)
	}
	stored, err := reopened.Conversation(row.ID)
	if err != nil || !stored.LegacyPlan || stored.Permission != "ask" {
		t.Fatalf("plan marker lost on reopen: %+v %v", stored, err)
	}
}
