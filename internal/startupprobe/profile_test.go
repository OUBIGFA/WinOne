package startupprobe

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProfileSnapshotBindsControlFilesWithoutCreatingMissingState(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "configuration.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	first, err := ReadProfileSnapshot(root)
	if err != nil {
		t.Fatal(err)
	}
	if first.ModuleState != nil || first.Registry != nil {
		t.Fatal("missing control files were invented")
	}
	if _, err = os.Stat(filepath.Join(root, "packages")); !os.IsNotExist(err) {
		t.Fatal("readonly snapshot created package storage")
	}
	if err = os.WriteFile(filepath.Join(root, "module-state.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	second, err := ReadProfileSnapshot(root)
	if err != nil || second.Digest() == first.Digest() {
		t.Fatal("module state not bound", err)
	}
	if err = os.Mkdir(filepath.Join(root, "packages"), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "packages", ".winone-module-registry.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	third, err := ReadProfileSnapshot(root)
	if err != nil || third.Digest() == second.Digest() {
		t.Fatal("package registry not bound", err)
	}
	if err = os.WriteFile(filepath.Join(root, "module-state.json"), make([]byte, (1<<20)+1), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = ReadProfileSnapshot(root); err == nil {
		t.Fatal("unbounded control file accepted")
	}
}

func TestProfileSnapshotBindsSharedRuntimeInventoryAndCleanup(t *testing.T) {
	root := t.TempDir()
	before, err := ReadProfileSnapshot(root)
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(root, "packages", "shared-runtimes")
	if err = os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".winone-runtime-registry.json", ".winone-module-reclaim.json"} {
		if err = os.WriteFile(filepath.Join(directory, name), []byte(`{"revision":1}`), 0600); err != nil {
			t.Fatal(err)
		}
		after, err := ReadProfileSnapshot(root)
		if err != nil {
			t.Fatal(err)
		}
		if !after.RequiresPackageCheck() || after.Digest() == before.Digest() {
			t.Fatal("shared runtime state was omitted from the profile approval", name)
		}
		before = after
	}
}
