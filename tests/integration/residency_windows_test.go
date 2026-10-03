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
	"winone/internal/residency"
	"winone/internal/transport"
)

// This profile has no real login tasks, program launches, or active shortcuts.
func TestLoginHostWaitsForDesktopThenExits(t *testing.T) {
	exe := os.Getenv("WINONE_RESIDENCY_HOST_EXE")
	if exe == "" {
		t.Skip("set WINONE_RESIDENCY_HOST_EXE with sibling WinTray worker")
	}
	dir := t.TempDir()
	moduleDir := filepath.Join(dir, "modules", "wintray")
	if err := os.MkdirAll(moduleDir, 0700); err != nil {
		t.Fatal(err)
	}
	for path, data := range map[string]string{
		filepath.Join(dir, "settings.json"):       `{"schemaVersion":1,"theme":"system","captureShortcut":"","pinShortcut":""}`,
		filepath.Join(moduleDir, "settings.json"): `{"schemaVersion":3,"language":"zh-CN","runAtLogon":false,"exitAfterManagedAppsCompleted":true,"managedApps":[]}`,
	} {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	pipe := fmt.Sprintf(`\\.\pipe\WinOne-residency-%d`, time.Now().UnixNano())
	release, err := residency.MarkWindow(pipe)
	if err != nil {
		t.Fatal(err)
	}
	released := false
	defer func() {
		if !released {
			release()
		}
	}()
	cmd := exec.Command(exe, "--autorun", "--pipe", pipe, "--data-dir", dir)
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	waited := false
	defer func() {
		if !waited {
			cmd.Process.Kill()
			<-done
		}
	}()
	deadline := time.Now().Add(10 * time.Second)
	for {
		response, err := transport.Call(context.Background(), pipe, host.Request{Version: 1, ID: "lifetime-test", Method: "residency.snapshot"})
		var state residency.State
		if err == nil && response.Error == nil && json.Unmarshal(response.Result, &state) == nil && state.Requested {
			found := false
			for _, reason := range state.Reasons {
				if reason == "设置窗口仍在使用" {
					found = true
				}
			}
			if !found {
				t.Fatalf("desktop blocker absent: %+v", state)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("residency unavailable: %+v %v", response, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
	select {
	case err := <-done:
		waited = true
		t.Fatalf("Host exited with desktop open: %v", err)
	case <-time.After(6 * time.Second):
	}
	release()
	released = true
	select {
	case err := <-done:
		waited = true
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Host did not exit after blockers cleared")
	}
}
