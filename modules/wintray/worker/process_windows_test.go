//go:build windows

package worker

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
	"winone/modules/wintray/internal/config"
)

// This integration test only starts a test-owned script in an isolated data profile.
func TestWorkerPersistsAndRunsScript(t *testing.T) {
	exe := os.Getenv("WINONE_WINTRAY_EXE")
	if exe == "" {
		t.Skip("set WINONE_WINTRAY_EXE for process integration")
	}
	dir := t.TempDir()
	pipe := fmt.Sprintf(`\\.\pipe\WinOne-WinTray-test-%d`, time.Now().UnixNano())
	cmd := exec.Command(exe, "--pipe", pipe, "--data-dir", dir)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	var exitError error
	go func() { exitError = cmd.Wait(); close(done) }()
	t.Cleanup(func() {
		select {
		case <-done:
			return
		default:
			_ = cmd.Process.Kill()
			<-done
		}
	})
	call := func(method string, params any, out any) error {
		data, _ := json.Marshal(params)
		r, err := transport.Call(context.Background(), pipe, host.Request{Version: 1, ID: "test", Method: method, Params: data})
		if err != nil {
			return err
		}
		if r.Error != nil {
			return fmt.Errorf("%s", r.Error.Message)
		}
		return json.Unmarshal(r.Result, out)
	}
	var current Snapshot
	deadline := time.Now().Add(10 * time.Second)
	for {
		if err := call("snapshot", nil, &current); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Worker did not start")
		}
		time.Sleep(50 * time.Millisecond)
	}
	if current.Settings.RunAtLogon {
		t.Fatal("fresh worker enabled real startup")
	}
	script := filepath.Join(dir, "test script.cmd")
	output := filepath.Join(dir, "output value.txt")
	if err := os.WriteFile(script, []byte("@echo worker> \"%~dp0output value.txt\"\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	next := current.Settings
	next.ManagedApps = []config.ManagedAppEntry{{ID: "test", Name: "test script", ExePath: script, RunOnStartup: true, LaunchHiddenInBackground: true}}
	waitJob := func(j job) {
		t.Helper()
		limit := time.Now().Add(10 * time.Second)
		for j.State == "running" && time.Now().Before(limit) {
			time.Sleep(50 * time.Millisecond)
			if err := call("jobs.get", map[string]string{"id": j.ID}, &j); err != nil {
				t.Fatal(err)
			}
		}
		if j.State != "completed" {
			t.Fatalf("job failed: %+v", j)
		}
	}
	var j job
	if err := call("settings.save", map[string]any{"revision": current.Revision, "settings": next}, &j); err != nil {
		t.Fatal(err)
	}
	waitJob(j)
	if err := call("launch", map[string]string{"id": "test"}, &j); err != nil {
		t.Fatal(err)
	}
	waitJob(j)
	deadline = time.Now().Add(3 * time.Second)
	for {
		if data, err := os.ReadFile(output); err == nil {
			if string(data) != "worker\r\n" {
				t.Fatalf("output %q", data)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("script did not produce output")
		}
		time.Sleep(25 * time.Millisecond)
	}
	var stopping map[string]bool
	if err := call("shutdown", nil, &stopping); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
		if exitError != nil {
			t.Fatal(exitError)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Worker did not restore and exit")
	}
}
