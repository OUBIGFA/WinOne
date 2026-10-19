//go:build windows

package main

import (
	"encoding/json"
	"sync"
	"winone/internal/configjson"
	"winone/internal/host"
	"winone/internal/quickassistant"
	"winone/internal/resultwindow"
)

// Enabling this module registers only a shortcut. UI and AI resources are
// acquired on show and released by the shared result-window lifecycle.
type quickAssistantRuntime struct {
	mu          sync.Mutex
	results     *resultWindows
	preferences quickassistant.Preferences
}

func (r *quickAssistantRuntime) configure(p quickassistant.Preferences) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p.Actions = p.EffectiveActions()
	r.preferences = p
}
func (r *quickAssistantRuntime) status() host.Module {
	return host.Module{ID: "quick-assistant", Name: "快捷助手", State: "ready"}
}
func (r *quickAssistantRuntime) start() error {
	if r.results != nil {
		r.results.allowQuickAssistant(true)
	}
	return nil
}
func (r *quickAssistantRuntime) stop() error {
	if r.results == nil {
		return nil
	}
	r.results.allowQuickAssistant(false)
	return r.results.closeSource("quick-assistant")
}
func (r *quickAssistantRuntime) handle(request host.Request) host.Response {
	if request.Method == "snapshot" {
		data, _ := json.Marshal(r.status())
		return host.Response{ID: request.ID, Result: data}
	}
	if request.Method != "show" && request.Method != "fromSelection" || r.results == nil {
		return host.Response{ID: request.ID, Error: &host.RPCError{Code: "quick_assistant", Message: "快捷助手暂时不可用，请重试"}}
	}
	var input resultwindow.SelectionHandoff
	if request.Method == "fromSelection" {
		err := configjson.DecodeObject(request.Params, &input)
		if err == nil {
			err = r.results.validateSelectionHandoff(input, request.PeerPID)
		}
		if err != nil {
			return host.Response{ID: request.ID, Error: &host.RPCError{Code: "quick_assistant", Message: err.Error()}}
		}
	}
	r.mu.Lock()
	actions := r.preferences.EffectiveActions()
	options := resultwindow.QuickOptions{AlwaysOnTop: r.preferences.AlwaysOnTop, ShowReasoning: r.preferences.ShowReasoning}
	r.mu.Unlock()
	if err := r.results.openQuickAssistantWithText(input.Text, actions, options); err != nil {
		return host.Response{ID: request.ID, Error: &host.RPCError{Code: "quick_assistant", Message: err.Error()}}
	}
	target, err := r.results.quickAssistantTarget()
	// In-process hotkeys have no pipe peer. Windows grants their owner the
	// foreground right; clicks instead activate in the native caller below.
	if err == nil && request.PeerPID == 0 {
		err = target.Activate()
	}
	if err != nil {
		return host.Response{ID: request.ID, Error: &host.RPCError{Code: "quick_assistant", Message: err.Error()}}
	}
	data, _ := json.Marshal(struct {
		resultwindow.WindowTarget
		Shown bool `json:"shown"`
	}{target, true})
	return host.Response{ID: request.ID, Result: data}
}
