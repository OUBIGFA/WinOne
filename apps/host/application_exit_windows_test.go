//go:build windows

package main

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
	"unsafe"
	"winone/internal/residency"
	"winone/internal/windowinstance"
)

func TestApplicationExitClosesRealSettings(t *testing.T) {
	bundle := os.Getenv("WINONE_EXIT_BUNDLE")
	if bundle == "" {
		t.Skip("explicit private-desktop packaged settings test")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	desktop, _, _ := exitUser32.NewProc("GetThreadDesktop").Call(uintptr(windows.GetCurrentThreadId()))
	var name [256]uint16
	var needed uint32
	ok, _, _ := exitUser32.NewProc("GetUserObjectInformationW").Call(desktop, 2, uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)*2), uintptr(unsafe.Pointer(&needed)))
	if ok == 0 || !strings.HasPrefix(windows.UTF16ToString(name[:]), "WinOne-Resource-") {
		t.Fatal("Use a private desktop")
	}
	directory := t.TempDir()
	pipe := fmt.Sprintf(`\\.\pipe\WinOne-exit-test-%d`, time.Now().UnixNano())
	// Headless Host supplies settings without requiring Explorer's tray.
	host := exec.Command(filepath.Join(bundle, "WinOne.Host.exe"), "--headless", "--pipe", pipe, "--data-dir", directory)
	if err := host.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = host.Process.Kill(); _ = host.Wait() }()
	ui := exec.Command(filepath.Join(bundle, "WinOne.exe"), "--standard-session", "--open-settings", "--pipe", pipe, "--data-dir", directory)
	if err := ui.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ui.Process.Kill(); _ = ui.Wait() }()
	// A second profile shares the executable but belongs to another Host.
	otherDirectory := t.TempDir()
	otherPipe := pipe + "-other"
	otherHost := exec.Command(filepath.Join(bundle, "WinOne.Host.exe"), "--headless", "--pipe", otherPipe, "--data-dir", otherDirectory)
	if err := otherHost.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = otherHost.Process.Kill(); _ = otherHost.Wait() }()
	otherUI := exec.Command(filepath.Join(bundle, "WinOne.exe"), "--standard-session", "--open-settings", "--pipe", otherPipe, "--data-dir", otherDirectory)
	if err := otherUI.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = otherUI.Process.Kill(); _ = otherUI.Wait() }()
	otherInstance := fmt.Sprintf("winone-desktop-%x", sha256.Sum256([]byte(otherPipe)))
	otherDeadline := time.Now().Add(20 * time.Second)
	for {
		pid, err := windowinstance.FindPID(otherInstance)
		if err != nil {
			t.Fatal(err)
		}
		if pid == uint32(otherUI.Process.Pid) {
			break
		}
		if time.Now().After(otherDeadline) {
			t.Fatal("Other profile did not initialize")
		}
		time.Sleep(50 * time.Millisecond)
	}
	instance := fmt.Sprintf("winone-desktop-%x", sha256.Sum256([]byte(pipe)))
	deadline := time.Now().Add(20 * time.Second)
	for {
		pid, err := windowinstance.FindPID(instance)
		if err != nil {
			t.Fatal(err)
		}
		if pid == uint32(ui.Process.Pid) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Settings instance did not initialize")
		}
		time.Sleep(50 * time.Millisecond)
	}
	// Wait for the actual settings HWND rather than assuming the notification
	// window means Wails has finished creating its main window.
	for {
		exitWindowsMu.Lock()
		exitWindows = nil
		enumErr := windows.EnumWindows(enumExitWindows, nil)
		var owned uintptr
		for _, window := range exitWindows {
			var pid uint32
			exitUser32.NewProc("GetWindowThreadProcessId").Call(window, uintptr(unsafe.Pointer(&pid)))
			if pid == uint32(ui.Process.Pid) {
				owned = window
				break
			}
		}
		exitWindowsMu.Unlock()
		if enumErr != nil {
			t.Fatal(enumErr)
		}
		if owned != 0 {
			// Hidden settings remain part of this Host session.
			exitUser32.NewProc("ShowWindow").Call(owned, 0)
			if visible, _, _ := exitUser32.NewProc("IsWindowVisible").Call(owned); visible != 0 {
				t.Fatal("Fixture window did not hide")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Settings native window did not initialize")
		}
		time.Sleep(50 * time.Millisecond)
	}
	// A UI may connect from another installation. Session ownership, rather
	// than matching directories, determines which settings window must close.
	otherInstallation := filepath.Join(t.TempDir(), "WinOne.Host.exe")
	for {
		err := closeSettingsForExit(otherInstallation, pipe)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal(err)
		}
		time.Sleep(50 * time.Millisecond)
	}
	otherPID, otherErr := windowinstance.FindPID(otherInstance)
	if otherErr != nil || otherPID != uint32(otherUI.Process.Pid) {
		t.Fatal("Closing one profile closed another", otherPID, otherErr)
	}
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(ui.Process.Pid))
	if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(handle)
	if result, err := windows.WaitForSingleObject(handle, 1000); err != nil || result != windows.WAIT_OBJECT_0 {
		t.Fatal("Settings survived close", result, err)
	}
}

func TestApplicationExitWaitsForSettingsBeforeModules(t *testing.T) {
	var calls []string
	err := prepareApplicationExit(func() error { calls = append(calls, "settings"); return nil }, func() error { calls = append(calls, "modules"); return nil })
	if err != nil || !reflect.DeepEqual(calls, []string{"settings", "modules"}) {
		t.Fatal(calls, err)
	}
}
func TestApplicationExitKeepsModulesWhenSettingsRefuseClose(t *testing.T) {
	refused := errors.New("unsaved settings")
	called := false
	err := prepareApplicationExit(func() error { return refused }, func() error { called = true; return nil })
	if !errors.Is(err, refused) || called {
		t.Fatal("Exit bypassed settings protection", err)
	}
}

func TestApplicationExitPreservesModuleRecoveryFailure(t *testing.T) {
	failure := errors.New("tray restoration failed")
	err := prepareApplicationExit(func() error { return nil }, func() error { return failure })
	if !errors.Is(err, failure) {
		t.Fatal("Module recovery failure was reported as successful exit", err)
	}
}

func TestApplicationExitDoesNotIgnoreStartingSettings(t *testing.T) {
	pipe := fmt.Sprintf("WinOne-starting-settings-%d", time.Now().UnixNano())
	release, err := residency.MarkWindow(pipe)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if err := closeSettingsForExit(filepath.Join(t.TempDir(), "WinOne.Host.exe"), pipe); err == nil {
		t.Fatal("Starting settings process was reported as already closed")
	}
}
