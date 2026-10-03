package worker

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
	"winone/modules/wintray/internal/config"
)

func TestImportPreservesAllCurrentSettingsWithoutWritingSource(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	original := config.DefaultSettings()
	original.ManagedApps = []config.ManagedAppEntry{{ID: "a", Name: "Example", ExePath: filepath.Join(dir, "example.exe"), Args: "--name \"中文 参数\"", RunOnStartup: true, CollectTrayIcon: true, LaunchViaLogonTask: true, Schedule: config.Schedule{Enabled: true, StartDelayMinutes: 17, AutoExitMinutes: 33, FrequencyEnabled: true, FrequencyDays: 7, FrequencyRuns: 2}}}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	preview, err := PreviewImport(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(original, preview.Settings) {
		t.Fatalf("settings changed: %+v", preview.Settings)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(data, after) {
		t.Fatal("source changed")
	}
	files, _ := os.ReadDir(dir)
	if len(files) != 1 {
		t.Fatal("preview wrote extra files")
	}
}

func TestImportedHistorySurvivesFailedSettingsAndRetry(t *testing.T) {
	dir := t.TempDir()
	store := config.NewStore(filepath.Join(dir, "settings.json"))
	initial := config.DefaultSettings()
	initial.RunAtLogon = false
	effect := &effects{fail: true}
	manager := NewManager(store, initial, effect)
	now := time.Now().UTC()
	path := filepath.Join(dir, "example.exe")
	history, _ := json.Marshal(map[string]any{"version": 1, "apps": map[string][]time.Time{path: {now}}})
	next := initial
	next.ManagedApps = []config.ManagedAppEntry{{ID: "a", Name: "Example", ExePath: path, RunOnStartup: true, Schedule: config.Schedule{Enabled: true, FrequencyEnabled: true, FrequencyDays: 1, FrequencyRuns: 1}}}
	before := manager.Snapshot()
	if _, err := manager.SaveImported(before.Revision, next, history); err == nil {
		t.Fatal("expected system failure")
	}
	if manager.Snapshot().Revision != before.Revision {
		t.Fatal("failed settings not restored")
	}
	effect.fail = false
	if _, err := manager.SaveImported(before.Revision, next, history); err != nil {
		t.Fatal(err)
	}
	allowed, err := config.NewStartupHistory(filepath.Join(dir, "startup-history.json")).Launch(next.ManagedApps[0], now, func() error { t.Fatal("import refunded previous launch"); return nil })
	if err != nil || allowed {
		t.Fatalf("frequency limit lost: %v %t", err, allowed)
	}
}
func TestImportLegacyDefaultsAndInvalidFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	legacy := map[string]any{"schemaVersion": 1, "managedApps": []map[string]any{{"id": "legacy", "name": "Legacy", "exePath": filepath.Join(dir, "legacy.exe")}}}
	data, _ := json.Marshal(legacy)
	os.WriteFile(path, data, 0600)
	preview, err := PreviewImport(path)
	if err != nil {
		t.Fatal(err)
	}
	app := preview.Settings.ManagedApps[0]
	if preview.Settings.SchemaVersion != 3 || !app.RunOnStartup || !app.TrayBehavior.AutoMinimizeAndHideOnLaunch || app.Schedule.AutoExitMinutes != 30 {
		t.Fatalf("legacy conversion: %+v", preview)
	}
	for _, invalid := range []string{`{}`, `null`, `{"schemaVersion":4,"managedApps":[]}`, `{"schemaVersion":3,"managedApps":null}`, `broken`} {
		os.WriteFile(path, []byte(invalid), 0600)
		if _, err := PreviewImport(path); err == nil {
			t.Fatalf("accepted %s", invalid)
		}
	}
}
