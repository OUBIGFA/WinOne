package worker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"winone/modules/wintray/internal/config"
)

func writeManaged(t *testing.T, path string, revision uint64, value config.Settings) {
	t.Helper()
	data, err := json.Marshal(map[string]any{"revision": revision, "settings": value})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestManagedWinTrayDoesNotWriteLegacySettingsOrHistory(t *testing.T) {
	directory := t.TempDir()
	store := config.NewStore(filepath.Join(directory, "settings.json"))
	initial := config.DefaultSettings()
	initial.RunAtLogon = false
	if err := store.Save(initial); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	projection := filepath.Join(directory, "managed-settings.json")
	writeManaged(t, projection, 4, initial)
	manager, err := newManagedManager(store, projection, &effects{})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := manager.Snapshot()
	if !snapshot.ConfigurationManaged || snapshot.Revision != "4" {
		t.Fatal(snapshot)
	}
	if _, err = manager.SaveImported(snapshot.Revision, initial, json.RawMessage(`{"invalid":true}`)); err == nil {
		t.Fatal("worker accepted direct save")
	}
	next := initial
	next.StartupIntervalSeconds = 12
	writeManaged(t, projection, 5, next)
	if _, err = manager.ReloadManaged(false); err != nil {
		t.Fatal(err)
	}
	if manager.Snapshot().Revision != "5" || manager.Snapshot().Settings.StartupIntervalSeconds != 12 {
		t.Fatal(manager.Snapshot())
	}
	after, _ := os.ReadFile(store.Path())
	if string(after) != string(original) {
		t.Fatal("legacy preferences overwritten")
	}
	if _, err = os.Stat(filepath.Join(directory, "startup-history.json")); !os.IsNotExist(err) {
		t.Fatal("history changed")
	}
}

func TestManagedWinTrayCompensatesFailedNativeApply(t *testing.T) {
	directory := t.TempDir()
	projection := filepath.Join(directory, "managed-settings.json")
	initial := config.DefaultSettings()
	initial.RunAtLogon = false
	writeManaged(t, projection, 3, initial)
	effect := &effects{fail: true}
	manager, err := newManagedManager(config.NewStore(filepath.Join(directory, "settings.json")), projection, effect)
	if err != nil {
		t.Fatal(err)
	}
	next := initial
	next.ManagedApps = []config.ManagedAppEntry{{ID: "editor", Name: "Editor", ExePath: `C:\editor.exe`}}
	writeManaged(t, projection, 4, next)
	if _, err = manager.ReloadManaged(false); err == nil {
		t.Fatal("native failure accepted")
	}
	if len(effect.applied.ManagedApps) != 0 || manager.Snapshot().Revision != "3" {
		t.Fatal("compensation failed", manager.Snapshot())
	}
	effect.fail = false
	if _, err = manager.ReloadManaged(false); err != nil {
		t.Fatal(err)
	}
	if manager.Snapshot().Revision != "4" {
		t.Fatal(manager.Snapshot())
	}
}

func TestManagedWinTrayRefusesUnresolvedLegacyJournal(t *testing.T) {
	directory := t.TempDir()
	store := config.NewStore(filepath.Join(directory, "settings.json"))
	if err := os.WriteFile(store.Path()+".transaction.json", []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	projection := filepath.Join(directory, "managed-settings.json")
	value := config.DefaultSettings()
	value.RunAtLogon = false
	writeManaged(t, projection, 1, value)
	if _, err := newManagedManager(store, projection, &effects{}); err == nil {
		t.Fatal("legacy recovery bypassed")
	}
}

func TestManagedProjectionRequiresRevision(t *testing.T) {
	path := filepath.Join(t.TempDir(), "managed-settings.json")
	data, err := json.Marshal(map[string]any{"settings": config.DefaultSettings()})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = readManagedProjection(path); err == nil {
		t.Fatal("missing configuration revision accepted")
	}
}
