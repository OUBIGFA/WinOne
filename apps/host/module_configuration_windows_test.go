//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"winone/internal/catalog"
	"winone/internal/configuration"
	"winone/internal/host"
	"winone/internal/modules"
	"winone/internal/modulestate"
	"winone/internal/settings"
)

func TestDormantUnloadedPackageDoesNotBlockBuiltinLifecycle(t *testing.T) {
	directory := t.TempDir()
	state, err := modulestate.Open(filepath.Join(directory, "module-state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = state.BindPackage("example.tool", strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	gates := map[string]*modules.Gate{}
	for id := range state.Snapshot().Modules {
		if id == "example.tool" {
			continue
		}
		gate, err := modules.NewGate(id, id, state, modules.LifecycleHooks{Start: func() error { return nil }, Stop: func() error { return nil }, IntentChanged: func() error { return nil }, Status: func() host.Module { return host.Module{ID: id, State: "ready"} }, Handle: func(r host.Request) host.Response { return host.Response{ID: r.ID} }})
		if err != nil {
			t.Fatal(err)
		}
		gates[id] = gate
	}
	lifecycle := nativeModuleLifecycle(state, func() map[string]*modules.Gate { return gates })
	if len(lifecycle.Initial) != 6 {
		t.Fatal("unloaded package leaked into active configuration")
	}
	if err = lifecycle.Before(lifecycle.Initial); err != nil {
		t.Fatal("dormant package blocked builtin settings", err)
	}
	configurationStore, err := configuration.OpenHostWithLifecycle(directory, lifecycle)
	if err != nil {
		t.Fatal("dormant package blocked configuration loading", err)
	}
	document := configurationStore.CommittedDocument()
	var global map[string]any
	if err = json.Unmarshal(document.Values["host"], &global); err != nil {
		t.Fatal(err)
	}
	global["theme"] = "dark"
	document.Values["host"], err = json.Marshal(global)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = configurationStore.CommitDocument(document); err != nil {
		t.Fatal("dormant package blocked saving settings", err)
	}
	if _, err = configuration.OpenHostWithLifecycle(directory, nativeModuleLifecycle(state, func() map[string]*modules.Gate { return gates })); err != nil {
		t.Fatal("dormant package blocked reopening configuration", err)
	}
	if _, err = state.SetIntent(state.Snapshot().Revision, "example.tool", true); err != nil {
		t.Fatal(err)
	}
	if err = lifecycle.Before(lifecycle.Initial); err == nil {
		t.Fatal("active unloaded package silently ignored")
	}
	if _, err = state.SetIntent(state.Snapshot().Revision, "example.tool", false); err != nil {
		t.Fatal(err)
	}
	if err = lifecycle.Before(lifecycle.Initial); err == nil {
		t.Fatal("pending recovery silently ignored")
	}
}

func TestNativeLifecycleObservesModulesRegisteredAfterStartup(t *testing.T) {
	state, err := modulestate.Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	global, err := settings.Open(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	service := host.New(global)
	gates := map[string]*modules.Gate{}
	hooks := modules.LifecycleHooks{Start: func() error { return nil }, Stop: func() error { return nil }, IntentChanged: func() error { return nil }, Status: func() host.Module { return host.Module{} }, Handle: func(r host.Request) host.Response { return host.Response{ID: r.ID} }}
	for id := range state.Snapshot().Modules {
		gate, err := modules.NewGate(id, id, state, hooks)
		if err != nil {
			t.Fatal(err)
		}
		gates[id] = gate
	}
	control := modules.NewControl(state, gates, service.FeatureCatalog)
	lifecycle := nativeModuleLifecycle(state, control.LifecycleGates)
	manifest := catalog.Manifest{ManifestVersion: 1, ID: "example.dynamic", Name: catalog.LocalizedName{Chinese: "测试模块", English: "Test module"}, Distribution: "optional", Worker: "worker.exe", Activation: "on-demand", SettingsSchemaVersion: 1}
	digest := strings.Repeat("a", 64)
	if _, err = state.BindPackage(manifest.ID, digest); err != nil {
		t.Fatal(err)
	}
	starts, stops := 0, 0
	hooks.Start = func() error { starts++; return nil }
	hooks.Stop = func() error { stops++; return nil }
	gate, err := modules.NewPackageGate(manifest, digest, state, hooks)
	if err != nil {
		t.Fatal(err)
	}
	if err = control.RegisterPackage(service, manifest, gate); err != nil {
		t.Fatal(err)
	}
	target := lifecycle.Initial
	target[manifest.ID] = true
	if err = lifecycle.Before(target); err != nil {
		t.Fatal("new module rejected by configuration lifecycle", err)
	}
	if err = lifecycle.After(target); err != nil || !gate.Enabled() || starts != 1 {
		t.Fatal("new module was not enabled", err, starts)
	}
	target[manifest.ID] = false
	if err = lifecycle.Before(target); err != nil || gate.Enabled() || stops != 1 {
		t.Fatal("new module was not stopped", err, stops)
	}
	if err = control.UnregisterPackage(manifest.ID, gate); err != nil {
		t.Fatal(err)
	}
	delete(target, manifest.ID)
	if err = lifecycle.Before(target); err != nil {
		t.Fatal("removed module remained in lifecycle", err)
	}
}

// Registration must reach the same public enable handler used by the UI,
// rather than changing only the in-memory lifecycle map used by older tests.
func TestNewPackageConfigurationUsesHostEnableAndRemovalFlow(t *testing.T) {
	directory := t.TempDir()
	state, err := modulestate.Open(filepath.Join(directory, "module-state.json"))
	if err != nil {
		t.Fatal(err)
	}
	hooks := modules.LifecycleHooks{Start: func() error { return nil }, Stop: func() error { return nil }, IntentChanged: func() error { return nil }, Status: func() host.Module { return host.Module{State: "ready"} }, Handle: func(r host.Request) host.Response { return host.Response{ID: r.ID} }}
	gates := map[string]*modules.Gate{}
	for id := range state.Snapshot().Modules {
		gate, gateErr := modules.NewGate(id, id, state, hooks)
		if gateErr != nil {
			t.Fatal(gateErr)
		}
		gates[id] = gate
	}
	var service *host.Service
	control := modules.NewControl(state, gates, func() []catalog.Manifest {
		if service == nil {
			return catalog.Builtins()
		}
		return service.FeatureCatalog()
	})
	store, err := configuration.OpenHostWithLifecycle(directory, nativeModuleLifecycle(state, control.LifecycleGates))
	if err != nil {
		t.Fatal(err)
	}
	service = host.New(store)
	for _, manifest := range catalog.Builtins() {
		gate := gates[manifest.ID]
		service.RegisterModule(manifest.ID, gate.Status, gate.Handle)
	}
	service.RegisterModule("modules", func() host.Module { return host.Module{State: "ready"} }, moduleConfigurationHandler(store, state, control))
	manifest := catalog.Manifest{ManifestVersion: 1, ID: "example.dynamic", Name: catalog.LocalizedName{Chinese: "测试模块", English: "Test module"}, Distribution: "optional", Worker: "worker.exe", Activation: "on-demand", SettingsSchemaVersion: 1}
	digest := strings.Repeat("a", 64)
	if _, err = state.BindPackage(manifest.ID, digest); err != nil {
		t.Fatal(err)
	}
	projected, active := "", false
	hooks.Start = func() error {
		if projected != `{"size":3}` {
			return errors.New("enabled before configuration")
		}
		active = true
		return nil
	}
	hooks.Stop = func() error { active = false; return nil }
	gate, err := modules.NewPackageGate(manifest, digest, state, hooks)
	if err != nil {
		t.Fatal(err)
	}
	if err = control.RegisterPackage(service, manifest, gate); err != nil {
		t.Fatal(err)
	}
	adapter := configuration.ModuleAdapter{ID: manifest.ID, PackageSchemaVersion: 1, Load: func() (json.RawMessage, error) { return json.RawMessage(`{"size":3}`), nil }, Validate: func(json.RawMessage) error { return nil }, Apply: func(raw json.RawMessage, _ uint64) error { projected = string(raw); return nil }}
	if err = store.RegisterPackageModule(adapter, store.CommittedDocument().Revision); err != nil {
		t.Fatal(err)
	}
	if active || gate.Enabled() {
		t.Fatal("package registration enabled runtime")
	}
	params, _ := json.Marshal(map[string]any{"id": manifest.ID, "enabled": true, "revision": state.Snapshot().Revision})
	reply := service.Handle(host.Request{Version: 1, ID: "enable", Method: "modules.setEnabled", Params: params})
	if reply.Error != nil || !active || !gate.Enabled() || !store.CommittedDocument().ModuleEnabled[manifest.ID] {
		t.Fatal("Host enable handler rejected newly installed configuration", reply.Error)
	}
	if err = store.UnregisterPackageModule(manifest.ID, store.CommittedDocument().Revision); err != nil {
		t.Fatal(err)
	}
	if active || gate.Enabled() {
		t.Fatal("configuration removal left runtime enabled")
	}
	if err = control.UnregisterPackage(manifest.ID, gate); err != nil {
		t.Fatal(err)
	}
	if len(control.Snapshot().Modules) != 6 {
		t.Fatal("removed package remains visible")
	}
	next := store.Snapshot()
	next.Theme = "dark"
	if _, err = store.Save(next); err != nil {
		t.Fatal("removed package blocked builtin preferences", err)
	}
}

func TestNativeLifecycleRetainsRecoveryRouteAfterStartupFailure(t *testing.T) {
	directory := t.TempDir()
	state, err := modulestate.Open(filepath.Join(directory, "module-state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = state.SetIntent(0, "capture", false); err != nil {
		t.Fatal(err)
	}
	failed := true
	gates := map[string]*modules.Gate{}
	for id := range state.Snapshot().Modules {
		moduleID := id
		gate, err := modules.NewGate(id, id, state, modules.LifecycleHooks{
			Start: func() error { return nil }, Stop: func() error {
				if moduleID == "capture" && failed {
					return errors.New("editor is still open")
				}
				return nil
			}, IntentChanged: func() error { return nil },
			Status: func() host.Module { return host.Module{ID: moduleID, State: "available"} }, Handle: func(r host.Request) host.Response { return host.Response{ID: r.ID} },
		})
		if err != nil {
			t.Fatal(err)
		}
		gates[id] = gate
	}
	store, err := configuration.OpenHostWithLifecycle(directory, nativeModuleLifecycle(state, func() map[string]*modules.Gate { return gates }))
	if err == nil || store == nil {
		t.Fatal("startup failure was hidden", err)
	}
	handler := moduleConfigurationHandler(store, state, modules.NewControl(state, gates, host.New(store).FeatureCatalog))
	response := handler(host.Request{Version: 1, ID: "snapshot", Method: "snapshot"})
	var snapshot struct {
		ConfigurationError string `json:"configurationError"`
	}
	if err = json.Unmarshal(response.Result, &snapshot); err != nil || snapshot.ConfigurationError == "" {
		t.Fatal(snapshot, err)
	}
	failed = false
	response = handler(host.Request{Version: 1, ID: "retry", Method: "recoverConfiguration"})
	if response.Error != nil {
		t.Fatal(response.Error)
	}
	if state.Snapshot().Modules["capture"].PendingStop || store.RecoveryError() != nil {
		t.Fatal("recovery did not finish")
	}
}
