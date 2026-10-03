package configuration

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"winone/internal/settings"
)

func TestStartupReadUsesOnlyCommittedPreferenceWithoutWriting(t *testing.T) {
	root := t.TempDir()
	committed, _ := json.Marshal(settings.Settings{SchemaVersion: 1, Theme: "system", Startup: &settings.StartupPreferences{PreferAdministrator: false}})
	pending, _ := json.Marshal(settings.Settings{SchemaVersion: 1, Theme: "system", Startup: &settings.StartupPreferences{PreferAdministrator: true}})
	state := diskState{Committed: Document{SchemaVersion: 1, Revision: 4, Values: map[string]json.RawMessage{"host": committed}}, Pending: &Document{SchemaVersion: 1, Revision: 5, Values: map[string]json.RawMessage{"host": pending}}}
	raw, _ := json.Marshal(state)
	path := filepath.Join(root, "configuration.json")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	value, err := ReadCommittedSettings(root)
	if err != nil || value.Revision != 4 || value.Startup == nil || value.Startup.PreferAdministrator {
		t.Fatal(value, err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(raw, after) {
		t.Fatal("bootstrap changed configuration", err)
	}
}
func TestStartupReadDoesNotCreateMissingConfiguration(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing")
	value, err := ReadCommittedSettings(root)
	if err != nil || value.Startup != nil {
		t.Fatal(value, err)
	}
	if _, err = os.Stat(root); !os.IsNotExist(err) {
		t.Fatal("bootstrap created a profile", err)
	}
}
