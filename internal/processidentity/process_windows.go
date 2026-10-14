//go:build windows

// Package processidentity pins a verified product process while a handoff uses
// its identity. It never terminates a process or grants additional rights.
package processidentity

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"path/filepath"
	"strings"
	"sync"
)

var ErrExited = errors.New("进程已经退出")

// ErrDifferentInstallation is a verified path mismatch, not a failed identity lookup.
var ErrDifferentInstallation = errors.New("进程来自其他安装目录")

type Process struct {
	PID      uint32
	Elevated bool
	handle   windows.Handle
	once     sync.Once
}

func (p *Process) Close() { p.once.Do(func() { windows.CloseHandle(p.handle) }) }

// Wait observes this pinned process handle, including processes adopted after
// a Host restart. Cancellation never terminates the process being observed.
func (p *Process) Wait(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		state, err := windows.WaitForSingleObject(p.handle, 50)
		if err != nil {
			return err
		}
		switch state {
		case windows.WAIT_OBJECT_0:
			var code uint32
			if err := windows.GetExitCodeProcess(p.handle, &code); err != nil {
				return err
			}
			if code != 0 {
				return fmt.Errorf("进程退出代码：%d", code)
			}
			return nil
		case uint32(windows.WAIT_TIMEOUT):
		default:
			return fmt.Errorf("无法确认进程状态：%d", state)
		}
	}
}
func (p *Process) Exited() (bool, error) {
	state, err := windows.WaitForSingleObject(p.handle, 0)
	if err != nil {
		return false, err
	}
	switch state {
	case windows.WAIT_OBJECT_0:
		return true, nil
	case uint32(windows.WAIT_TIMEOUT):
		return false, nil
	default:
		return false, fmt.Errorf("无法确认进程状态：%d", state)
	}
}
func Open(pid uint32, expectedPath string) (result *Process, err error) {
	if pid == 0 || !filepath.IsAbs(expectedPath) {
		return nil, errors.New("进程身份无效")
	}
	return open(pid, expectedPath, "")
}

// OpenRelated pins a same-user process already associated with an application
// session by the caller. It verifies the executable name, not its directory.
// Use only for graceful close/observation; privilege and installation operations
// must continue using Open with the exact expected executable path.
func OpenRelated(pid uint32, executableName string) (*Process, error) {
	if pid == 0 || executableName == "" || filepath.Base(executableName) != executableName || strings.ContainsAny(executableName, `/\:`) {
		return nil, errors.New("进程身份无效")
	}
	return open(pid, "", executableName)
}

func open(pid uint32, expectedPath, executableName string) (result *Process, err error) {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, pid)
	if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
		return nil, ErrExited
	}
	if err != nil {
		return nil, err
	}
	process := &Process{PID: pid, handle: handle}
	defer func() {
		if result == nil {
			process.Close()
		}
	}()
	fail := func(cause error) (*Process, error) {
		if exited, checkErr := process.Exited(); checkErr == nil && exited {
			return nil, ErrExited
		}
		return nil, cause
	}
	buffer := make([]uint16, 32768)
	size := uint32(len(buffer))
	if err = windows.QueryFullProcessImageName(handle, 0, &buffer[0], &size); err != nil {
		return fail(err)
	}
	actualPath := windows.UTF16ToString(buffer[:size])
	if executableName != "" && !strings.EqualFold(filepath.Base(actualPath), executableName) {
		return nil, ErrDifferentInstallation
	}
	if expectedPath != "" && !strings.EqualFold(comparablePath(actualPath), comparablePath(expectedPath)) {
		// The scheduled-task validator already accepts junctions and short DOS
		// paths to this installation. Resolve both sides here too, while still
		// refusing a genuinely different executable or an unreadable identity.
		actual, err := resolvedProcessPath(actualPath)
		if err != nil {
			return fail(fmt.Errorf("无法核对运行进程路径：%w", err))
		}
		expected, err := resolvedProcessPath(expectedPath)
		if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND) {
			return nil, ErrDifferentInstallation
		}
		if err != nil {
			return nil, fmt.Errorf("无法核对安装路径：%w", err)
		}
		if !strings.EqualFold(comparablePath(actual), comparablePath(expected)) {
			return nil, ErrDifferentInstallation
		}
	}
	var token windows.Token
	if err = windows.OpenProcessToken(handle, windows.TOKEN_QUERY, &token); err != nil {
		return fail(err)
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		return fail(err)
	}
	current, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	if !user.User.Sid.Equals(current.User.Sid) {
		return nil, errors.New("进程属于其他 Windows 账户")
	}
	process.Elevated = token.IsElevated()
	if exited, err := process.Exited(); err != nil {
		return nil, err
	} else if exited {
		return nil, ErrExited
	}
	return process, nil
}

// Resolve using an opened file, as the installation lease does. This handles
// junctions, short names and extended paths using Windows' own path resolver.
func resolvedProcessPath(path string) (string, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	file, err := windows.CreateFile(name, windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(file)
	buffer := make([]uint16, 32768)
	size, err := windows.GetFinalPathNameByHandle(file, &buffer[0], uint32(len(buffer)), 0)
	if err != nil {
		return "", err
	}
	if size == 0 || size >= uint32(len(buffer)) {
		return "", errors.New("进程路径无效或过长")
	}
	return windows.UTF16ToString(buffer[:size]), nil
}

// Rust can launch the same executable through a Windows extended-length path.
// QueryFullProcessImageName returns the ordinary spelling; compare equivalent
// DOS/UNC prefixes without weakening the exact installation-path check.
func comparablePath(path string) string {
	if strings.HasPrefix(strings.ToUpper(path), `\\?\UNC\`) {
		path = `\\` + path[8:]
	} else if strings.HasPrefix(path, `\\?\`) {
		path = path[4:]
	}
	return filepath.Clean(path)
}
