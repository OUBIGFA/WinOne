//go:build windows

package main

import (
	"bytes"
	"encoding/json"
	"testing"
	"winone/internal/configuration"
)

// Uninstall retention is a local preference backup, not a portable payload.
// Without a module's export/mapping adapter it must never enter .winone files.
func TestRemovedPackageLocalSettingsDoNotEnterProfileExport(t *testing.T) {
	_, store, states, request := importFixture(t)
	adapter := configuration.ModuleAdapter{ID: "example.local", PackageSchemaVersion: 1, Load: func() (json.RawMessage, error) {
		return json.RawMessage(`{"localPath":"C:\\Private-Fixture","token":"unsent-test-marker"}`), nil
	}, Validate: func(json.RawMessage) error { return nil }, Apply: func(json.RawMessage, uint64) error { return nil }}
	if err := store.RegisterPackageModule(adapter, store.CommittedDocument().Revision); err != nil {
		t.Fatal(err)
	}
	if err := store.UnregisterPackageModule(adapter.ID, store.CommittedDocument().Revision); err != nil {
		t.Fatal(err)
	}
	archive, err := collectProfile(store, states, "portable")
	if err != nil {
		t.Fatal("local retention broke profile export", err)
	}
	raw, err := json.Marshal(archive)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("Private-Fixture")) || bytes.Contains(raw, []byte("unsent-test-marker")) {
		t.Fatal("local package data was exported without a migration adapter")
	}
	if _, exists := archive.Modules[adapter.ID]; exists {
		t.Fatal("retained local settings became portable module settings")
	}
	retained := append([]byte(nil), store.CommittedDocument().RetainedModules[adapter.ID]...)
	preview, err := previewProfileFile(request.Path, request.Bindings, store, states)
	if err != nil {
		t.Fatal(err)
	}
	request.ConfigurationRevision = &preview.ConfigurationRevision
	request.ModuleRevision = &preview.ModuleRevision
	if _, err = applyProfile(request, store, states); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(retained, store.CommittedDocument().RetainedModules[adapter.ID]) {
		t.Fatal("profile import replaced local retained settings")
	}
}
