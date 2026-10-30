//go:build windows

package timedpower

import (
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"unsafe"
)

var (
	user32          = windows.NewLazySystemDLL("user32.dll")
	exitWindows     = user32.NewProc("ExitWindowsEx")
	lockWorkstation = user32.NewProc("LockWorkStation")
	suspendState    = windows.NewLazySystemDLL("powrprof.dll").NewProc("SetSuspendState")
)

// shutdownFlags is independently tested without ever loading an executor.
// EWX_FORCE (not FORCEIFHUNG) matches the user's explicit force-close default.
func shutdownFlags(p Preferences) (uintptr, error) {
	var flags uintptr
	switch p.Action {
	case "shutdown":
		flags = 0x8
	case "restart":
		flags = 0x2
	case "logoff":
		flags = 0
	default:
		return 0, errors.New("不是退出会话操作")
	}
	if p.Force {
		flags |= 0x4
	}
	return flags, nil
}

// ExecuteNative uses fixed system APIs, never a shell or a scheduled OS task.
// A successful ExitWindowsEx only means Windows accepted the asynchronous request.
func ExecuteNative(p Preferences) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if p.DryRun {
		return errors.New("仅提醒任务禁止调用电源 API")
	}
	if p.Action == "lock" {
		return callBool(lockWorkstation)
	}
	if p.Action == "logoff" {
		flags, _ := shutdownFlags(p)
		return callBool(exitWindows, flags, 0x80000000)
	}
	return withShutdownPrivilege(func() error {
		if p.Action == "sleep" || p.Action == "hibernate" {
			var hibernate uintptr
			if p.Action == "hibernate" {
				hibernate = 1
			}
			return callBool(suspendState, hibernate, 0, 0)
		}
		flags, err := shutdownFlags(p)
		if err != nil {
			return err
		}
		return callBool(exitWindows, flags, 0x80000000)
	})
}
func callBool(proc *windows.LazyProc, args ...uintptr) error {
	result, _, err := proc.Call(args...)
	if result == 0 {
		if err == windows.ERROR_SUCCESS {
			return fmt.Errorf("%s 未能执行，请检查系统电源设置", proc.Name)
		}
		return fmt.Errorf("%s: %w", proc.Name, err)
	}
	return nil
}
func withShutdownPrivilege(run func() error) (result error) {
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_QUERY, &token); err != nil {
		return err
	}
	defer token.Close()
	name, _ := windows.UTF16PtrFromString("SeShutdownPrivilege")
	var luid windows.LUID
	if err := windows.LookupPrivilegeValue(nil, name, &luid); err != nil {
		return err
	}
	wanted := windows.Tokenprivileges{PrivilegeCount: 1}
	wanted.Privileges[0] = windows.LUIDAndAttributes{Luid: luid, Attributes: windows.SE_PRIVILEGE_ENABLED}
	var previous windows.Tokenprivileges
	var size uint32
	// x/sys checks ERROR_NOT_ALL_ASSIGNED even when the BOOL return is success.
	if err := windows.AdjustTokenPrivileges(token, false, &wanted, uint32(unsafe.Sizeof(previous)), &previous, &size); err != nil {
		return fmt.Errorf("无法获得关机权限，请检查 Windows 权限策略：%w", err)
	}
	defer func() {
		result = errors.Join(result, windows.AdjustTokenPrivileges(token, false, &previous, 0, nil, nil))
	}()
	return run()
}
