//go:build windows

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"winone/internal/host"
	"winone/internal/transport"
)

// TestHostProcess exercises the packaged executable, not an in-process service substitute.
func TestHostProcess(t *testing.T) {
	exe := os.Getenv("WINONE_HOST_EXE")
	if exe == "" {
		t.Skip("set WINONE_HOST_EXE to run the built Host")
	}
	dir := t.TempDir()
	pipe := fmt.Sprintf(`\\.\pipe\WinOne-process-test-%d`, time.Now().UnixNano())
	cmd := exec.Command(exe, "--headless", "--pipe", pipe, "--data-dir", dir)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	call := func(method string, params json.RawMessage) (host.Response, error) {
		return transport.Call(context.Background(), pipe, host.Request{Version: 1, ID: "smoke", Method: method, Params: params})
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := call("snapshot", nil); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Host did not become ready")
		}
		time.Sleep(50 * time.Millisecond)
	}
	duplicate := exec.Command(exe, "--headless", "--pipe", pipe, "--data-dir", dir)
	if err := duplicate.Run(); err == nil {
		t.Fatal("duplicate Host started")
	}
	// Independent short-lived clients must not own the Host lifetime.
	for i := 0; i < 30; i++ {
		r, err := call("snapshot", nil)
		if err != nil || r.Error != nil {
			t.Fatalf("client %d: %+v %v", i, r, err)
		}
	}
	r, err := call("settings.save", json.RawMessage(`{"schemaVersion":1,"revision":0,"theme":"dark"}`))
	if err != nil || r.Error != nil {
		t.Fatalf("save: %+v %v", r, err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved struct {
		Theme    string `json:"theme"`
		Revision int    `json:"revision"`
	}
	if err = json.Unmarshal(data, &saved); err != nil || saved.Theme != "dark" || saved.Revision != 1 {
		t.Fatalf("stored %s", data)
	}
	r, err = call("settings.save", json.RawMessage(`{"schemaVersion":1,"revision":0,"theme":"light"}`))
	if err != nil || r.Error == nil || r.Error.Code != "conflict" {
		t.Fatalf("stale update: %+v %v", r, err)
	}
}
