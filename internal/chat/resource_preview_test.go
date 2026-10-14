package chat

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"winone/internal/ai"
)

// The resource listing is a trusted Worker response, not a renderer path grant.
func TestLoadedResourcesPreviewExactFilesAndRevokeRemovedResources(t *testing.T) {
	root := t.TempDir()
	store, err := Open(filepath.Join(root, "profile"))
	if err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(root, "work")
	if err = os.Mkdir(workspace, 0700); err != nil {
		t.Fatal(err)
	}
	first, err := store.NewConversation("", workspace, "First", ai.Selection{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.NewConversation("", workspace, "Second", ai.Selection{})
	if err != nil {
		t.Fatal(err)
	}
	skill, secret := filepath.Join(root, "SKILL.md"), filepath.Join(root, "secret.txt")
	for path, body := range map[string]string{skill: "# 中文技能\n", secret: "private"} {
		if err = os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	registrar, ok := any(store).(interface {
		RememberResources(string, json.RawMessage) error
	})
	if !ok {
		t.Fatal("loaded resources have no scoped preview registration")
	}
	raw, _ := json.Marshal(map[string]any{"agents": []any{}, "skills": []any{map[string]string{"path": skill}}})
	if err = registrar.RememberResources(first.ID, raw); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server, err := NewPreviewServer(ctx, store)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	preview, err := server.Open(first.ID, skill)
	if err != nil || !preview.ReadOnly {
		t.Fatalf("resource preview: %+v %v", preview, err)
	}
	response, err := http.Get(preview.URL)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != 200 || string(data) != "# 中文技能\n" {
		t.Fatalf("resource content: %d %q %v", response.StatusCode, data, err)
	}
	if _, err = store.PreviewFile(first.ID, secret); err == nil {
		t.Fatal("resource directory exposed")
	}
	if _, err = store.PreviewFile(second.ID, skill); err == nil {
		t.Fatal("resource exposed to another conversation")
	}
	if _, err = store.SavePreviewFile(first.ID, skill, preview.Revision, "changed"); err == nil {
		t.Fatal("external resource became writable")
	}
	if err = registrar.RememberResources(first.ID, json.RawMessage(`{"agents":[],"skills":[]}`)); err != nil {
		t.Fatal(err)
	}
	response, err = http.Get(preview.URL)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatal("removed resource grant still works", response.StatusCode)
	}
}
