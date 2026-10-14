//go:build windows

package engine

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"
	"winone/internal/diagnostics"
)

type Native struct {
	mu                                 sync.Mutex
	dll                                *windows.DLL
	start, pin, configure, clear, stop *windows.Proc
	owner                              string
	closed                             bool
	logID                              uintptr
	logOutput                          *diagnostics.Writer
}

var nativeLogID atomic.Uint64
var nativeLogWriters sync.Map
var nativeLogCallback = windows.NewCallback(writeNativeLog)

func writeNativeLog(id uintptr, text *byte, length uintptr) uintptr {
	value, ok := nativeLogWriters.Load(id)
	if !ok || text == nil || length > 1<<20 {
		return 1
	}
	writer := value.(*diagnostics.Writer)
	if _, err := writer.Write(unsafe.Slice(text, int(length))); err != nil {
		return 1
	}
	if _, err := writer.Write([]byte{'\n'}); err != nil {
		return 1
	}
	return 0
}

// OpenNative loads only the application-shipped DLL using restricted Windows search paths.
func OpenNative(path, data string) (*Native, error) {
	if !filepath.IsAbs(path) || !filepath.IsAbs(data) {
		return nil, fmt.Errorf("native paths must be absolute")
	}
	h, err := windows.LoadLibraryEx(path, 0, windows.LOAD_LIBRARY_SEARCH_DLL_LOAD_DIR|windows.LOAD_LIBRARY_SEARCH_SYSTEM32)
	if err != nil {
		return nil, err
	}
	n := &Native{dll: &windows.DLL{Name: path, Handle: h}, owner: fmt.Sprintf("WinOne_AOT_%x", sha256.Sum256([]byte(strings.ToLower(filepath.Clean(data)))))}
	for name, target := range map[string]**windows.Proc{"WinOneAotStartWithLogger": &n.start, "WinOneAotPin": &n.pin, "WinOneAotConfigure": &n.configure, "WinOneAotClear": &n.clear, "WinOneAotStop": &n.stop} {
		p, err := n.dll.FindProc(name)
		if err != nil {
			n.dll.Release()
			return nil, err
		}
		*target = p
	}
	dir, err := windows.UTF16PtrFromString(data)
	if err != nil {
		n.dll.Release()
		return nil, err
	}
	key, _ := windows.UTF16PtrFromString(n.owner)
	moduleDirectory := data
	if filepath.Base(data) == "native" {
		moduleDirectory = filepath.Dir(data)
	}
	n.logOutput, err = diagnostics.NewModuleWriter(moduleDirectory, "always-on-top")
	if err != nil {
		n.dll.Release()
		return nil, err
	}
	n.logID = uintptr(nativeLogID.Add(1))
	nativeLogWriters.Store(n.logID, n.logOutput)
	// One static Go callback is reused across DLL lifetimes. Only the bounded
	// writer registry changes; repeated module toggles cannot exhaust callbacks.
	result, _, _ := n.start.Call(uintptr(unsafe.Pointer(dir)), uintptr(unsafe.Pointer(key)), nativeLogCallback, n.logID)
	if err = checkResult(result); err != nil {
		n.stop.Call()
		nativeLogWriters.Delete(n.logID)
		n.logOutput.Close()
		n.dll.Release()
		return nil, err
	}
	return n, nil
}

func checkResult(value uintptr) error {
	code := uint32(value)
	if int32(code) >= 0 {
		return nil
	}
	message := "置顶操作失败"
	switch code {
	case 0x80070005:
		message = "权限不足，请使用管理员置顶模式"
	case 0x800700b7:
		message = "该窗口已由其他程序置顶"
	case 0x80070578:
		message = "窗口已关闭或发生变化"
	case 0x800704ec:
		message = "该程序在排除列表中"
	case 0x800700aa:
		message = "游戏模式下已暂停置顶"
	case 0x800705b4:
		message = "窗口操作超时，请刷新状态"
	case 0x8007139f:
		message = "目标窗口未接受置顶状态变更"
	}
	return fmt.Errorf("%s (0x%08X)", message, code)
}
func (n *Native) Apply(s Settings) error {
	if err := s.Validate(); err != nil {
		return err
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed {
		return fmt.Errorf("置顶组件已停止")
	}
	excluded, err := windows.UTF16PtrFromString(strings.Join(s.ExcludedApps, "\n"))
	if err != nil {
		return err
	}
	result, _, _ := n.configure.Call(uintptr(s.Flags()), uintptr(s.FrameThickness), uintptr(s.ColorRef()), uintptr(s.FrameOpacity), uintptr(unsafe.Pointer(excluded)))
	return checkResult(result)
}
func (n *Native) Pin(window Window, enabled bool) error {
	h, err := strconv.ParseUint(window.Handle, 10, 64)
	if err != nil {
		return err
	}
	created, err := strconv.ParseUint(window.Created, 10, 64)
	if err != nil {
		return err
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed {
		return fmt.Errorf("置顶组件已停止")
	}
	var on uintptr
	if enabled {
		on = 1
	}
	result, _, _ := n.pin.Call(uintptr(h), uintptr(window.PID), uintptr(created), on)
	return checkResult(result)
}
func (n *Native) Clear() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed {
		return nil
	}
	r, _, _ := n.clear.Call()
	return checkResult(r)
}
func (n *Native) Close() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed {
		return nil
	}
	r, _, _ := n.stop.Call()
	if err := checkResult(r); err != nil {
		return err
	}
	n.closed = true
	nativeLogWriters.Delete(n.logID)
	return errors.Join(n.logOutput.Close(), n.dll.Release())
}
