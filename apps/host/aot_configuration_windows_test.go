//go:build windows

package main

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"winone/internal/configuration"
	"winone/internal/host"
	"winone/internal/modules"
	"winone/modules/always-on-top/engine"
)

func TestAOTPreferencesPersistWithoutActivatingDisabledModule(t *testing.T) {
	directory := t.TempDir()
	store, err := configuration.OpenHost(directory, aotConfiguration(directory, func() *modules.Lazy { return nil }))
	if err != nil {
		t.Fatal(err)
	}
	handler := aotConfigurationHandler(store, func(r host.Request) host.Response {
		return host.Response{ID: r.ID, Error: &host.RPCError{Code: "module_disabled"}}
	})
	response := handler(host.Request{Version: 1, ID: "read", Method: "settings.get"})
	if response.Error != nil {
		t.Fatal(response.Error)
	}
	var value engine.Settings
	if err = json.Unmarshal(response.Result, &value); err != nil {
		t.Fatal(err)
	}
	value.FrameThickness = 11
	data, _ := json.Marshal(value)
	response = handler(host.Request{Version: 1, ID: "save", Method: "settings.save", Params: data})
	if response.Error != nil {
		t.Fatal(response.Error)
	}
	projected, err := engine.OpenSettings(filepath.Join(directory, "modules", "always-on-top", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !projected.IsManaged() || projected.Snapshot().FrameThickness != 11 || projected.Snapshot().Revision != 1 {
		t.Fatal(projected.Snapshot())
	}
	response = handler(host.Request{Version: 1, ID: "action", Method: "window.set"})
	if response.Error == nil || response.Error.Code != "module_disabled" {
		t.Fatal("feature bypassed disable gate")
	}
	response = handler(host.Request{Version: 1, ID: "stale", Method: "settings.save", Params: data})
	if response.Error == nil {
		t.Fatal("stale preferences accepted")
	}
}
