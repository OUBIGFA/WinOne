package modules

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"winone/internal/catalog"
	"winone/internal/host"
	"winone/internal/modulestate"
	"winone/internal/settings"
)

func removablePackage(t *testing.T, stop func() error) (*Control, *host.Service, *Gate, *modulestate.Store, catalog.Manifest) {
	t.Helper()
	state, err := modulestate.Open(filepath.Join(t.TempDir(), "modules.json"))
	if err != nil {
		t.Fatal(err)
	}
	global, err := settings.Open(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	service := host.New(global)
	control := NewControl(state, nil, service.FeatureCatalog)
	manifest := catalog.Manifest{ManifestVersion: 1, ID: "example.tool", Name: catalog.LocalizedName{Chinese: "示例", English: "Example"}, Distribution: "optional", Worker: "worker.exe", Activation: "on-demand", SettingsSchemaVersion: 1}
	digest := strings.Repeat("a", 64)
	if _, err = state.BindPackage(manifest.ID, digest); err != nil {
		t.Fatal(err)
	}
	gate, err := NewPackageGate(manifest, digest, state, LifecycleHooks{Start: func() error { return nil }, Stop: stop, IntentChanged: func() error { return nil }, Status: func() host.Module { return host.Module{ID: manifest.ID, State: "ready"} }, Handle: func(r host.Request) host.Response { return host.Response{ID: r.ID, Result: json.RawMessage(`"old"`)} }})
	if err != nil {
		t.Fatal(err)
	}
	if err = control.RegisterPackage(service, manifest, gate); err != nil {
		t.Fatal(err)
	}
	return control, service, gate, state, manifest
}

func TestUnregisterRetiresOldGateAcrossReinstallation(t *testing.T) {
	control, service, gate, state, manifest := removablePackage(t, func() error { return nil })
	if err := gate.SetEnabled(state.Snapshot().Revision, true); err != nil {
		t.Fatal(err)
	}
	if err := control.UnregisterPackage(manifest.ID, gate); err == nil {
		t.Fatal("enabled package was removed")
	}
	if reply := service.Handle(host.Request{Version: 1, ID: "kept", Method: "example.tool.ping"}); reply.Error != nil {
		t.Fatal("refused removal changed route", reply)
	}
	if err := gate.SetEnabled(state.Snapshot().Revision, false); err != nil {
		t.Fatal(err)
	}
	before := state.Snapshot()
	if err := control.UnregisterPackage(manifest.ID, gate); err != nil {
		t.Fatal(err)
	}
	if state.Snapshot().Revision != before.Revision {
		t.Fatal("runtime removal changed persisted preferences")
	}
	if len(control.Snapshot().Modules) != 0 {
		t.Fatal("removed gate remains in module controls")
	}
	if reply := service.Handle(host.Request{Version: 1, ID: "gone", Method: "example.tool.ping"}); reply.Error == nil {
		t.Fatal("removed route still callable")
	}
	if _, err := state.SetIntent(state.Snapshot().Revision, manifest.ID, true); err != nil {
		t.Fatal(err)
	}
	if gate.Enabled() || gate.Status().State != "unavailable" {
		t.Fatal("old gate became available again")
	}
	if err := gate.SetEnabled(state.Snapshot().Revision, true); err == nil {
		t.Fatal("old gate could re-enable itself")
	}
	if reply := gate.Handle(host.Request{Version: 1, ID: "captured", Method: "ping"}); reply.Error == nil {
		t.Fatal("captured old route ran after reinstallation")
	}
	if err := control.RegisterPackage(service, manifest, gate); err == nil {
		t.Fatal("retired gate was rebound")
	}
	newGate, err := NewPackageGate(manifest, state.Snapshot().Modules[manifest.ID].PackageSHA256, state, LifecycleHooks{Start: func() error { return nil }, Stop: func() error { return nil }, IntentChanged: func() error { return nil }, Status: func() host.Module { return host.Module{ID: manifest.ID, State: "ready"} }, Handle: func(r host.Request) host.Response { return host.Response{ID: r.ID, Result: json.RawMessage(`"new"`)} }})
	if err != nil {
		t.Fatal(err)
	}
	if err = control.RegisterPackage(service, manifest, newGate); err != nil {
		t.Fatal(err)
	}
	if err = control.UnregisterPackage(manifest.ID, gate); err != nil {
		t.Fatal("removal replay failed", err)
	}
	if reply := service.Handle(host.Request{Version: 1, ID: "new", Method: "example.tool.ping"}); reply.Error != nil || string(reply.Result) != `"new"` {
		t.Fatal("old replay removed current binding", reply)
	}
}

func TestUnregisterPreservesFailedRestorationAndChangedRoute(t *testing.T) {
	refuse := true
	control, service, gate, state, manifest := removablePackage(t, func() error {
		if refuse {
			return errors.New("restoration pending")
		}
		return nil
	})
	if err := gate.SetEnabled(state.Snapshot().Revision, true); err != nil {
		t.Fatal(err)
	}
	if err := gate.SetEnabled(state.Snapshot().Revision, false); err == nil {
		t.Fatal("fixture did not refuse restoration")
	}
	if err := control.UnregisterPackage(manifest.ID, gate); err == nil {
		t.Fatal("pending restoration was discarded")
	}
	if !state.Snapshot().Modules[manifest.ID].PendingStop || len(control.Snapshot().Modules) != 1 {
		t.Fatal("recovery information was lost")
	}
	refuse = false
	if err := gate.SetEnabled(state.Snapshot().Revision, false); err != nil {
		t.Fatal(err)
	}
	service.RegisterModule(manifest.ID, func() host.Module { return host.Module{ID: manifest.ID} }, func(r host.Request) host.Response {
		return host.Response{ID: r.ID, Result: json.RawMessage(`"replacement"`)}
	})
	if err := control.UnregisterPackage(manifest.ID, gate); err == nil {
		t.Fatal("changed binding was removed")
	}
	if len(control.Snapshot().Modules) != 1 {
		t.Fatal("failed removal discarded controls")
	}
	if err := gate.SetEnabled(state.Snapshot().Revision, true); err != nil {
		t.Fatal("failed removal retired the old gate", err)
	}
	if reply := service.Handle(host.Request{Version: 1, ID: "replacement", Method: "example.tool.ping"}); reply.Error != nil || string(reply.Result) != `"replacement"` {
		t.Fatal(reply)
	}
}

func TestUnregisterKeepsStatusResponsiveDuringResourceChecks(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	control, _, gate, _, manifest := removablePackage(t, func() error { close(entered); <-release; return nil })
	done := make(chan error, 1)
	go func() { done <- control.UnregisterPackage(manifest.ID, gate) }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("resource check did not start")
	}
	status := make(chan ControlSnapshot, 1)
	go func() { status <- control.Snapshot() }()
	select {
	case snapshot := <-status:
		if len(snapshot.Modules) != 1 {
			t.Fatal("entry vanished before restoration finished")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("resource check blocked status queries")
	}
	if !control.Busy() {
		t.Fatal("retirement was not reported as busy")
	}
	if err := control.UnregisterPackage(manifest.ID, gate); err == nil {
		t.Fatal("overlapping retirement accepted")
	}
	once.Do(func() { close(release) })
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("retirement did not finish")
	}
	if control.Busy() || len(control.Snapshot().Modules) != 0 {
		t.Fatal("completed retirement retained control state")
	}
}
