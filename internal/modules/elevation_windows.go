//go:build windows

package modules

import (
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"path/filepath"
	"unsafe"
)

// Layout follows SHELLEXECUTEINFOW; no arbitrary executable is accepted through RPC.
type shellExecuteInfo struct {
	Size, Mask                        uint32
	Window                            windows.Handle
	Verb, File, Parameters, Directory *uint16
	Show                              int32
	Instance                          windows.Handle
	IDList                            uintptr
	Class                             *uint16
	ClassKey                          windows.Handle
	HotKey                            uint32
	Monitor, Process                  windows.Handle
}

// Test seams replace only the OS consent launch; normal worker supervision remains real.
var elevatedLaunch = launchElevated

// LaunchElevated is for fixed, trusted product helpers. UI input must never
// select its executable or supply an arbitrary command line.
func LaunchElevated(exe string, args []string) (uint32, func() error, error) {
	return launchElevated(exe, args)
}

// launchElevated is called only from an explicit asynchronous privilege transition.
// The process handle is retained so exit and failure use the same supervision as ordinary workers.
func launchElevated(exe string, args []string) (uint32, func() error, error) {
	if !filepath.IsAbs(exe) {
		return 0, nil, errors.New("管理员组件路径必须为绝对路径")
	}
	verb, _ := windows.UTF16PtrFromString("runas")
	file, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return 0, nil, err
	}
	parameters, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(args))
	if err != nil {
		return 0, nil, err
	}
	directory, err := windows.UTF16PtrFromString(filepath.Dir(exe))
	if err != nil {
		return 0, nil, err
	}
	info := shellExecuteInfo{Mask: 0x40 | 0x100 | 0x400, Verb: verb, File: file, Parameters: parameters, Directory: directory, Show: 0}
	info.Size = uint32(unsafe.Sizeof(info))
	ok, _, err := windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW").Call(uintptr(unsafe.Pointer(&info)))
	if ok == 0 {
		if errors.Is(err, windows.ERROR_CANCELLED) {
			return 0, nil, errors.New("已取消管理员授权")
		}
		return 0, nil, err
	}
	if info.Process == 0 {
		return 0, nil, errors.New("无法跟踪管理员组件进程")
	}
	pid, err := windows.GetProcessId(info.Process)
	if err != nil {
		windows.CloseHandle(info.Process)
		return 0, nil, err
	}
	wait := func() error {
		defer windows.CloseHandle(info.Process)
		state, err := windows.WaitForSingleObject(info.Process, windows.INFINITE)
		if err != nil {
			return err
		}
		if state != windows.WAIT_OBJECT_0 {
			return fmt.Errorf("管理员组件等待失败：%d", state)
		}
		var code uint32
		if err := windows.GetExitCodeProcess(info.Process, &code); err != nil {
			return err
		}
		if code != 0 {
			return fmt.Errorf("管理员组件退出代码：%d", code)
		}
		return nil
	}
	return pid, wait, nil
}
