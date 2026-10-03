//go:build windows

package main

import (
	"encoding/json"
	"winone/internal/configuration"
	"winone/internal/host"
	"winone/internal/modules"
	"winone/internal/residency"
	"winone/modules/wintray/preferences"
)

// Read-only lifetime observation must not turn an idle race into a new process.
type observedModule struct {
	gate   *modules.Gate
	worker *modules.Lazy
	idle   func(host.Request) host.Response
}

func (m observedModule) Status() host.Module { return m.gate.Status() }
func (m observedModule) Enabled() bool       { return m.gate.Enabled() }
func (m observedModule) Handle(request host.Request) host.Response {
	if response, called := m.worker.HandleRunning(request); called {
		return response
	}
	if m.worker.Status().State == "faulted" {
		return host.Response{ID: request.ID, Error: &host.RPCError{Code: "module_faulted", Message: "组件需要恢复"}}
	}
	if m.idle != nil {
		return m.idle(request)
	}
	return host.Response{ID: request.ID, Error: &host.RPCError{Code: "stopped", Message: "组件已退出"}}
}

func idleTrayLifetime(store *configuration.HostStore, autorun bool) func(host.Request) host.Response {
	return func(request host.Request) host.Response {
		fail := func() host.Response {
			return host.Response{ID: request.ID, Error: &host.RPCError{Code: "unavailable", Message: "无法确认程序任务状态"}}
		}
		if request.Method != "lifetime" || store.RecoveryError() != nil {
			return fail()
		}
		raw, _, err := store.Module("wintray")
		if err != nil {
			return fail()
		}
		value, err := preferences.Decode(raw)
		if err != nil {
			return fail()
		}
		state := residency.State{Requested: autorun && value.ExitAfterManagedAppsCompleted, Reasons: []string{}}
		if len(value.ManagedApps) > 0 {
			state.Reasons = append(state.Reasons, "程序管理组件未运行，请检查模块状态")
		}
		data, err := json.Marshal(state)
		if err != nil {
			return fail()
		}
		return host.Response{ID: request.ID, Result: data}
	}
}
