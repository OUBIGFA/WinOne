//go:build windows

package selection

import (
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"runtime"
	"sync"
	"sync/atomic"
	"unsafe"
)

var user32 = windows.NewLazySystemDLL("user32.dll")
var nextHook = user32.NewProc("CallNextHookEx")
var activeWatcher atomic.Pointer[Watcher]
var watcherLock sync.Mutex
var mouseCallback = windows.NewCallback(mouseHook)
var keyboardCallback = windows.NewCallback(keyboardHook)
var focusCallback = windows.NewCallback(focusHook)

type Event struct {
	Origin      [2]int32
	OriginValid bool
	Kind        string
	Window      uintptr
	ProcessID   uint32
	Point       [2]int32
	Keyboard    bool
	Sequence    uint64
}
type nativeMessage struct {
	Window         uintptr
	ID             uint32
	WParam, LParam uintptr
	Time           uint32
	X, Y           int32
	Private        uint32
}
type mouseInput struct {
	X, Y              int32
	Data, Flags, Time uint32
	Extra             uintptr
}
type keyInput struct {
	Key, Scan, Flags, Time uint32
	Extra                  uintptr
}

// Watcher owns only three OS hooks and their message thread. Callbacks enqueue
// metadata; no UIA, disk, clipboard or model work runs on the input thread.
type Watcher struct {
	events     chan Event
	done       chan struct{}
	thread     uint32
	sequence   atomic.Uint64
	foreground atomic.Uintptr
	gesture    Gesture
	mu         sync.Mutex
}

func StartWatcher() (*Watcher, error) {
	watcherLock.Lock()
	if activeWatcher.Load() != nil {
		watcherLock.Unlock()
		return nil, errors.New("划词监听已启动")
	}
	w := &Watcher{events: make(chan Event, 1), done: make(chan struct{})}
	activeWatcher.Store(w)
	watcherLock.Unlock()
	ready := make(chan error, 1)
	go w.run(ready)
	if err := <-ready; err != nil {
		<-w.done
		return nil, err
	}
	return w, nil
}
func (w *Watcher) Events() <-chan Event         { return w.events }
func (w *Watcher) Current(sequence uint64) bool { return w.sequence.Load() == sequence }
func (w *Watcher) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	select {
	case <-w.done:
		return nil
	default:
	}
	if ok, _, err := user32.NewProc("PostThreadMessageW").Call(uintptr(w.thread), 0x12, 0, 0); ok == 0 {
		return fmt.Errorf("无法停止划词监听: %w", err)
	}
	<-w.done
	return nil
}
func (w *Watcher) run(ready chan<- error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer func() { activeWatcher.CompareAndSwap(w, nil); close(w.done) }()
	var msg nativeMessage
	user32.NewProc("PeekMessageW").Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, 0)
	w.thread = windows.GetCurrentThreadId()
	dx, _, _ := user32.NewProc("GetSystemMetrics").Call(68)
	dy, _, _ := user32.NewProc("GetSystemMetrics").Call(69)
	double, _, _ := user32.NewProc("GetDoubleClickTime").Call()
	w.gesture = Gesture{DragX: int32(dx), DragY: int32(dy), DoubleClickMilliseconds: uint32(double)}
	set := user32.NewProc("SetWindowsHookExW")
	mouse, _, err := set.Call(14, mouseCallback, 0, 0)
	if mouse == 0 {
		ready <- err
		return
	}
	defer user32.NewProc("UnhookWindowsHookEx").Call(mouse)
	keyboard, _, err := set.Call(13, keyboardCallback, 0, 0)
	if keyboard == 0 {
		ready <- err
		return
	}
	defer user32.NewProc("UnhookWindowsHookEx").Call(keyboard)
	focus, _, err := user32.NewProc("SetWinEventHook").Call(3, 3, 0, focusCallback, 0, 0, 2)
	if focus == 0 {
		ready <- err
		return
	}
	defer user32.NewProc("UnhookWinEvent").Call(focus)
	ready <- nil
	for {
		result, _, _ := user32.NewProc("GetMessageW").Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(result) <= 0 {
			return
		}
		user32.NewProc("TranslateMessage").Call(uintptr(unsafe.Pointer(&msg)))
		user32.NewProc("DispatchMessageW").Call(uintptr(unsafe.Pointer(&msg)))
	}
}
func Foreground() (uintptr, uint32) {
	window, _, _ := user32.NewProc("GetForegroundWindow").Call()
	var pid uint32
	user32.NewProc("GetWindowThreadProcessId").Call(window, uintptr(unsafe.Pointer(&pid)))
	return window, pid
}
func modifier(key uintptr) bool {
	value, _, _ := user32.NewProc("GetAsyncKeyState").Call(key)
	return value&0x8000 != 0
}
func (w *Watcher) emit(event Event) {
	if event.Kind == "selection" {
		w.foreground.Store(event.Window)
	}
	event.Sequence = w.sequence.Add(1)
	// Latest input wins, including cancellation. A backlog must not reopen a
	// toolbar for text selected before the user clicked somewhere else.
	select {
	case w.events <- event:
	default:
		select {
		case <-w.events:
		default:
		}
		select {
		case w.events <- event:
		default:
		}
	}
}
func mouseHook(code int32, message uintptr, data unsafe.Pointer) uintptr {
	w := activeWatcher.Load()
	if code >= 0 && w != nil {
		input := (*mouseInput)(data)
		// Same-integrity accessibility and macro input is still a selection.
		// WinOne never synthesizes copy keys, so this cannot create a copy loop.
		if input.Flags&2 == 0 {
			window, pid := Foreground()
			event := Event{Kind: "dismiss", Window: window, ProcessID: pid, Point: [2]int32{input.X, input.Y}}
			switch message {
			case 0x201:
				w.gesture.Pointer(true, input.X, input.Y, input.Time, window, modifier(0x10))
				w.emit(event)
			case 0x202:
				if w.gesture.Pointer(false, input.X, input.Y, input.Time, window, modifier(0x10)) {
					event.Origin = [2]int32{w.gesture.x, w.gesture.y}
					event.OriginValid = true
					event.Kind = "selection"
					w.emit(event)
				}
			case 0x204, 0x207, 0x20A, 0x20E:
				w.emit(event)
			}
		}
	}
	result, _, _ := nextHook.Call(0, uintptr(code), message, uintptr(data))
	return result
}
func keyboardHook(code int32, message uintptr, data unsafe.Pointer) uintptr {
	w := activeWatcher.Load()
	if code >= 0 && w != nil {
		input := (*keyInput)(data)
		if input.Flags&2 == 0 {
			window, pid := Foreground()
			event := Event{Kind: "dismiss", Window: window, ProcessID: pid, Keyboard: true}
			if message == 0x100 || message == 0x104 {
				w.emit(event)
			}
			if (message == 0x101 || message == 0x105) && SelectionKey(input.Key, modifier(0x10), modifier(0x11), modifier(0x12)) {
				event.Kind = "selection"
				w.emit(event)
			}
		}
	}
	result, _, _ := nextHook.Call(0, uintptr(code), message, uintptr(data))
	return result
}
func focusHook(hook, event, window, object, child, thread, stamp uintptr) uintptr {
	if w := activeWatcher.Load(); w != nil {
		current, _ := Foreground()
		w.emitFocus(window, current)
	}
	return 0
}

func (w *Watcher) emitFocus(window, current uintptr) {
	// WinEvent delivery can lag behind mouse-up/UIA. A notification for the
	// foreground window already sampled by that selection is not a focus loss.
	if window != current || w.foreground.Swap(window) == window {
		return
	}
	w.emit(Event{Kind: "focus", Window: window})
}
