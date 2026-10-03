//go:build windows

package main

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"io/fs"
	"winone/internal/capturehandoff"
)

const captureDesktopWindowClass = "WinOne.CaptureDesktop"

type desktopRole struct{ InstanceID, WindowClass, WebviewDirectory string }

// Window identity belongs to a responsibility, not merely to the Host pipe.
// A settings notification must never reuse a fullscreen capture window.
func desktopRoleFor(pipe string, capture bool) desktopRole {
	if capture {
		return desktopRole{capturehandoff.WindowInstanceID(pipe), captureDesktopWindowClass, "webview-capture"}
	}
	return desktopRole{fmt.Sprintf("winone-desktop-%x", sha256.Sum256([]byte(pipe))), desktopWindowClass, "webview"}
}

// Like the fullscreen surfaces in ShareX and Flameshot, a capture is a tool
// window from its first appearance, not a taskbar settings window to restore.
func (a *App) prepareDedicatedCaptureWindow() error {
	if !a.dedicatedCapture {
		return errors.New("not a screenshot window")
	}
	window, err := desktopWindowHandle()
	if err != nil {
		return err
	}
	index := int32(-20)
	style, _, err := desktopUser32.NewProc("GetWindowLongPtrW").Call(window, uintptr(index))
	if style == 0 && err != windows.ERROR_SUCCESS {
		return err
	}
	if previous, _, err := desktopUser32.NewProc("SetWindowLongPtrW").Call(window, uintptr(index), (style|0x80)&^uintptr(0x40000)); previous == 0 && err != windows.ERROR_SUCCESS {
		return err
	}
	return nil
}

type captureEntryAssets struct{ fs.FS }

func (files captureEntryAssets) Open(name string) (fs.File, error) {
	if name == "index.html" {
		name = "capture.html"
	}
	return files.FS.Open(name)
}
func desktopPageAssets(root fs.FS, capture bool) fs.FS {
	if capture {
		return captureEntryAssets{root}
	}
	return root
}
