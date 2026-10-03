//go:build windows

package main

import (
	"testing"
	"winone/internal/catalog"
	"winone/internal/host"
	"winone/internal/modules"
)

func snapshotFor(enabled bool) healthSnapshot {
	result := healthSnapshot{}
	state := "available"
	if !enabled {
		state = "disabled"
	}
	for _, manifest := range catalog.Builtins() {
		result.Modules = append(result.Modules, modules.ModuleView{Manifest: manifest, Enabled: enabled, Runtime: host.Module{ID: manifest.ID, State: state}})
	}
	return result
}
func TestHealthChecksBothEnabledAndDisabledScenarios(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		if ready, err := healthy(snapshotFor(enabled), enabled); err != nil || !ready {
			t.Fatal(enabled, ready, err)
		}
		if _, err := healthy(snapshotFor(enabled), !enabled); err == nil {
			t.Fatal("wrong module state accepted")
		}
	}
}
func TestHealthRejectsIncompleteOrFaultedInventories(t *testing.T) {
	for _, mutate := range []func(*healthSnapshot){
		func(s *healthSnapshot) { s.Modules = s.Modules[:2] },
		func(s *healthSnapshot) { s.Modules[1].Manifest.ID = s.Modules[0].Manifest.ID },
		func(s *healthSnapshot) { s.Modules[0].PendingStop = true },
		func(s *healthSnapshot) { s.Modules[0].Runtime.State = "unavailable" },
		func(s *healthSnapshot) { s.ConfigurationError = "recovery pending" },
	} {
		snapshot := snapshotFor(true)
		mutate(&snapshot)
		if _, err := healthy(snapshot, true); err == nil {
			t.Fatal("invalid inventory accepted", snapshot)
		}
	}
	snapshot := snapshotFor(true)
	snapshot.Modules[0].Runtime.State = "starting"
	if ready, err := healthy(snapshot, true); err != nil || ready {
		t.Fatal("startup treated as ready", ready, err)
	}
	snapshot = snapshotFor(false)
	snapshot.Modules[0].Runtime.State = "ready"
	if _, err := healthy(snapshot, false); err == nil {
		t.Fatal("running disabled module accepted")
	}
}
