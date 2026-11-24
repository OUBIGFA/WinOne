//go:build windows

// Package desktopparent lets WinOne start a process under the desktop shell,
// as Explorer and Windows startup do. Such a process receives the shell's
// token instead of WinOne's (possibly elevated) one, and Task Manager no longer
// files it under WinOne. It never changes the shell or grants extra rights.
package desktopparent

import (
	"errors"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	// ErrDesktopUnavailable means Explorer has no desktop window yet, e.g.
	// early at sign-in or while Explorer restarts.
	ErrDesktopUnavailable = errors.New("Windows 桌面尚未就绪")
	// ErrDifferentAccount means the desktop belongs to another Windows account
	// than WinOne, so starting under it would switch the program's user.
	ErrDifferentAccount = errors.New("Windows 桌面属于其他账户")
)

var (
	user32                       = windows.NewLazySystemDLL("user32.dll")
	procGetShellWindow           = user32.NewProc("GetShellWindow")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
)

// Parent is an open desktop shell process usable as an explicit parent.
type Parent struct {
	handle windows.Handle
	// Elevated reports whether the desktop shell itself runs elevated, which
	// happens only when Windows runs Explorer with administrator rights.
	Elevated bool
}

// Handle is valid until Close.
func (p *Parent) Handle() windows.Handle { return p.handle }

// Close releases the parent handle; a process already created keeps running.
func (p *Parent) Close() {
	if p != nil && p.handle != 0 {
		windows.CloseHandle(p.handle)
		p.handle = 0
	}
}

// Open returns the current desktop shell process after verifying that it runs
// for the same Windows account as this process.
func Open() (*Parent, error) {
	shell, _, _ := procGetShellWindow.Call()
	if shell == 0 {
		return nil, ErrDesktopUnavailable
	}
	var pid uint32
	procGetWindowThreadProcessId.Call(shell, uintptr(unsafe.Pointer(&pid)))
	if pid == 0 {
		return nil, ErrDesktopUnavailable
	}
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_CREATE_PROCESS|windows.PROCESS_DUP_HANDLE, false, pid)
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*Parent, error) { windows.CloseHandle(process); return nil, err }
	var token windows.Token
	if err = windows.OpenProcessToken(process, windows.TOKEN_QUERY, &token); err != nil {
		return fail(err)
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		return fail(err)
	}
	owner, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return fail(err)
	}
	if !user.User.Sid.Equals(owner.User.Sid) {
		return fail(ErrDifferentAccount)
	}
	// The desktop may have been recreated while it was being opened; a stale
	// process must not become the parent of a newly started program.
	if current, _, _ := procGetShellWindow.Call(); current != shell {
		return fail(ErrDesktopUnavailable)
	}
	return &Parent{handle: process, Elevated: token.IsElevated()}, nil
}

// Apply makes an exec.Cmd start under this parent. Go duplicates the standard
// handles into the parent for the child, using PROCESS_DUP_HANDLE.
func (p *Parent) Apply(attr *syscall.SysProcAttr) {
	attr.ParentProcess = syscall.Handle(p.handle)
}

// StartupInfo returns an extended startup block naming this parent for a
// direct CreateProcess call. The caller adds EXTENDED_STARTUPINFO_PRESENT and
// calls release after CreateProcess has returned.
func (p *Parent) StartupInfo(base windows.StartupInfo) (info *windows.StartupInfoEx, release func(), err error) {
	list, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return nil, nil, err
	}
	// The container keeps this value alive until Delete.
	handle := new(windows.Handle)
	*handle = p.handle
	if err = list.Update(windows.PROC_THREAD_ATTRIBUTE_PARENT_PROCESS, unsafe.Pointer(handle), unsafe.Sizeof(*handle)); err != nil {
		list.Delete()
		return nil, nil, err
	}
	info = &windows.StartupInfoEx{StartupInfo: base, ProcThreadAttributeList: list.List()}
	info.Cb = uint32(unsafe.Sizeof(*info))
	return info, list.Delete, nil
}
