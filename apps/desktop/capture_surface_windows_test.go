//go:build windows

package main

import (
	"golang.org/x/sys/windows"
	"runtime"
	"testing"
	"unsafe"
)

func TestCaptureSurfaceFocusUsesCropOnNegativeOriginDesktop(t *testing.T) {
	p := captureSurfaceGeometry{X: -1920, Y: 0, DesktopX: -1920, DesktopY: 0, DesktopWidth: 3840, DesktopHeight: 1080}
	x, y, err := captureSurfaceFocus(p, 3840, 1080, &surfaceCrop{X: 100, Y: 100, Width: 200, Height: 100})
	if err != nil || x != -1720 || y != 150 {
		t.Fatal(x, y, err)
	}
	if _, _, err := captureSurfaceFocus(p, 3840, 1080, &surfaceCrop{X: 3800, Y: 0, Width: 200, Height: 100}); err == nil {
		t.Fatal("out-of-bounds crop accepted")
	}
}

func TestCaptureSurfaceRejectsChangedOrInvalidDesktop(t *testing.T) {
	p := captureSurfaceGeometry{X: -1800, Y: 100, DesktopX: -1920, DesktopY: 0, DesktopWidth: 3840, DesktopHeight: 1080}
	current := surfaceRect{-1920, 0, 1920, 1080}
	if !p.valid(200, 100, current) {
		t.Fatal("valid negative-origin capture rejected")
	}
	if p.valid(200, 100, surfaceRect{0, 0, 1920, 1080}) {
		t.Fatal("changed desktop accepted")
	}
	for _, bad := range []captureSurfaceGeometry{
		{X: -2000, Y: 100, DesktopX: -1920, DesktopY: 0, DesktopWidth: 3840, DesktopHeight: 1080},
		{X: 1800, Y: 100, DesktopX: -1920, DesktopY: 0, DesktopWidth: 3840, DesktopHeight: 1080},
		{X: 0, Y: 0, DesktopX: 2147483647, DesktopY: 0, DesktopWidth: 3840, DesktopHeight: 1080},
	} {
		if bad.valid(200, 100, current) {
			t.Fatal("invalid capture bounds accepted", bad)
		}
	}
}

func TestSurfaceViewportAllowsOnlyDpiRounding(t *testing.T) {
	bounds := surfaceRect{0, 0, 2560, 1440}
	if !surfaceViewportMatches(bounds, 1707, 960, 1.5) {
		t.Fatal("fractional DPI rounding rejected")
	}
	if surfaceViewportMatches(bounds, 1709, 960, 1.5) {
		t.Fatal("incorrect viewport accepted")
	}
}

func TestCaptureSurfaceRejectsInvalidIdentityBeforeAccess(t *testing.T) {
	app := &App{}
	if _, err := app.BeginCaptureSurface("../other"); err == nil {
		t.Fatal("invalid capture identity accepted")
	}
	if err := app.ShowCaptureSurface("missing", 0, 0, 1); err == nil {
		t.Fatal("missing capture surface shown")
	}
	if err := app.EndCaptureSurface("missing"); err != nil {
		t.Fatal("empty cleanup must be idempotent", err)
	}
}

func TestEditorRecoveryCannotShowAnUnrestoredSurface(t *testing.T) {
	app := &App{captureSurface: &captureSurfaceState{ID: "active"}}
	if err := app.RevealCaptureEditor(); err == nil {
		t.Fatal("unrestored surface was shown")
	}
	if app.captureSurface == nil {
		t.Fatal("recovery state was lost")
	}
}

func TestCaptureWindowRestoresHiddenOwnedWindow(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	name, _ := windows.UTF16PtrFromString("WinOne.CaptureSurface.Test")
	instance, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetModuleHandleW").Call(0)
	callback := windows.NewCallback(func(hwnd, message, wparam, lparam uintptr) uintptr {
		result, _, _ := surfaceUser32.NewProc("DefWindowProcW").Call(hwnd, message, wparam, lparam)
		return result
	})
	class := struct {
		Size, Style                        uint32
		Procedure                          uintptr
		ClassExtra, WindowExtra            int32
		Instance, Icon, Cursor, Background uintptr
		Menu, Name                         *uint16
		SmallIcon                          uintptr
	}{Procedure: callback, Instance: instance, Name: name}
	class.Size = uint32(unsafe.Sizeof(class))
	atom, _, err := surfaceUser32.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&class)))
	if atom == 0 {
		t.Fatal(err)
	}
	defer surfaceUser32.NewProc("UnregisterClassW").Call(uintptr(unsafe.Pointer(name)), instance)
	// No WS_VISIBLE and a tool-window style: the test never shows or activates it.
	window, _, err := surfaceUser32.NewProc("CreateWindowExW").Call(0x80, uintptr(unsafe.Pointer(name)), 0, 0x00CF0000, 100, 100, 640, 480, 0, 0, instance, 0)
	if window == 0 {
		t.Fatal(err)
	}
	defer surfaceUser32.NewProc("DestroyWindow").Call(window)
	bounds := func() surfaceRect {
		var rect surfaceRect
		if ok, _, err := surfaceUser32.NewProc("GetWindowRect").Call(window, uintptr(unsafe.Pointer(&rect))); ok == 0 {
			t.Fatal(err)
		}
		return rect
	}
	original := bounds()
	state, err := readSurfaceWindow(window)
	if err != nil {
		t.Fatal(err)
	}
	if ok, _, err := surfaceUser32.NewProc("SetWindowPos").Call(window, ^uintptr(0), 30, 50, 800, 600, 0x10); ok == 0 {
		t.Fatal(err)
	}
	if err = state.restore(); err != nil {
		t.Fatal(err)
	}
	if bounds() != original {
		t.Fatal("window position or size was not restored", bounds(), original)
	}
	visible, _, _ := surfaceUser32.NewProc("IsWindowVisible").Call(window)
	if visible != 0 {
		t.Fatal("restoration showed a hidden window")
	}
	if err := confirmCaptureWindowHidden(window); err != nil {
		t.Fatal("hidden owned window did not complete its presentation", err)
	}
	if err := confirmCaptureWindowHidden(0); err == nil {
		t.Fatal("missing capture window was accepted")
	}
	restored, err := readSurfaceWindow(window)
	if err != nil || restored.Topmost != state.Topmost {
		t.Fatal("topmost state was not restored", err)
	}
}
