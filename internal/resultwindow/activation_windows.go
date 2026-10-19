//go:build windows

package resultwindow

import (
	"errors"
	"strconv"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Activate is called by the process handling the user's click or hotkey, not
// by a background renderer. Validate again because an HWND may have been
// destroyed since the authenticated Host reply. Never change topmost state.
func (target WindowTarget) Activate() error {
	window, err := strconv.ParseUint(target.Window, 10, strconv.IntSize)
	if err != nil || window == 0 || target.ProcessID == 0 {
		return errors.New("快捷助手窗口信息无效，请重新打开")
	}
	user := windows.NewLazySystemDLL("user32.dll")
	var actual uint32
	user.NewProc("GetWindowThreadProcessId").Call(uintptr(window), uintptr(unsafe.Pointer(&actual)))
	if actual != target.ProcessID {
		return errors.New("快捷助手窗口已关闭，请重新打开")
	}
	user.NewProc("AllowSetForegroundWindow").Call(uintptr(target.ProcessID))
	if iconic, _, _ := user.NewProc("IsIconic").Call(uintptr(window)); iconic != 0 {
		user.NewProc("ShowWindow").Call(uintptr(window), 9) // SW_RESTORE
	}
	user.NewProc("SetForegroundWindow").Call(uintptr(window))
	if active, _, _ := user.NewProc("GetForegroundWindow").Call(); active != uintptr(window) {
		return errors.New("Windows 未激活快捷助手，请再次点击打开")
	}
	return nil
}
