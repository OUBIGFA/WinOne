//go:build windows

package desktop

import (
	"errors"
	"syscall"

	"golang.org/x/sys/windows"
)

var (
	procEnumThreadWindows = user32.NewProc("EnumThreadWindows")
	procPostMessageW      = user32.NewProc("PostMessageW")
	procIsWindowVisible   = user32.NewProc("IsWindowVisible")
)

// windowClass is the class of MyGo's native windows (internal/windows).
const windowClass = "MyGoWindow"

// found collects the window EnumThreadWindows reports; ExitWindow runs on
// the UI thread only, so it needs no lock.
var (
	found     uintptr
	findFrame = syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
		var name [64]uint16
		if n, _ := windows.GetClassName(windows.HWND(hwnd), &name[0], int32(len(name))); n > 0 && windows.UTF16ToString(name[:n]) == windowClass {
			// Prefer the visible window; a hidden one is the window kept in
			// the background, which is still the one to close.
			if visible, _, _ := procIsWindowVisible.Call(hwnd); visible != 0 || found == 0 {
				found = hwnd
			}
		}
		return 1
	})
)

// ExitWindow ends this settings window and its process the way the Host's
// tray exit does: WM_CLOSE with wParam 1, which the window treats as an
// exit rather than a close to the background, through its usual close
// (unsaved-change guard, saved placement). A restart as administrator
// uses it once the Host committed the handoff, as the previous window
// closed itself then. Call it on the UI thread, which owns the window.
func ExitWindow() error {
	found = 0
	procEnumThreadWindows.Call(uintptr(windows.GetCurrentThreadId()), findFrame, 0)
	if found == 0 {
		return errors.New("settings window not found")
	}
	if ok, _, err := procPostMessageW.Call(found, 0x0010, 1, 0); ok == 0 { // WM_CLOSE
		return err
	}
	return nil
}

// ShellOpen opens a folder in File Explorer or a web page in the browser,
// as the previous window opened the log folder and the release page
// (ShellExecute "open"). Callers pass only validated targets.
func ShellOpen(path string) error {
	file, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	verb, _ := windows.UTF16PtrFromString("open")
	return windows.ShellExecute(0, verb, file, nil, nil, windows.SW_SHOWNORMAL)
}
