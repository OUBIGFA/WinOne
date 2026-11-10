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
	"winone/internal/modules"
	"winone/modules/always-on-top/engine"
)

// Exercise the same lazy launch and window.toggle used by the Host's shortcut.
// The runner owns a private desktop, worker data and target window; no SendInput
// or user-window access is needed to verify the actual native topmost bit.
func TestAlwaysOnTopFirstActionLatency(t *testing.T) {
	exe, fixture := os.Getenv("WINONE_AOT_EXE"), os.Getenv("WINONE_WINDOW_FIXTURE")
	if exe == "" || fixture == "" || os.Getenv("WINONE_MODULE_STARTUP_TEST") != "1" {
		t.Skip("use build/test-module-startup.ps1 -RunSystemChecks")
	}
	directory := t.TempDir()
	identityPath := filepath.Join(directory, "window.json")
	cmd := exec.Command(fixture, identityPath)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	var target engine.Window
	deadline := time.Now().Add(5 * time.Second)
	for {
		data, err := os.ReadFile(identityPath)
		if err == nil && json.Unmarshal(data, &target) == nil && target.Handle != "" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("owned window did not open")
		}
		time.Sleep(10 * time.Millisecond)
	}
	handle, _ := strconv.ParseUint(target.Handle, 10, 64)
	style := windows.NewLazySystemDLL("user32.dll").NewProc("GetWindowLongPtrW")
	isTopmost := func() bool { value, _, _ := style.Call(uintptr(handle), ^uintptr(19)); return value&8 != 0 }
	params, _ := json.Marshal(target)
	for cycle := 0; cycle < 5; cycle++ {
		t.Run(strconv.Itoa(cycle+1), func(t *testing.T) {
			data := t.TempDir()
			store, err := engine.OpenSettings(filepath.Join(data, "settings.json"))
			if err != nil {
				t.Fatal(err)
			}
			settings := store.Snapshot()
			// DWM borders require the input desktop. State changes and recovery
			// are checked here without moving the user's input desktop.
			settings.Frame, settings.Sound, settings.BlockInGameMode = false, false, false
			if _, err = store.Save(settings); err != nil {
				t.Fatal(err)
			}
			pipe := fmt.Sprintf(`\\.\pipe\WinOne-startup-native-%d-%d`, os.Getpid(), time.Now().UnixNano())
			lazy := modules.NewLazy(context.Background(), "always-on-top", "Pin", exe, pipe, data)
			t.Cleanup(func() {
				if err := lazy.Close(); err != nil {
					t.Error(err)
				}
			})
			if isTopmost() {
				t.Fatal("previous cycle retained a pin")
			}
			toggle := func() time.Duration {
				t.Helper()
				begin := time.Now()
				response := lazy.Handle(host.Request{Version: 1, ID: "toggle", Method: "window.toggle", Params: params})
				if response.Error != nil {
					t.Fatalf("toggle failed: %+v", response.Error)
				}
				return time.Since(begin)
			}
			first := toggle()
			if !isTopmost() {
				t.Fatal("first action did not pin the owned window")
			}
			warm := toggle()
			if isTopmost() {
				t.Fatal("second action did not unpin the owned window")
			}
			toggle()
			begin := time.Now()
			if err := lazy.Close(); err != nil {
				t.Fatal(err)
			}
			exit := time.Since(begin)
			if isTopmost() || lazy.Running() {
				t.Fatal("close retained native state or worker")
			}
			t.Logf("first=%v warm=%v exit=%v", first, warm, exit)
			if first > 2*time.Second || warm > 500*time.Millisecond {
				t.Fatal("module action exceeded response budget")
			}
		})
	}
}
