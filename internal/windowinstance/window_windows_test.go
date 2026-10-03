//go:build windows

package windowinstance

import (
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"runtime"
	"testing"
	"time"
	"unsafe"
)

var testWindowCallback = windows.NewCallback(func(hwnd, msg, wparam, lparam uintptr) uintptr {
	result, _, _ := user32.NewProc("DefWindowProcW").Call(hwnd, msg, wparam, lparam)
	return result
})

func TestFindPIDUsesRealMessageOnlyWindow(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	id := fmt.Sprintf("winone-test-%d", time.Now().UnixNano())
	class, _ := windows.UTF16PtrFromString("wails-app-" + id + "-sic")
	title, _ := windows.UTF16PtrFromString("wails-app-" + id + "-siw")
	instance, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetModuleHandleW").Call(0)
	wc := struct {
		Size, Style                        uint32
		Proc                               uintptr
		ClassExtra, WindowExtra            int32
		Instance, Icon, Cursor, Background uintptr
		Menu, Name                         *uint16
		SmallIcon                          uintptr
	}{Proc: testWindowCallback, Instance: instance, Name: class}
	wc.Size = uint32(unsafe.Sizeof(wc))
	if atom, _, err := user32.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&wc))); atom == 0 {
		t.Fatal(err)
	}
	defer user32.NewProc("UnregisterClassW").Call(uintptr(unsafe.Pointer(class)), instance)
	hwnd, _, err := user32.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(title)), 0, 0, 0, 0, 0, ^uintptr(2), 0, instance, 0)
	if hwnd == 0 {
		t.Fatal(err)
	}
	defer user32.NewProc("DestroyWindow").Call(hwnd)
	pid, err := FindPID(id)
	if err != nil || pid != uint32(os.Getpid()) {
		t.Fatal(pid, err)
	}
	if pid, err := FindPID(id + "-missing"); err != nil || pid != 0 {
		t.Fatal(pid, err)
	}
	if _, err := FindPID("bad\x00id"); err == nil {
		t.Fatal("invalid instance accepted")
	}
}
