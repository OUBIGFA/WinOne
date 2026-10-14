package chat

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"winone/internal/ai"
)

func TestHTMLPreviewServesItsLocalAssetsAndRejectsParentPaths(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "profile"))
	if err != nil {
		t.Fatal(err)
	}
	c, err := store.NewConversation("", "", "HTML", ai.Selection{})
	if err != nil {
		t.Fatal(err)
	}
	folder := filepath.Join(c.Directory, "page")
	os.MkdirAll(filepath.Join(folder, "assets"), 0700)
	for name, text := range map[string]string{"index.html": "<script src='./assets/app.js'></script>", "assets/app.js": "document.body.textContent='Rendered';"} {
		if err = os.WriteFile(filepath.Join(folder, name), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server, err := NewPreviewServer(ctx, store)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	preview, err := server.Open(c.ID, filepath.Join(folder, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	base := preview.URL[:strings.LastIndex(preview.URL, "/")+1]
	response, err := http.Get(base + "assets/app.js")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 200 || string(body) != "document.body.textContent='Rendered';" {
		t.Fatalf("HTML asset: %d %q", response.StatusCode, body)
	}
	response, err = http.Get(base + "%2e%2e/secret.json")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode == 200 {
		t.Fatal("parent traversal accepted")
	}
}

func TestPreviewSandboxAppliesToHTMLWithoutDisablingPDFViewer(t *testing.T) {
	root := t.TempDir()
	store, err := Open(filepath.Join(root, "profile"))
	if err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(root, "workspace")
	if err = os.Mkdir(workspace, 0700); err != nil {
		t.Fatal(err)
	}
	conversation, err := store.NewConversation("", workspace, "Preview", ai.Selection{})
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
	for _, name := range []string{"page.html", "report.pdf"} {
		file := filepath.Join(workspace, name)
		if err = os.WriteFile(file, []byte("Preview fixture"), 0600); err != nil {
			t.Fatal(err)
		}
		preview, err := server.Open(conversation.ID, file)
		if err != nil {
			t.Fatal(err)
		}
		response, err := http.Get(preview.URL)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatal(response.Status)
		}
		sandboxed := strings.Contains(response.Header.Get("Content-Security-Policy"), "sandbox")
		if sandboxed != (name == "page.html") {
			t.Fatalf("%s sandbox=%v; PDF's native viewer must remain enabled", name, sandboxed)
		}
	}
}
