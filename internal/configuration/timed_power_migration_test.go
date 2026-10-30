package configuration

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"winone/internal/timedpower"
)

func TestTimedPowerUpgradeKeepsDraftJournalAndExistingPreferences(t *testing.T) {
	for _, pending := range []bool{false, true} {
		root := t.TempDir()
		path := filepath.Join(root, "configuration.json")
		doc := Document{SchemaVersion: 1, BuiltinVersion: 4, Revision: 18, ModuleEnabled: map[string]bool{}, Values: map[string]json.RawMessage{"host": json.RawMessage(`{"schemaVersion":1,"theme":"dark","captureShortcut":"","pinShortcut":""}`)}}
		adapters := []ModuleAdapter{}
		for _, id := range []string{"capture", "always-on-top", "wintray", "selection", "quick-assistant", "stroke"} {
			doc.ModuleEnabled[id] = false
			doc.Values[id] = json.RawMessage(`{"kept":true}`)
			adapters = append(adapters, ModuleAdapter{ID: id, Load: func() (json.RawMessage, error) { t.Fatal("existing preferences reloaded"); return nil, nil }, Validate: func(json.RawMessage) error { return nil }, Apply: func(json.RawMessage, uint64) error { return nil }})
		}
		adapters = append(adapters, ModuleAdapter{ID: "timed-power", Load: func() (json.RawMessage, error) { return json.Marshal(timedpower.Defaults()) }, Validate: func(raw json.RawMessage) error {
			var p timedpower.Preferences
			if err := json.Unmarshal(raw, &p); err != nil {
				return err
			}
			return p.Validate()
		}, Apply: func(json.RawMessage, uint64) error { return nil }})
		state := diskState{Committed: doc}
		if pending {
			next := clone(doc)
			next.Revision++
			next.ModuleEnabled["capture"] = true
			state.Pending = &next
		}
		before, _ := json.Marshal(state)
		if !pending {
			if err := CheckUpgrade(before, adapters...); err != nil {
				t.Fatal("legacy upgrade probe", err)
			}
		}
		if err := os.WriteFile(path, before, 0600); err != nil {
			t.Fatal(err)
		}
		initial := copyEnabled(doc.ModuleEnabled)
		initial["timed-power"] = true
		lifecycle := ModuleLifecycle{Initial: initial, Before: func(map[string]bool) error { return nil }, After: func(map[string]bool) error { return nil }}
		store, err := OpenHostWithLifecycle(root, lifecycle, adapters...)
		if err != nil {
			t.Fatal(err)
		}
		got := store.CommittedDocument()
		if got.BuiltinVersion != 5 || !got.ModuleEnabled["timed-power"] {
			t.Fatal(got)
		}
		for id, want := range doc.ModuleEnabled {
			if got.ModuleEnabled[id] != want || !bytes.Equal(got.Values[id], doc.Values[id]) {
				t.Fatalf("old module changed: %s", id)
			}
		}
		var p timedpower.Preferences
		if err := json.Unmarshal(got.Values["timed-power"], &p); err != nil || !p.Force || p.DryRun {
			t.Fatal(p, err)
		}
		backup, err := os.ReadFile(path + ".before-timed-power.bak")
		if err != nil || !bytes.Equal(backup, before) {
			t.Fatal("journal backup", err)
		}
		reopened, err := OpenHostWithLifecycle(root, lifecycle, adapters...)
		if err != nil || reopened.CommittedDocument().Revision != got.Revision {
			t.Fatal("upgrade repeated", err)
		}
		current, _ := json.Marshal(diskState{Committed: got})
		if err := CheckUpgrade(current, adapters...); err != nil {
			t.Fatal("current probe", err)
		}
		delete(got.ModuleEnabled, "timed-power")
		invalid, _ := json.Marshal(diskState{Committed: got})
		if err := CheckUpgrade(invalid, adapters...); err == nil {
			t.Fatal("missing current switch accepted")
		}
		if err := os.WriteFile(path, invalid, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := OpenHostWithLifecycle(root, lifecycle, adapters...); err == nil {
			t.Fatal("current corruption silently repaired")
		}
	}
}
