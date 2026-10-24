package main

import (
	"errors"
	"golang.org/x/sys/windows"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

// A native tool window composes Windows' standard caption and push buttons.
// It only stages an action in the normal guarded page; dropping never publishes.
type interconnectEntry struct {
	mu           sync.Mutex
	hwnd         atomic.Uintptr
	done         chan struct{}
	failure      string
	open         func([]string, bool) error
	actions      sync.Mutex
	shuttingDown atomic.Bool
}

var entryUser32 = windows.NewLazySystemDLL("user32.dll")
var entryShell32 = windows.NewLazySystemDLL("shell32.dll")

type entryClass struct {
	Size, Style                        uint32
	Procedure                          uintptr
	ClassExtra, WindowExtra            int32
	Instance, Icon, Cursor, Background uintptr
	Menu, Name                         *uint16
	SmallIcon                          uintptr
}
type entryMessage struct {
	Window         uintptr
	Message        uint32
	WParam, LParam uintptr
	Time           uint32
	X, Y           int32
	Private        uint32
}

func entryText(value string) *uint16 { return windows.StringToUTF16Ptr(value) }
func (e *interconnectEntry) state() map[string]any {
	e.mu.Lock()
	defer e.mu.Unlock()
	return map[string]any{"visible": e.hwnd.Load() != 0, "error": e.failure}
}
func (e *interconnectEntry) activate(paths []string, receive bool) {
	go func() {
		e.actions.Lock()
		defer e.actions.Unlock()
		if e.shuttingDown.Load() {
			return
		}
		if err := e.open(paths, receive); err != nil {
			e.mu.Lock()
			e.failure = err.Error()
			e.mu.Unlock()
		}
	}()
}
func (e *interconnectEntry) show(english bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.shuttingDown.Load() {
		return errors.New("互联助手正在退出")
	}
	if window := e.hwnd.Load(); window != 0 {
		entryUser32.NewProc("ShowWindow").Call(window, 5)
		return nil
	}
	if e.done != nil {
		<-e.done
	}
	ready := make(chan error, 1)
	e.done = make(chan struct{})
	done := e.done
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(done)
		instance, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetModuleHandleW").Call(0)
		name := entryText("WinOne.Interconnect.Entry")
		proc := windows.NewCallback(func(window uintptr, message uint32, wparam, lparam uintptr) uintptr {
			switch message {
			case 0x0111:
				if wparam&0xffff == 100 {
					e.activate(nil, false)
				} else if wparam&0xffff == 101 {
					e.activate(nil, true)
				}
				return 0
			case 0x0233:
				query := entryShell32.NewProc("DragQueryFileW")
				count, _, _ := query.Call(wparam, 0xffffffff, 0, 0)
				paths := []string{}
				if count <= 200 {
					for i := uintptr(0); i < count; i++ {
						length, _, _ := query.Call(wparam, i, 0, 0)
						if length > 32760 {
							continue
						}
						buffer := make([]uint16, length+1)
						query.Call(wparam, i, uintptr(unsafe.Pointer(&buffer[0])), length+1)
						paths = append(paths, windows.UTF16ToString(buffer))
					}
				}
				entryShell32.NewProc("DragFinish").Call(wparam)
				if len(paths) > 0 {
					e.activate(paths, false)
				}
				return 0
			case 0x0010:
				entryUser32.NewProc("DestroyWindow").Call(window)
				return 0
			case 0x0002:
				e.hwnd.Store(0)
				entryUser32.NewProc("PostQuitMessage").Call(0)
				return 0
			}
			result, _, _ := entryUser32.NewProc("DefWindowProcW").Call(window, uintptr(message), wparam, lparam)
			return result
		})
		cursor, _, _ := entryUser32.NewProc("LoadCursorW").Call(0, 32512)
		class := entryClass{Size: uint32(unsafe.Sizeof(entryClass{})), Procedure: proc, Instance: instance, Cursor: cursor, Background: 16, Name: name}
		atom, _, err := entryUser32.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&class)))
		if atom == 0 {
			ready <- err
			return
		}
		defer entryUser32.NewProc("UnregisterClassW").Call(uintptr(unsafe.Pointer(name)), instance)
		title, send, receive := "WinOne · 互联助手", "发送 / 拖入文件", "接收"
		if english {
			title, send, receive = "WinOne · Interconnect", "Send / drop files", "Receive"
		}
		dpi, _, _ := entryUser32.NewProc("GetDpiForSystem").Call()
		if dpi == 0 {
			dpi = 96
		}
		scale := func(value uintptr) uintptr { return value * dpi / 96 }
		create := entryUser32.NewProc("CreateWindowExW")
		window, _, err := create.Call(0x88, uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(entryText(title))), 0x00c80000, 0x80000000, 0x80000000, scale(280), scale(106), 0, 0, instance, 0)
		if window == 0 {
			ready <- err
			return
		}
		e.hwnd.Store(window)
		font, _, _ := windows.NewLazySystemDLL("gdi32.dll").NewProc("GetStockObject").Call(17)
		for i, label := range []string{send, receive} {
			button, _, buttonErr := create.Call(0, uintptr(unsafe.Pointer(entryText("BUTTON"))), uintptr(unsafe.Pointer(entryText(label))), 0x50010000, scale(10+uintptr(i)*156), scale(12), scale([]uintptr{146, 86}[i]), scale(38), window, uintptr(100+i), instance, 0)
			if button == 0 {
				entryUser32.NewProc("DestroyWindow").Call(window)
				ready <- buttonErr
				return
			}
			entryUser32.NewProc("SendMessageW").Call(button, 0x30, font, 1)
		}
		entryShell32.NewProc("DragAcceptFiles").Call(window, 1)
		entryUser32.NewProc("ShowWindow").Call(window, 5)
		ready <- nil
		var message entryMessage
		for {
			result, _, _ := entryUser32.NewProc("GetMessageW").Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0)
			if result == 0 || int32(result) == -1 {
				break
			}
			entryUser32.NewProc("TranslateMessage").Call(uintptr(unsafe.Pointer(&message)))
			entryUser32.NewProc("DispatchMessageW").Call(uintptr(unsafe.Pointer(&message)))
		}
	}()
	return <-ready
}
func (e *interconnectEntry) close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	window := e.hwnd.Load()
	if window == 0 {
		return nil
	}
	entryUser32.NewProc("PostMessageW").Call(window, 0x0010, 0, 0)
	select {
	case <-e.done:
		return nil
	case <-time.After(3 * time.Second):
		return errors.New("悬浮收件入口未能退出")
	}
}

// Hide is reversible; Host shutdown additionally drains entry callbacks so
// a queued click cannot reopen the desktop after the Host has exited.
func (e *interconnectEntry) shutdown() error {
	e.shuttingDown.Store(true)
	err := e.close()
	e.actions.Lock()
	e.actions.Unlock()
	return err
}
