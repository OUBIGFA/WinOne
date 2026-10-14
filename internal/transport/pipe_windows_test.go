//go:build windows

package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"
	"winone/internal/host"
	"winone/internal/settings"
)

type pipeTestHandler func(host.Request) host.Response

func (handler pipeTestHandler) Handle(request host.Request) host.Response { return handler(request) }

// A title may finish after the idle-client deadline. The real pipe must return
// its single completed result instead of disconnecting after the title is saved.
func TestChatTitleResponseSurvivesIdlePipeDeadline(t *testing.T) {
	name := fmt.Sprintf(`\\.\pipe\WinOne-title-test-%d`, time.Now().UnixNano())
	listener, err := Listen(name)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go Serve(ctx, listener, pipeTestHandler(func(request host.Request) host.Response {
		time.Sleep(5100 * time.Millisecond)
		return host.Response{ID: request.ID, Result: json.RawMessage(`{"title":"已生成的中文标题"}`)}
	}))
	response, err := CallWithTimeout(ctx, name, host.Request{Version: 1, ID: "title", Method: "chat.title"}, 10*time.Second)
	if err != nil || string(response.Result) != `{"title":"已生成的中文标题"}` {
		t.Fatalf("completed title lost on pipe: %+v, %v", response, err)
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
