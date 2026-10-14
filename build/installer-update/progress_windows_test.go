//go:build windows

package main

import (
	"os"
	"runtime"
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestInstallerProgressUpdatesOnlyOwnedLabel(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	class, _ := windows.UTF16PtrFromString("Static")
	window, _, err := progressUser32.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(class)), 0, 0, 0, 0, 100, 20, 0, 0, 0, 0)
	if window == 0 {
		t.Fatal(err)
	}
	defer progressUser32.NewProc("DestroyWindow").Call(window)
	pid := uint32(os.Getpid())
	if _, err := installerProgress(window, pid+1, 1033); err == nil {
		t.Fatal("another process's label accepted")
	}
	update, err := installerProgress(window, pid, 2052)
	if err != nil {
		t.Fatal(err)
	}
	update("rolling-back")
	var text [256]uint16
	progressUser32.NewProc("GetWindowTextW").Call(window, uintptr(unsafe.Pointer(&text[0])), uintptr(len(text)))
	if got := windows.UTF16ToString(text[:]); !strings.Contains(got, "恢复原版本") || strings.Contains(got, "100%") {
		t.Fatalf("rollback label is not truthful: %s", got)
	}
	if err := setInstallerProgress(window, pid+1, "wrong owner"); err == nil {
		t.Fatal("reassigned label accepted")
	}
}
