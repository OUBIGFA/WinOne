package settings

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// A stale window must not overwrite settings committed by another client.
func TestRevisionConflictAndReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	first := store.Snapshot()
	first.Theme = "dark"
	saved, err := store.Save(first)
	if err != nil || saved.Revision != 1 {
		t.Fatalf("save: %+v %v", saved, err)
	}
	first.Theme = "light"
	if _, err := store.Save(first); !errors.Is(err, ErrConflict) {
		t.Fatalf("got %v", err)
	}
	reopened, err := Open(path)
	if err != nil || reopened.Snapshot().Theme != "dark" {
		t.Fatalf("reload: %v", err)
	}
}

func TestCorruptionIsPreserved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	_ = os.WriteFile(path, []byte("broken"), 0600)
	if _, err := Open(path); err == nil {
		t.Fatal("corruption accepted")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "broken" {
		t.Fatal("corruption overwritten")
	}
}

func TestInvalidThemeDoesNotPersist(t *testing.T) {
	store, _ := Open(filepath.Join(t.TempDir(), "settings.json"))
	next := store.Snapshot()
	next.Theme = "invalid"
	if _, err := store.Save(next); err == nil {
		t.Fatal("invalid theme accepted")
	}
	if store.Snapshot().Revision != 0 {
		t.Fatal("failed save advanced revision")
	}
}
