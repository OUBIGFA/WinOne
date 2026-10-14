package configuration

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
	"winone/internal/settings"
)

func packageAdapter() ModuleAdapter {
	return ModuleAdapter{ID: "example.tool", PackageSchemaVersion: 1, Load: func() (json.RawMessage, error) { return json.RawMessage(`{"size":3}`), nil }, Validate: func(raw json.RawMessage) error {
		var value struct {
			Size int `json:"size"`
		}
		if json.Unmarshal(raw, &value) != nil || value.Size < 1 {
			return errors.New("invalid size")
		}
		return nil
	}, Apply: func(json.RawMessage, uint64) error { return nil }}
}

func TestPackageConfigurationFailedRemovalRestoresIntentAndSettings(t *testing.T) {
	lifecycle := packageLifecycle()
	failStop := false
	lifecycle.Before = func(target map[string]bool) error {
		if failStop && !target["example.tool"] {
			return errors.New("resource restoration failed")
		}
		return nil
	}
	store, err := OpenHostWithLifecycle(t.TempDir(), lifecycle)
	if err != nil {
		t.Fatal(err)
	}
	adapter := packageAdapter()
	if err = store.RegisterPackageModule(adapter, store.CommittedDocument().Revision); err != nil {
		t.Fatal(err)
	}
	next := store.CommittedDocument()
	next.ModuleEnabled[adapter.ID] = true
	if _, err = store.CommitDocument(next); err != nil {
		t.Fatal(err)
	}
	failStop = true
	previous := store.CommittedDocument()
	if err = store.UnregisterPackageModule(adapter.ID, previous.Revision); err == nil {
		t.Fatal("failed stop accepted")
	}
	next = store.CommittedDocument()
	if next.Revision != previous.Revision || !next.ModuleEnabled[adapter.ID] || string(next.Values[adapter.ID]) != `{"size":3}` || len(next.RetainedModules[adapter.ID]) != 0 {
		t.Fatal("failed removal partly committed")
	}
	failStop = false
	if err = store.UnregisterPackageModule(adapter.ID, next.Revision); err != nil {
		t.Fatal("removal retry failed", err)
	}
}

func TestPackageConfigurationPendingRecoveryKeepsAdapterUntilRestored(t *testing.T) {
	store, err := OpenHostWithLifecycle(t.TempDir(), packageLifecycle())
	if err != nil {
		t.Fatal(err)
	}
	adapter := packageAdapter()
	store.SetApply(func(_ settings.Settings) error { return errors.New("global projection unavailable") })
	if err = store.RegisterPackageModule(adapter, store.CommittedDocument().Revision); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatal("failed rollback was hidden", err)
	}
	if err = store.RegisterPackageModule(adapter, store.CommittedDocument().Revision); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatal("registration bypassed recovery", err)
	}
	store.SetApply(nil)
	if err = store.Recover(); err != nil {
		t.Fatal(err)
	}
	if _, exists := store.CommittedDocument().Values[adapter.ID]; exists {
		t.Fatal("uncommitted module became active")
	}
	if err = store.RegisterPackageModule(adapter, store.CommittedDocument().Revision); err != nil {
		t.Fatal("recovered registration cannot retry", err)
	}
}

func TestPackageConfigurationSerializesCatalogChangeWithStaleSettingsSave(t *testing.T) {
	store, err := OpenHostWithLifecycle(t.TempDir(), packageLifecycle())
	if err != nil {
		t.Fatal(err)
	}
	adapter := packageAdapter()
	entered, release := make(chan struct{}), make(chan struct{})
	adapter.Apply = func(json.RawMessage, uint64) error { close(entered); <-release; return nil }
	finished := make(chan error, 1)
	before := store.CommittedDocument()
	go func() { finished <- store.RegisterPackageModule(adapter, before.Revision) }()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("registration never reached projection")
	}
	// Readers must still see the last committed catalog while projection runs.
	if _, exists := store.CommittedDocument().Values[adapter.ID]; exists {
		t.Fatal("pending module leaked into committed settings")
	}
	stale := store.Snapshot()
	stale.Theme = "dark"
	staleSave := make(chan error, 1)
	go func() { _, saveErr := store.Save(stale); staleSave <- saveErr }()
	close(release)
	if err = <-finished; err != nil {
		t.Fatal(err)
	}
	if err = <-staleSave; !errors.Is(err, settings.ErrConflict) && !errors.Is(err, ErrRecoveryRequired) {
		t.Fatal("stale settings crossed catalog transaction", err)
	}
	if _, err = store.Save(stale); !errors.Is(err, settings.ErrConflict) {
		t.Fatal("stale revision accepted after registration finished", err)
	}
	if store.Snapshot().Theme == "dark" {
		t.Fatal("stale settings were published")
	}
}
func packageLifecycle() ModuleLifecycle {
	return ModuleLifecycle{Initial: map[string]bool{}, Before: func(map[string]bool) error { return nil }, After: func(map[string]bool) error { return nil }}
}

// A package installed after Host startup must participate in the same durable
// revision, enable intent and setting projection as the existing modules.
func TestPackageConfigurationRegisterSaveRemoveAndReinstall(t *testing.T) {
	root := t.TempDir()
	store, err := OpenHostWithLifecycle(root, packageLifecycle())
	if err != nil {
		t.Fatal(err)
	}
	adapter := packageAdapter()
	if err = store.RegisterPackageModule(adapter, store.CommittedDocument().Revision); err != nil {
		t.Fatal(err)
	}
	raw, rev, err := store.Module(adapter.ID)
	if err != nil || string(raw) != `{"size":3}` || store.CommittedDocument().ModuleEnabled[adapter.ID] {
		t.Fatal("new module not registered disabled", string(raw), err)
	}
	if _, _, err = store.SaveModule(adapter.ID, rev, json.RawMessage(`{"size":7}`)); err != nil {
		t.Fatal(err)
	}
	next := store.CommittedDocument()
	next.ModuleEnabled[adapter.ID] = true
	if _, err = store.CommitDocument(next); err != nil {
		t.Fatal(err)
	}
	lifecycle := packageLifecycle()
	lifecycle.Initial[adapter.ID] = true
	store, err = OpenHostWithLifecycle(root, lifecycle, adapter)
	if err != nil || !store.CommittedDocument().ModuleEnabled[adapter.ID] {
		t.Fatal("restart lost enable intent", err)
	}
	if err = store.UnregisterPackageModule(adapter.ID, store.CommittedDocument().Revision); err != nil {
		t.Fatal(err)
	}
	removed := store.CommittedDocument()
	if _, exists := removed.Values[adapter.ID]; exists {
		t.Fatal("removed module still has active settings")
	}
	if _, exists := removed.ModuleEnabled[adapter.ID]; exists {
		t.Fatal("removed module still has enable intent")
	}
	var retained struct {
		SchemaVersion int             `json:"schemaVersion"`
		Settings      json.RawMessage `json:"settings"`
	}
	if json.Unmarshal(removed.RetainedModules[adapter.ID], &retained) != nil || retained.SchemaVersion != 1 || string(retained.Settings) != `{"size":7}` {
		t.Fatal("uninstall discarded settings")
	}
	store, err = OpenHostWithLifecycle(root, packageLifecycle())
	if err != nil {
		t.Fatal("removed package blocks Host startup", err)
	}
	adapter.Load = func() (json.RawMessage, error) { return nil, errors.New("must restore retained settings") }
	if err = store.RegisterPackageModule(adapter, store.CommittedDocument().Revision); err != nil {
		t.Fatal(err)
	}
	raw, _, err = store.Module(adapter.ID)
	if err != nil || string(raw) != `{"size":7}` || store.CommittedDocument().ModuleEnabled[adapter.ID] {
		t.Fatal("reinstall did not restore disabled preferences", err)
	}
}

func TestPackageConfigurationRejectsStaleRegistrationAndSchemaMismatch(t *testing.T) {
	store, err := OpenHostWithLifecycle(t.TempDir(), packageLifecycle())
	if err != nil {
		t.Fatal(err)
	}
	adapter := packageAdapter()
	revision := store.CommittedDocument().Revision
	next := store.Snapshot()
	next.Theme = "dark"
	if _, err = store.Save(next); err != nil {
		t.Fatal(err)
	}
	if err = store.RegisterPackageModule(adapter, revision); !errors.Is(err, ErrConflict) {
		t.Fatal("stale package preview accepted", err)
	}
	if err = store.RegisterPackageModule(adapter, store.CommittedDocument().Revision); err != nil {
		t.Fatal(err)
	}
	if err = store.UnregisterPackageModule(adapter.ID, store.CommittedDocument().Revision); err != nil {
		t.Fatal(err)
	}
	adapter.PackageSchemaVersion = 2
	if err = store.RegisterPackageModule(adapter, store.CommittedDocument().Revision); err == nil {
		t.Fatal("incompatible retained settings accepted")
	}
	if len(store.CommittedDocument().RetainedModules[adapter.ID]) == 0 {
		t.Fatal("schema failure discarded retained settings")
	}
}

func TestPackageConfigurationFailureLeavesNoPartialRegistration(t *testing.T) {
	store, err := OpenHostWithLifecycle(t.TempDir(), packageLifecycle())
	if err != nil {
		t.Fatal(err)
	}
	adapter := packageAdapter()
	adapter.Apply = func(json.RawMessage, uint64) error { return errors.New("projection unavailable") }
	revision := store.CommittedDocument().Revision
	if err = store.RegisterPackageModule(adapter, revision); err == nil {
		t.Fatal("failed projection was accepted")
	}
	if store.RecoveryError() != nil || store.CommittedDocument().Revision != revision {
		t.Fatal("rollback changed committed state")
	}
	adapter = packageAdapter()
	if err = store.RegisterPackageModule(adapter, revision); err != nil {
		t.Fatal("failed registration was not retryable", err)
	}
}

func TestPackageConfigurationRestartRestoresFailedRemoval(t *testing.T) {
	root := t.TempDir()
	lifecycle := packageLifecycle()
	enabled, fail := false, false
	lifecycle.Before = func(target map[string]bool) error {
		if !target["example.tool"] {
			enabled = false
		}
		return nil
	}
	lifecycle.After = func(target map[string]bool) error {
		if fail {
			return errors.New("native recovery unavailable")
		}
		enabled = target["example.tool"]
		return nil
	}
	store, err := OpenHostWithLifecycle(root, lifecycle)
	if err != nil {
		t.Fatal(err)
	}
	adapter := packageAdapter()
	if err = store.RegisterPackageModule(adapter, store.CommittedDocument().Revision); err != nil {
		t.Fatal(err)
	}
	next := store.CommittedDocument()
	next.ModuleEnabled[adapter.ID] = true
	if _, err = store.CommitDocument(next); err != nil {
		t.Fatal(err)
	}
	fail = true
	if err = store.UnregisterPackageModule(adapter.ID, store.CommittedDocument().Revision); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatal("unfinished recovery disappeared", err)
	}
	fail = false
	lifecycle.Initial[adapter.ID] = false
	store, err = OpenHostWithLifecycle(root, lifecycle, adapter)
	if err != nil || !enabled {
		t.Fatal("restart did not restore prior enabled state", err)
	}
	raw, _, err := store.Module(adapter.ID)
	if err != nil || string(raw) != `{"size":3}` || !store.CommittedDocument().ModuleEnabled[adapter.ID] {
		t.Fatal("restart lost module preferences", err)
	}
}

func TestPackageConfigurationKeepsBuiltinAndUnknownSwitchValidation(t *testing.T) {
	lifecycle := packageLifecycle()
	lifecycle.Initial["capture"] = false
	store, err := OpenHostWithLifecycle(t.TempDir(), lifecycle)
	if err != nil {
		t.Fatal(err)
	}
	for _, unknown := range []bool{false, true} {
		next := store.CommittedDocument()
		if unknown {
			next.ModuleEnabled["example.unknown"] = true
		} else {
			delete(next.ModuleEnabled, "capture")
		}
		if _, err = store.CommitDocument(next); err == nil {
			t.Fatal("invalid fixed module catalog accepted")
		}
	}
	adapter := packageAdapter()
	adapter.ID = "capture.tool"
	if err = store.RegisterPackageModule(adapter, store.CommittedDocument().Revision); err == nil {
		t.Fatal("reserved package namespace accepted")
	}
	if err = store.UnregisterPackageModule("capture", store.CommittedDocument().Revision); err == nil {
		t.Fatal("builtin was removed as an optional package")
	}
}

func TestPackageConfigurationRejectsPartlyRemovedLiveSettings(t *testing.T) {
	store, err := OpenHostWithLifecycle(t.TempDir(), packageLifecycle())
	if err != nil {
		t.Fatal(err)
	}
	adapter := packageAdapter()
	if err = store.RegisterPackageModule(adapter, store.CommittedDocument().Revision); err != nil {
		t.Fatal(err)
	}
	for _, missingSettings := range []bool{false, true} {
		next := store.CommittedDocument()
		if missingSettings {
			delete(next.Values, adapter.ID)
			next.ModuleEnabled[adapter.ID] = true
		} else {
			delete(next.ModuleEnabled, adapter.ID)
		}
		if _, err = store.CommitDocument(next); err == nil {
			t.Fatal("partial package settings were accepted", missingSettings)
		}
	}
}
