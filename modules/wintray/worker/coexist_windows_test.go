//go:build windows

package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/windows"
	"winone/internal/host"
	"winone/internal/transport"
	"winone/modules/wintray/internal/config"
)

// Hold the independent product's presence signal without acquiring ownership or
// interacting with any running WinTray. Every worker uses a private profile.
func TestWorkerCoexistsWithIndependentWinTray(t *testing.T) {
	exe := os.Getenv("WINONE_WINTRAY_EXE")
	if exe == "" {
		t.Skip("set WINONE_WINTRAY_EXE")
	}
	key, _ := windows.UTF16PtrFromString("WinTray_SingleInstance")
	mutex, err := windows.CreateMutex(nil, false, key)
	if err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		t.Fatal(err)
	}
	defer windows.CloseHandle(mutex)
	for _, recovery := range []bool{false, true} {
		t.Run(fmt.Sprintf("recovery=%t", recovery), func(t *testing.T) {
			directory := t.TempDir()
			pipe := fmt.Sprintf(`\\.\pipe\WinOne-WinTray-coexist-%d`, time.Now().UnixNano())
			args := []string{"--pipe", pipe, "--data-dir", directory}
			if recovery {
				args = append(args, "--recover-only")
			}
			cmd := exec.Command(exe, args...)
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
			call := func(method string, params, out any) error {
				data, _ := json.Marshal(params)
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				response, err := transport.Call(ctx, pipe, host.Request{Version: 1, ID: "coexist", Method: method, Params: data})
				if err != nil {
					return err
				}
				if response.Error != nil {
					return errors.New(response.Error.Message)
				}
				return json.Unmarshal(response.Result, out)
			}
			var current Snapshot
			deadline := time.Now().Add(10 * time.Second)
			for {
				err := call("snapshot", nil, &current)
				if err == nil && current.InitializationState != "starting" {
					if current.InitializationState != "ready" {
						t.Fatalf("independent WinTray blocked worker: %+v", current)
					}
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("worker did not become ready", err)
				}
				time.Sleep(30 * time.Millisecond)
			}
			awaitJob := func(method string, params any) {
				t.Helper()
				var operation job
				if err := call(method, params, &operation); err != nil {
					t.Fatal(method, err)
				}
				deadline := time.Now().Add(10 * time.Second)
				for operation.State == "running" && time.Now().Before(deadline) {
					time.Sleep(30 * time.Millisecond)
					if err := call("jobs.get", map[string]string{"id": operation.ID}, &operation); err != nil {
						t.Fatal(err)
					}
				}
				if operation.State != "completed" {
					t.Fatal(method, operation)
				}
			}
			if !recovery {
				script := filepath.Join(directory, "coexist script.cmd")
				if err := os.WriteFile(script, []byte("@echo coexist>\"%~dp0ran.txt\"\r\n"), 0600); err != nil {
					t.Fatal(err)
				}
				current.Settings.ManagedApps = []config.ManagedAppEntry{{ID: "coexist-script", Name: "Owned script", ExePath: script, LaunchHiddenInBackground: true}}
				current.Settings.StartupIntervalSeconds = 4
				awaitJob("settings.save", map[string]any{"revision": current.Revision, "settings": current.Settings})
				awaitJob("startup.retry", nil)
				awaitJob("launch", map[string]string{"id": "coexist-script"})
				deadline := time.Now().Add(3 * time.Second)
				for {
					if raw, err := os.ReadFile(filepath.Join(directory, "ran.txt")); err == nil && string(raw) == "coexist\r\n" {
						break
					}
					if time.Now().After(deadline) {
						t.Fatal("independent WinTray blocked the owned script")
					}
					time.Sleep(30 * time.Millisecond)
				}
				if err := call("snapshot", nil, &current); err != nil {
					t.Fatal(err)
				}
				if current.Settings.StartupIntervalSeconds != 4 {
					t.Fatal("settings were not saved")
				}
				if _, err := os.Stat(filepath.Join(directory, "settings.json")); err != nil {
					t.Fatal(err)
				}
			}
			// Installation and disabling use the same resource-release commands.
			awaitJob("uninstall.prepare", nil)
		})
	}
}
