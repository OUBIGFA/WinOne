//go:build windows

package worker

import (
	"bytes"
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
	// Exercise the same preview + explicit save protocol as the desktop import flow.
	legacyDir := filepath.Join(dir, "legacy")
	if err := os.MkdirAll(legacyDir, 0700); err != nil {
		t.Fatal(err)
	}
	legacyPath := filepath.Join(legacyDir, "settings.json")
	legacyData, err := json.Marshal(next)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(legacyPath, legacyData, 0600); err != nil {
		t.Fatal(err)
	}
	historyData, err := json.Marshal(map[string]any{"version": 1, "apps": map[string][]time.Time{script: {time.Now().UTC()}}})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(legacyDir, "startup-history.json"), historyData, 0600); err != nil {
		t.Fatal(err)
	}
	var preview ImportPreview
	if err = call("import.preview", map[string]string{"path": legacyPath}, &preview); err != nil {
		t.Fatal(err)
	}
	if len(preview.History) == 0 || len(preview.Settings.ManagedApps) != 1 {
		t.Fatal("import preview incomplete")
	}
	if _, err = os.Stat(filepath.Join(dir, "settings.json")); !os.IsNotExist(err) {
		t.Fatal("preview wrote destination settings")
	}
	next = preview.Settings
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
	if err := call("settings.save", map[string]any{"revision": current.Revision, "settings": next, "history": preview.History}, &j); err != nil {
		t.Fatal(err)
	}
	waitJob(j)
	storedHistory, err := os.ReadFile(filepath.Join(dir, "startup-history.json"))
	if err != nil {
		t.Fatal(err)
	}
	records, err := config.DecodeHistory(storedHistory)
	if err != nil || len(records) != 1 {
		t.Fatalf("history not persisted: %v", err)
	}
	unchanged, err := os.ReadFile(legacyPath)
	if err != nil || !bytes.Equal(unchanged, legacyData) {
		t.Fatal("import changed source settings")
	}
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
	// A startup queue failure remains visible even after an unrelated successful save.
	if err := call("snapshot", nil, &current); err != nil {
		t.Fatal(err)
	}
	broken := current.Settings
	broken.ManagedApps = append([]config.ManagedAppEntry{}, broken.ManagedApps...)
	broken.ManagedApps[0].ExePath = filepath.Join(dir, "missing.cmd")
	if err := call("settings.save", map[string]any{"revision": current.Revision, "settings": broken}, &j); err != nil {
		t.Fatal(err)
	}
	waitJob(j)
	if err := call("startup.retry", nil, &j); err != nil {
		t.Fatal(err)
	}
	limit := time.Now().Add(10 * time.Second)
	for j.State == "running" && time.Now().Before(limit) {
		time.Sleep(25 * time.Millisecond)
		if err := call("jobs.get", map[string]string{"id": j.ID}, &j); err != nil {
			t.Fatal(err)
		}
	}
	if j.State != "failed" {
		t.Fatalf("failed startup reported success: %+v", j)
	}
	if err := call("snapshot", nil, &current); err != nil {
		t.Fatal(err)
	}
	if current.StartupError == "" {
		t.Fatal("startup failure not exposed")
	}
	if err := call("settings.save", map[string]any{"revision": current.Revision, "settings": current.Settings}, &j); err != nil {
		t.Fatal(err)
	}
	waitJob(j)
	if err := call("snapshot", nil, &current); err != nil {
		t.Fatal(err)
	}
	if current.StartupError == "" {
		t.Fatal("unrelated save erased startup failure")
	}
	if err := call("settings.save", map[string]any{"revision": current.Revision, "settings": next}, &j); err != nil {
		t.Fatal(err)
	}
	waitJob(j)
	if err := call("startup.retry", nil, &j); err != nil {
		t.Fatal(err)
	}
	waitJob(j)
	if err := call("snapshot", nil, &current); err != nil {
		t.Fatal(err)
	}
	if current.StartupError != "" {
		t.Fatalf("successful retry did not clear failure: %s", current.StartupError)
	}
	beforeDisable := current.Revision
	if err := call("deactivate", nil, &j); err != nil {
		t.Fatal(err)
	}
	waitJob(j)
	if err := call("snapshot", nil, &current); err != nil || current.Revision != beforeDisable {
		t.Fatalf("deactivation rewrote user preferences: %v", err)
	}
	if err := call("launch", map[string]string{"id": "test"}, &j); err != nil {
		t.Fatal(err)
	}
	if j.State != "failed" {
		t.Fatal("deactivated worker still accepted launches")
	}
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
	// Recovery-only startup must bypass activation, even with saved login preferences.
	recoverySettings := current.Settings
	recoverySettings.RunAtLogon = true
	if err := config.NewStore(filepath.Join(dir, "settings.json")).Save(recoverySettings); err != nil {
		t.Fatal(err)
	}
	recoveryCmd := exec.Command(exe, "--pipe", pipe, "--data-dir", dir, "--autorun", "--recover-only")
	if err := recoveryCmd.Start(); err != nil {
		t.Fatal(err)
	}
	recoveryDone := make(chan error, 1)
	go func() { recoveryDone <- recoveryCmd.Wait() }()
	recoveryWaited := false
	defer func() {
		if !recoveryWaited {
			recoveryCmd.Process.Kill()
			<-recoveryDone
		}
	}()
	deadline = time.Now().Add(10 * time.Second)
	for {
		current = Snapshot{}
		if err := call("snapshot", nil, &current); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("recovery worker not ready")
		}
		time.Sleep(25 * time.Millisecond)
	}
	if current.BackgroundError != "" {
		t.Fatalf("recovery-only startup activated native settings: %s", current.BackgroundError)
	}
	recoveryRevision := current.Revision
	if err := call("startup.retry", nil, &j); err != nil {
		t.Fatal(err)
	}
	if j.State != "failed" {
		t.Fatal("recovery-only process accepted a startup queue")
	}
	if err := call("uninstall.prepare", nil, &j); err != nil {
		t.Fatal(err)
	}
	waitJob(j)
	if err := call("snapshot", nil, &current); err != nil || current.Revision != recoveryRevision {
		t.Fatalf("uninstall preparation changed preferences: %v", err)
	}
	// Repeating preparation is safe, and must not reactivate saved tasks.
	if err := call("uninstall.prepare", nil, &j); err != nil {
		t.Fatal(err)
	}
	waitJob(j)
	if err := call("shutdown", nil, &stopping); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-recoveryDone:
		recoveryWaited = true
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("recovery worker did not exit")
	}
}
