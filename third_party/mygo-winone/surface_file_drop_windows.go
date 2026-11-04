//go:build windows && (amd64 || arm64)

// WinOne adaptation of its existing Rust component_file_drop.rs. MyGo's
// native OLE/hit testing remains unchanged. Source/license: source.json.
package windows

import (
	"fmt"
	"os"
	"sync"
	"syscall"
	"unsafe"
)

const shellDropClass = "WinOne.FileDropTarget"

var (
	shellDropOnce        sync.Once
	shellDropClassErr    error
	shellDropWindows     = map[uintptr]*surface{}
	shellDropHook        uintptr
	shellDropProc        = syscall.NewCallback(shellFileDropProc)
	shellCaptureProc     = syscall.NewCallback(shellCaptureEvent)
	procShellAcceptFiles = shell32.NewProc("DragAcceptFiles")
	procShellQueryPoint  = shell32.NewProc("DragQueryPoint")
	procShellDragFinish  = shell32.NewProc("DragFinish")
	procShellFilter      = user32.NewProc("ChangeWindowMessageFilterEx")
	procShellSetHook     = user32.NewProc("SetWinEventHook")
	procShellUnhook      = user32.NewProc("UnhookWinEvent")
	procShellAncestor    = user32.NewProc("GetAncestor")
	procShellAsyncKey    = user32.NewProc("GetAsyncKeyState")
	procShellMapPoints   = user32.NewProc("MapWindowPoints")
	procShellValidate    = user32.NewProc("ValidateRect")
)

// Ordinary Explorer cannot call an elevated OLE drop target (UIPI). The
// Shell compatibility window has no OLE target and admits ONLY the two
// legacy file-transfer messages. It exists only while external mouse capture
// holds a left-button drag; normal clicks/scroll/focus still go to MyGo.
func (s *surface) acceptShellFileDrops() {
	if !shellProcessElevated() {
		return
	}
	shellDropOnce.Do(func() {
		wc := wndClassEx{WndProc: shellDropProc, Instance: instance(), ClassName: u16(shellDropClass)}
		wc.Size = uint32(unsafe.Sizeof(wc))
		if r, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
			shellDropClassErr = err
		}
	})
	if shellDropClassErr != nil {
		fmt.Fprintln(os.Stderr, "WinOne file drop:", shellDropClassErr)
		return
	}
	child := createWindow(0x08000020, shellDropClass, "", wsChild, 0, 0, 1, 1, s.w.hwnd)
	if child == 0 {
		fmt.Fprintln(os.Stderr, "WinOne file drop: compatibility window unavailable")
		return
	}
	for _, message := range []uintptr{0x233, 0x49} {
		if ok, _, err := procShellFilter.Call(child, message, 1, 0); ok == 0 {
			procDestroyWindow.Call(child)
			fmt.Fprintln(os.Stderr, "WinOne file drop:", err)
			return
		}
	}
	shellDropWindows[child] = s
	procShellAcceptFiles.Call(child, 1)
	if shellDropHook == 0 {
		hook, _, err := procShellSetHook.Call(8, 9, 0, shellCaptureProc, 0, 0, 0)
		if hook == 0 {
			delete(shellDropWindows, child)
			procDestroyWindow.Call(child)
			fmt.Fprintln(os.Stderr, "WinOne file drop:", err)
			return
		}
		shellDropHook = hook
	}
}

func (s *surface) revokeShellFileDrops() {
	for child, owner := range shellDropWindows {
		if owner == s {
			delete(shellDropWindows, child)
			procDestroyWindow.Call(child)
		}
	}
	if len(shellDropWindows) == 0 && shellDropHook != 0 {
		procShellUnhook.Call(shellDropHook)
		shellDropHook = 0
	}
}

func shellCaptureEvent(hook, event, source, object, child, thread, when uintptr) uintptr {
	for hwnd, s := range shellDropWindows {
		if event == 9 {
			procShowWindow.Call(hwnd, 0)
			continue
		}
		owner, _, _ := procShellAncestor.Call(source, 3) // GA_ROOTOWNER
		key, _, _ := procShellAsyncKey.Call(1)
		visible, _, _ := procIsWindowVisible.Call(s.w.hwnd)
		iconic, _, _ := procIsIconic.Call(s.w.hwnd)
		if owner == s.w.hwnd || key&0x8000 == 0 || visible == 0 || iconic != 0 {
			continue
		}
		var r rect
		procGetClientRect.Call(s.w.hwnd, uintptr(unsafe.Pointer(&r)))
		procSetWindowPos.Call(hwnd, 0, 0, 0, uintptr(r.Right), uintptr(r.Bottom), swpNoActivate)
		procShowWindow.Call(hwnd, 4) // SW_SHOWNOACTIVATE
	}
	return 0
}

func shellFileDropProc(hwnd, message, wp, lp uintptr) uintptr {
	switch message {
	case 0x84:
		return 1 // HTCLIENT; hidden outside external capture
	case wmEraseBkgnd:
		return 1
	case wmPaint:
		procShellValidate.Call(hwnd, 0)
		return 0
	case 0x233:
		defer procShellDragFinish.Call(wp)
		s := shellDropWindows[hwnd]
		if s == nil || s.w.closed {
			return 0
		}
		procShowWindow.Call(hwnd, 0)
		paths := droppedHDropPaths(wp)
		if len(paths) == 0 || len(paths) > 1024 {
			return 0
		}
		var pt point
		// BOOL classifies client/nonclient, it does not signal a missing point.
		procShellQueryPoint.Call(wp, uintptr(unsafe.Pointer(&pt)))
		procShellMapPoints.Call(hwnd, s.hwnd, uintptr(unsafe.Pointer(&pt)), 1)
		var r rect
		procGetClientRect.Call(s.hwnd, uintptr(unsafe.Pointer(&r)))
		if pt.X < 0 || pt.Y < 0 || pt.X >= r.Right || pt.Y >= r.Bottom {
			return 0
		}
		// Deliver the actual native event at its drop coordinates, directly to
		// MyGo's existing hit chain. Do not synthesize mouse moves or defer to
		// Hovered: a subsequent WM_MOUSELEAVE must not change the destination.
		s.dropFiles(s.toDIP(pt.X), s.toDIP(pt.Y), paths)
		return 0
	case 0x82:
		delete(shellDropWindows, hwnd)
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, message, wp, lp)
	return r
}

func shellProcessElevated() bool {
	var token uintptr
	open := advapi32.NewProc("OpenProcessToken")
	if ok, _, _ := open.Call(^uintptr(0), 8, uintptr(unsafe.Pointer(&token))); ok == 0 {
		return false
	}
	defer procCloseHandle.Call(token)
	var elevated, needed uint32
	query := advapi32.NewProc("GetTokenInformation")
	ok, _, _ := query.Call(token, 20, uintptr(unsafe.Pointer(&elevated)), 4, uintptr(unsafe.Pointer(&needed)))
	return ok != 0 && elevated != 0
}
