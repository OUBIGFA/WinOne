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

type recordingProductStartupTask struct {
	synced  string
	removed bool
}

func (s *recordingProductStartupTask) Sync(executable, args string) error {
	s.synced = executable + " " + args
	return nil
}
func (s *recordingProductStartupTask) Remove() error { s.removed = true; return nil }

func TestTemporaryCopyWithDefaultProfileCannotReplaceOrDeleteInstalledStartup(t *testing.T) {
	local := t.TempDir()
	t.Setenv("LOCALAPPDATA", local)
	profile := filepath.Join(local, "WinOne")
	copyExe := filepath.Join(t.TempDir(), "WinOne.Host.exe")
	conflict := errors.New("registration belongs to another installation")
	for _, enabled := range []bool{true, false} {
		t.Run(map[bool]string{true: "enable", false: "disable"}[enabled], func(t *testing.T) {
			opened, checked := false, false
			task := &recordingProductStartupTask{}
			err := syncProductStartupWith(profile, copyExe, settings.Settings{Startup: &settings.StartupPreferences{RunAtLogon: enabled}}, func(executable string) error {
				checked = true
				if executable != copyExe {
					t.Fatal("wrong installation checked", executable)
				}
				return conflict
			}, func() (productStartupTask, error) { opened = true; return task, nil })
			if !errors.Is(err, conflict) || !checked || opened || task.synced != "" || task.removed {
				t.Fatalf("temporary copy reached login task: err=%v checked=%v opened=%v synced=%q removed=%v", err, checked, opened, task.synced, task.removed)
			}
		})
	}
}

func TestOwnedInstallationKeepsExplicitStartupEnableAndDisable(t *testing.T) {
	local := t.TempDir()
	t.Setenv("LOCALAPPDATA", local)
	profile := filepath.Join(local, "WinOne")
	executable := filepath.Join(t.TempDir(), "WinOne.Host.exe")
	for _, enabled := range []bool{true, false} {
		task := &recordingProductStartupTask{}
		checked := false
		err := syncProductStartupWith(profile, executable, settings.Settings{Startup: &settings.StartupPreferences{RunAtLogon: enabled}}, func(path string) error {
			checked = true
			if path != executable {
				t.Fatal(path)
			}
			return nil
		}, func() (productStartupTask, error) {
			if !checked {
				t.Fatal("scheduler opened before installation verification")
			}
			return task, nil
		})
		if err != nil || !checked || task.removed == enabled {
			t.Fatal(enabled, checked, task, err)
		}
		if enabled && task.synced != executable+" --autorun" {
			t.Fatal("login launch changed", task.synced)
		}
	}
}

func TestIsolatedStartupNeverReadsInstallationOrOpensScheduler(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		err := syncProductStartupWith(t.TempDir(), "test.exe", settings.Settings{Startup: &settings.StartupPreferences{RunAtLogon: enabled}}, func(string) error {
			t.Fatal("isolated profile accessed installer registration")
			return nil
		}, func() (productStartupTask, error) { t.Fatal("isolated profile accessed scheduler"); return nil, nil })
		if (err != nil) != enabled {
			t.Fatal(enabled, err)
		}
	}
}

// Optional read-only integration: uses the real installer ownership check but a
// recording task backend. Neither the registered task nor user settings change.
func TestInstalledStartupOwnerRejectsTemporaryCopyReadOnly(t *testing.T) {
	if os.Getenv("WINONE_TEST_INSTALLED_STARTUP_OWNER") != "1" {
		t.Skip("requires an existing WinOne installer registration; read-only")
	}
	profile := filepath.Join(os.Getenv("LOCALAPPDATA"), "WinOne")
	executable := filepath.Join(t.TempDir(), "WinOne.Host.exe")
	for _, enabled := range []bool{true, false} {
		err := syncProductStartupWith(profile, executable, settings.Settings{Startup: &settings.StartupPreferences{RunAtLogon: enabled}}, productStartupOwner, func() (productStartupTask, error) {
			t.Fatal("temporary copy crossed real installed ownership boundary")
			return nil, nil
		})
		if err == nil {
			t.Fatal("expected installed ownership conflict; ensure WinOne is registered")
		}
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
