package chat

import (
	"mime"
	"os"
	"path/filepath"
	"testing"
	"winone/internal/ai"
)

func TestSourceFilePreviewIgnoresWindowsTransportStreamAssociation(t *testing.T) {
	// Windows may register .ts as video/mp2t. Valid TypeScript is still text.
	if err := mime.AddExtensionType(".ts", "video/mp2t"); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	s, err := Open(filepath.Join(root, "profile"))
	if err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(root, "work")
	if err = os.Mkdir(workspace, 0700); err != nil {
		t.Fatal(err)
	}
	c, err := s.NewConversation("", workspace, "Chat", ai.Selection{})
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(workspace, "example.ts")
	if err = os.WriteFile(file, []byte("export const answer = 42;\n"), 0600); err != nil {
		t.Fatal(err)
	}
	preview, err := s.PreviewFile(c.ID, file)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Kind != "text" || preview.Revision == "" {
		t.Fatalf("source file is not editable: %#v", preview)
	}
}

func TestNewPageCreatesOnlyANewWorkspaceFile(t *testing.T) {
	root := t.TempDir()
	s, err := Open(filepath.Join(root, "profile"))
	if err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(root, "work")
	if err = os.Mkdir(workspace, 0700); err != nil {
		t.Fatal(err)
	}
	c, err := s.NewConversation("", workspace, "Chat", ai.Selection{})
	if err != nil {
		t.Fatal(err)
	}
	creator, ok := any(s).(interface {
		CreatePreviewFile(string, string) (FilePreview, error)
	})
	if !ok {
		t.Fatal("new page does not have a real file creation operation")
	}
	created, err := creator.CreatePreviewFile(c.ID, "New page.md")
	if err != nil {
		t.Fatal(err)
	}
	if created.Kind != "text" || created.Path != filepath.Join(workspace, "New page.md") {
		t.Fatal(created)
	}
	if err = os.WriteFile(created.Path, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = creator.CreatePreviewFile(c.ID, "New page.md"); err == nil {
		t.Fatal("existing file overwritten")
	}
	if _, err = creator.CreatePreviewFile(c.ID, filepath.Join("..", "outside.md")); err == nil {
		t.Fatal("file created outside workspace")
	}
	contents, err := os.ReadFile(created.Path)
	if err != nil || string(contents) != "keep" {
		t.Fatal("existing content changed", err)
	}
}

func TestFilePreviewAndSaveStayInWorkspaceAndRejectStaleContent(t *testing.T) {
	root := t.TempDir()
	s, err := Open(filepath.Join(root, "profile"))
	if err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(root, "work")
	if err = os.Mkdir(workspace, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(workspace, "hello.md")
	if err = os.WriteFile(file, []byte("原文\n"), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := s.NewConversation("", workspace, "Chat", ai.Selection{})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := s.PreviewFile(c.ID, file)
	if err != nil || preview.Kind != "text" {
		t.Fatal(preview, err)
	}
	if _, err = s.SavePreviewFile(c.ID, file, preview.Revision, "新内容\n"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SavePreviewFile(c.ID, file, preview.Revision, "过期覆盖"); err == nil {
		t.Fatal("stale edit accepted")
	}
	data, _ := os.ReadFile(file)
	if string(data) != "新内容\n" {
		t.Fatal(string(data))
	}
	outside := filepath.Join(root, "outside.txt")
	os.WriteFile(outside, []byte("private"), 0600)
	if _, err = s.PreviewFile(c.ID, outside); err == nil {
		t.Fatal("outside workspace preview accepted")
	}
	if _, err = s.PreviewFile(c.ID, filepath.Join(workspace, "..", "outside.txt")); err == nil {
		t.Fatal("traversal accepted")
	}
}
