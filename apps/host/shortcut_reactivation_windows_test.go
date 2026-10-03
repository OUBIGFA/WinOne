//go:build windows

package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"winone/internal/host"
	"winone/internal/hotkeys"
	"winone/internal/settings"
)

type activationRegistrar struct{ events chan string }

func (r *activationRegistrar) Update([]hotkeys.Binding) ([]hotkeys.Status, error) { return nil, nil }
func (r *activationRegistrar) Events() <-chan string                              { return r.events }
func (r *activationRegistrar) Close() error                                       { return nil }

func TestRepeatedShortcutActivatesWithoutQueuingAnotherCapture(t *testing.T) {
	store, err := settings.Open(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	service := host.New(store)
	started, activated, completed := make(chan struct{}), make(chan struct{}), make(chan struct{})
	release, finish := make(chan struct{}), make(chan struct{})
	var releaseOnce, completedOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	var starts, activations atomic.Int32
	service.RegisterModule("capture", func() host.Module { return host.Module{} }, func(req host.Request) host.Response {
		switch req.Method {
		case "draft.get":
			return host.Response{ID: req.ID, Result: json.RawMessage(`{"draft":null}`)}
		case "capture.unified":
			if starts.Add(1) == 1 {
				close(started)
			}
			return host.Response{ID: req.ID, Result: json.RawMessage(`{"id":"one","state":"running"}`)}
		case "capture.activate":
			if activations.Add(1) == 1 {
				close(activated)
			}
			<-release
			return host.Response{ID: req.ID, Result: json.RawMessage(`{"active":false}`)}
		case "jobs.get":
			select {
			case <-finish:
				completedOnce.Do(func() { close(completed) })
				return host.Response{ID: req.ID, Result: json.RawMessage(`{"id":"one","state":"completed","result":{"copied":true}}`)}
			default:
				return host.Response{ID: req.ID, Result: json.RawMessage(`{"id":"one","state":"running"}`)}
			}
		}
		return host.Response{ID: req.ID, Error: &host.RPCError{Code: "unexpected", Message: req.Method}}
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	registrar := &activationRegistrar{events: make(chan string)}
	runtime := &shortcutRuntime{registrar: registrar}
	stopped := make(chan struct{})
	go func() { defer close(stopped); runtime.run(ctx, service) }()
	send := func(id string) {
		t.Helper()
		select {
		case registrar.events <- id:
		case <-time.After(3 * time.Second):
			t.Fatal("shortcut loop blocked")
		}
	}
	wait := func(signal <-chan struct{}) {
		t.Helper()
		select {
		case <-signal:
		case <-time.After(3 * time.Second):
			t.Fatal("shortcut did not reach expected stage")
		}
	}
	send("capture")
	wait(started)
	send("capture")
	wait(activated)
	for range 8 {
		send("capture")
	}
	send("pin") // Other actions keep their existing no-queue behavior.
	close(finish)
	wait(completed)
	releaseOnce.Do(func() { close(release) })
	cancel()
	wait(stopped)
	if starts.Load() != 1 || activations.Load() != 1 {
		t.Fatalf("starts=%d activations=%d", starts.Load(), activations.Load())
	}
}
