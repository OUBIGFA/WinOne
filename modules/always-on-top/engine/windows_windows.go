//go:build windows

package engine

import (
	"fmt"
	"golang.org/x/sys/windows"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"
)

var user32 = windows.NewLazySystemDLL("user32.dll")

type enumeration struct {
	owner     string
	mu        sync.Mutex
	result    []Window
	ownedOnly bool
}

var enumerationGate sync.Mutex
var currentEnumeration atomic.Pointer[enumeration]

// NewCallback allocates permanent trampolines, so enumeration must reuse one callback.
var enumerationCallback = windows.NewCallback(func(hwnd, lparam uintptr) uintptr {
	current := currentEnumeration.Load()
	if current == nil {
		return 0
	}
	item, err := windowInfo(hwnd, current.owner, current.ownedOnly)
	if current.ownedOnly && !item.Pinned {
		return 1
	}
	if err == nil && (current.ownedOnly || strings.TrimSpace(item.Title) != "") && item.PID != windows.GetCurrentProcessId() {
		current.mu.Lock()
		current.result = append(current.result, item)
		current.mu.Unlock()
	}
	return 1
})

// Handle and creation time are strings so JavaScript never rounds 64-bit identities.
type Window struct {
	Handle  string `json:"handle"`
	PID     uint32 `json:"pid"`
	Created string `json:"created"`
	Title   string `json:"title"`
	Process string `json:"process"`
	Pinned  bool   `json:"pinned"`
	Topmost bool   `json:"topmost"`
}

func windowAt(handle uintptr, owner string) (Window, error) {
	return windowInfo(handle, owner, false)
}

func windowInfo(handle uintptr, owner string, allowHidden bool) (Window, error) {
	visible, _, _ := user32.NewProc("IsWindowVisible").Call(handle)
	if visible == 0 && !allowHidden {
		return Window{}, fmt.Errorf("窗口不可见")
	}
	root, _, _ := user32.NewProc("GetAncestor").Call(handle, 2)
	if root != handle {
		return Window{}, fmt.Errorf("请选择顶层窗口")
	}
	var class [256]uint16
	user32.NewProc("GetClassNameW").Call(handle, uintptr(unsafe.Pointer(&class[0])), uintptr(len(class)))
	switch windows.UTF16ToString(class[:]) {
	case "Progman", "WorkerW", "Shell_TrayWnd", "Shell_SecondaryTrayWnd":
		return Window{}, fmt.Errorf("系统桌面窗口不支持置顶")
	}
	var pid uint32
	user32.NewProc("GetWindowThreadProcessId").Call(handle, uintptr(unsafe.Pointer(&pid)))
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return Window{}, err
	}
	defer windows.CloseHandle(process)
	var created, exit, kernel, cpu windows.Filetime
	if err = windows.GetProcessTimes(process, &created, &exit, &kernel, &cpu); err != nil {
		return Window{}, err
	}
	var path [32768]uint16
	size := uint32(len(path))
	if err = windows.QueryFullProcessImageName(process, 0, &path[0], &size); err != nil {
		return Window{}, err
	}
	var title [1024]uint16
	user32.NewProc("GetWindowTextW").Call(handle, uintptr(unsafe.Pointer(&title[0])), uintptr(len(title)))
	style, _, _ := user32.NewProc("GetWindowLongPtrW").Call(handle, ^uintptr(19))
	key, _ := windows.UTF16PtrFromString(owner)
	pinned, _, _ := user32.NewProc("GetPropW").Call(handle, uintptr(unsafe.Pointer(key)))
	return Window{Handle: strconv.FormatUint(uint64(handle), 10), PID: pid, Created: strconv.FormatUint(uint64(created.HighDateTime)<<32|uint64(created.LowDateTime), 10), Title: windows.UTF16ToString(title[:]), Process: filepath.Base(windows.UTF16ToString(path[:size])), Pinned: pinned != 0, Topmost: style&8 != 0}, nil
}

func (n *Native) Windows() []Window {
	return n.enumerate(false)
}

// OwnedWindows includes hidden windows so privilege changes can restore every pin.
func (n *Native) OwnedWindows() []Window { return n.enumerate(true) }
func (n *Native) enumerate(ownedOnly bool) []Window {
	enumerationGate.Lock()
	defer enumerationGate.Unlock()
	current := &enumeration{owner: n.owner, result: []Window{}, ownedOnly: ownedOnly}
	currentEnumeration.Store(current)
	defer currentEnumeration.Store(nil)
	user32.NewProc("EnumWindows").Call(enumerationCallback, 0)
	current.mu.Lock()
	result := current.result
	current.mu.Unlock()
	sort.Slice(result, func(i, j int) bool { return result[i].Title < result[j].Title })
	return result
}
func (n *Native) Foreground() (Window, error) {
	h, _, _ := user32.NewProc("GetForegroundWindow").Call()
	return windowAt(h, n.owner)
}
func (n *Native) Inspect(handle uintptr) (Window, error) { return windowAt(handle, n.owner) }
