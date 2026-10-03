//go:build windows

package systray

import (
	"os"
	"runtime"
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestNativeMenuPopup(t *testing.T) {
	if os.Getenv("WINONE_PRIVATE_MENU_TEST") != "1" {
		t.Skip("explicit private-desktop test")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	desktop, _, _ := u32.NewProc("GetThreadDesktop").Call(uintptr(windows.GetCurrentThreadId()))
	name := make([]uint16, 256)
	var needed uint32
	if ok, _, _ := u32.NewProc("GetUserObjectInformationW").Call(desktop, 2, uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)*2), uintptr(unsafe.Pointer(&needed))); ok == 0 || !strings.HasPrefix(windows.UTF16ToString(name), "WinOne-Resource-") {
		t.Fatal("use an isolated private desktop")
	}
	var style *nativeMenuStyle
	measured, drawn := 0, 0
	callback := windows.NewCallback(func(window uintptr, message uint32, wParam, lParam uintptr) uintptr {
		if message == 0x113 {
			u32.NewProc("KillTimer").Call(window, 1)
			u32.NewProc("EndMenu").Call()
			return 0
		}
		if style != nil {
			if result, handled := style.handleMessage(message, wParam, lParam); handled {
				if message == 0x2c {
					measured++
				}
				if message == 0x2b {
					drawn++
				}
				return result
			}
		}
		result, _, _ := pDefWindowProc.Call(window, uintptr(message), wParam, lParam)
		return result
	})
	className, _ := windows.UTF16PtrFromString("WinOnePrivateMenuStyleTest")
	instance, _, _ := pGetModuleHandle.Call(0)
	class := wndClassEx{WndProc: callback, Instance: windows.Handle(instance), ClassName: className}
	if err := class.register(); err != nil {
		t.Fatal(err)
	}
	defer class.unregister()
	owner, _, err := pCreateWindowEx.Call(0, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(className)), 0x80000000, 20, 20, 1, 1, 0, 0, instance, 0)
	if owner == 0 {
		t.Fatal(err)
	}
	defer pDestroyWindow.Call(owner)
	root, _, _ := pCreatePopupMenu.Call()
	if root == 0 {
		t.Fatal("menu allocation failed")
	}
	defer pDestroyMenu.Call(root)
	appendStyleFixture(t, windows.Handle(root), 1, "&Open WinOne", 0, 0, false)
	appendStyleFixture(t, windows.Handle(root), 2, "退出 WinOne", 0, 0, false)
	for i := 0; i < 3; i++ {
		style, err = newNativeMenuStyle(windows.Handle(root), i%2 == 0, 96)
		if err != nil {
			t.Fatal(err)
		}
		if timer, _, err := u32.NewProc("SetTimer").Call(owner, 1, 200, 0); timer == 0 {
			style.dispose()
			t.Fatal(err)
		}
		pTrackPopupMenu.Call(uintptr(style.root), 0x100, 50, 50, 0, owner, 0)
		style.dispose()
		style = nil
	}
	if measured < 6 || drawn < 6 {
		t.Fatalf("native popup did not dispatch menu callbacks: measure=%d draw=%d", measured, drawn)
	}
	t.Logf("native popup callbacks: measure=%d draw=%d; closed three owned popups", measured, drawn)
}
