//go:build windows

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"
	"unsafe"
	"winone/internal/host"
	"winone/internal/transport"
	"winone/modules/always-on-top/engine"
)

func TestAlwaysOnTopOwnedWindowRestoration(t *testing.T) {
	testAlwaysOnTopOwnedWindowRestoration(t, true)
}

// State restoration can be verified on a private Win32 desktop. The separate
// border-enabled test still covers the Explorer virtual-desktop requirement.
func TestAlwaysOnTopStateRestorationWithoutFrame(t *testing.T) {
	testAlwaysOnTopOwnedWindowRestoration(t, false)
}

func testAlwaysOnTopOwnedWindowRestoration(t *testing.T, frame bool) {
	exe, fixture := os.Getenv("WINONE_AOT_EXE"), os.Getenv("WINONE_WINDOW_FIXTURE")
	if exe == "" || fixture == "" {
		t.Skip("set WINONE_AOT_EXE and WINONE_WINDOW_FIXTURE for native checks")
	}
	dir := t.TempDir()
	startWindow := func(name string, mode string) engine.Window {
		path := filepath.Join(dir, name+".json")
		args := []string{path}
		if mode != "" {
			args = append(args, mode)
		}
		cmd := exec.Command(fixture, args...)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
		limit := time.Now().Add(5 * time.Second)
		for {
			data, err := os.ReadFile(path)
			if err == nil {
				var ref engine.Window
				if json.Unmarshal(data, &ref) == nil && ref.Handle != "" {
					return ref
				}
			}
			if time.Now().After(limit) {
				t.Fatal("fixture did not open")
			}
			time.Sleep(25 * time.Millisecond)
		}
	}
	normal := startWindow("normal", "")
	foreign := startWindow("foreign", "--topmost")
	isTopmost := func(ref engine.Window) bool {
		h, _ := strconv.ParseUint(ref.Handle, 10, 64)
		style, _, _ := windows.NewLazySystemDLL("user32.dll").NewProc("GetWindowLongPtrW").Call(uintptr(h), ^uintptr(19))
		return style&8 != 0
	}
	if isTopmost(normal) || !isTopmost(foreign) {
		t.Fatalf("fixture initial z-order: normal=%t %+v foreign=%t %+v", isTopmost(normal), normal, isTopmost(foreign), foreign)
	}
	pipe := fmt.Sprintf(`\\.\pipe\WinOne-aot-test-%d`, time.Now().UnixNano())
	cmd := exec.Command(exe, "--pipe", pipe, "--data-dir", filepath.Join(dir, "module"))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	var exitError error
	go func() { exitError = cmd.Wait(); close(done) }()
	t.Cleanup(func() {
		select {
		case <-done:
		default:
			_ = cmd.Process.Kill()
			<-done
		}
	})
	call := func(method string, params any) (host.Response, error) {
		data, _ := json.Marshal(params)
		return transport.Call(context.Background(), pipe, host.Request{Version: 1, ID: "aot-test", Method: method, Params: data})
	}
	var settings engine.Settings
	limit := time.Now().Add(10 * time.Second)
	for {
		r, err := call("snapshot", nil)
		if err == nil && r.Error == nil {
			var s struct {
				Settings engine.Settings `json:"settings"`
			}
			if err = json.Unmarshal(r.Result, &s); err != nil {
				t.Fatal(err)
			}
			settings = s.Settings
			break
		}
		select {
		case <-done:
			t.Fatalf("worker exited: %s", stderr.String())
		default:
		}
		if time.Now().After(limit) {
			t.Fatalf("worker not ready: %v", err)
		}
		time.Sleep(25 * time.Millisecond)
	}
	settings.Frame = frame
	settings.BlockInGameMode = false
	legacyDisabled := settings
	legacyDisabled.Enabled = false
	if r, err := call("settings.save", legacyDisabled); err != nil || r.Error == nil {
		t.Fatalf("legacy engine switch bypassed module management: %v %v", r.Error, err)
	}
	if r, err := call("settings.save", settings); err != nil || r.Error != nil {
		t.Fatalf("settings: %+v %v", r, err)
	}
	if os.Getenv("WINONE_AOT_HOVER_CHECK") == "1" {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		api := windows.NewLazySystemDLL("user32.dll")
		normalID, _ := strconv.ParseUint(normal.Handle, 10, 64)
		foreignID, _ := strconv.ParseUint(foreign.Handle, 10, 64)
		var pointer struct{ X, Y int32 }
		if ok, _, _ := api.NewProc("GetCursorPos").Call(uintptr(unsafe.Pointer(&pointer))); ok == 0 {
			t.Fatal("cannot read pointer")
		}
		t.Cleanup(func() { api.NewProc("SetCursorPos").Call(uintptr(pointer.X), uintptr(pointer.Y)) })
		api.NewProc("SetWindowPos").Call(uintptr(normalID), 0, 100, 100, 450, 300, 0x14)
		api.NewProc("SetWindowPos").Call(uintptr(foreignID), 0, 340, 120, 450, 300, 0x14)
		// The console test does not own foreground input. Join the two owned
		// window queues briefly, as the native UI probe does, then detach them.
		previous, _, _ := api.NewProc("GetForegroundWindow").Call()
		previousThread, _, _ := api.NewProc("GetWindowThreadProcessId").Call(previous, 0)
		targetThread, _, _ := api.NewProc("GetWindowThreadProcessId").Call(uintptr(foreignID), 0)
		currentThread := uintptr(windows.GetCurrentThreadId())
		for _, thread := range []uintptr{previousThread, targetThread} {
			if thread != 0 && thread != currentThread {
				api.NewProc("AttachThreadInput").Call(currentThread, thread, 1)
			}
		}
		api.NewProc("SetForegroundWindow").Call(uintptr(foreignID))
		for _, thread := range []uintptr{targetThread, previousThread} {
			if thread != 0 && thread != currentThread {
				api.NewProc("AttachThreadInput").Call(currentThread, thread, 0)
			}
		}
		foreground, _, _ := api.NewProc("GetForegroundWindow").Call()
		if foreground != uintptr(foreignID) {
			t.Fatal("owned foreground fixture was not activated")
		}
		api.NewProc("SetCursorPos").Call(160, 200)
		for _, want := range []bool{true, false} {
			if response, err := call("toggle", nil); err != nil || response.Error != nil {
				t.Fatalf("hover toggle: %+v %v", response, err)
			}
			if isTopmost(normal) != want || !isTopmost(foreign) {
				t.Fatal("hover toggled the foreground instead of the pointed window")
			}
			foreground, _, _ = api.NewProc("GetForegroundWindow").Call()
			if foreground != uintptr(foreignID) {
				t.Fatal("hover pin activated its target")
			}
		}
	}
	if r, err := call("window.set", map[string]any{"window": normal, "pinned": true}); err != nil || r.Error != nil {
		t.Fatalf("pin: %+v %v", r, err)
	}
	if !isTopmost(normal) {
		t.Fatal("native window was not pinned")
	}
	user32 := windows.NewLazySystemDLL("user32.dll")
	class, _ := windows.UTF16PtrFromString("AlwaysOnTop_Border")
	findBorder := func() uintptr {
		var after uintptr
		for {
			h, _, _ := user32.NewProc("FindWindowExW").Call(0, after, uintptr(unsafe.Pointer(class)), 0)
			if h == 0 {
				return 0
			}
			var pid uint32
			user32.NewProc("GetWindowThreadProcessId").Call(h, uintptr(unsafe.Pointer(&pid)))
			if pid == uint32(cmd.Process.Pid) {
				return h
			}
			after = h
		}
	}
	type rect struct{ Left, Top, Right, Bottom int32 }
	getRect := func(h uintptr) rect {
		var bounds rect
		if ok, _, err := user32.NewProc("GetWindowRect").Call(h, uintptr(unsafe.Pointer(&bounds))); ok == 0 {
			t.Fatal(err)
		}
		return bounds
	}
	border := findBorder()
	if frame && border == 0 {
		t.Fatal("native border window not created")
	}
	if !frame && border != 0 {
		t.Fatal("disabled border was created")
	}
	h, _ := strconv.ParseUint(normal.Handle, 10, 64)
	if frame {
		before := getRect(border)
		windowBefore := getRect(uintptr(h))
		if ok, _, err := user32.NewProc("SetWindowPos").Call(uintptr(h), 0, uintptr(windowBefore.Left+40), uintptr(windowBefore.Top+30), 0, 0, 0x15); ok == 0 {
			t.Fatal(err)
		}
		limit = time.Now().Add(3 * time.Second)
		for {
			after := getRect(border)
			if after.Left-before.Left == 40 && after.Top-before.Top == 30 {
				break
			}
			if time.Now().After(limit) {
				t.Fatalf("border did not follow movement: before=%+v after=%+v", before, after)
			}
			time.Sleep(25 * time.Millisecond)
		}
	}
	if !isTopmost(foreign) {
		t.Fatal("foreign state changed before its pin request")
	}
	if r, err := call("window.set", map[string]any{"window": foreign, "pinned": true}); err != nil || r.Error == nil {
		t.Fatalf("foreign pin was adopted: %+v %v", r, err)
	}
	stale := normal
	stale.Created = "1"
	if r, err := call("window.set", map[string]any{"window": stale, "pinned": false}); err != nil || r.Error == nil {
		t.Fatalf("stale identity accepted: %+v %v", r, err)
	}
	if !isTopmost(normal) {
		t.Fatal("stale identity altered the window")
	}
	if os.Getenv("WINONE_AOT_CYCLE_TEST") == "1" {
		// Reuse the actual worker and owned window; never toggle user windows.
		for cycle := 0; cycle < 100; cycle++ {
			for _, pinned := range []bool{false, true} {
				if r, err := call("window.set", map[string]any{"window": normal, "pinned": pinned}); err != nil || r.Error != nil {
					t.Fatalf("cycle %d pinned=%t: %+v %v", cycle, pinned, r, err)
				}
				if isTopmost(normal) != pinned || !isTopmost(foreign) {
					t.Fatalf("cycle %d changed the wrong window state", cycle)
				}
			}
		}
		t.Log("100 unpin/pin cycles preserved the owned and foreign window states")
	}
	// WinTray may hide a pinned window. Recovery must retain ownership even
	// when the old worker crashed while that window was not visible.
	user32.NewProc("ShowWindow").Call(uintptr(h), 0)
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	<-done
	if !isTopmost(normal) {
		t.Fatal("fixture did not retain pin across worker crash")
	}
	cmd = exec.Command(exe, "--pipe", pipe, "--data-dir", filepath.Join(dir, "module"))
	cmd.Stderr = &stderr
	done = make(chan struct{})
	exitError = nil
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { exitError = cmd.Wait(); close(done) }()
	limit = time.Now().Add(10 * time.Second)
	for {
		r, err := call("snapshot", nil)
		if err == nil && r.Error == nil {
			break
		}
		if time.Now().After(limit) {
			t.Fatalf("recovery worker not ready: %v", err)
		}
		time.Sleep(25 * time.Millisecond)
	}
	if r, err := call("clear", nil); err != nil || r.Error != nil {
		t.Fatalf("recovery clear: %+v %v", r, err)
	}
	if isTopmost(normal) {
		t.Fatal("hidden pin was not recovered after worker crash")
	}
	user32.NewProc("ShowWindow").Call(uintptr(h), 4)
	if r, err := call("window.set", map[string]any{"window": normal, "pinned": true}); err != nil || r.Error != nil {
		t.Fatalf("repin: %+v %v", r, err)
	}
	if r, err := call("shutdown", nil); err != nil || r.Error != nil {
		t.Fatalf("shutdown: %+v %v", r, err)
	}
	select {
	case <-done:
		if exitError != nil {
			t.Fatalf("exit: %v %s", exitError, stderr.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("worker did not exit")
	}
	if isTopmost(normal) {
		t.Fatal("owned pin survived shutdown")
	}
	if findBorder() != 0 {
		t.Fatal("border survived worker shutdown")
	}
	if !isTopmost(foreign) {
		t.Fatal("foreign topmost state was removed")
	}
}
