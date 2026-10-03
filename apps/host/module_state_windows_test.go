//go:build windows

package main

import (
	"os"
	"path/filepath"
	"testing"
	"winone/internal/modulestate"
)

func writeLegacySwitch(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "modules", "always-on-top", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
func TestLegacyDisabledSwitchCreatesPendingRecovery(t *testing.T) {
	dir := t.TempDir()
	content := `{"schemaVersion":1,"enabled":false,"frameThickness":7}`
	path := writeLegacySwitch(t, dir, content)
	store, recover, err := openModuleState(dir)
	if err != nil {
		t.Fatal(err)
	}
	record := store.Snapshot().Modules["always-on-top"]
	if record.Enabled || !record.PendingStop || len(recover) != 1 {
		t.Fatalf("legacy disable not migrated: %+v", record)
	}
	data, _ := os.ReadFile(path)
	if string(data) != content {
		t.Fatal("engine preferences were changed during migration")
	}
}
func TestExistingModuleIntentWinsOverLegacyEngineSwitch(t *testing.T) {
	dir := t.TempDir()
	writeLegacySwitch(t, dir, `{"schemaVersion":1,"enabled":false}`)
	store, _ := modulestate.Open(filepath.Join(dir, "module-state.json"))
	if _, err := store.SetIntent(0, "always-on-top", true); err != nil {
		t.Fatal(err)
	}
	loaded, recover, err := openModuleState(dir)
	if err != nil || len(recover) != 0 || !loaded.Snapshot().Modules["always-on-top"].Enabled {
		t.Fatal("legacy flag overrode product intent")
	}
}
func TestBrokenLegacySettingsDoNotDisableOtherModules(t *testing.T) {
	dir := t.TempDir()
	writeLegacySwitch(t, dir, "broken")
	store, _, err := openModuleState(dir)
	if err != nil {
		t.Fatal(err)
	}
	state := store.Snapshot()
	if state.Modules["always-on-top"].Enabled || !state.Modules["capture"].Enabled || !state.Modules["wintray"].Enabled {
		t.Fatal("legacy corruption was not isolated")
	}
}
