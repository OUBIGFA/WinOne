//go:build windows

package main

import (
	"errors"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/sys/windows"
	"os"
	"sync"
	"unsafe"
)

const desktopWindowClass = "WinOne.Desktop"

var desktopUser32 = windows.NewLazySystemDLL("user32.dll")

type windowBounds struct{ Width, Height, MinWidth, MinHeight int }
type WindowFrameState struct {
	Maximized bool `json:"maximized"`
}

// Wails sizes are logical pixels. Fit inside the current monitor's work area,
// reserving a margin and capping large displays to a usable settings window.
func initialWindowBounds(width, height int, dpi uint32) (windowBounds, error) {
	if width <= 0 || height <= 0 || dpi == 0 {
		return windowBounds{}, errors.New("显示区域无效")
	}
	w, h := int(int64(width)*96/int64(dpi)), int(int64(height)*96/int64(dpi))
	if w <= 0 || h <= 0 {
		return windowBounds{}, errors.New("显示区域过小")
	}
	availableW, availableH := max(1, w-min(32, w/4)), max(1, h-min(32, h/4))
	return windowBounds{min(availableW, min(1600, max(1280, w*4/5))), min(availableH, min(1000, max(840, h*4/5))), min(960, availableW), min(640, availableH)}, nil
}

// Windows callback thunks cannot be freed. Reuse one across capture sessions.
var desktopSearchMu sync.Mutex
var desktopSearchResult uintptr
var desktopEnumCallback = windows.NewCallback(func(hwnd, parameter uintptr) uintptr {
	user32 := desktopUser32
	var pid uint32
	user32.NewProc("GetWindowThreadProcessId").Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid != uint32(os.Getpid()) {
		return 1
	}
	name := make([]uint16, 256)
	user32.NewProc("GetClassNameW").Call(hwnd, uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)))
	if class := windows.UTF16ToString(name); class != desktopWindowClass && class != captureDesktopWindowClass {
		return 1
	}
	desktopSearchResult = hwnd
	return 0
})

func desktopWindowHandle() (uintptr, error) {
	desktopSearchMu.Lock()
	defer desktopSearchMu.Unlock()
	desktopSearchResult = 0
	desktopUser32.NewProc("EnumWindows").Call(desktopEnumCallback, 0)
	if desktopSearchResult == 0 {
		return 0, errors.New("应用窗口尚未就绪")
	}
	return desktopSearchResult, nil
}
func currentWindowBounds() (windowBounds, error) {
	window, err := desktopWindowHandle()
	if err != nil {
		return windowBounds{}, err
	}
	user32 := desktopUser32
	monitor, _, _ := user32.NewProc("MonitorFromWindow").Call(window, 2)
	type rect struct{ Left, Top, Right, Bottom int32 }
	info := struct {
		Size          uint32
		Monitor, Work rect
		Flags         uint32
	}{}
	info.Size = uint32(unsafe.Sizeof(info))
	if result, _, _ := user32.NewProc("GetMonitorInfoW").Call(monitor, uintptr(unsafe.Pointer(&info))); result == 0 {
		return windowBounds{}, errors.New("无法读取显示器可用区域")
	}
	dpi, _, _ := user32.NewProc("GetDpiForWindow").Call(window)
	return initialWindowBounds(int(info.Work.Right)-int(info.Work.Left), int(info.Work.Bottom)-int(info.Work.Top), uint32(dpi))
}

// InitializeWindow is idempotent across DOM-ready and React startup calls.
func (a *App) InitializeWindow() (WindowFrameState, error) {
	a.frameInitMu.Lock()
	defer a.frameInitMu.Unlock()
	ctx := a.windowContext()
	if ctx == nil {
		return WindowFrameState{}, errors.New("窗口尚未就绪")
	}
	if !a.frameInitialized {
		bounds, err := currentWindowBounds()
		if err != nil {
			runtime.WindowMaximise(ctx)
			if a.startupProbe == nil {
				runtime.WindowShow(ctx)
			}
			return WindowFrameState{}, err
		}
		runtime.WindowSetMinSize(ctx, bounds.MinWidth, bounds.MinHeight)
		runtime.WindowSetSize(ctx, bounds.Width, bounds.Height)
		runtime.WindowCenter(ctx)
		a.frameInitialized = true
		if a.startupProbe != nil {
			a.startupProbe.mu.Lock()
			a.startupProbe.frameReady = true
			a.startupProbe.mu.Unlock()
		} else if !a.isCaptureOnly() {
			runtime.WindowShow(ctx)
		}
	}
	return a.WindowFrameState()
}

func (a *App) WindowFrameState() (WindowFrameState, error) {
	ctx := a.windowContext()
	if ctx == nil {
		return WindowFrameState{}, errors.New("窗口尚未就绪")
	}
	return WindowFrameState{Maximized: runtime.WindowIsMaximised(ctx)}, nil
}

// Close uses Wails Quit, which still invokes the unsaved-settings/capture guard.
func (a *App) WindowCommand(action string) error {
	if action != "minimize" && action != "toggle-maximize" && action != "close" {
		return errors.New("不支持的窗口操作")
	}
	ctx := a.windowContext()
	if ctx == nil {
		return errors.New("窗口尚未就绪")
	}
	switch action {
	case "minimize":
		runtime.WindowMinimise(ctx)
	case "toggle-maximize":
		runtime.WindowToggleMaximise(ctx)
	case "close":
		runtime.Quit(ctx)
	}
	return nil
}
