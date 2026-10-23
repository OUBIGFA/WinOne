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
	measured, drawn, submenuDrawn, rounded := 0, 0, 0, 0
	var frameFailure string
	var submenu windows.Handle
	rows := make(map[uintptr]menuDraw)
	paintPhase := 0
	var activeMenu windows.Handle
	callback := windows.NewCallback(func(window uintptr, message uint32, wParam, lParam uintptr) uintptr {
		if message == 0x113 {
			for window := range style.frames {
				if alive, _, _ := u32.NewProc("IsWindow").Call(window); alive != 0 {
					verifyPopupArrowPixels(t, style, window, rows)
				}
			}
			if paintPhase == 0 {
				// Exercise USER32's incremental hover repaint as well as initial
				// WM_PAINT/WM_PRINTCLIENT. A stale clipping region must not leave
				// the old unselected surface behind in the trailing gutter.
				u32.NewProc("HiliteMenuItem").Call(window, uintptr(activeMenu), 1, 0x480)
				paintPhase++
				return 0
			}
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
					item := *(*menuDraw)(unsafe.Pointer(lParam))
					rows[item.Data] = item
					drawn++
					// Verify DWM's actual window attribute, not the buffered DC,
					// which Windows may deliberately leave without an HWND.
					for window := range style.frames {
						if alive, _, _ := u32.NewProc("IsWindow").Call(window); alive == 0 {
							continue // The previously opened root may already be closed.
						}
						var preference uint32
						hr, _, _ := windows.NewLazySystemDLL("dwmapi.dll").NewProc("DwmGetWindowAttribute").Call(window, 33, uintptr(unsafe.Pointer(&preference)), 4)
						if int32(hr) < 0 || preference != 2 {
							frameFailure = "popup did not retain DWMWCP_ROUND"
						} else {
							rounded++
						}
					}
					if (*menuDraw)(unsafe.Pointer(lParam)).Window == uintptr(submenu) {
						submenuDrawn++
					}
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
	child, _, _ := pCreatePopupMenu.Call()
	if child == 0 {
		t.Fatal("submenu allocation failed")
	}
	grandchild, _, _ := pCreatePopupMenu.Call()
	if grandchild == 0 { t.Fatal("nested submenu allocation failed") }
	appendStyleFixture(t, windows.Handle(grandchild), 10, "Nested action", 0, 0, false)
	appendStyleFixture(t, windows.Handle(child), 9, "Restore collected program", 0, 0, false)
	appendStyleFixture(t, windows.Handle(child), 0, "Nested program actions", 0, windows.Handle(grandchild), false)
	appendStyleFixture(t, windows.Handle(root), 1, "&Open WinOne", 0, 0, false)
	appendStyleFixture(t, windows.Handle(root), 0, "Collected program", 0, windows.Handle(child), false)
	appendStyleFixture(t, windows.Handle(root), 2, "退出 WinOne", 0, 0, false)
	for i := 0; i < 3; i++ {
		style, err = newNativeMenuStyle(windows.Handle(root), i%2 == 0, 96)
		if err != nil {
			t.Fatal(err)
		}
		entry, err := readStyledMenuEntry(style.root, 1)
		if err != nil {
			style.dispose()
			t.Fatal(err)
		}
		submenu = entry.submenu
		// A private desktop has no physical keyboard focus. Exercise both native
		// menu handles directly; mnemonic and state contracts are tested separately.
		for _, menu := range []windows.Handle{style.root, submenu} {
			activeMenu, paintPhase = menu, 0
			if timer, _, err := u32.NewProc("SetTimer").Call(owner, 1, 200, 0); timer == 0 {
				style.dispose()
				t.Fatal(err)
			}
			pTrackPopupMenu.Call(uintptr(menu), 0x100, 50, 50, 0, owner, 0)
		}
		if style.frameError != nil {
			t.Fatal(style.frameError)
		}
		style.dispose()
		style = nil
	}
	if measured < 12 || drawn < 12 || submenuDrawn < 3 {
		t.Fatalf("native popup callbacks: measure=%d draw=%d submenu=%d", measured, drawn, submenuDrawn)
	}
	if frameFailure != "" || rounded < 12 {
		t.Fatalf("rounded root and submenu frames: %d; %s", rounded, frameFailure)
	}
	t.Logf("native popup callbacks: measure=%d draw=%d submenu=%d rounded=%d; opened and closed three owned menus and their submenu handles", measured, drawn, submenuDrawn, rounded)
}
