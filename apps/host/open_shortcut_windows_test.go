//go:build windows

package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
	"winone/internal/host"
	"winone/internal/modulestate"
	"winone/internal/settings"
)

func TestOpenShortcutRemainsAvailableWhileCaptureIsBusy(t *testing.T) {
	store, err := settings.Open(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	service := host.New(store)
	started, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	service.RegisterModule("capture", func() host.Module { return host.Module{} }, func(req host.Request) host.Response {
		if req.Method == "draft.get" {
			return host.Response{Result: json.RawMessage(`{"draft":null}`)}
		}
		if req.Method == "capture.unified" {
			close(started)
			<-release
			return host.Response{Result: json.RawMessage(`{"state":"cancelled"}`)}
		}
		return host.Response{Error: &host.RPCError{Message: "open shortcut reached capture"}}
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	opened := make(chan struct{}, 1)
	registrar := &activationRegistrar{events: make(chan string, 2)}
	runtime := &shortcutRuntime{registrar: registrar, openWindow: func() error { opened <- struct{}{}; return nil }}
	go runtime.run(ctx, service)
	registrar.events <- "capture"
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("capture did not start")
	}
	registrar.events <- "open-winone"
	select {
	case <-opened:
	case <-time.After(3 * time.Second):
		t.Fatal("open shortcut was blocked by capture")
	}
}

func TestOpenShortcutReportsLegacyDefaultConflict(t *testing.T) {
	runtime := &shortcutRuntime{registrar: &recordingRegistrar{}}
	if err := runtime.applyConfiguration(settings.Settings{CaptureShortcut: "Alt+A"}); err != nil {
		t.Fatal(err)
	}
	response := runtime.handle(host.Request{Method: "snapshot"})
	var result struct {
		Bindings []struct {
			ID, Shortcut, Error string
			Registered          bool
		}
	}
	if err := json.Unmarshal(response.Result, &result); err != nil {
		t.Fatal(err)
	}
	for _, binding := range result.Bindings {
		if binding.ID == "open-winone" {
			if binding.Registered || binding.Error == "" {
				t.Fatalf("conflict not visible: %+v", binding)
			}
			return
		}
	}
	t.Fatal("global shortcut is missing")
}

func TestOpenShortcutSurvivesDisablingEveryModule(t *testing.T) {
	states, err := modulestate.Open(filepath.Join(t.TempDir(), "modules.json"))
	if err != nil {
		t.Fatal(err)
	}
	for id := range states.Snapshot().Modules {
		if _, err := states.SetIntent(states.Snapshot().Revision, id, false); err != nil {
			t.Fatal(err)
		}
	}
	runtime := &shortcutRuntime{registrar: &recordingRegistrar{}, moduleState: states}
	if err := runtime.applyConfiguration(settings.Settings{CaptureShortcut: "Shift+A", PinShortcut: "Alt+Q"}); err != nil {
		t.Fatal(err)
	}
	active := []string{}
	for _, status := range runtime.statuses {
		if status.Registered {
			active = append(active, status.ID+":"+status.Shortcut)
		}
	}
	if len(active) != 1 || active[0] != "open-winone:Alt+A" {
		t.Fatalf("unexpected application bindings: %v", active)
	}
}
