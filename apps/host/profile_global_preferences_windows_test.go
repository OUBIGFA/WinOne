//go:build windows

package main

import (
	"encoding/json"
	"os"
	"testing"
	"winone/internal/profiles"
)

func TestProfileGlobalPreferencesExportAndImport(t *testing.T) {
	_, store, states, request := importFixture(t)
	local := store.Snapshot()
	keep, open := false, "Ctrl+Alt+F9"
	local.DefaultPage, local.OpenShortcut = "chat", &open
	local.Startup.KeepUIRunning, local.Startup.PreferAdministrator = &keep, true
	if _, err := store.Save(local); err != nil {
		t.Fatal(err)
	}
	archive, err := collectProfile(store, states, "global")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(archive.Preferences)
	var exported map[string]any
	_ = json.Unmarshal(raw, &exported)
	if exported["defaultPage"] != "chat" || exported["keepUIRunning"] != false || exported["openShortcut"] != open {
		t.Fatalf("portable preferences omitted: %s", raw)
	}
	// Clear/false are values, not missing-field markers.
	if err := json.Unmarshal([]byte(`{"defaultPage":"capture","keepUIRunning":false,"openShortcut":""}`), &archive.Preferences); err != nil {
		t.Fatal(err)
	}
	data, err := profiles.Encode(archive)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(request.Path, data, 0600); err != nil {
		t.Fatal(err)
	}
	local = store.Snapshot()
	enabled, original := true, "Alt+A"
	local.DefaultPage, local.OpenShortcut, local.Startup.KeepUIRunning = "overview", &original, &enabled
	if _, err := store.Save(local); err != nil {
		t.Fatal(err)
	}
	preview, err := previewProfileFile(request.Path, request.Bindings, store, states)
	if err != nil {
		t.Fatal(err)
	}
	request.SourceHash, request.ConfigurationRevision, request.ModuleRevision = preview.SourceHash, &preview.ConfigurationRevision, &preview.ModuleRevision
	request.Modules = nil
	if _, err := applyProfile(request, store, states); err != nil {
		t.Fatal(err)
	}
	got := store.Snapshot()
	if got.DefaultPage != "capture" || got.OpenBinding() != "" || got.Startup.KeepUIRunning == nil || *got.Startup.KeepUIRunning || !got.Startup.PreferAdministrator {
		t.Fatalf("global import lost preferences: %+v / %+v", got, got.Startup)
	}
}

func TestOldProfilePreservesNewLocalGlobalPreferences(t *testing.T) {
	_, store, states, request := importFixture(t)
	// importFixture's source is rewritten without the newly optional fields.
	data, _ := os.ReadFile(request.Path)
	archive, err := profiles.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(archive.Preferences)
	var values map[string]json.RawMessage
	_ = json.Unmarshal(raw, &values)
	for _, key := range []string{"defaultPage", "keepUIRunning", "openShortcut"} {
		delete(values, key)
	}
	raw, _ = json.Marshal(values)
	archive.Preferences = profiles.Preferences{}
	_ = json.Unmarshal(raw, &archive.Preferences)
	data, err = profiles.Encode(archive)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(request.Path, data, 0600); err != nil {
		t.Fatal(err)
	}
	local := store.Snapshot()
	keep, open := false, "Ctrl+Alt+F9"
	local.DefaultPage, local.OpenShortcut, local.Startup.KeepUIRunning, local.Startup.PreferAdministrator = "chat", &open, &keep, true
	if _, err := store.Save(local); err != nil {
		t.Fatal(err)
	}
	preview, err := previewProfileFile(request.Path, request.Bindings, store, states)
	if err != nil {
		t.Fatal(err)
	}
	request.SourceHash, request.ConfigurationRevision, request.ModuleRevision = preview.SourceHash, &preview.ConfigurationRevision, &preview.ModuleRevision
	if _, err := applyProfile(request, store, states); err != nil {
		t.Fatal(err)
	}
	got := store.Snapshot()
	if got.DefaultPage != "chat" || got.OpenBinding() != open || got.Startup.KeepUIRunning == nil || *got.Startup.KeepUIRunning || !got.Startup.PreferAdministrator {
		t.Fatalf("old profile reset local preferences: %+v / %+v", got, got.Startup)
	}
}

func TestGlobalProfileConflictIsAtomicAndUnselectedGlobalValuesStayLocal(t *testing.T) {
	_, store, states, request := importFixture(t)
	archive, err := collectProfile(store, states, "collision")
	if err != nil {
		t.Fatal(err)
	}
	key := "shift+a"
	archive.Preferences.OpenShortcut = &key
	data, err := profiles.Encode(archive)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(request.Path, data, 0600); err != nil {
		t.Fatal(err)
	}
	preview, err := previewProfileFile(request.Path, request.Bindings, store, states)
	if err != nil {
		t.Fatal(err)
	}
	if preview.CurrentShortcuts["application.open"] != "Alt+A" {
		t.Fatal("local global binding missing from preview")
	}
	request.SourceHash, request.ConfigurationRevision, request.ModuleRevision = preview.SourceHash, &preview.ConfigurationRevision, &preview.ModuleRevision
	request.Modules = []string{"capture"}
	before := store.Snapshot()
	if _, err = applyProfile(request, store, states); err == nil {
		t.Fatal("imported open shortcut collided with retained capture")
	}
	if got := store.Snapshot(); got.Revision != before.Revision || got.OpenBinding() != before.OpenBinding() {
		t.Fatal("rejected import changed global preferences")
	}
	request.Global = false
	if _, err = applyProfile(request, store, states); err != nil {
		t.Fatalf("unselected global value blocked valid module import: %v", err)
	}
	if store.Snapshot().OpenBinding() != before.OpenBinding() {
		t.Fatal("unselected global shortcut was imported")
	}
}
