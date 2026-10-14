//go:build windows

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"golang.org/x/sys/windows"
	"winone/internal/host"
	"winone/internal/transport"
	"winone/modules/always-on-top/engine"
)

// The real tray command must restore a pin before both Host and worker exit.
// The window owners are separate processes and must remain alive throughout.
func TestTrayExitRestoresActiveAlwaysOnTop(t *testing.T) {
	bundle, fixture := os.Getenv("WINONE_DESKTOP_EXIT_BUNDLE"), os.Getenv("WINONE_WINDOW_FIXTURE")
	if bundle == "" || fixture == "" {
		t.Skip("explicit owned-window tray exit test")
	}
	profile := t.TempDir()
	seedLifecycleConfiguration(t, profile, map[string]any{"schemaVersion": 1, "theme": "light", "language": "en-US", "captureShortcut": "", "pinShortcut": "", "ocrShortcut": "", "translateShortcut": "", "selectionShortcut": ""})
	startWindow := func(name string, topmost bool) (engine.Window, *exec.Cmd) {
		identity := filepath.Join(profile, name+".json")
		args := []string{identity}
		if topmost {
			args = append(args, "--topmost")
		}
		command := exec.Command(fixture, args...)
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = command.Process.Kill(); _ = command.Wait() })
		return readOverlapWindow(t, identity), command
	}
	pinned, _ := startWindow("owned-pin", false)
	foreign, _ := startWindow("foreign-topmost", true)
	pipe := fmt.Sprintf(`\\.\pipe\WinOne-tray-aot-exit-%d`, time.Now().UnixNano())
	backend := exec.Command(filepath.Join(bundle, "WinOne.Host.exe"), "--standard-session", "--pipe", pipe, "--data-dir", profile)
	logPath := filepath.Join(profile, "host-stderr.log")
	log, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	backend.Stderr = log
	if err := backend.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- backend.Wait() }()
	exited := false
	t.Cleanup(func() {
		if !exited {
			_ = backend.Process.Kill()
			<-done
		}
	})
	call := func(method string, params, result any) error {
		raw, err := json.Marshal(params)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		response, err := transport.CallCompleting(ctx, pipe, host.Request{Version: 1, ID: strconv.FormatInt(time.Now().UnixNano(), 10), Method: method, Params: raw}, time.Second)
		if err != nil {
			return err
		}
		if response.Error != nil {
			return fmt.Errorf("%s: %s", method, response.Error.Message)
		}
		if result != nil {
			return json.Unmarshal(response.Result, result)
		}
		return nil
	}
	waitOverlap(t, "Host ready", func() bool { return call("snapshot", nil, nil) == nil })
	var settings engine.Settings
	if err := call("always-on-top.settings.get", nil, &settings); err != nil {
		t.Fatal(err)
	}
	// Borders depend on Explorer's virtual-desktop service; pin ownership and
	// process shutdown remain testable on a private, never-switched desktop.
	settings.Frame, settings.Sound, settings.BlockInGameMode = false, false, false
	if err := call("always-on-top.settings.save", settings, &settings); err != nil {
		t.Fatal(err)
	}
	if err := call("always-on-top.window.set", map[string]any{"window": pinned, "pinned": true}, nil); err != nil {
		t.Fatal(err)
	}
	if !overlapTopmost(pinned) || !overlapTopmost(foreign) {
		t.Fatal("fixture pin state incorrect")
	}
	worker := moduleChildHandle(t, uint32(backend.Process.Pid), "WinOne.AlwaysOnTop.exe")
	defer windows.CloseHandle(worker)
	tray := findOverlapWindow(uint32(backend.Process.Pid), "SystrayClass", false)
	if tray == 0 {
		t.Fatal("owned tray window missing")
	}
	if ok, _, err := overlapUser32.NewProc("PostMessageW").Call(tray, 0x0111, 3, 0); ok == 0 {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		exited = true
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(15 * time.Second):
		raw, _ := os.ReadFile(logPath)
		t.Fatalf("Host survived normal tray exit with active pin: %s", raw)
	}
	if state, err := windows.WaitForSingleObject(worker, 0); err != nil || state != windows.WAIT_OBJECT_0 {
		t.Fatal("pin worker survived Host exit", state, err)
	}
	if overlapTopmost(pinned) || !overlapTopmost(foreign) {
		t.Fatal("tray exit restored the wrong pin state")
	}
	for _, window := range []engine.Window{pinned, foreign} {
		if findOverlapWindow(window.PID, "", true) == 0 {
			t.Fatal("tray exit closed a window owner", window.PID)
		}
	}
}
