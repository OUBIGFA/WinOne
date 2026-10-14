package configuration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSelectionAdditionMigratesOldPreferencesAndPendingRecovery(t *testing.T) {
	for _, pending := range []bool{false, true} {
		root := t.TempDir()
		doc := Document{SchemaVersion: 1, Revision: 9, ModuleEnabled: map[string]bool{"capture": false, "always-on-top": true, "wintray": false}, Values: map[string]json.RawMessage{"host": json.RawMessage(`{"schemaVersion":1,"theme":"dark","captureShortcut":"","pinShortcut":""}`)}}
		adapters := []ModuleAdapter{}
		for _, id := range []string{"capture", "always-on-top", "wintray", "selection"} {
			if id != "selection" {
				doc.Values[id] = json.RawMessage(`{"kept":true}`)
			}
			adapters = append(adapters, ModuleAdapter{ID: id, Load: func() (json.RawMessage, error) { return json.RawMessage(`{"kept":true}`), nil }, Validate: func(json.RawMessage) error { return nil }, Apply: func(json.RawMessage, uint64) error { return nil }})
		}
		state := diskState{Committed: doc}
		if pending {
			next := clone(doc)
			next.Revision++
			next.ModuleEnabled["capture"] = true
			state.Pending = &next
		}
		before, _ := json.Marshal(state)
		path := filepath.Join(root, "configuration.json")
		if err := os.WriteFile(path, before, 0600); err != nil {
			t.Fatal(err)
		}
		applied := false
		lifecycle := ModuleLifecycle{Initial: map[string]bool{"capture": false, "always-on-top": true, "wintray": false, "selection": true}, Before: func(target map[string]bool) error {
			if len(target) != 4 {
				t.Fatal("partial lifecycle map")
			}
			return nil
		}, After: func(target map[string]bool) error { applied = target["selection"]; return nil }}
		store, err := OpenHostWithLifecycle(root, lifecycle, adapters...)
		if err != nil {
			t.Fatal(err)
		}
		got := store.CommittedDocument()
		if !applied || !got.ModuleEnabled["selection"] || got.ModuleEnabled["capture"] || got.ModuleEnabled["wintray"] || !got.ModuleEnabled["always-on-top"] {
			t.Fatalf("migration or rollback changed old preferences: %+v", got.ModuleEnabled)
		}
		if got.BuiltinVersion != 2 {
			t.Fatal("migration version was not recorded")
		}
		backup, err := os.ReadFile(path + ".before-selection.bak")
		if err != nil || string(backup) != string(before) {
			t.Fatal("migration did not preserve original journal", err)
		}
		reopened, err := OpenHostWithLifecycle(root, lifecycle, adapters...)
		if err != nil || reopened.CommittedDocument().Revision != got.Revision {
			t.Fatal("migration repeated", err)
		}
		// Once the new catalog version has been written, a missing selection
		// entry is corruption, not another reason to enable a module silently.
		delete(got.ModuleEnabled, "selection")
		data, _ := json.Marshal(diskState{Committed: got})
		os.WriteFile(path, data, 0600)
		if _, err = OpenHostWithLifecycle(root, lifecycle, adapters...); err == nil {
			t.Fatal("new state with missing module was accepted")
		}
	}
}
