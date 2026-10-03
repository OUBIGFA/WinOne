//go:build windows

package modulepackage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"winone/internal/host"
	"winone/internal/modules"
	"winone/internal/modulestate"
)

func TestPreparedModuleStartsDisabledAndReplayPreservesLaterIntent(t *testing.T) {
	registry, stage, options := registeredFixture(t)
	state, err := modulestate.Open(filepath.Join(t.TempDir(), "intent.json"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := registry.PrepareModule(context.Background(), filepath.Base(stage.Directory()), options, 0, state)
	if err != nil {
		t.Fatal(err)
	}
	record := state.Snapshot().Modules[result.Package.ModuleID]
	if record.Enabled || record.PendingStop || record.PackageSHA256 != result.Package.ManifestSHA256 {
		t.Fatal("new installation is not bound and disabled", record)
	}
	starts := 0
	gate, err := modules.NewPackageGate(stage.Manifest().Metadata().Module, stage.Manifest().Digest(), state, modules.LifecycleHooks{Start: func() error { starts++; return nil }, Stop: func() error { return nil }, IntentChanged: func() error { return nil }, Status: func() host.Module { return host.Module{ID: result.Package.ModuleID, State: "ready"} }, Handle: func(r host.Request) host.Response { return host.Response{ID: r.ID} }})
	if err != nil || starts != 0 || gate.Status().State != "disabled" {
		t.Fatal("preparation activated module", err)
	}
	if err = gate.SetEnabled(state.Snapshot().Revision, true); err != nil {
		t.Fatal(err)
	}
	if _, err = registry.PrepareModule(context.Background(), filepath.Base(stage.Directory()), options, 0, state); err != nil {
		t.Fatal(err)
	}
	if !state.Snapshot().Modules[result.Package.ModuleID].Enabled || starts != 1 {
		t.Fatal("replayed installation changed user intent")
	}
}

func TestInterruptedIntentBindingRemainsInactiveAndCanResume(t *testing.T) {
	registry, stage, options := registeredFixture(t)
	statePath := filepath.Join(t.TempDir(), "intent.json")
	state, err := modulestate.Open(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(statePath, 0700); err != nil {
		t.Fatal(err)
	}
	result, err := registry.PrepareModule(context.Background(), filepath.Base(stage.Directory()), options, 0, state)
	if err == nil || result.Package.ModuleID == "" {
		t.Fatal("failed binding lost retry identity", err)
	}
	if _, err = modules.NewPackageGate(stage.Manifest().Metadata().Module, stage.Manifest().Digest(), state, modules.LifecycleHooks{}); err == nil {
		t.Fatal("unbound package could create runtime gate")
	}
	if err = os.Remove(statePath); err != nil {
		t.Fatal(err)
	}
	result, err = registry.PrepareModule(context.Background(), filepath.Base(stage.Directory()), options, 0, state)
	if err != nil {
		t.Fatal(err)
	}
	if result.RegistryRevision != 1 || state.Snapshot().Modules[result.Package.ModuleID].Enabled {
		t.Fatal("resume duplicated or activated installation", result)
	}
	restarted, err := modulestate.Open(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if restarted.Snapshot().Modules[result.Package.ModuleID].PackageSHA256 != result.Package.ManifestSHA256 {
		t.Fatal("binding did not persist")
	}
}
