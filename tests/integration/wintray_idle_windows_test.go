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
	"winone/modules/wintray/preferences"
)

func TestEmptyWinTraySleepsAcrossReadsAndWakesForConfiguration(t *testing.T) {
	exe := os.Getenv("WINONE_CONFIG_HOST_EXE")
	if exe == "" {
		t.Skip("set WINONE_CONFIG_HOST_EXE")
	}
	directory := t.TempDir()
	pipe := fmt.Sprintf(`\\.\pipe\WinOne-idle-%d`, time.Now().UnixNano())
	cmd := exec.Command(exe, "--pipe", pipe, "--data-dir", directory)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	call := func(method string, params any, out any) error {
		data, _ := json.Marshal(params)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		response, err := transport.CallCompleting(ctx, pipe, host.Request{Version: 1, ID: fmt.Sprint(time.Now().UnixNano()), Method: method, Params: data}, 5*time.Second)
		if err != nil {
			return err
		}
		if response.Error != nil {
			return fmt.Errorf("%s", response.Error.Message)
		}
		return json.Unmarshal(response.Result, out)
	}
	var current struct {
		Revision string               `json:"revision"`
		Settings preferences.Settings `json:"settings"`
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		if call("wintray.settings.get", nil, &current) == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Host not ready")
		}
		time.Sleep(50 * time.Millisecond)
	}
	time.Sleep(6 * time.Second)
	for range 3 {
		var snapshot struct {
			ProcessID            uint32 `json:"processId"`
			InitializationState  string `json:"initializationState"`
			InitializationError  string `json:"initializationError"`
			BackgroundError      string `json:"backgroundError"`
			RecoveryError        string `json:"recoveryError"`
			ConfigurationManaged bool   `json:"configurationManaged"`
		}
		if err := call("wintray.snapshot", nil, &snapshot); err != nil {
			t.Fatal(err)
		}
		if snapshot.ProcessID != 0 {
			t.Fatal("snapshot kept or restarted an empty worker", snapshot)
		}
		var residency any
		if err := call("residency.snapshot", nil, &residency); err != nil {
			t.Fatal(err)
		}
	}
	script := filepath.Join(directory, "not-launched.cmd")
	if err := os.WriteFile(script, []byte("@echo unexpected>\"%~dp0launched.txt\"\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal([]map[string]any{{"id": "script", "name": "Test script", "exePath": script, "args": "", "runOnStartup": true, "launchHiddenInBackground": true}})
	if err := json.Unmarshal(data, &current.Settings.ManagedApps); err != nil {
		t.Fatal(err)
	}
	var result any
	if err := call("wintray.settings.save", current, &result); err != nil {
		t.Fatal(err)
	}
	var awake struct {
		ProcessID uint32 `json:"processId"`
	}
	if err := call("wintray.snapshot", nil, &awake); err != nil {
		t.Fatal(err)
	}
	if awake.ProcessID == 0 {
		t.Fatal("configuration did not activate its native runtime")
	}
	if _, err := os.Stat(filepath.Join(directory, "launched.txt")); !os.IsNotExist(err) {
		t.Fatal("saving configuration launched a program")
	}
}
