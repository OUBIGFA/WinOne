//go:build windows

package chat

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"winone/internal/ai"
)

// A restored file tab can read its linked skill before the resource tab opens.
func TestOpenedSessionRestoresLinkedResourcePreviewGrants(t *testing.T) {
	store, _, link, target := workspaceLinkFixture(t)
	conversation, err := store.NewConversation("", "", "Restored", ai.Selection{})
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(context.Background(), store, ServiceOptions{})
	alias := filepath.Join(link, "SKILL.md")
	raw, _ := json.Marshal(map[string]any{"resources": map[string]any{"skills": []any{map[string]string{"path": alias}}}})
	if err = service.remember(conversation.ID, raw); err != nil {
		t.Fatal(err)
	}
	preview, err := store.PreviewFile(conversation.ID, alias)
	if err != nil || !preview.ReadOnly {
		t.Fatalf("restored linked skill: %+v %v", preview, err)
	}
	if _, err = store.PreviewFile(conversation.ID, filepath.Join(target, "SKILL.md")); err == nil {
		t.Fatal("unregistered target alias accepted")
	}
	raw, _ = json.Marshal(map[string]any{"resources": map[string]any{"skills": []any{}}})
	if err = service.remember(conversation.ID, raw); err != nil {
		t.Fatal(err)
	}
	if _, err = store.PreviewFile(conversation.ID, alias); err == nil {
		t.Fatal("removed skill remains authorized")
	}
}
