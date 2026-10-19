//go:build windows

package directorylinks

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func renameRequest(t *testing.T, m *Manager, path, name string) RenameRequest {
	t.Helper()
	listing, err := m.Browse(context.Background(), filepath.Dir(path), 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range listing.Entries {
		if strings.EqualFold(entry.Path, path) {
			return RenameRequest{Path: path, Name: name, EntryIdentity: entry.Identity, ParentIdentity: listing.Identity}
		}
	}
	t.Fatalf("entry missing: %s", path)
	return RenameRequest{}
}

func TestRenamePlainFileAndFolderPreservesContents(t *testing.T) {
	for _, directory := range []bool{false, true} {
		t.Run(map[bool]string{false: "file", true: "directory"}[directory], func(t *testing.T) {
			root := t.TempDir()
			m, err := New(filepath.Join(root, "history"))
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, "原项目.txt")
			data := path
			if directory {
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
				data = filepath.Join(path, "keep.txt")
			}
			if err := os.WriteFile(data, []byte("kept"), 0600); err != nil {
				t.Fatal(err)
			}
			result, err := m.Rename(context.Background(), renameRequest(t, m, path, "重命名 😀.txt"))
			if err != nil || result.Warning != "" || len(result.Records) != 0 {
				t.Fatal(result, err)
			}
			if _, err := os.Lstat(path); !os.IsNotExist(err) {
				t.Fatal("old name remains", err)
			}
			data = result.Path
			if directory {
				data = filepath.Join(data, "keep.txt")
			}
			if raw, err := os.ReadFile(data); err != nil || string(raw) != "kept" {
				t.Fatal("renamed content damaged", err)
			}
			if len(m.List()) != 0 {
				t.Fatal("plain entry was added to link history")
			}
		})
	}
}

func TestRenameLinksOnlyChangesEntryAndPersistsHistory(t *testing.T) {
	for _, tc := range []struct{ kind, object string }{{"hardlink", "file"}, {"junction", "directory"}, {"symlink", "file"}, {"symlink", "directory"}} {
		t.Run(tc.kind+"-"+tc.object, func(t *testing.T) {
			m, record, source := linkFixture(t, tc.kind, tc.object)
			request := renameRequest(t, m, record.LinkPath, "new-link")
			result, err := m.Rename(context.Background(), request)
			if err != nil || result.Warning != "" || len(result.Records) != 1 {
				t.Fatal(result, err)
			}
			actual := result.Records[0]
			if actual.LinkPath != result.Path || actual.TargetPath != source || actual.Status != "active" || !sameIdentity(actual.EntryIdentity, record.EntryIdentity) {
				t.Fatal("renamed link lost identity/target", actual)
			}
			if tc.object == "directory" {
				source = filepath.Join(source, "keep.txt")
			}
			if raw, err := os.ReadFile(source); err != nil || string(raw) != "target data" {
				t.Fatal("link rename altered source", err)
			}
			reopened, err := New(m.directory)
			if err != nil || reopened.List()[0].LinkPath != result.Path {
				t.Fatal("renamed history was not persisted", err)
			}
			if _, err := m.Rename(context.Background(), request); err == nil {
				t.Fatal("stale original name accepted twice")
			}
		})
	}
}

func TestRenameRejectsCollisionsInvalidNamesAndStaleEntries(t *testing.T) {
	m, record, source := linkFixture(t, "hardlink", "file")
	base := renameRequest(t, m, record.LinkPath, "valid.txt")
	// A collision with another hard link must not be treated as a case-only rename.
	for _, name := range []string{filepath.Base(source), "", ".", "..", `..\escaped`, `sub\file`, "CON.txt", "NUL", "aux.log", "LPT¹ .txt", "COM9.data", "CONIN$.txt", "nul .txt", "file:stream", "trailing.", "trailing ", "a\tb", strings.Repeat("界", 256)} {
		request := base
		request.Name = name
		if _, err := m.Rename(context.Background(), request); err == nil {
			t.Fatalf("unsafe name accepted: %q", name)
		}
	}
	if raw, err := os.ReadFile(source); err != nil || string(raw) != "target data" {
		t.Fatal("collision damaged source", err)
	}
	if err := os.Rename(record.LinkPath, record.LinkPath+".kept"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(record.LinkPath, []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Rename(context.Background(), base); err == nil || !strings.Contains(err.Error(), "替换") {
		t.Fatal("stale entry was renamed", err)
	}
	base = renameRequest(t, m, record.LinkPath, "valid.txt")
	base.ParentIdentity.File = "not-this-parent"
	if _, err := m.Rename(context.Background(), base); err == nil {
		t.Fatal("stale parent was accepted")
	}
	base.EntryIdentity = Identity{}
	if _, err := m.Rename(context.Background(), base); err == nil {
		t.Fatal("missing identity was accepted")
	}
}

func TestRenameAllowsOrdinaryNamesNearDeviceNames(t *testing.T) {
	m, record, _ := linkFixture(t, "hardlink", "file")
	path := record.LinkPath
	for _, name := range []string{".config", "console.txt", "com10.log", "auxiliary.txt"} {
		result, err := m.Rename(context.Background(), renameRequest(t, m, path, name))
		if err != nil {
			t.Fatalf("ordinary name rejected: %q: %v", name, err)
		}
		path = result.Path
		if data, err := os.ReadFile(path); err != nil || string(data) != "target data" {
			t.Fatalf("renamed content changed: %q: %v", name, err)
		}
	}
}

func TestRenameCaseOnlyAndCancellation(t *testing.T) {
	m, record, _ := linkFixture(t, "hardlink", "file")
	request := renameRequest(t, m, record.LinkPath, strings.ToUpper(filepath.Base(record.LinkPath)))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := m.Rename(ctx, request); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost", err)
	}
	result, err := m.Rename(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	listing, err := m.Browse(context.Background(), filepath.Dir(result.Path), 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range listing.Entries {
		if entry.Name == request.Name {
			return
		}
	}
	t.Fatal("case-only rename did not preserve requested spelling")
}

func TestRenameUsesPinnedParentAfterAncestorMoves(t *testing.T) {
	root := t.TempDir()
	original := filepath.Join(root, "original")
	if err := os.Mkdir(original, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(original, "file.txt")
	if err := os.WriteFile(path, []byte("owned"), 0600); err != nil {
		t.Fatal(err)
	}
	parent, err := openState(original, true, false)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.close()
	moved := filepath.Join(root, "moved")
	if err := os.Rename(original, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(original, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	entry, err := openRenameEntry(parent.handle, path)
	if err != nil {
		t.Fatal(err)
	}
	defer entry.close()
	if err := renameEntry(entry.handle, parent.handle, "new.txt"); err != nil {
		t.Fatal(err)
	}
	entry.close()
	for file, expected := range map[string]string{path: "replacement", filepath.Join(moved, "new.txt"): "owned"} {
		if raw, err := os.ReadFile(file); err != nil || string(raw) != expected {
			t.Fatal("rename followed a replaced ancestor", file, err)
		}
	}
}

func TestRenameReportsHistoryFailureWithoutPretendingFilesystemFailed(t *testing.T) {
	m, record, _ := linkFixture(t, "hardlink", "file")
	request := renameRequest(t, m, record.LinkPath, "new.txt")
	history := filepath.Join(m.directory, "history.json")
	if err := os.Rename(history, history+".kept"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(history, 0700); err != nil {
		t.Fatal(err)
	}
	result, err := m.Rename(context.Background(), request)
	if err != nil || result.Path == "" || result.Warning == "" || len(result.Records) != 1 {
		t.Fatal("filesystem success / history failure not distinguished", result, err)
	}
	if _, err := os.Lstat(result.Path); err != nil {
		t.Fatal(err)
	}
}
