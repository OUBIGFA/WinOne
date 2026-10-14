package modules

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"winone/internal/catalog"
	"winone/internal/host"
	"winone/internal/modulestate"
	"winone/internal/settings"
)

func TestControlRegistersPackageCreatedAfterControl(t *testing.T) {
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
	manifest := catalog.Manifest{ManifestVersion: 1, ID: "example.tool", Name: catalog.LocalizedName{Chinese: "测试工具", English: "Test tool"}, Distribution: "optional", Worker: "worker.exe", Activation: "on-demand", SettingsSchemaVersion: 1}
	digest := strings.Repeat("a", 64)
	if _, err = state.BindPackage(manifest.ID, digest); err != nil {
		t.Fatal(err)
	}
	gate, err := NewPackageGate(manifest, digest, state, LifecycleHooks{
		Start: func() error { return nil }, Stop: func() error { return nil }, IntentChanged: func() error { return nil },
		Status: func() host.Module { return host.Module{ID: manifest.ID, State: "available"} },
		Handle: func(r host.Request) host.Response {
			return host.Response{ID: r.ID, Result: json.RawMessage(`"called"`)}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = control.RegisterPackage(service, manifest, gate); err != nil {
		t.Fatal(err)
	}
	listed := control.Snapshot().Modules
	if len(listed) != 1 || listed[0].Manifest.ID != manifest.ID || listed[0].Enabled {
		t.Fatal("new package was not listed disabled", listed)
	}
	if reply := service.Handle(host.Request{Version: 1, ID: "disabled", Method: "example.tool.ping"}); reply.Error == nil || reply.Error.Code != "module_disabled" {
		t.Fatal("registration bypassed enable intent", reply)
	}
	if err = gate.SetEnabled(state.Snapshot().Revision, true); err != nil {
		t.Fatal(err)
	}
	if reply := service.Handle(host.Request{Version: 1, ID: "enabled", Method: "example.tool.ping"}); reply.Error != nil || string(reply.Result) != `"called"` {
		t.Fatal("new package route unavailable", reply)
	}
	if err = control.RegisterPackage(service, manifest, gate); err == nil {
		t.Fatal("duplicate runtime registration accepted")
	}
	if len(control.Snapshot().Modules) != 1 {
		t.Fatal("duplicate registration changed control")
	}
}

func TestControlFailedPackageRegistrationLeavesNoGate(t *testing.T) {
	state, err := modulestate.Open(filepath.Join(t.TempDir(), "modules.json"))
	if err != nil {
		t.Fatal(err)
	}
	global, err := settings.Open(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	service := host.New(global)
	service.RegisterModule("example.tool.private", func() host.Module { return host.Module{} }, func(r host.Request) host.Response {
		return host.Response{ID: r.ID, Result: json.RawMessage(`"original"`)}
	})
	control := NewControl(state, nil, service.FeatureCatalog)
	manifest := catalog.Manifest{ManifestVersion: 1, ID: "example.tool", Name: catalog.LocalizedName{Chinese: "测试", English: "Test"}, Distribution: "optional", Worker: "worker.exe", Activation: "on-demand", SettingsSchemaVersion: 1}
	digest := strings.Repeat("a", 64)
	if _, err := state.BindPackage(manifest.ID, digest); err != nil {
		t.Fatal(err)
	}
	gate, err := NewPackageGate(manifest, digest, state, LifecycleHooks{Start: func() error { return nil }, Stop: func() error { return nil }, IntentChanged: func() error { return nil }, Status: func() host.Module { return host.Module{} }, Handle: func(r host.Request) host.Response { return host.Response{ID: r.ID} }})
	if err != nil {
		t.Fatal(err)
	}
	if err = control.RegisterPackage(service, manifest, gate); err == nil {
		t.Fatal("conflicting namespace registered")
	}
	if len(control.Snapshot().Modules) != 0 {
		t.Fatal("failed registration left a controllable gate")
	}
	if reply := service.Handle(host.Request{Version: 1, ID: "keep", Method: "example.tool.private.ping"}); reply.Error != nil || string(reply.Result) != `"original"` {
		t.Fatal("existing service changed", reply)
	}
}
