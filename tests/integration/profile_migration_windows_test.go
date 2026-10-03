//go:build windows

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"winone/internal/host"
	"winone/internal/profiles"
	"winone/internal/transport"
	"winone/modules/wintray/preferences"
)

// This opt-in test uses two isolated Host profiles. It never registers login
// tasks, captures the desktop, or launches the inert example executable.
func TestNativeProfileMigrationAcrossDirectories(t *testing.T) {
	exe := os.Getenv("WINONE_PROFILE_HOST_EXE")
	if exe == "" {
		t.Skip("set WINONE_PROFILE_HOST_EXE with sibling workers")
	}
	type callFunc func(string, any, any) error
	start := func(directory string) (callFunc, func()) {
		pipe := fmt.Sprintf(`\\.\pipe\WinOne-profile-migration-%d`, time.Now().UnixNano())
		cmd := exec.Command(exe, "--pipe", pipe, "--data-dir", directory)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		closed := false
		t.Cleanup(func() {
			if !closed {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
			}
		})
		call := func(method string, params any, out any) error {
			raw, err := json.Marshal(params)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			response, err := transport.CallCompleting(ctx, pipe, host.Request{Version: 1, ID: fmt.Sprint(time.Now().UnixNano()), Method: method, Params: raw}, 5*time.Second)
			if err != nil {
				return err
			}
			if response.Error != nil {
				return fmt.Errorf("%s", response.Error.Message)
			}
			if out == nil {
				return nil
			}
			return json.Unmarshal(response.Result, out)
		}
		deadline := time.Now().Add(15 * time.Second)
		for {
			if err := call("snapshot", nil, nil); err == nil {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("test Host did not start")
			}
			time.Sleep(50 * time.Millisecond)
		}
		stop := func() {
			deadline := time.Now().Add(10 * time.Second)
			for !closeLogFailureTestHost(uint32(cmd.Process.Pid)) {
				if time.Now().After(deadline) {
					t.Fatal("test Host tray owner not ready")
				}
				time.Sleep(25 * time.Millisecond)
			}
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			select {
			case err := <-done:
				closed = true
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(15 * time.Second):
				_ = cmd.Process.Kill()
				<-done
				closed = true
				t.Fatal("test Host did not stop")
			}
		}
		return call, stop
	}
	must := func(call callFunc, method string, params any, out any) {
		t.Helper()
		if err := call(method, params, out); err != nil {
			t.Fatalf("%s: %v", method, err)
		}
	}
	root := t.TempDir()
	source := filepath.Join(root, "office")
	target := filepath.Join(root, "home")
	for _, directory := range []string{source, target} {
		if err := os.MkdirAll(directory, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "settings.json"), []byte(`{"schemaVersion":1,"theme":"system","captureShortcut":"","pinShortcut":""}`), 0600); err != nil {
			t.Fatal(err)
		}
	}
	sourceCall, sourceStop := start(source)
	sourceProgram := filepath.Join(source, "example.exe")
	if err := os.WriteFile(sourceProgram, []byte("inert test file"), 0600); err != nil {
		t.Fatal(err)
	}
	var global host.Snapshot
	must(sourceCall, "snapshot", nil, &global)
	global.Settings.Theme = "dark"
	global.Settings.Language = "en-US"
	must(sourceCall, "settings.save", global.Settings, nil)
	var pin map[string]any
	must(sourceCall, "always-on-top.settings.get", nil, &pin)
	pin["frameThickness"] = 7
	pin["sound"] = false
	pin["excludedApps"] = []string{sourceProgram}
	must(sourceCall, "always-on-top.settings.save", pin, nil)
	var capture map[string]any
	must(sourceCall, "capture.settings.get", nil, &capture)
	capture["outputDirectory"] = source
	capture["scrollDelay"] = 650
	capture["toolbarTools"] = []string{"text", "arrow"}
	capture["fileNameTemplate"] = "Office-{timestamp}"
	must(sourceCall, "capture.settings.save", capture, &capture)
	var tray struct {
		Revision string               `json:"revision"`
		Settings preferences.Settings `json:"settings"`
	}
	must(sourceCall, "wintray.settings.get", nil, &tray)
	// The inert program is never enabled, scheduled, collected or launched.
	programs, err := json.Marshal(map[string]any{"managedApps": []any{map[string]any{
		"id": "example", "name": "Example", "exePath": sourceProgram, "args": "--example-local-argument",
		"runOnStartup": false, "collectTrayIcon": false, "launchViaLogonTask": false, "launchHiddenInBackground": false,
		"trayBehavior": map[string]any{"autoMinimizeAndHideOnLaunch": false, "closeDelaySeconds": 0},
		"schedule":     map[string]any{"enabled": false, "startDelayMinutes": 5, "autoExitMinutes": 30, "frequencyEnabled": false, "frequencyDays": 1, "frequencyRuns": 1},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(programs, &tray.Settings); err != nil {
		t.Fatal(err)
	}
	tray.Settings.StartupIntervalSeconds = 17
	must(sourceCall, "wintray.settings.save", map[string]any{"revision": tray.Revision, "settings": tray.Settings}, nil)
	archivePath := filepath.Join(root, "office.winone")
	must(sourceCall, "profiles.export", map[string]any{"path": archivePath, "name": "Office"}, nil)
	data, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := profiles.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	portable, err := json.Marshal(archive)
	if err != nil {
		t.Fatal(err)
	}
	sourceJSON, _ := json.Marshal(source)
	if strings.Contains(string(portable), string(sourceJSON[1:len(sourceJSON)-1])) {
		t.Fatal("archive retained source device path")
	}
	sourceStop()

	targetCall, targetStop := start(target)
	type previewResult struct {
		SourceHash            string               `json:"sourceHash"`
		ConfigurationRevision uint64               `json:"configurationRevision"`
		ModuleRevision        uint64               `json:"moduleRevision"`
		Plan                  profiles.ArchivePlan `json:"plan"`
	}
	var preview previewResult
	must(targetCall, "profiles.import.preview", map[string]any{"path": archivePath}, &preview)
	needsMapping := false
	for _, item := range preview.Plan.Modules {
		if item.ID == "capture" {
			needsMapping = item.State == "needs_mapping"
		}
	}
	if !needsMapping {
		t.Fatal("missing destination folder was not identified")
	}
	apply := func(bindings map[string]string) map[string]any {
		return map[string]any{"path": archivePath, "sourceHash": preview.SourceHash, "configurationRevision": preview.ConfigurationRevision, "moduleRevision": preview.ModuleRevision, "bindings": bindings, "modules": []string{"capture", "always-on-top", "wintray"}, "global": true}
	}
	if err := targetCall("profiles.import.apply", apply(nil), nil); err == nil {
		t.Fatal("import accepted missing path mapping")
	}
	var before map[string]any
	must(targetCall, "capture.settings.get", nil, &before)
	if before["scrollDelay"] == float64(650) {
		t.Fatal("failed import partially changed destination")
	}
	must(targetCall, "snapshot", nil, &global)
	if global.Settings.Theme != "system" {
		t.Fatal("failed import changed global preferences")
	}
	destination := filepath.Join(target, "Pictures")
	if err := os.MkdirAll(destination, 0700); err != nil {
		t.Fatal(err)
	}
	targetProgram := filepath.Join(target, "example.exe")
	bindings := map[string]string{"capture.output": destination, "wintray.program.0": targetProgram, "always-on-top.excluded.0": targetProgram}
	must(targetCall, "profiles.import.preview", map[string]any{"path": archivePath, "bindings": bindings}, &preview)
	if err := targetCall("profiles.import.apply", apply(bindings), nil); err == nil {
		t.Fatal("import accepted a missing destination program")
	}
	if err := os.WriteFile(targetProgram, []byte("inert test file"), 0600); err != nil {
		t.Fatal(err)
	}
	must(targetCall, "profiles.import.preview", map[string]any{"path": archivePath, "bindings": bindings}, &preview)
	for _, item := range preview.Plan.Modules {
		if item.State != "ready" {
			t.Fatalf("module not ready: %+v", item)
		}
	}
	must(targetCall, "profiles.import.apply", apply(bindings), nil)
	targetStop()
	restored, restoredStop := start(target)
	must(restored, "snapshot", nil, &global)
	if global.Settings.Theme != "dark" || global.Settings.Language != "en-US" || global.Settings.Startup == nil || global.Settings.Startup.RunAtLogon || global.Settings.Startup.PreferAdministrator {
		t.Fatal("global preferences changed during migration", global.Settings)
	}
	must(restored, "always-on-top.settings.get", nil, &pin)
	if pin["frameThickness"] != float64(7) || pin["sound"] != false {
		t.Fatal("pin preferences did not survive restart", pin)
	}
	excluded, _ := json.Marshal(pin["excludedApps"])
	expectedExcluded, _ := json.Marshal([]string{targetProgram})
	if string(excluded) != string(expectedExcluded) {
		t.Fatal("pin exclusion was not mapped to destination")
	}
	must(restored, "capture.settings.get", nil, &capture)
	if capture["outputDirectory"] != destination || capture["scrollDelay"] != float64(650) || capture["fileNameTemplate"] != "Office-{timestamp}" {
		t.Fatal("capture preferences did not survive import/restart", capture)
	}
	tools, _ := json.Marshal(capture["toolbarTools"])
	if string(tools) != `["text","arrow"]` {
		t.Fatal("toolbar preferences changed", string(tools))
	}
	must(restored, "wintray.settings.get", nil, &tray)
	if tray.Settings.StartupIntervalSeconds != 17 {
		t.Fatal("program preferences did not survive import/restart")
	}
	if len(tray.Settings.ManagedApps) != 1 || tray.Settings.ManagedApps[0].ExePath != targetProgram || tray.Settings.ManagedApps[0].Args != "" || tray.Settings.ManagedApps[0].RunOnStartup {
		t.Fatal("program mapping, omitted arguments or disabled startup changed")
	}
	secondPath := filepath.Join(root, "home.winone")
	must(restored, "profiles.export", map[string]any{"path": secondPath, "name": "Home"}, nil)
	secondData, err := os.ReadFile(secondPath)
	if err != nil {
		t.Fatal(err)
	}
	second, err := profiles.Decode(secondData)
	if err != nil {
		t.Fatal(err)
	}
	for id, expected := range archive.Modules {
		if string(second.Modules[id].Settings) != string(expected.Settings) {
			t.Fatalf("portable preferences changed for %s", id)
		}
	}
	restoredStop()
	t.Log("two-directory export/import, missing mapping rejection, restart and portable round trip passed")
}
