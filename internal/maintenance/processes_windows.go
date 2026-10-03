//go:build windows

package maintenance

import (
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"strings"
	"unsafe"
)

func IsProductProcessName(name string) bool {
	switch strings.ToLower(name) {
	case "winone.exe", "winone.host.exe", "winone.wintray.exe", "winone.capture.exe", "winone.alwaysontop.exe":
		return true
	}
	return false
}

// Toolhelp enumerates names without terminating processes or opening protected
// process handles. All copies/sessions are conservatively treated as in use.
// Only ERROR_NO_MORE_FILES proves successful enumeration to completion.
func RequireStopped() error {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return fmt.Errorf("无法检查运行中的程序：%w", err)
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	err = windows.Process32First(snapshot, &entry)
	for err == nil {
		name := windows.UTF16ToString(entry.ExeFile[:])
		if entry.ProcessID != uint32(os.Getpid()) && IsProductProcessName(name) {
			return fmt.Errorf("请先退出 WinOne 及其模块：%s", name)
		}
		err = windows.Process32Next(snapshot, &entry)
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return fmt.Errorf("无法完成程序检查：%w", err)
	}
	return nil
}
