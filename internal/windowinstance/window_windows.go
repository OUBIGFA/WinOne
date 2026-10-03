//go:build windows

// Package windowinstance locates the notification window of the locked Wails
// version. Callers must verify the returned process before acting on it.
package windowinstance

import (
	"errors"
	"golang.org/x/sys/windows"
	"unsafe"
)

var user32 = windows.NewLazySystemDLL("user32.dll")

func FindPID(instance string) (uint32, error) {
	if instance == "" {
		return 0, errors.New("empty window instance")
	}
	class, err := windows.UTF16PtrFromString("wails-app-" + instance + "-sic")
	if err != nil {
		return 0, err
	}
	title, err := windows.UTF16PtrFromString("wails-app-" + instance + "-siw")
	if err != nil {
		return 0, err
	}
	// Wails 2.14 creates a message-only notification window (HWND_MESSAGE).
	window, _, _ := user32.NewProc("FindWindowExW").Call(^uintptr(2), 0, uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(title)))
	if window == 0 {
		return 0, nil
	}
	var pid uint32
	if thread, _, err := user32.NewProc("GetWindowThreadProcessId").Call(window, uintptr(unsafe.Pointer(&pid))); thread == 0 {
		return 0, err
	}
	return pid, nil
}
