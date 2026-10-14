package configuration

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
	"winone/internal/settings"
)

func TestUnavailablePackagePreservesOpaqueSettingsWithoutBlockingOtherSettings(t *testing.T) {
	root := t.TempDir()
	lifecycle := packageLifecycle()
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
	original := store.CommittedDocument()
	lifecycle.Initial[adapter.ID] = true
	blocked := ModuleAdapter{ID: adapter.ID, Unavailable: errors.New("package signature is invalid")}
	store, err = OpenHostWithLifecycle(root, lifecycle, blocked)
	if err != nil {
		t.Fatal("blocked package stopped Host", err)
	}
	if _, _, err = store.Module(adapter.ID); err == nil {
		t.Fatal("unverified settings exposed as editable")
	}
	next = store.CommittedDocument()
	var global map[string]any
	_ = json.Unmarshal(next.Values["host"], &global)
	global["theme"] = "dark"
	next.Values["host"], _ = json.Marshal(global)
	if _, err = store.CommitDocument(next); err != nil {
		t.Fatal("unrelated settings blocked", err)
	}
	if !bytes.Equal(store.CommittedDocument().Values[adapter.ID], original.Values[adapter.ID]) || !store.CommittedDocument().ModuleEnabled[adapter.ID] {
		t.Fatal("quarantine changed saved package data")
	}
	next = store.CommittedDocument()
	next.Values[adapter.ID] = json.RawMessage(`{"size":8}`)
	if _, err = store.CommitDocument(next); err == nil {
		t.Fatal("opaque package settings modified")
	}
	next = store.CommittedDocument()
	next.ModuleEnabled[adapter.ID] = false
	if _, err = store.CommitDocument(next); err == nil {
		t.Fatal("quarantine silently completed a disable")
	}
}

func TestUnavailablePackageCannotHidePendingConfigurationRecovery(t *testing.T) {
	root := t.TempDir()
	lifecycle := packageLifecycle()
	store, err := OpenHostWithLifecycle(root, lifecycle)
	if err != nil {
		t.Fatal(err)
	}
	adapter := packageAdapter()
	if err = store.RegisterPackageModule(adapter, store.CommittedDocument().Revision); err != nil {
		t.Fatal(err)
	}
	store.SetApply(func(_ settings.Settings) error { return errors.New("restore interrupted") })
	next := store.CommittedDocument()
	next.Values[adapter.ID] = json.RawMessage(`{"size":8}`)
	if _, err = store.CommitDocument(next); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatal(err)
	}
	lifecycle.Initial[adapter.ID] = false
	store, err = OpenHostWithLifecycle(root, lifecycle, ModuleAdapter{ID: adapter.ID, Unavailable: errors.New("missing package")})
	if store == nil || !errors.Is(err, ErrRecoveryRequired) {
		t.Fatal("pending recovery was discarded or Host became unreadable", err)
	}
	if !store.store.NeedsRecovery() {
		t.Fatal("unfinished package projection was called recovered")
	}
	if err = store.Recover(); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatal("retry hid unavailable recovery", err)
	}
}

func TestUnavailablePackageCanAdoptVerifiedAdapterWithoutChangingSavedData(t *testing.T) {
	root := t.TempDir()
	lifecycle := packageLifecycle()
	store, err := OpenHostWithLifecycle(root, lifecycle)
	if err != nil {
		t.Fatal(err)
	}
	adapter := packageAdapter()
	if err = store.RegisterPackageModule(adapter, store.CommittedDocument().Revision); err != nil {
		t.Fatal(err)
	}
	before := store.CommittedDocument()
	lifecycle.Initial[adapter.ID] = false
	store, err = OpenHostWithLifecycle(root, lifecycle, ModuleAdapter{ID: adapter.ID, Unavailable: errors.New("missing")})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.RestorePackageAdapter(adapter, before.Revision); err != nil {
		t.Fatal(err)
	}
	raw, revision, err := store.Module(adapter.ID)
	if err != nil || revision != before.Revision || !bytes.Equal(raw, before.Values[adapter.ID]) {
		t.Fatal("reload changed canonical data", err)
	}
	if err = store.QuarantinePackage(adapter.ID, errors.New("not finalized")); err != nil {
		t.Fatal(err)
	}
	if _, _, err = store.Module(adapter.ID); err == nil {
		t.Fatal("failed package activation remained editable")
	}
}
