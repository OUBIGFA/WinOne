//go:build windows

package main

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"winone/internal/host"
	"winone/internal/modules"
	"winone/internal/settings"
)

func TestDesktopProbeHomepageUsesReadOnlyModuleController(t *testing.T) {
	directory := t.TempDir()
	store, err := settings.Open(filepath.Join(directory, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	service, err := desktopProbeService(directory, store)
	if err != nil {
		t.Fatal(err)
	}
	result := service.Handle(host.Request{Version: 1, ID: "test", Method: "modules.snapshot"})
	var snapshot modules.ControlSnapshot
	if err = json.Unmarshal(result.Result, &snapshot); err != nil || len(snapshot.Modules) != 3 {
		t.Fatal(snapshot, err)
	}
	for _, module := range snapshot.Modules {
		if module.Runtime.State != "unavailable" {
			t.Fatal("probe claimed a running worker", module)
		}
	}
	result = service.Handle(host.Request{Version: 1, ID: "test", Method: "modules.setEnabled", Params: json.RawMessage(`{"id":"capture","enabled":true,"revision":0}`)})
	if result.Error == nil {
		t.Fatal("probe allowed module activation")
	}
}
