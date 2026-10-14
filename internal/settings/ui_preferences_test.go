package settings

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

// These cases protect missing-field migration and an explicit disable across
// persistence. Treating false/empty as absent would silently re-enable both.
func TestUIBackgroundAndOpenShortcutRoundTrip(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	initial, _ := json.Marshal(store.Snapshot().ResolvedShortcuts())
	var defaults map[string]any
	_ = json.Unmarshal(initial, &defaults)
	if defaults["openShortcut"] != "Alt+A" {
		t.Fatalf("missing open default: %s", initial)
	}
	var next Settings
	if err := json.Unmarshal([]byte(`{"schemaVersion":1,"theme":"system","captureShortcut":"Shift+A","pinShortcut":"Alt+Q","openShortcut":"","startup":{"runAtLogon":false,"startMinimized":true,"keepUIRunning":false}}`), &next); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Save(next); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(store.path)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(reopened.Snapshot().ResolvedShortcuts())
	var result map[string]any
	_ = json.Unmarshal(data, &result)
	if result["openShortcut"] != "" || result["startup"].(map[string]any)["keepUIRunning"] != false {
		t.Fatalf("explicit disable lost: %s", data)
	}
}

func TestOpenShortcutRejectsExplicitCollisionWithoutBreakingLegacySettings(t *testing.T) {
	var legacy Settings
	_ = json.Unmarshal([]byte(`{"schemaVersion":1,"theme":"system","captureShortcut":"Alt+A","pinShortcut":"Alt+Q"}`), &legacy)
	if err := Validate(legacy); err != nil {
		t.Fatalf("legacy binding must survive: %v", err)
	}
	_ = json.Unmarshal([]byte(`{"openShortcut":"Alt+A"}`), &legacy)
	if err := Validate(legacy); err == nil {
		t.Fatal("explicit collision was accepted")
	}
}

func TestUIBackgroundSnapshotsAreIndependent(t *testing.T) {
	keep := false
	value := Settings{Startup: &StartupPreferences{KeepUIRunning: &keep}}
	copy := copySettings(value)
	*copy.Startup.KeepUIRunning = true
	if *value.Startup.KeepUIRunning {
		t.Fatal("editing a snapshot mutated the saved preference")
	}
}
