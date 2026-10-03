//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"winone/internal/configuration"
	"winone/internal/host"
	"winone/internal/modules"
	"winone/internal/modulestate"
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
	lifecycle := nativeModuleLifecycle(state, gates)
	if len(lifecycle.Initial) != 3 {
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
	if _, err = configuration.OpenHostWithLifecycle(directory, nativeModuleLifecycle(state, gates)); err != nil {
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
	store, err := configuration.OpenHostWithLifecycle(directory, nativeModuleLifecycle(state, gates))
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
