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

	"winone/internal/catalog"
	"winone/internal/host"
	"winone/internal/hotkeys"
	"winone/internal/transport"
	"winone/modules/always-on-top/engine"
)

// Physical input is opt-in and starts only after the parent switches to its
// owned desktop under a watchdog. Ordinary source tests never send input.
func TestHostPinShortcutFirstPress(t *testing.T) {
	output := os.Getenv("WINONE_PIN_HOTKEY_OUTPUT")
	if output == "" {
		t.Skip("explicit owned foreground-desktop test only")
	}
	bundle, fixture, desktopCLI := os.Getenv("WINONE_PIN_HOTKEY_BUNDLE"), os.Getenv("WINONE_WINDOW_FIXTURE"), os.Getenv("WINONE_DESKTOP_CLI")
	profile := filepath.Join(output, "profile")
	enabled := map[string]bool{}
	for _, manifest := range catalog.Builtins() {
		enabled[manifest.ID] = manifest.ID == "always-on-top"
	}
	pin := engine.Defaults()
	pin.BlockInGameMode, pin.Sound = false, false
	writeOverlapJSON(t, filepath.Join(profile, "configuration.json"), map[string]any{"committed": map[string]any{
		"schemaVersion": 1, "revision": 1, "builtinVersion": 2, "moduleEnabled": enabled,
		"values": map[string]any{"always-on-top": pin, "host": map[string]any{"schemaVersion": 1, "theme": "system", "captureShortcut": "", "pinShortcut": "Ctrl+Alt+F10", "ocrShortcut": "", "translateShortcut": "", "selectionShortcut": "", "quickAssistantShortcut": "", "openShortcut": "", "startup": map[string]bool{"runAtLogon": false, "startMinimized": true}}},
	}})
	pipe := fmt.Sprintf(`\\.\pipe\WinOne-pin-input-%d`, time.Now().UnixNano())
	hostProcess := exec.Command(filepath.Join(bundle, "WinOne.Host.exe"), "--standard-session", "--pipe", pipe, "--data-dir", profile)
	hostLog, err := os.Create(filepath.Join(output, "host-stderr.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer hostLog.Close()
	hostProcess.Stderr = hostLog
	if err := hostProcess.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = hostProcess.Process.Kill(); _ = hostProcess.Wait() })
	call := func(method string, target any) error {
		reply, err := transport.Call(context.Background(), pipe, host.Request{Version: 1, ID: "hotkey-test", Method: method})
		if err != nil {
			return err
		}
		if reply.Error != nil {
			return fmt.Errorf("%s", reply.Error.Message)
		}
		return json.Unmarshal(reply.Result, target)
	}
	var shortcuts struct {
		Bindings []hotkeys.Status `json:"bindings"`
	}
	waitOverlap(t, "Host shortcuts", func() bool { return call("shortcuts.snapshot", &shortcuts) == nil })
	registered := false
	for _, binding := range shortcuts.Bindings {
		if binding.ID == "pin" && binding.Registered {
			registered = true
		}
	}
	if !registered {
		t.Fatalf("shortcut not registered: %+v", shortcuts)
	}
	identity := filepath.Join(output, "window.json")
	window := exec.Command(fixture, identity)
	if err := window.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = window.Process.Kill(); _ = window.Wait() })
	var target engine.Window
	waitOverlap(t, "owned target", func() bool {
		data, err := os.ReadFile(identity)
		return err == nil && json.Unmarshal(data, &target) == nil && target.Handle != ""
	})
	handle, _ := strconv.ParseUint(target.Handle, 10, 64)
	isPinned := func() bool {
		value, _, _ := overlapUser32.NewProc("GetWindowLongPtrW").Call(uintptr(handle), ^uintptr(19))
		return value&8 != 0
	}
	writeOverlapJSON(t, filepath.Join(output, "ready.json"), map[string]any{"hostPid": hostProcess.Process.Pid, "targetPid": window.Process.Pid})
	waitOverlap(t, "foreground authorization", func() bool { _, err := os.Stat(filepath.Join(output, "input-active")); return err == nil })
	start := time.Now()
	for index, want := range []bool{true, false, true} {
		if time.Since(start) > 10*time.Second {
			t.Fatal("foreground input budget exhausted")
		}
		begin := time.Now()
		command := exec.Command("powershell.exe", "-NoProfile", "-File", desktopCLI, "press", "ctrl+alt+f10", "--headed", "--app", "WinOne.WindowFixture")
		result, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("desktop input failed: %v %s", err, result)
		}
		if err = os.WriteFile(filepath.Join(output, fmt.Sprintf("press-%d.json", index+1)), result, 0600); err != nil {
			t.Fatal(err)
		}
		waitOverlap(t, "shortcut changes owned window", func() bool { return isPinned() == want })
		t.Logf("physical press %d to verified state (including CLI startup): %v", index+1, time.Since(begin))
		if time.Since(begin) > 3*time.Second {
			t.Fatal("shortcut response exceeded budget")
		}
	}
	// End the input-desktop portion immediately; cleanup uses the real worker
	// shutdown path and does not need further keyboard or mouse input.
	writeOverlapJSON(t, filepath.Join(output, "input-finished"), map[string]bool{"passed": true})
	// Private desktops have no Explorer tray. Request real module shutdown
	// through the Host instead of assuming that a tray menu was registered.
	var stopped any
	if err := call("always-on-top.shutdown", &stopped); err != nil {
		t.Fatal(err)
	}
	waitOverlap(t, "module shutdown restores pin", func() bool { return !isPinned() })
}
