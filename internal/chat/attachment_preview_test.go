package chat

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"winone/internal/ai"
)

func TestManagedAttachmentPreviewIsScopedAndImmutable(t *testing.T) {
	root := t.TempDir()
	store, err := Open(filepath.Join(root, "profile"))
	if err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(root, "work")
	if err = os.Mkdir(workspace, 0700); err != nil {
		t.Fatal(err)
	}
	history := filepath.Join(root, "history")
	if err = os.Mkdir(history, 0700); err != nil {
		t.Fatal(err)
	}
	first, err := store.NewConversation("", workspace, "First", ai.Selection{}, history)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.NewConversation("", workspace, "Second", ai.Selection{}, history)
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "notes.md")
	if err = os.WriteFile(source, []byte("# Attached notes\n"), 0600); err != nil {
		t.Fatal(err)
	}
	files, err := store.AddAttachments(first.ID, []string{source})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server, err := NewPreviewServer(ctx, store)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	preview, err := server.Open(first.ID, files[0].Path)
	if err != nil {
		t.Fatalf("managed attachment cannot be previewed: %v", err)
	}
	response, err := http.Get(preview.URL)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != http.StatusOK || string(data) != "# Attached notes\n" {
		t.Fatal(response.StatusCode, string(data), err)
	}
	for _, path := range []string{source, filepath.Join(first.SessionDirectory, "index.json")} {
		if _, err = server.Open(first.ID, path); err == nil {
			t.Fatalf("unmanaged path exposed: %s", path)
		}
	}
	if _, err = server.Open(second.ID, files[0].Path); err == nil {
		t.Fatal("another conversation's attachment exposed")
	}
	if _, err = store.SavePreviewFile(first.ID, files[0].Path, preview.Revision, "changed"); err == nil {
		t.Fatal("sent attachment was modified")
	}
	data, err = os.ReadFile(files[0].Path)
	if err != nil || string(data) != "# Attached notes\n" {
		t.Fatal("attachment copy changed", err)
	}
}
