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
	"winone/internal/transport"
)

// This explicit fixture uses the real Host and recycle API, never user data.
func TestNativeHostConsolidatesIndependentLogs(t *testing.T) {
	bundle := os.Getenv("WINONE_UNIFIED_LOG_BUNDLE")
	if bundle == "" {
		t.Skip("explicit full-bundle log migration")
	}
	dir := t.TempDir()
	seedLifecycleConfiguration(t, dir, map[string]any{"schemaVersion": 1, "theme": "light", "language": "zh-CN", "captureShortcut": "", "pinShortcut": "", "ocrShortcut": "", "translateShortcut": "", "selectionShortcut": ""})
	var config map[string]any
	raw, err := os.ReadFile(filepath.Join(dir, "configuration.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &config); err != nil {
		t.Fatal(err)
	}
	enabled := map[string]bool{"capture": false, "always-on-top": false, "wintray": false, "selection": false}
	config["committed"].(map[string]any)["moduleEnabled"] = enabled
	writeOverlapJSON(t, filepath.Join(dir, "configuration.json"), config)
	state := map[string]any{}
	for id := range enabled {
		state[id] = map[string]bool{"enabled": false, "pendingStop": false}
	}
	writeOverlapJSON(t, filepath.Join(dir, "module-state.json"), map[string]any{"schemaVersion": 1, "revision": 0, "modules": state})
	files := map[string]string{"modules/capture/worker.log": "capture-migration-proof\n", "modules/wintray/wintray.log": time.Now().Format(time.RFC3339) + " [ERROR] wintray-migration-proof\n", "modules/always-on-top/native/native_2026-10-07.log": "native-migration-proof\n"}
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	database := filepath.Join(dir, "webview-components", "Local Storage", "000003.log")
	if err = os.MkdirAll(filepath.Dir(database), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(database, []byte("database sentinel"), 0600); err != nil {
		t.Fatal(err)
	}
	pipe := fmt.Sprintf(`\\.\pipe\WinOne-unified-logs-%d`, time.Now().UnixNano())
	cmd := exec.Command(filepath.Join(bundle, "WinOne.Host.exe"), "--standard-session", "--pipe", pipe, "--data-dir", dir)
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	exited := false
	defer func() {
		if !exited {
			cmd.Process.Kill()
			<-done
		}
	}()
	var text string
	waitOverlap(t, "unified diagnostics", func() bool {
		params, _ := json.Marshal(map[string]string{"module": "host"})
		response, err := transport.Call(context.Background(), pipe, host.Request{Version: 1, ID: "logs", Method: "diagnostics.read", Params: params})
		if err != nil || response.Error != nil {
			return false
		}
		text = string(response.Result)
		return strings.Contains(text, "native-migration-proof") && strings.Contains(text, "capture-migration-proof") && strings.Contains(text, "wintray-migration-proof")
	})
	for name := range files {
		if _, err = os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatal("independent log remains", name, err)
		}
	}
	if content, err := os.ReadFile(database); err != nil || string(content) != "database sentinel" {
		t.Fatal("runtime database was changed", err)
	}
	tray := findOverlapWindow(uint32(cmd.Process.Pid), "SystrayClass", false)
	if tray == 0 {
		t.Fatal("missing Host tray")
	}
	// This migration fixture has no module tasks or settings window. WM_CLOSE
	// uses systray's normal message-loop shutdown without racing menu creation.
	overlapUser32.NewProc("PostMessageW").Call(tray, 0x0010, 0, 0)
	select {
	case err := <-done:
		exited = true
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("Host did not exit")
	}
	t.Log("Known legacy diagnostics migrated to host.log; originals recycled; runtime database preserved; Host exited normally")
}
