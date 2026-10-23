//go:build windows

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
	"winone/internal/configuration"
	"winone/internal/host"
	"winone/internal/modules"
	"winone/internal/modulestate"
	"winone/modules/stroke"
)

func TestStrokeRecoversExitedListenerAndRespectsMaintenanceAndDisable(t *testing.T) {
	exe := os.Getenv("WINONE_STROKE_TEST_EXE")
	if exe == "" {
		t.Skip("set WINONE_STROKE_TEST_EXE")
	}
	directory := t.TempDir()
	worker := modules.NewLazy(context.Background(), "stroke", "鼠标手势", exe, fmt.Sprintf(`\\.\pipe\WinOne-Stroke-Recovery-%d-%d`, os.Getpid(), time.Now().UnixNano()), filepath.Join(directory, "modules", "stroke"), "--fixture")
	defer worker.Close()
	if _, err := configuration.OpenHost(directory, strokeConfiguration(directory, func() *modules.Lazy { return worker })); err != nil {
		t.Fatal(err)
	}
	state, err := modulestate.Open(filepath.Join(directory, "module-state.json"))
	if err != nil {
		t.Fatal(err)
	}
	changed := make(chan struct{}, 1)
	gate, err := modules.NewGate("stroke", "鼠标手势", state, modules.LifecycleHooks{
		Start: func() error { return startStroke(worker) }, Stop: worker.Close, IntentChanged: func() error {
			select {
			case changed <- struct{}{}:
			default:
			}
			return nil
		},
		Status: func() host.Module { return strokeStatus(worker) }, Handle: worker.Handle,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = gate.SetEnabled(state.Snapshot().Revision, true); err != nil {
		t.Fatal(err)
	}
	var before struct{ ProcessID int }
	if err = json.Unmarshal(worker.Handle(host.Request{Version: 1, ID: "before", Method: "snapshot"}).Result, &before); err != nil {
		t.Fatal(err)
	}
	queue := host.NewConfigurationQueue(nil)
	release, ok := queue.AcquireMaintenance()
	if !ok {
		t.Fatal("maintenance was not admitted")
	}
	defer release()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		superviseStroke(ctx, queue, state, gate, worker, directory, func() bool { return true }, changed)
	}()
	defer func() { cancel(); <-done }()
	// Kill only the PID returned by our own fixture. A real listener can also
	// exit independently of Host; no saved gesture/action should be replayed.
	process, err := os.FindProcess(before.ProcessID)
	if err != nil {
		t.Fatal(err)
	}
	if err = process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = process.Release()
	time.Sleep(1200 * time.Millisecond)
	if worker.Running() {
		t.Fatal("listener restarted during maintenance")
	}
	revision := state.Snapshot().Revision
	release()
	deadline := time.Now().Add(5 * time.Second)
	for gate.Status().State != "ready" && time.Now().Before(deadline) {
		time.Sleep(25 * time.Millisecond)
	}
	if !worker.Running() || gate.Status().State != "ready" {
		t.Fatalf("enabled listener did not recover: %+v", gate.Status())
	}
	if got := strokeStatus(worker); got.State != "ready" {
		t.Fatalf("listener did not reactivate: %+v", got)
	}
	if state.Snapshot().Revision != revision {
		t.Fatal("runtime recovery rewrote saved module intent")
	}
	if err = gate.SetEnabled(state.Snapshot().Revision, false); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1200 * time.Millisecond)
	if worker.Running() || gate.Status().State != "disabled" {
		t.Fatal("disabled listener restarted")
	}
}

func TestStrokeStatusDoesNotCallAnExitedEngineAvailable(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	worker := modules.NewLazy(context.Background(), "stroke", "鼠标手势", exe, "unused", t.TempDir())
	if status := strokeStatus(worker); status.State != "faulted" {
		t.Fatalf("enabled gesture engine has no process, got %+v", status)
	}
	if worker.Running() {
		t.Fatal("reading status started an engine")
	}
}

func TestStrokeSettingsPersistWithoutStartingDisabledWorker(t *testing.T) {
	directory := t.TempDir()
	worker := modules.NewLazy(context.Background(), "stroke", "鼠标手势", filepath.Join(directory, "missing.exe"), "unused", filepath.Join(directory, "modules", "stroke"))
	adapter := strokeConfiguration(directory, func() *modules.Lazy { return worker })
	store, err := configuration.OpenHost(directory, adapter)
	if err != nil {
		t.Fatal(err)
	}
	handle := strokeConfigurationHandler(store, worker.Handle)
	get := handle(host.Request{ID: "get", Method: "settings.get"})
	if get.Error != nil {
		t.Fatal(get.Error)
	}
	var p stroke.Preferences
	_ = json.Unmarshal(get.Result, &p)
	p.Scopes[1].Enabled = true
	p.Gestures[0].Name = "Custom recorded gesture"
	raw, _ := json.Marshal(p)
	saved := handle(host.Request{ID: "save", Method: "settings.save", Params: raw})
	if saved.Error != nil {
		t.Fatal(saved.Error)
	}
	if worker.HasStarted() {
		t.Fatal("preference edit started disabled worker")
	}
	reopened, err := configuration.OpenHost(directory, adapter)
	if err != nil {
		t.Fatal(err)
	}
	stored, _, err := reopened.Module("stroke")
	if err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(stored, &p)
	if !p.Scopes[1].Enabled || p.Gestures[0].Name != "Custom recorded gesture" {
		t.Fatal("reopen reset user scopes or gestures")
	}
	if stale := handle(host.Request{ID: "stale", Method: "settings.save", Params: raw}); stale.Error == nil {
		t.Fatal("stale settings overwrote a newer revision")
	}
}

func TestStrokeHostRollsBackFailedScriptCompilation(t *testing.T) {
	exe := os.Getenv("WINONE_STROKE_TEST_EXE")
	if exe == "" {
		t.Skip("set WINONE_STROKE_TEST_EXE")
	}
	directory := t.TempDir()
	pipe := fmt.Sprintf(`\\.\pipe\WinOne-Stroke-Host-Fixture-%d-%d`, os.Getpid(), time.Now().UnixNano())
	worker := modules.NewLazy(context.Background(), "stroke", "鼠标手势", exe, pipe, filepath.Join(directory, "modules", "stroke"), "--fixture")
	defer worker.Close()
	store, err := configuration.OpenHost(directory, strokeConfiguration(directory, func() *modules.Lazy { return worker }))
	if err != nil {
		t.Fatal(err)
	}
	if err = startStroke(worker); err != nil {
		t.Fatal(err)
	}
	raw, revision, err := store.Module("stroke")
	if err != nil {
		t.Fatal(err)
	}
	var p stroke.Preferences
	_ = json.Unmarshal(raw, &p)
	original := p.ActionPackages[0].Actions[0].Code
	p.Revision = revision
	p.ActionPackages[0].Actions[0].Code = "This is invalid C#;"
	bad, _ := json.Marshal(p)
	handle := strokeConfigurationHandler(store, worker.Handle)
	response := handle(host.Request{ID: "save", Method: "settings.save", Params: bad})
	if response.Error == nil {
		t.Fatal("invalid script was accepted")
	}
	current, _, err := store.Module("stroke")
	if err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(current, &p)
	if p.ActionPackages[0].Actions[0].Code != original {
		t.Fatal("failed compilation changed committed settings")
	}
	var state struct{ Running, MouseHook bool }
	snapshot := worker.Handle(host.Request{Version: 1, ID: "state", Method: "snapshot"})
	if snapshot.Error != nil {
		t.Fatal(snapshot.Error)
	}
	_ = json.Unmarshal(snapshot.Result, &state)
	if !state.Running || state.MouseHook {
		t.Fatal("rollback did not restore the isolated running state", state)
	}
	if err = worker.CloseConfirmed(); err != nil {
		t.Fatal(err)
	}
}
