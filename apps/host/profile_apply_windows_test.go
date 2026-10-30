//go:build windows

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"winone/internal/configuration"
	"winone/internal/modules"
	"winone/internal/modulestate"
	"winone/internal/profiles"
)

func TestProfileImportKeepsLocalAdministratorPreference(t *testing.T) {
	_, store, states, request := importFixture(t)
	current := store.Snapshot()
	current.Startup.PreferAdministrator = true
	if _, err := store.Save(current); err != nil {
		t.Fatal(err)
	}
	preview, err := previewProfileFile(request.Path, request.Bindings, store, states)
	if err != nil {
		t.Fatal(err)
	}
	request.ConfigurationRevision = &preview.ConfigurationRevision
	request.ModuleRevision = &preview.ModuleRevision
	if _, err = applyProfile(request, store, states); err != nil {
		t.Fatal(err)
	}
	if !store.Snapshot().Startup.PreferAdministrator {
		t.Fatal("import reset the local permission preference")
	}
	archive, err := collectProfile(store, states, "portable")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(archive)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("preferAdministrator")) {
		t.Fatal("local administrator preference was exported")
	}
}

func importFixture(t *testing.T, failCapture ...bool) (string, *configuration.HostStore, *modulestate.Store, profileApplyRequest) {
	t.Helper()
	directory := t.TempDir()
	states, err := modulestate.Open(filepath.Join(directory, "module-state.json"))
	if err != nil {
		t.Fatal(err)
	}
	get := func() *modules.Lazy { return nil }
	lifecycle := configuration.ModuleLifecycle{Initial: map[string]bool{"capture": true, "always-on-top": true, "wintray": true, "selection": true, "quick-assistant": true, "stroke": true, "timed-power": true}, Before: func(map[string]bool) error { return nil }, After: func(map[string]bool) error { return nil }}
	adapters := []configuration.ModuleAdapter{aotConfiguration(directory, get), captureConfiguration(directory, get), wintrayConfiguration(directory, get), selectionConfiguration(directory, nil), quickAssistantConfiguration(directory, nil), strokeConfiguration(directory, get), timedPowerConfiguration()}
	if len(failCapture) > 0 && failCapture[0] {
		apply := adapters[1].Apply
		adapters[1].Apply = func(raw json.RawMessage, revision uint64) error {
			var value capturePreferences
			if err := json.Unmarshal(raw, &value); err != nil {
				return err
			}
			if value.Theme == "dark" {
				return errors.New("capture apply failed")
			}
			return apply(raw, revision)
		}
	}
	store, err := configuration.OpenHostWithLifecycle(directory, lifecycle, adapters...)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := collectProfile(store, states, "Imported")
	if err != nil {
		t.Fatal(err)
	}
	archive.Preferences.Theme = "dark"
	archive.Preferences.Enabled["capture"] = false
	archive.Modules["example.optional"] = profiles.ModuleSettings{SchemaVersion: 4, Settings: json.RawMessage(`{"choice":true}`)}
	data, err := profiles.Encode(archive)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "source.winone")
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	bindings := map[string]string{"capture.output": directory}
	preview, err := previewProfileFile(path, bindings, store, states)
	if err != nil {
		t.Fatal(err)
	}
	configRevision, moduleRevision := preview.ConfigurationRevision, preview.ModuleRevision
	return directory, store, states, profileApplyRequest{Path: path, Bindings: bindings, SourceHash: preview.SourceHash, ConfigurationRevision: &configRevision, ModuleRevision: &moduleRevision, Modules: []string{"capture", "example.optional"}, Global: true}
}

func TestImportCommitsPreferencesSwitchesAndDeferredModulesTogether(t *testing.T) {
	_, store, states, request := importFixture(t)
	if _, err := applyProfile(request, store, states); err != nil {
		t.Fatal(err)
	}
	document := store.CommittedDocument()
	if document.Revision != 1 || document.ModuleEnabled["capture"] || store.Snapshot().Theme != "dark" || len(document.DeferredModules) != 1 {
		t.Fatal(document)
	}
	exported, err := collectProfile(store, states, "Again")
	if err != nil {
		t.Fatal(err)
	}
	if exported.Modules["example.optional"].SchemaVersion != 4 || string(exported.Modules["example.optional"].Settings) != `{"choice":true}` {
		t.Fatal("unknown settings lost")
	}
}

func TestImportRejectsChangedSourceAndStaleConfiguration(t *testing.T) {
	_, store, states, request := importFixture(t)
	data, err := os.ReadFile(request.Path)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := profiles.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	archive.Preferences.Name = "Changed"
	data, err = profiles.Encode(archive)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(request.Path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = applyProfile(request, store, states); err == nil {
		t.Fatal("changed source accepted")
	}
	if store.Snapshot().Revision != 0 {
		t.Fatal("rejected source changed preferences")
	}
	preview, err := previewProfileFile(request.Path, request.Bindings, store, states)
	if err != nil {
		t.Fatal(err)
	}
	request.SourceHash = preview.SourceHash
	next := store.Snapshot()
	next.Theme = "light"
	if _, err = store.Save(next); err != nil {
		t.Fatal(err)
	}
	if _, err = applyProfile(request, store, states); err == nil {
		t.Fatal("stale preview accepted")
	}
	if store.Snapshot().Revision != 1 || store.Snapshot().Theme != "light" {
		t.Fatal(store.Snapshot())
	}
}

func TestImportFailureDoesNotCommitSwitchesGlobalSettingsOrDeferredData(t *testing.T) {
	_, store, states, request := importFixture(t, true)
	if _, err := applyProfile(request, store, states); err == nil {
		t.Fatal("failed apply reported success")
	}
	document := store.CommittedDocument()
	if document.Revision != 0 || !document.ModuleEnabled["capture"] || store.Snapshot().Theme != "system" || len(document.DeferredModules) != 0 {
		t.Fatal("partial import committed", document)
	}
	if store.RecoveryError() != nil {
		t.Fatal("successful compensation remained blocked")
	}
}
