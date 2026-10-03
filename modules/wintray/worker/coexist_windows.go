//go:build windows

package worker

import (
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"unsafe"
)

// The upstream fixed mutex also detects renamed WinTray executables.
func ensureLegacyStopped() error { return ensureMutexAbsent("WinTray_SingleInstance") }
func ensureMutexAbsent(name string) error {
	key, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	handle, _, callErr := windows.NewLazySystemDLL("kernel32.dll").NewProc("OpenMutexW").Call(windows.SYNCHRONIZE, 0, uintptr(unsafe.Pointer(key)))
	if handle != 0 {
		windows.CloseHandle(windows.Handle(handle))
		return errors.New("原 WinTray 正在运行，请先退出后再接管程序")
	}
	if errors.Is(callErr, windows.ERROR_FILE_NOT_FOUND) {
		return nil
	}
	return fmt.Errorf("无法确认原 WinTray 是否退出：%w", callErr)
}
