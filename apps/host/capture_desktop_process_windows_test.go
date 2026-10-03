//go:build windows

package main

import (
	"bufio"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"
	"winone/internal/capturehandoff"
)

// A real message-only window stands in for Wails' instance notification
// endpoint. It never creates a visible window or changes desktop focus.
func TestCaptureWindowIdentityHelper(t *testing.T) {
	pipe := os.Getenv("WINONE_CAPTURE_IDENTITY_HELPER")
	if pipe == "" {
		return
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	user32 := windows.NewLazySystemDLL("user32.dll")
	callback := windows.NewCallback(func(hwnd, msg, w, l uintptr) uintptr {
		r, _, _ := user32.NewProc("DefWindowProcW").Call(hwnd, msg, w, l)
		return r
	})
	instance, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetModuleHandleW").Call(0)
	id := capturehandoff.WindowInstanceID(pipe)
	class, _ := windows.UTF16PtrFromString("wails-app-" + id + "-sic")
	title, _ := windows.UTF16PtrFromString("wails-app-" + id + "-siw")
	wc := struct {
		Size, Style                        uint32
		Proc                               uintptr
		ClassExtra, WindowExtra            int32
		Instance, Icon, Cursor, Background uintptr
		Menu, Name                         *uint16
		SmallIcon                          uintptr
	}{Proc: callback, Instance: instance, Name: class}
	wc.Size = uint32(unsafe.Sizeof(wc))
	if atom, _, _ := user32.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&wc))); atom == 0 {
		os.Exit(9)
	}
	hwnd, _, _ := user32.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(title)), 0, 0, 0, 0, 0, ^uintptr(2), 0, instance, 0)
	if hwnd == 0 {
		os.Exit(10)
	}
	fmt.Println("ready")
	var input [1]byte
	if _, err := os.Stdin.Read(input[:]); err != nil {
		os.Exit(11)
	}
	user32.NewProc("DestroyWindow").Call(hwnd)
	user32.NewProc("UnregisterClassW").Call(uintptr(unsafe.Pointer(class)), instance)
	os.Exit(0)
}

func TestCaptureLauncherAdoptsExistingRealProcess(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	pipe := fmt.Sprintf("capture-adoption-%d", time.Now().UnixNano())
	child := exec.Command(exe, "-test.run=^TestCaptureWindowIdentityHelper$")
	child.Env = append(os.Environ(), "WINONE_CAPTURE_IDENTITY_HELPER="+pipe)
	child.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
	input, err := child.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { input.Close(); child.Process.Kill(); child.Wait() }()
	ready := make(chan string, 1)
	go func() { line, _ := bufio.NewReader(output).ReadString('\n'); ready <- strings.TrimSpace(line) }()
	select {
	case line := <-ready:
		if line != "ready" {
			t.Fatal(line)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("test window not ready")
	}
	done, err := startCaptureWindow(exe, pipe, t.TempDir(), strings.Repeat("a", 32))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		t.Fatal("existing screenshot reported completed before exit", err)
	case <-time.After(80 * time.Millisecond):
	}
	if _, err = input.Write([]byte{1}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("existing process exit was not observed")
	}
}
