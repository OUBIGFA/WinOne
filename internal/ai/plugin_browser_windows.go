//go:build windows

package ai

import (
	"errors"
	"golang.org/x/sys/windows"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"
)

var browserUser32 = windows.NewLazySystemDLL("user32.dll")
var pluginBrowserClosures sync.Map
var pluginBrowserCloseID atomic.Uintptr

type pluginBrowserClose struct {
	job       windows.Handle
	processes map[uint32]windows.Handle
	err       error
}

// One callback serves every completed login. Only windows inside the exact
// login Job can be closed; personal browsers are never matched by name/path.
var closePluginBrowserCallback = syscall.NewCallback(func(hwnd, data uintptr) uintptr {
	value, ok := pluginBrowserClosures.Load(data)
	if !ok {
		return 0
	}
	state := value.(*pluginBrowserClose)
	var class [64]uint16
	browserUser32.NewProc("GetClassNameW").Call(hwnd, uintptr(unsafe.Pointer(&class[0])), uintptr(len(class)))
	name := windows.UTF16ToString(class[:])
	if name != "Chrome_WidgetWin_1" && name != "MozillaWindowClass" {
		return 1
	}
	var pid uint32
	browserUser32.NewProc("GetWindowThreadProcessId").Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, pid)
	if err != nil {
		return 1
	}
	var owned int32
	result, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("IsProcessInJob").Call(uintptr(process), uintptr(state.job), uintptr(unsafe.Pointer(&owned)))
	if result == 0 || owned == 0 {
		windows.CloseHandle(process)
		return 1
	}
	if _, ok := state.processes[pid]; ok {
		windows.CloseHandle(process)
	} else {
		state.processes[pid] = process
	}
	// WM_CLOSE lets Chromium flush its authenticated profile before a request
	// reopens it on the private desktop. Hiding it would allow new tabs to flash.
	if posted, _, err := browserUser32.NewProc("PostMessageW").Call(hwnd, 0x10, 0, 0); posted == 0 {
		state.err = err
	}
	return 1
})

func closePluginLoginBrowsers(job windows.Handle) error {
	state := pluginBrowserClose{job: job, processes: map[uint32]windows.Handle{}}
	// Pass an integer token through Win32, never a Go heap pointer disguised
	// as uintptr. Concurrent logins retain separate callback state.
	id := pluginBrowserCloseID.Add(1)
	pluginBrowserClosures.Store(id, &state)
	browserUser32.NewProc("EnumWindows").Call(closePluginBrowserCallback, id)
	pluginBrowserClosures.Delete(id)
	defer func() {
		for _, process := range state.processes {
			windows.CloseHandle(process)
		}
	}()
	if state.err != nil {
		return errors.New("登录已保存，但专用浏览器无法关闭，请重试")
	}
	for _, process := range state.processes {
		status, err := windows.WaitForSingleObject(process, 10000)
		if err != nil || status != windows.WAIT_OBJECT_0 {
			return errors.New("登录已保存，但专用浏览器退出超时，请重试")
		}
	}
	return nil
}
