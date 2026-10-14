//go:build windows

package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"sync"
	"time"
	"unsafe"
	"winone/internal/processidentity"
	"winone/internal/residency"
	"winone/internal/windowinstance"
)

func prepareApplicationExit(closeSettings, closeModules func() error) error {
	if err := closeSettings(); err != nil {
		return err
	}
	return closeModules()
}

var exitWindowsMu sync.Mutex
var exitWindows []uintptr
var exitUser32 = windows.NewLazySystemDLL("user32.dll")

// Reuse the callback: Windows callback allocations cannot be released.
var enumExitWindows = windows.NewCallback(func(window, _ uintptr) uintptr {
	var name [256]uint16
	exitUser32.NewProc("GetClassNameW").Call(window, uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)))
	if windows.UTF16ToString(name[:]) == "WinOne.Desktop" {
		exitWindows = append(exitWindows, window)
	}
	return 1
})

func closeSettingsForExit(_ string, pipe string) error {
	// Same binary may serve several independent profiles. Resolve the exact
	// settings instance for this Host before enumerating its native window.
	expectedPID, err := windowinstance.FindPID(fmt.Sprintf("winone-desktop-%x", sha256.Sum256([]byte(pipe))))
	if err != nil {
		return err
	}
	if expectedPID == 0 {
		// Presence begins before Wails creates its notification window.
		// Do not let an in-flight settings launch outlive the Host unnoticed.
		present, err := residency.WindowOpen(pipe)
		if err != nil {
			return err
		}
		if present {
			return errors.New("设置窗口正在启动，请稍后重试退出")
		}
		return nil
	}
	exitWindowsMu.Lock()
	defer exitWindowsMu.Unlock()
	exitWindows = nil
	if err := windows.EnumWindows(enumExitWindows, nil); err != nil {
		return err
	}
	found := false
	for _, window := range exitWindows {
		var pid uint32
		exitUser32.NewProc("GetWindowThreadProcessId").Call(window, uintptr(unsafe.Pointer(&pid)))
		if pid != expectedPID {
			continue
		}
		found = true
		// This notification instance is bound to our exact Host pipe. The UI
		// may run from another installation while still using this session.
		// Pin its same-user WinOne identity before requesting normal close.
		process, err := processidentity.OpenRelated(pid, "WinOne.exe")
		if errors.Is(err, processidentity.ErrExited) {
			continue
		}
		if err != nil {
			return err
		}
		err = func() error {
			defer process.Close()
			var current uint32
			exitUser32.NewProc("GetWindowThreadProcessId").Call(window, uintptr(unsafe.Pointer(&current)))
			if current != pid {
				return errors.New("设置窗口已变化，请重试退出")
			}
			// wParam=1 requests application exit rather than close-to-background.
			// WM_CLOSE still keeps the existing unsaved-settings/capture guards;
			// older window hosts ignore the marker and retain normal close behavior.
			if ok, _, sendErr := exitUser32.NewProc("PostMessageW").Call(window, 0x10, 1, 0); ok == 0 {
				return sendErr
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := process.Wait(ctx); err != nil {
				if errors.Is(err, context.DeadlineExceeded) {
					return fmt.Errorf("设置窗口尚未退出，请检查窗口中的提示后重试：%w", err)
				}
				return fmt.Errorf("无法确认设置窗口正常退出：%w", err)
			}
			return nil
		}()
		if err != nil {
			return err
		}
	}
	if !found {
		return errors.New("设置进程仍在运行但窗口尚未就绪，请稍后重试退出")
	}
	return nil
}
