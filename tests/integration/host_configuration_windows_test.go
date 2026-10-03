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
	"winone/internal/profiles"
	"winone/internal/transport"
	"winone/modules/wintray/preferences"
)

func TestHostWinTrayConfigurationTransaction(t *testing.T) {
	exe := os.Getenv("WINONE_CONFIG_HOST_EXE")
	if exe == "" {
		t.Skip("set WINONE_CONFIG_HOST_EXE with sibling WinTray worker")
	}
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "settings.json"), []byte(`{"schemaVersion":1,"theme":"system","captureShortcut":"","pinShortcut":""}`), 0600); err != nil {
		t.Fatal(err)
	}
	pipe := fmt.Sprintf(`\\.\pipe\WinOne-host-config-%d`, time.Now().UnixNano())
	cmd := exec.Command(exe, "--pipe", pipe, "--data-dir", directory)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	call := func(method string, params any, out any) error {
		data, _ := json.Marshal(params)
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
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
		if err := call("wintray.settings.get", nil, &current); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Host did not start")
		}
		time.Sleep(50 * time.Millisecond)
	}
	current.Settings.StartupIntervalSeconds = 19
	history := map[string]any{"version": 1, "apps": map[string][]time.Time{filepath.Join(directory, "example.exe"): {time.Now().UTC()}}}
	var result struct {
		State string `json:"state"`
	}
	if err := call("wintray.settings.save", map[string]any{"revision": current.Revision, "settings": current.Settings, "history": history}, &result); err != nil {
		t.Fatal(err)
	}
	if result.State != "completed" {
		t.Fatal(result)
	}
	if err := call("wintray.snapshot", nil, &current); err != nil {
		t.Fatal(err)
	}
	if current.Revision != "1" || current.Settings.StartupIntervalSeconds != 19 {
		t.Fatal(current)
	}
	if _, err := os.Stat(filepath.Join(directory, "modules", "wintray", "settings.json")); !os.IsNotExist(err) {
		t.Fatal("legacy source written")
	}
	data, err := os.ReadFile(filepath.Join(directory, "modules", "wintray", "startup-history.json"))
	if err != nil || !json.Valid(data) {
		t.Fatal("history merge not persisted", err)
	}
	archivePath := filepath.Join(directory, "export.winone")
	var exported struct {
		Path string `json:"path"`
	}
	if err = call("profiles.export", map[string]string{"path": archivePath, "name": "Test"}, &exported); err != nil {
		t.Fatal(err)
	}
	archiveBytes, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := profiles.Decode(archiveBytes)
	if err != nil {
		t.Fatal(err)
	}
	var trayPreferences struct {
		StartupIntervalSeconds int `json:"startupIntervalSeconds"`
	}
	if err = json.Unmarshal(archive.Modules["wintray"].Settings, &trayPreferences); err != nil {
		t.Fatal(err)
	}
	if len(archive.Modules) != 3 || trayPreferences.StartupIntervalSeconds != 19 || exported.Path != archivePath {
		t.Fatal("export did not reflect committed preferences")
	}
	var preview struct {
		SourceHash            string               `json:"sourceHash"`
		ConfigurationRevision uint64               `json:"configurationRevision"`
		Plan                  profiles.ArchivePlan `json:"plan"`
	}
	if err = call("profiles.import.preview", map[string]any{"path": archivePath, "bindings": map[string]string{"capture.output": directory}}, &preview); err != nil {
		t.Fatal(err)
	}
	if preview.SourceHash == "" || preview.ConfigurationRevision != 1 || len(preview.Plan.Modules) != 3 {
		t.Fatal(preview)
	}
	for _, module := range preview.Plan.Modules {
		if module.State != "ready" {
			t.Fatal(module)
		}
	}
	if err = call("wintray.settings.get", nil, &current); err != nil {
		t.Fatal(err)
	}
	if current.Revision != "1" {
		t.Fatal("preview changed configuration")
	}
	var importedSettings map[string]json.RawMessage
	module := archive.Modules["wintray"]
	if err = json.Unmarshal(module.Settings, &importedSettings); err != nil {
		t.Fatal(err)
	}
	importedSettings["startupIntervalSeconds"] = json.RawMessage(`23`)
	module.Settings, err = json.Marshal(importedSettings)
	if err != nil {
		t.Fatal(err)
	}
	archive.Modules["wintray"] = module
	archive.Preferences.Enabled["wintray"] = false
	archiveBytes, err = profiles.Encode(archive)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(archivePath, archiveBytes, 0600); err != nil {
		t.Fatal(err)
	}
	var applyPreview struct {
		SourceHash            string `json:"sourceHash"`
		ConfigurationRevision uint64 `json:"configurationRevision"`
		ModuleRevision        uint64 `json:"moduleRevision"`
	}
	if err = call("profiles.import.preview", map[string]any{"path": archivePath, "bindings": map[string]string{"capture.output": directory}}, &applyPreview); err != nil {
		t.Fatal(err)
	}
	var applied struct {
		Revision uint64 `json:"revision"`
	}
	if err = call("profiles.import.apply", map[string]any{"path": archivePath, "sourceHash": applyPreview.SourceHash, "configurationRevision": applyPreview.ConfigurationRevision, "moduleRevision": applyPreview.ModuleRevision, "bindings": map[string]string{"capture.output": directory}, "modules": []string{"wintray"}, "global": false}, &applied); err != nil {
		t.Fatal(err)
	}
	if applied.Revision != 2 {
		t.Fatal(applied)
	}
	if err = call("wintray.settings.get", nil, &current); err != nil {
		t.Fatal(err)
	}
	if current.Settings.StartupIntervalSeconds != 23 {
		t.Fatal("imported preference was not committed")
	}
	var moduleSnapshot modules.ControlSnapshot
	if err = call("modules.snapshot", nil, &moduleSnapshot); err != nil {
		t.Fatal(err)
	}
	for _, entry := range moduleSnapshot.Modules {
		if entry.Manifest.ID == "wintray" && (entry.Enabled || entry.PendingStop) {
			t.Fatal("imported disable was not completed", entry)
		}
	}
	var enabled modules.ControlJob
	if err = call("modules.setEnabled", map[string]any{"id": "wintray", "enabled": true, "revision": moduleSnapshot.Revision}, &enabled); err != nil {
		t.Fatal(err)
	}
	if err = call("wintray.snapshot", nil, &current); err != nil {
		t.Fatal(err)
	}
	if current.Settings.StartupIntervalSeconds != 23 || current.Revision != "3" {
		t.Fatal("re-enabled worker lost imported settings", current)
	}
	var global host.Snapshot
	if err = call("snapshot", nil, &global); err != nil {
		t.Fatal(err)
	}
	global.Settings.Theme = "dark"
	global.Settings.Language = "en-US"
	if err = call("settings.save", global.Settings, &global.Settings); err != nil {
		t.Fatal(err)
	}
	if err = call("snapshot", nil, &global); err != nil {
		t.Fatal(err)
	}
	if global.Locale != "en-US" || global.Settings.Language != "en-US" {
		t.Fatal("Host locale did not follow preference")
	}
	if err = call("wintray.snapshot", nil, &current); err != nil {
		t.Fatal(err)
	}
	if current.Settings.Language != "en-US" {
		t.Fatal("native menu language diverged")
	}
	var capture struct {
		Theme string `json:"theme"`
	}
	if err = call("capture.settings.get", nil, &capture); err != nil {
		t.Fatal(err)
	}
	if capture.Theme != "dark" {
		t.Fatal("capture theme diverged")
	}
}

// An isolated profile cannot register real logon tasks. Use that deliberate
// native rejection to verify an enable is not committed before initialization.
func TestHostFailedModuleEnableRestoresDisabledState(t *testing.T) {
	exe := os.Getenv("WINONE_CONFIG_HOST_EXE")
	if exe == "" {
		t.Skip("set WINONE_CONFIG_HOST_EXE")
	}
	directory := t.TempDir()
	moduleDirectory := filepath.Join(directory, "modules", "wintray")
	if err := os.MkdirAll(moduleDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	value := preferences.Defaults()
	value.RunAtLogon = true
	data, _ := json.Marshal(value)
	if err := os.WriteFile(filepath.Join(moduleDirectory, "settings.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	state := `{"schemaVersion":1,"revision":0,"modules":{"capture":{"enabled":false,"pendingStop":false},"always-on-top":{"enabled":false,"pendingStop":false},"wintray":{"enabled":false,"pendingStop":false}}}`
	if err := os.WriteFile(filepath.Join(directory, "module-state.json"), []byte(state), 0600); err != nil {
		t.Fatal(err)
	}
	pipe := fmt.Sprintf(`\\.\pipe\WinOne-enable-failure-%d`, time.Now().UnixNano())
	cmd := exec.Command(exe, "--pipe", pipe, "--data-dir", directory)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	call := func(method string, params any) (host.Response, error) {
		data, _ := json.Marshal(params)
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		return transport.CallCompleting(ctx, pipe, host.Request{Version: 1, ID: fmt.Sprint(time.Now().UnixNano()), Method: method, Params: data}, 5*time.Second)
	}
	deadline := time.Now().Add(10 * time.Second)
	var snapshot modules.ControlSnapshot
	for {
		response, err := call("modules.snapshot", nil)
		if err == nil && response.Error == nil {
			if err = json.Unmarshal(response.Result, &snapshot); err != nil {
				t.Fatal(err)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Host not ready")
		}
		time.Sleep(50 * time.Millisecond)
	}
	response, err := call("modules.setEnabled", map[string]any{"id": "wintray", "enabled": true, "revision": snapshot.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if response.Error == nil {
		t.Fatal("native initialization failure was reported as enabled")
	}
	response, err = call("modules.snapshot", nil)
	if err != nil || response.Error != nil {
		t.Fatal(err, response.Error)
	}
	if err = json.Unmarshal(response.Result, &snapshot); err != nil {
		t.Fatal(err)
	}
	for _, entry := range snapshot.Modules {
		if entry.Manifest.ID == "wintray" && (entry.Enabled || entry.PendingStop) {
			t.Fatal("failed enable was not compensated", entry)
		}
	}
}
