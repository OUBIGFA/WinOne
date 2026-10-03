//go:build windows

package main

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

var captureDwmFlush = windows.NewLazySystemDLL("dwmapi.dll").NewProc("DwmFlush")

func (a *App) captureHiddenWindowReady() error {
	a.frameInitMu.Lock()
	defer a.frameInitMu.Unlock()
	state := a.captureWindowBefore
	if state == nil {
		state = a.captureSurface
	}
	if state == nil {
		return errors.New("截图窗口状态丢失")
	}
	return confirmCaptureWindowHidden(state.Window)
}

func confirmCaptureWindowHidden(window uintptr) error {
	var pid uint32
	thread, _, err := surfaceUser32.NewProc("GetWindowThreadProcessId").Call(window, uintptr(unsafe.Pointer(&pid)))
	if thread == 0 {
		return fmt.Errorf("读取截图窗口失败: %w", err)
	}
	if pid != windows.GetCurrentProcessId() {
		return errors.New("截图窗口身份不匹配")
	}
	visible, _, _ := surfaceUser32.NewProc("IsWindowVisible").Call(window)
	if visible != 0 {
		return errors.New("WinOne 窗口尚未隐藏")
	}
	// Wails 2.14 WindowHide calls ShowWindow(SW_HIDE) synchronously. Once the
	// window is hidden, wait for this application's queued presentation work.
	// https://learn.microsoft.com/windows/win32/api/dwmapi/nf-dwmapi-dwmflush
	result, _, _ := captureDwmFlush.Call()
	if int32(result) < 0 {
		return fmt.Errorf("提交截图前的窗口画面失败: 0x%08x", uint32(result))
	}
	return nil
}
