//go:build windows

package traybox

import (
	"errors"
	"fmt"
	"github.com/lxn/win"
	"golang.org/x/sys/windows"
	"sync"
	"unsafe"
)

var openSnapshotMu sync.Mutex
var openSnapshotHandles []win.HWND
var openSnapshotCallback = windows.NewCallback(func(hwnd uintptr, _ uintptr) uintptr {
	openSnapshotHandles = append(openSnapshotHandles, win.HWND(hwnd))
	return 1
})
var openDwmAttribute = windows.NewLazySystemDLL("dwmapi.dll").NewProc("DwmGetWindowAttribute")

// Enumerate top-level windows only, using a reusable callback. The caller pins
// the target process across snapshots; a PID by itself is not durable identity.
func snapshotOpenWindows(pid uint32) ([]openWindowState, error) {
	if pid == 0 {
		return nil, errors.New("missing target process")
	}
	openSnapshotMu.Lock()
	defer openSnapshotMu.Unlock()
	openSnapshotHandles = nil
	if err := windows.EnumWindows(openSnapshotCallback, nil); err != nil {
		return nil, err
	}
	foreground := win.GetForegroundWindow()
	var states []openWindowState
	for _, hwnd := range openSnapshotHandles {
		var owner uint32
		if win.GetWindowThreadProcessId(hwnd, &owner) == 0 || owner != pid {
			continue
		}
		state := openWindowState{handle: uintptr(hwnd), pid: pid, visible: win.IsWindowVisible(hwnd), minimized: win.IsIconic(hwnd), foreground: hwnd == foreground}
		if state.visible {
			var cloaked uint32
			hr, _, _ := openDwmAttribute.Call(uintptr(hwnd), 14, uintptr(unsafe.Pointer(&cloaked)), unsafe.Sizeof(cloaked))
			if int32(hr) < 0 {
				if !isWindow(hwnd) {
					continue
				}
				return nil, fmt.Errorf("cannot read window visibility: HRESULT 0x%08x", uint32(hr))
			}
			state.cloaked = cloaked != 0
		}
		var current uint32
		if win.GetWindowThreadProcessId(hwnd, &current) == 0 || current != pid {
			continue
		}
		states = append(states, state)
	}
	return states, nil
}
