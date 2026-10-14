package configuration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestQuickAssistantUpgradePreservesSelectionBackupAndIntent(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "configuration.json")
	doc := Document{SchemaVersion: 1, BuiltinVersion: 2, Revision: 8, Values: map[string]json.RawMessage{"host": json.RawMessage(`{"schemaVersion":1,"theme":"dark","captureShortcut":"","pinShortcut":"","translateShortcut":"Ctrl+Shift+Q"}`)}, ModuleEnabled: map[string]bool{"capture": false, "always-on-top": true, "wintray": false, "selection": false}}
	adapters := []ModuleAdapter{}
	for _, id := range []string{"capture", "always-on-top", "wintray", "selection", "quick-assistant"} {
		if id != "quick-assistant" {
			doc.Values[id] = json.RawMessage(`{"kept":true}`)
		}
		adapters = append(adapters, ModuleAdapter{ID: id, Load: func() (json.RawMessage, error) { return json.RawMessage(`{"kept":true}`), nil }, Validate: func(json.RawMessage) error { return nil }, Apply: func(json.RawMessage, uint64) error { return nil }})
	}
	before, _ := json.Marshal(diskState{Committed: doc})
	if err := os.WriteFile(path, before, 0600); err != nil {
		t.Fatal(err)
	}
	oldBackup := []byte("previous selection migration")
	os.WriteFile(path+".before-selection.bak", oldBackup, 0600)
	lifecycle := ModuleLifecycle{Initial: map[string]bool{"capture": false, "always-on-top": true, "wintray": false, "selection": false, "quick-assistant": true}, Before: func(map[string]bool) error { return nil }, After: func(map[string]bool) error { return nil }}
	store, err := OpenHostWithLifecycle(directory, lifecycle, adapters...)
	if err != nil {
		t.Fatal(err)
	}
	got := store.CommittedDocument()
	if got.BuiltinVersion != 3 || !got.ModuleEnabled["quick-assistant"] || got.ModuleEnabled["selection"] || got.ModuleEnabled["capture"] {
		t.Fatal(got.ModuleEnabled)
	}
	if store.Snapshot().TranslateBinding() != "Ctrl+Shift+Q" || store.Snapshot().QuickAssistantBinding() != "" {
		t.Fatal("existing shortcut must win migration")
	}
	backup, err := os.ReadFile(path + ".before-quick-assistant.bak")
	if err != nil || string(backup) != string(before) {
		t.Fatal("new backup missing", err)
	}
	original, _ := os.ReadFile(path + ".before-selection.bak")
	if string(original) != string(oldBackup) {
		t.Fatal("old backup overwritten")
	}
	reopened, err := OpenHostWithLifecycle(directory, lifecycle, adapters...)
	if err != nil || reopened.CommittedDocument().Revision != got.Revision {
		t.Fatal("upgrade repeated", err)
	}
	delete(got.ModuleEnabled, "quick-assistant")
	invalid, _ := json.Marshal(diskState{Committed: got})
	os.WriteFile(path, invalid, 0600)
	if _, err := OpenHostWithLifecycle(directory, lifecycle, adapters...); err == nil {
		t.Fatal("missing module in upgraded state accepted")
	}
}
