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
	if !strings.EqualFold(filepath.Clean(windows.UTF16ToString(buffer[:size])), filepath.Clean(expectedPath)) {
		return nil, errors.New("进程来自其他安装目录")
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
