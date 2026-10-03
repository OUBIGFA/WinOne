package configuration

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"winone/internal/settings"
)

func TestHostMigratesLegacyOnceAndPreservesSource(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, "settings.json")
	original := []byte(`{"schemaVersion":1,"revision":9,"theme":"dark","captureShortcut":"Ctrl+Alt+S","pinShortcut":""}`)
	if err := os.WriteFile(legacy, original, 0600); err != nil {
		t.Fatal(err)
	}
	store, err := OpenHost(root)
	if err != nil {
		t.Fatal(err)
	}
	if store.Snapshot().Revision != 9 || store.Snapshot().Theme != "dark" {
		t.Fatal(store.Snapshot())
	}
	next := store.Snapshot()
	next.Theme = "light"
	saved, err := store.Save(next)
	if err != nil || saved.Revision != 10 {
		t.Fatal(saved, err)
	}
	data, err := os.ReadFile(legacy)
	if err != nil || string(data) != string(original) {
		t.Fatal("legacy file changed", err)
	}
	if err = os.WriteFile(legacy, []byte("old file no longer used"), 0600); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenHost(root)
	if err != nil || reopened.Snapshot().Theme != "light" {
		t.Fatal(err)
	}
	if _, err = reopened.Save(next); !errors.Is(err, settings.ErrConflict) {
		t.Fatal(err)
	}
}

func TestHostFailedApplyRollsBackPersistedPreferences(t *testing.T) {
	root := t.TempDir()
	store, err := OpenHost(root)
	if err != nil {
		t.Fatal(err)
	}
	runtimeTheme := "system"
	store.SetApply(func(value settings.Settings) error {
		runtimeTheme = value.Theme
		if value.Theme == "dark" {
			return errors.New("runtime rejected change")
		}
		return nil
	})
	next := store.Snapshot()
	next.Theme = "dark"
	if _, err = store.Save(next); err == nil {
		t.Fatal("failed apply accepted")
	}
	reopened, err := OpenHost(root)
	if err != nil {
		t.Fatal(err)
	}
	if runtimeTheme != "system" || reopened.Snapshot().Theme != "system" || reopened.Snapshot().Revision != 0 {
		t.Fatal(runtimeTheme, reopened.Snapshot())
	}
}

func TestHostCorruptCanonicalNeverFallsBackToLegacy(t *testing.T) {
	root := t.TempDir()
	if _, err := OpenHost(root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "configuration.json"), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenHost(root); err == nil {
		t.Fatal("corruption hidden by legacy defaults")
	}
}

func TestHostRestartDiscardsUncommittedPreferences(t *testing.T) {
	root := t.TempDir()
	store, err := OpenHost(root)
	if err != nil {
		t.Fatal(err)
	}
	store.SetApply(func(settings.Settings) error { return errors.New("runtime unavailable") })
	next := store.Snapshot()
	next.Theme = "dark"
	if _, err = store.Save(next); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatal(err)
	}
	reopened, err := OpenHost(root)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Snapshot().Theme != "system" || reopened.Snapshot().Revision != 0 {
		t.Fatal(reopened.Snapshot())
	}
	next = reopened.Snapshot()
	next.Theme = "light"
	if _, err = reopened.Save(next); err != nil {
		t.Fatal(err)
	}
}
