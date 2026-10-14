//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

var progressUser32 = windows.NewLazySystemDLL("user32.dll")

// The installer passes its own status label. Check ownership and class before
// sending text; this channel affects presentation only, never transaction state.
func installerProgress(window uintptr, owner uint32, language uint) (func(string), error) {
	if window == 0 {
		return nil, nil
	}
	if err := checkProgressWindow(window, owner); err != nil {
		return nil, err
	}
	return func(stage string) {
		if err := setInstallerProgress(window, owner, progressText(stage, language)); err != nil {
			// A closed/hung installer cannot be allowed to block file recovery.
			fmt.Fprintln(os.Stderr, "installer progress:", err)
		}
	}, nil
}

func checkProgressWindow(window uintptr, owner uint32) error {
	var pid uint32
	progressUser32.NewProc("GetWindowThreadProcessId").Call(window, uintptr(unsafe.Pointer(&pid)))
	var class [32]uint16
	progressUser32.NewProc("GetClassNameW").Call(window, uintptr(unsafe.Pointer(&class[0])), uintptr(len(class)))
	if owner == 0 || pid != owner || windows.UTF16ToString(class[:]) != "Static" {
		return errors.New("invalid installer progress label")
	}
	return nil
}

func setInstallerProgress(window uintptr, owner uint32, text string) error {
	if err := checkProgressWindow(window, owner); err != nil {
		return err
	}
	value, err := windows.UTF16PtrFromString(text)
	if err != nil {
		return err
	}
	var result uintptr
	ok, _, err := progressUser32.NewProc("SendMessageTimeoutW").Call(window, 0x000C, 0, uintptr(unsafe.Pointer(value)), 2, 1000, uintptr(unsafe.Pointer(&result)))
	if ok == 0 {
		return err
	}
	return nil
}

func progressText(stage string, language uint) string {
	labels := map[string][2]string{
		"verifying":              {"正在校验安装文件…", "Verifying installation files…"},
		"backing-up":             {"正在备份并准备更新文件…", "Backing up and preparing update files…"},
		"checking-configuration": {"正在检查现有配置的兼容性…", "Checking existing configuration compatibility…"},
		"restoring-resources":    {"正在关闭组件并恢复系统状态…", "Closing components and restoring system state…"},
		"applying":               {"正在替换文件并更新安装登记…", "Replacing files and updating installation registration…"},
		"checking-startup":       {"正在验证新版本能否正常启动…", "Verifying the updated application starts correctly…"},
		"rolling-back":           {"更新未完成，正在恢复原版本…", "Update did not finish. Restoring the previous version…"},
	}
	index := 1
	if language == 2052 {
		index = 0
	}
	if text, ok := labels[stage]; ok {
		return text[index]
	}
	return [2]string{"正在更新 WinOne…", "Updating WinOne…"}[index]
}
