//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"winone/internal/configuration"
	"winone/internal/modules"
	"winone/internal/settings"
	"winone/modules/wintray/preferences"
)

func TestStartupMigratesAndRemainsGlobalWithoutWinTrayWorker(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "modules", "wintray", "settings.json")
	old := preferences.Defaults()
	old.RunAtLogon = true
	old.StartMinimizedToTray = true
	data, _ := json.Marshal(old)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	store, err := configuration.OpenHost(directory, wintrayConfiguration(directory, func() *modules.Lazy { return nil }))
	if err != nil {
		t.Fatal(err)
	}
	current := store.Snapshot()
	if current.Startup == nil || !current.Startup.RunAtLogon || !current.Startup.StartMinimized {
		t.Fatal(current)
	}
	current.Startup = &settings.StartupPreferences{RunAtLogon: false, StartMinimized: false}
	if _, err = store.Save(current); err != nil {
		t.Fatal(err)
	}
	raw, _, _ := store.Module("wintray")
	projected, err := preferences.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	if projected.RunAtLogon || projected.StartMinimizedToTray {
		t.Fatal("module retained stale global startup settings")
	}
	reopened, err := configuration.OpenHost(directory, wintrayConfiguration(directory, func() *modules.Lazy { return nil }))
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Snapshot().Startup.RunAtLogon || reopened.Snapshot().Startup.StartMinimized {
		t.Fatal("restart changed preferences")
	}
}
func TestStartupFailureRollsBackGlobalPreferences(t *testing.T) {
	store, err := configuration.OpenHost(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store.SetApply(func(value settings.Settings) error {
		if value.Startup.RunAtLogon {
			return errors.New("registration failed")
		}
		return nil
	})
	value := store.Snapshot()
	value.Startup.RunAtLogon = true
	if _, err = store.Save(value); err == nil {
		t.Fatal("registration failure hidden")
	}
	if store.Snapshot().Startup.RunAtLogon {
		t.Fatal("failed registration was published")
	}
}
func TestStartupTestProfilesCannotModifyLoginTask(t *testing.T) {
	directory := t.TempDir()
	value := settings.Settings{Startup: &settings.StartupPreferences{RunAtLogon: true}}
	if err := syncProductStartup(directory, "test.exe", value); err == nil {
		t.Fatal("test profile allowed system mutation")
	}
	value.Startup.RunAtLogon = false
	if err := syncProductStartup(directory, "test.exe", value); err != nil {
		t.Fatal(err)
	}
}
