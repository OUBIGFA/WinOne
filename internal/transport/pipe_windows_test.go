//go:build windows

package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"
	"winone/internal/host"
	"winone/internal/settings"
)

// Only the explicit lifetime-bound caller omits a total timeout. Its own
// cancellation still closes the real local pipe while the handler waits.
func TestZeroTimeoutPipeRetainsCallerCancellation(t *testing.T) {
	name := fmt.Sprintf(`\\.\pipe\WinOne-lifetime-%d`, time.Now().UnixNano())
	listener, err := Listen(name)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	serveCtx, stop := context.WithCancel(t.Context())
	defer stop()
	entered := make(chan struct{})
	go Serve(serveCtx, listener, pipeTestHandler(func(r host.Request) host.Response {
		close(entered)
		<-serveCtx.Done()
		return host.Response{ID: r.ID, Result: json.RawMessage(`{}`)}
	}))
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	finished := make(chan error, 1)
	go func() {
		_, err := CallWithVerifiedServer(ctx, name, host.Request{Version: 1, ID: "lifetime", Method: "capture.ai.accounts.warm-all"}, 0, nil)
		finished <- err
	}()
	select {
	case <-entered:
	case err := <-finished:
		t.Fatal("lifetime request ended before admission", err)
	case <-time.After(3 * time.Second):
		t.Fatal("lifetime request was not admitted")
	}
	cancel()
	select {
	case err := <-finished:
		if err == nil || !errors.Is(ctx.Err(), context.Canceled) {
			t.Fatal("caller cancellation was lost", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled lifetime pipe stayed open")
	}
}

// The edit baseline must fit together with a large draft through the real
// Host connection; framing unit tests alone cannot cover both pipe endpoints.
func TestLargeConfigurationEditCrossesHostPipe(t *testing.T) {
	name := fmt.Sprintf(`\\.\pipe\WinOne-large-edit-%d`, time.Now().UnixNano())
	listener, err := Listen(name)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go Serve(ctx, listener, pipeTestHandler(func(request host.Request) host.Response {
		return host.Response{ID: request.ID, Result: request.Params}
	}))
	payload, err := json.Marshal(map[string]string{"base": string(bytes.Repeat([]byte("a"), 700<<10)), "configuration": string(bytes.Repeat([]byte("b"), 700<<10))})
	if err != nil {
		t.Fatal(err)
	}
	response, err := CallWithTimeout(ctx, name, host.Request{Version: 1, ID: "large-save", Method: "capture.ai.configuration.save", Params: payload}, 5*time.Second)
	if err != nil || response.Error != nil || !bytes.Equal(payload, response.Result) {
		t.Fatal("large configuration baseline was lost", err, response.Error)
	}
}

type pipeTestHandler func(host.Request) host.Response

func (handler pipeTestHandler) Handle(request host.Request) host.Response { return handler(request) }

// A cold capture worker may finish after the idle-client deadline. The real
// pipe must keep the admitted operation alive until its response is returned.
func TestCaptureResponseSurvivesIdlePipeDeadline(t *testing.T) {
	name := fmt.Sprintf(`\\.\pipe\WinOne-capture-test-%d`, time.Now().UnixNano())
	listener, err := Listen(name)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go Serve(ctx, listener, pipeTestHandler(func(request host.Request) host.Response {
		time.Sleep(5100 * time.Millisecond)
		return host.Response{ID: request.ID, Result: json.RawMessage(`{"models":["fixture-model"]}`)}
	}))
	response, err := CallWithTimeout(ctx, name, host.Request{Version: 1, ID: "capture", Method: "capture.ai.models.fetch"}, 10*time.Second)
	if err != nil || string(response.Result) != `{"models":["fixture-model"]}` {
		t.Fatalf("completed capture response lost on pipe: %+v, %v", response, err)
	}
}

// Uses a real isolated Windows pipe; no user configuration or shell state is touched.
func TestPipeConnectsAndRejectsSecondListener(t *testing.T) {
	name := fmt.Sprintf(`\\.\pipe\WinOne-test-%d`, time.Now().UnixNano())
	store, _ := settings.Open(filepath.Join(t.TempDir(), "settings.json"))
	listener, err := Listen(name)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if duplicate, err := Listen(name); err == nil {
		duplicate.Close()
		t.Fatal("second Host accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go Serve(ctx, listener, host.New(store))
	response, err := Call(ctx, name, host.Request{Version: 1, ID: "test", Method: "snapshot"})
	if err != nil || response.Error != nil || response.ID != "test" {
		t.Fatalf("response %+v, %v", response, err)
	}
}

func TestConfigurationClientWaitsForFinalResponse(t *testing.T) {
	name := fmt.Sprintf(`\\.\pipe\WinOne-config-test-%d`, time.Now().UnixNano())
	listener, err := Listen(name)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store, _ := settings.Open(filepath.Join(t.TempDir(), "settings.json"))
	service := host.New(store)
	entered, release := make(chan struct{}), make(chan struct{})
	service.OnSettingsChanged(func(settings.Settings) { close(entered); <-release })
	go Serve(ctx, listener, host.NewConfigurationQueue(service))
	done := make(chan host.Response, 1)
	errors := make(chan error, 1)
	go func() {
		response, err := CallCompleting(ctx, name, host.Request{Version: 1, ID: "save", Method: "settings.save", Params: json.RawMessage(`{"schemaVersion":1,"revision":0,"theme":"dark"}`)}, time.Second)
		if err != nil {
			errors <- err
			return
		}
		done <- response
	}()
	select {
	case <-entered:
	case err := <-errors:
		t.Fatal(err)
	case <-time.After(3 * time.Second):
		t.Fatal("save did not start")
	}
	select {
	case response := <-done:
		close(release)
		t.Fatal("returned before completion", response)
	default:
	}
	close(release)
	select {
	case response := <-done:
		var saved settings.Settings
		if err = json.Unmarshal(response.Result, &saved); err != nil || saved.Revision != 1 || saved.Theme != "dark" {
			t.Fatal(response, err)
		}
	case err := <-errors:
		t.Fatal(err)
	case <-time.After(3 * time.Second):
		t.Fatal("save did not return")
	}
}

func TestCallCancellationClosesConnectedPipe(t *testing.T) {
	name := fmt.Sprintf(`\\.\pipe\WinOne-cancel-test-%d`, time.Now().UnixNano())
	listener, err := Listen(name)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := Call(ctx, name, host.Request{Version: 1, ID: "cancel", Method: "snapshot"})
		done <- err
	}()
	conn, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled call succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled connection did not close")
	}
}
