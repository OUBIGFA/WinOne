package chat

import (
	"os"
	"path/filepath"
	"testing"

	"winone/internal/ai"
)

func TestNewConfiguredConversationAppliesRouteAndPermissionAtomically(t *testing.T) {
	root := t.TempDir()
	workdir := filepath.Join(root, "work")
	if err := os.MkdirAll(workdir, 0700); err != nil {
		t.Fatal(err)
	}
	store, err := Open(filepath.Join(root, "profile"))
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := store.NewConfiguredConversation("", workdir, "Legacy lane", ai.Selection{Provider: "fixture", Model: "model"}, "ask", "high", true)
	if err != nil {
		t.Fatal(err)
	}
	if conversation.Model != (ai.Selection{Provider: "fixture", Model: "model"}) || conversation.Permission != "ask" || conversation.ThinkingLevel != "high" || !conversation.FastMode {
		t.Fatalf("configured route was not committed with the conversation: %+v", conversation)
	}
	stored, err := store.Conversation(conversation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored != conversation {
		t.Fatalf("configured conversation was not persisted atomically: %+v != %+v", stored, conversation)
	}
}
