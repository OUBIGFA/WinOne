//go:build windows

package main

import (
	"errors"
	"path/filepath"
	"winone/internal/catalog"
	"winone/internal/host"
	"winone/internal/modules"
	"winone/internal/modulestate"
	"winone/internal/settings"
)

// The homepage reads the same module controller as normal sessions. Probe
// gates expose only the real unstarted state and cannot activate any worker.
func desktopProbeService(directory string, store *settings.Store) (*host.Service, error) {
	state, err := modulestate.Open(filepath.Join(directory, "module-state.json"))
	if err != nil {
		return nil, err
	}
	gates := map[string]*modules.Gate{}
	deny := func() error { return errors.New("startup probe is read-only") }
	for _, manifest := range catalog.Builtins() {
		gate, err := modules.NewGate(manifest.ID, manifest.Name.Chinese, state, modules.LifecycleHooks{
			Start: deny, Stop: deny, IntentChanged: deny,
			Status: func() host.Module {
				return host.Module{ID: manifest.ID, Name: manifest.Name.Chinese, State: "unavailable", Detail: "组件未启动"}
			},
			Handle: func(r host.Request) host.Response {
				return host.Response{ID: r.ID, Error: &host.RPCError{Code: "probe_read_only", Message: "启动检查只允许读取状态"}}
			},
		})
		if err != nil {
			return nil, err
		}
		gates[manifest.ID] = gate
	}
	service := host.New(store)
	control := modules.NewControl(state, gates, service.FeatureCatalog)
	service.RegisterModule("modules", func() host.Module { return host.Module{ID: "modules", State: "ready"} }, func(r host.Request) host.Response {
		if r.Method != "snapshot" {
			return host.Response{ID: r.ID, Error: &host.RPCError{Code: "probe_read_only", Message: "启动检查只允许读取状态"}}
		}
		return control.Handle(r)
	})
	return service, nil
}
