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

func TestManagedWorkerReloadDoesNotLaunchPrograms(t *testing.T) {
	exe := os.Getenv("WINONE_WINTRAY_EXE")
	if exe == "" {
		t.Skip("set WINONE_WINTRAY_EXE")
	}
	directory := t.TempDir()
	projection := filepath.Join(directory, "managed-settings.json")
	value := config.DefaultSettings()
	value.RunAtLogon = false
	writeManaged(t, projection, 7, value)
	pipe := fmt.Sprintf(`\\.\pipe\WinOne-WinTray-managed-%d`, time.Now().UnixNano())
	cmd := exec.Command(exe, "--pipe", pipe, "--data-dir", directory)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	call := func(method string, params any, out any) error {
		data, _ := json.Marshal(params)
		response, err := transport.Call(context.Background(), pipe, host.Request{Version: 1, ID: "managed-test", Method: method, Params: data})
		if err != nil {
			return err
		}
		if response.Error != nil {
			return fmt.Errorf("%s", response.Error.Message)
		}
		return json.Unmarshal(response.Result, out)
	}
	var current Snapshot
	deadline := time.Now().Add(10 * time.Second)
	for {
		if err := call("snapshot", nil, &current); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("worker did not start")
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !current.ConfigurationManaged || current.Revision != "7" {
		t.Fatal(current)
	}
	awaitJob := func(operation job) job {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for operation.State == "running" {
			if time.Now().After(deadline) {
				t.Fatal("job did not finish")
			}
			time.Sleep(50 * time.Millisecond)
			if err := call("jobs.get", map[string]string{"id": operation.ID}, &operation); err != nil {
				t.Fatal(err)
			}
		}
		return operation
	}
	var operation job
	if err := call("settings.save", map[string]any{"revision": current.Revision, "settings": value}, &operation); err != nil {
		t.Fatal(err)
	}
	if result := awaitJob(operation); result.State != "failed" {
		t.Fatal("direct save accepted", result)
	}
	script := filepath.Join(directory, "must-not-run.cmd")
	if err := os.WriteFile(script, []byte("@echo started>\"%~dp0unexpected.txt\"\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	value.ManagedApps = []config.ManagedAppEntry{{ID: "script", Name: "Test script", ExePath: script, RunOnStartup: true, LaunchHiddenInBackground: true}}
	value.StartupIntervalSeconds = 11
	writeManaged(t, projection, 8, value)
	if err := call("settings.reload", nil, &operation); err != nil {
		t.Fatal(err)
	}
	if result := awaitJob(operation); result.State != "completed" {
		t.Fatal(result)
	}
	if err := call("snapshot", nil, &current); err != nil {
		t.Fatal(err)
	}
	if current.Revision != "8" || current.Settings.StartupIntervalSeconds != 11 {
		t.Fatal(current)
	}
	if _, err := os.Stat(filepath.Join(directory, "settings.json")); !os.IsNotExist(err) {
		t.Fatal("legacy settings written")
	}
	if _, err := os.Stat(filepath.Join(directory, "unexpected.txt")); !os.IsNotExist(err) {
		t.Fatal("reload launched program")
	}
}
