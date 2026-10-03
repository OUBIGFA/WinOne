//go:build windows

package hotkeys

import (
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"runtime"
	"sync"
	"unsafe"
)

var user32 = windows.NewLazySystemDLL("user32.dll")
var register = user32.NewProc("RegisterHotKey")
var unregister = user32.NewProc("UnregisterHotKey")
var post = user32.NewProc("PostThreadMessageW")

type message struct {
	Window         uintptr
	ID             uint32
	WParam, LParam uintptr
	Time           uint32
	X, Y           int32
	Private        uint32
}
type request struct {
	bindings []Binding
	close    bool
	reply    chan outcome
}
type outcome struct {
	statuses []Status
	err      error
}

// Registrar confines Win32 registrations and message dispatch to one OS thread.
// Events never run feature work on that thread; the Host consumes them separately.
type Registrar struct {
	mu       sync.Mutex
	thread   uint32
	requests chan request
	events   chan string
	done     chan struct{}
	closed   bool
}

func New() *Registrar {
	r := &Registrar{requests: make(chan request, 1), events: make(chan string, 32), done: make(chan struct{})}
	ready := make(chan struct{})
	go r.run(ready)
	<-ready
	return r
}
func (r *Registrar) Events() <-chan string { return r.events }
func (r *Registrar) Update(bindings []Binding) ([]Status, error) {
	if err := validate(bindings); err != nil {
		return nil, err
	}
	return r.submit(request{bindings: append([]Binding{}, bindings...), reply: make(chan outcome, 1)})
}
func (r *Registrar) Close() error {
	_, err := r.submit(request{close: true, reply: make(chan outcome, 1)})
	return err
}
func (r *Registrar) submit(q request) ([]Status, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		if q.close {
			return nil, nil
		}
		return nil, errors.New("快捷键服务已退出")
	}
	select {
	case <-r.done:
		return nil, errors.New("快捷键线程已退出")
	default:
	}
	r.requests <- q
	ok, _, err := post.Call(uintptr(r.thread), 0x8001, 0, 0)
	if ok == 0 {
		<-r.requests
		return nil, fmt.Errorf("无法唤醒快捷键线程：%w", err)
	}
	select {
	case result := <-q.reply:
		if q.close {
			r.closed = true
			<-r.done
		}
		return result.statuses, result.err
	case <-r.done:
		select {
		case result := <-q.reply:
			if q.close {
				r.closed = true
			}
			return result.statuses, result.err
		default:
			return nil, errors.New("快捷键线程异常退出")
		}
	}
}
func (r *Registrar) run(ready chan struct{}) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(r.done)
	defer close(r.events)
	var msg message
	user32.NewProc("PeekMessageW").Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, 0)
	r.thread = windows.GetCurrentThreadId()
	close(ready)
	active := map[int]Binding{}
	nextID := 1
	defer func() {
		for id := range active {
			unregister.Call(0, uintptr(id))
		}
	}()
	get := user32.NewProc("GetMessageW")
	for {
		result, _, _ := get.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(result) <= 0 {
			return
		}
		switch msg.ID {
		case 0x8001:
			q := <-r.requests
			if q.close {
				q.reply <- outcome{}
				return
			}
			// Unchanged bindings retain their registrations; replacing one cannot disable another.
			desired := map[string]Binding{}
			for _, b := range q.bindings {
				desired[b.ID] = b
			}
			for id, b := range active {
				if desired[b.ID] != b {
					unregister.Call(0, uintptr(id))
					delete(active, id)
				}
			}
			statuses := make([]Status, 0, len(q.bindings))
			for _, b := range q.bindings {
				s := Status{Binding: b}
				key, _ := Parse(b.Shortcut)
				if key.Key == 0 {
					statuses = append(statuses, s)
					continue
				}
				for _, old := range active {
					if old == b {
						s.Registered = true
						break
					}
				}
				if !s.Registered {
					// Do not reuse IDs: a queued WM_HOTKEY from an old binding must
					// never dispatch the action newly assigned to the same numeric ID.
					id := nextID
					if id > 0xbfff {
						s.Error = "快捷键注册次数已达上限，请重启 WinOne"
						statuses = append(statuses, s)
						continue
					}
					ok, _, err := register.Call(0, uintptr(id), uintptr(key.Modifiers|0x4000), uintptr(key.Key))
					if ok == 0 {
						s.Error = fmt.Sprintf("快捷键被占用或不可注册：%v", err)
					} else {
						active[id] = b
						nextID++
						s.Registered = true
					}
				}
				statuses = append(statuses, s)
			}
			q.reply <- outcome{statuses: statuses}
		case 0x312:
			if b, ok := active[int(msg.WParam)]; ok {
				// Coalesce repeated presses while the consumer is busy; never block Windows dispatch.
				select {
				case r.events <- b.ID:
				default:
				}
			}
		}
	}
}
