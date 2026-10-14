//go:build windows

package chat

import (
	"context"
	"errors"
	"golang.org/x/sys/windows"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"winone/internal/ai"
)

// Junctions require neither developer mode nor symbolic-link privileges. This
// fixture exercises the Windows filesystem, including the resource-folder case.
func workspaceLinkFixture(t *testing.T) (*Store, Conversation, string, string) {
	t.Helper()
	root := t.TempDir()
	workspace, target := filepath.Join(root, "workspace"), filepath.Join(root, "shared-skills")
	for _, dir := range []string{workspace, target} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(target, "SKILL.md"), []byte("Shared skill\n"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(workspace, "skills")
	command := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", "New-Item -ItemType Junction -Path $env:WINONE_TEST_LINK -Target $env:WINONE_TEST_TARGET -ErrorAction Stop | Out-Null")
	command.Env = append(os.Environ(), "WINONE_TEST_LINK="+link, "WINONE_TEST_TARGET="+target)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("create junction: %s: %v", output, err)
	}
	if _, err := os.Stat(filepath.Join(link, "SKILL.md")); err != nil {
		t.Fatalf("junction fixture cannot be read: %v", err)
	}
	store, err := Open(filepath.Join(root, "profile"))
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := store.NewConversation("", workspace, "Links", ai.Selection{})
	if err != nil {
		t.Fatal(err)
	}
	return store, conversation, link, target
}

func TestWorkspaceJunctionListsAndPreviewsThroughItsWorkspaceAlias(t *testing.T) {
	store, conversation, link, target := workspaceLinkFixture(t)
	rootPage, err := store.Files(conversation.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range rootPage.Entries {
		if entry.Path == link {
			found = entry.Directory
		}
	}
	if !found {
		t.Fatalf("junction is not a browsable directory: %+v", rootPage)
	}
	page, err := store.Files(conversation.ID, link)
	if err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(link, "SKILL.md")
	if page.Directory != link || len(page.Entries) != 1 || page.Entries[0].Path != alias {
		t.Fatalf("workspace link identity was lost: %+v", page)
	}
	preview, err := store.PreviewFile(conversation.ID, alias)
	if err != nil || preview.Path != alias || preview.Kind != "text" || !preview.ReadOnly {
		t.Fatalf("linked preview: %+v %v", preview, err)
	}
	if _, err = store.Files(conversation.ID, target); err == nil {
		t.Fatal("external directory accepted without workspace link")
	}
	if _, err = store.PreviewFile(conversation.ID, filepath.Join(target, "SKILL.md")); err == nil {
		t.Fatal("external file accepted without workspace link")
	}
	if _, err = store.CreatePreviewFile(conversation.ID, filepath.Join(link, "new.md")); err == nil {
		t.Fatal("read-only external link allowed new file creation")
	}
	if _, err = store.SavePreviewFile(conversation.ID, alias, preview.Revision, "Changed"); err == nil {
		t.Fatal("read-only external link allowed file editing")
	}
}

func TestWorkspaceJunctionPreviewGrantTracksAliasAndRejectsRetargeting(t *testing.T) {
	store, conversation, link, target := workspaceLinkFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server, err := NewPreviewServer(ctx, store)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	for _, file := range []string{filepath.Join(link, "SKILL.md")} {
		preview, err := server.Open(conversation.ID, file)
		if err != nil {
			t.Fatal(err)
		}
		response, err := http.Get(preview.URL)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil || response.StatusCode != http.StatusOK || string(body) != "Shared skill\n" {
			t.Fatalf("linked HTTP preview: %s %q %v", response.Status, body, err)
		}
		{
			// Rename only our junction entry; its target and contents remain intact.
			if err = os.Rename(link, link+"-old"); err != nil {
				t.Fatal(err)
			}
			if err = os.Mkdir(link, 0700); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(link, "SKILL.md"), []byte("Other target"), 0600); err != nil {
				t.Fatal(err)
			}
			response, err = http.Get(preview.URL)
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if response.StatusCode != http.StatusForbidden {
				t.Fatalf("retargeted link reused old grant: %s", response.Status)
			}
		}
	}
	if contents, err := os.ReadFile(filepath.Join(target, "SKILL.md")); err != nil || string(contents) != "Shared skill\n" {
		t.Fatal("original target changed", err)
	}
}

func TestWorkspaceFileSymlinkPreservesAlias(t *testing.T) {
	store, conversation, _, target := workspaceLinkFixture(t)
	alias := filepath.Join(conversation.Directory, "linked.md")
	if err := os.Symlink(filepath.Join(target, "SKILL.md"), alias); err != nil {
		if errors.Is(err, windows.ERROR_PRIVILEGE_NOT_HELD) {
			t.Skip("Windows does not grant this process file-symlink creation; junction coverage still runs")
		}
		t.Fatal(err)
	}
	preview, err := store.PreviewFile(conversation.ID, alias)
	if err != nil || preview.Path != alias || preview.Kind != "text" || !preview.ReadOnly {
		t.Fatalf("file symlink: %+v %v", preview, err)
	}
}
