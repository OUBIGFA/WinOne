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
	"winone/internal/modules"
	"winone/internal/transport"
	"winone/modules/wintray/preferences"
)

// Explicit complete-bundle test; only an isolated profile and test-owned script
// are used. The profile cannot register real Windows logon triggers.
func TestWinTrayResetPersistsAcrossRealHostRestart(t *testing.T) {
	exe := os.Getenv("WINONE_MODULE_HOST_EXE")
	if exe == "" {
		t.Skip("set WINONE_MODULE_HOST_EXE to a complete test bundle")
	}
	directory := t.TempDir()
	pipe := fmt.Sprintf(`\\.\pipe\WinOne-reset-%d`, time.Now().UnixNano())
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	var command *exec.Cmd
	stop := func() {
		if command != nil {
			_ = command.Process.Kill()
			_ = command.Wait()
			command = nil
		}
	}
	defer stop()
	call := func(method string, params any, target any) error {
		data, _ := json.Marshal(params)
		response, err := transport.CallCompleting(ctx, pipe, host.Request{Version: 1, ID: "reset-test", Method: method, Params: data}, 5*time.Second)
		if err != nil {
			return err
		}
		if response.Error != nil {
			return fmt.Errorf("%s", response.Error.Message)
		}
		return json.Unmarshal(response.Result, target)
	}
	start := func() {
		command = exec.Command(exe, "--pipe", pipe, "--data-dir", directory)
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(12 * time.Second)
		for {
			var snapshot modules.ControlSnapshot
			if call("modules.snapshot", nil, &snapshot) == nil {
				return
			}
			if time.Now().After(deadline) {
				t.Fatal("Host unavailable")
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	start()
	var config struct {
		Revision string               `json:"revision"`
		Settings preferences.Settings `json:"settings"`
	}
	if err := call("wintray.settings.get", nil, &config); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(directory, "never-run.cmd")
	if err := os.WriteFile(script, []byte("@echo unexpected>\"%~dp0unexpected.txt\"\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	entry, _ := json.Marshal([]map[string]any{{"id": "owned", "name": "test program", "exePath": script, "launchHiddenInBackground": true, "runOnStartup": true}})
	if err := json.Unmarshal(entry, &config.Settings.ManagedApps); err != nil {
		t.Fatal(err)
	}
	config.Settings.RunAtLogon = false
	history := map[string]any{"version": 1, "apps": map[string][]time.Time{script: {time.Now().UTC()}}}
	var job modules.ControlJob
	if err := call("wintray.settings.save", map[string]any{"revision": config.Revision, "settings": config.Settings, "history": history}, &job); err != nil || job.State != "completed" {
		t.Fatal("save", job, err)
	}
	historyPath := filepath.Join(directory, "modules", "wintray", "startup-history.json")
	before, err := os.ReadFile(historyPath)
	if err != nil {
		t.Fatal(err)
	}
	var preview struct {
		ConfigurationRevision uint64 `json:"configurationRevision"`
		ModuleRevision        uint64 `json:"moduleRevision"`
		ProgramCount          int    `json:"programCount"`
	}
	if err = call("modules.resetWinTray.preview", nil, &preview); err != nil || preview.ProgramCount != 1 {
		t.Fatal("preview", preview, err)
	}
	if err = call("modules.resetWinTray", map[string]any{"configurationRevision": preview.ConfigurationRevision, "moduleRevision": preview.ModuleRevision, "confirmed": true}, &job); err != nil || job.State != "completed" {
		t.Fatal("reset", job, err)
	}
	check := func() {
		var snapshot modules.ControlSnapshot
		if err := call("modules.snapshot", nil, &snapshot); err != nil {
			t.Fatal(err)
		}
		for _, module := range snapshot.Modules {
			if module.Manifest.ID == "wintray" {
				if module.Enabled || module.PendingStop || module.Runtime.State != "disabled" {
					t.Fatal("reset module remained active", module)
				}
			} else if !module.Enabled {
				t.Fatal("other module disabled", module)
			}
		}
		if err := call("wintray.settings.get", nil, &config); err != nil || len(config.Settings.ManagedApps) != 0 {
			t.Fatal("program list returned", err)
		}
		after, err := os.ReadFile(historyPath)
		if err != nil || string(after) != string(before) {
			t.Fatal("launch history changed", err)
		}
		if _, err = os.Stat(filepath.Join(directory, "unexpected.txt")); !os.IsNotExist(err) {
			t.Fatal("maintenance launched a program")
		}
	}
	check()
	stop()
	start()
	check()
}
