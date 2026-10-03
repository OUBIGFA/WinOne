//go:build windows

package main

import (
	"errors"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/sys/windows"
	"math"
	"unsafe"
	"winone/internal/capturehandoff"
)

func surfaceViewportMatches(bounds surfaceRect, width, height, scale float64) bool {
	// CSS viewport dimensions are integral while Windows scaling may be fractional.
	return !math.IsNaN(scale) && !math.IsInf(scale, 0) && scale > 0 && width > 0 && height > 0 && math.Abs(width*scale-float64(bounds.Right-bounds.Left)) <= 1 && math.Abs(height*scale-float64(bounds.Bottom-bounds.Top)) <= 1
}

func (a *App) BeginCaptureSurfaceProbe(nonce string) (map[string]int32, error) {
	if a.startupProbe == nil || nonce == "" || nonce != a.startupProbe.nonce {
		return nil, errors.New("no matching desktop probe")
	}
	a.frameInitMu.Lock()
	defer a.frameInitMu.Unlock()
	if a.captureSurface != nil {
		return nil, errors.New("capture surface already active")
	}
	window, err := desktopWindowHandle()
	if err != nil {
		return nil, err
	}
	state, err := readSurfaceWindow(window)
	if err != nil {
		return nil, err
	}
	state.ID = nonce
	state.Bounds = virtualDesktopBounds()
	a.captureSurface = state
	if err = state.enter(); err != nil {
		restore := state.restore()
		if restore == nil {
			a.captureSurface = nil
		}
		return nil, errors.Join(err, restore)
	}
	return map[string]int32{"width": state.Bounds.Right - state.Bounds.Left, "height": state.Bounds.Bottom - state.Bounds.Top}, nil
}

func (a *App) EndCaptureSurfaceProbe(nonce string, width, height, scale float64) error {
	if a.startupProbe == nil || nonce == "" || nonce != a.startupProbe.nonce {
		return errors.New("no matching desktop probe")
	}
	a.frameInitMu.Lock()
	defer a.frameInitMu.Unlock()
	state := a.captureSurface
	if state == nil || state.ID != nonce {
		return errors.New("capture surface probe missing")
	}
	matches := surfaceViewportMatches(state.Bounds, width, height, scale)
	if err := state.restore(); err != nil {
		return err
	}
	a.captureSurface = nil
	restored, err := readSurfaceWindow(state.Window)
	if err != nil {
		return err
	}
	if !matches || restored.Placement.Normal != state.Placement.Normal || restored.Topmost != state.Topmost || restored.Style&0x10000000 != state.Style&0x10000000 {
		return errors.New("capture viewport or window restoration mismatch")
	}
	a.startupProbe.mu.Lock()
	a.startupProbe.surfaceReady = true
	a.startupProbe.mu.Unlock()
	return nil
}

type surfaceRect struct {
	Left   int32 `json:"left"`
	Top    int32 `json:"top"`
	Right  int32 `json:"right"`
	Bottom int32 `json:"bottom"`
}

type surfaceCrop struct{ X, Y, Width, Height int32 }

// The stored image can span all monitors; place controls on the selected one.
func captureSurfaceFocus(p captureSurfaceGeometry, width, height int32, crop *surfaceCrop) (int32, int32, error) {
	x, y := int64(p.X)+int64(width)/2, int64(p.Y)+int64(height)/2
	if crop != nil {
		if crop.X < 0 || crop.Y < 0 || crop.Width < 1 || crop.Height < 1 || int64(crop.X)+int64(crop.Width) > int64(width) || int64(crop.Y)+int64(crop.Height) > int64(height) {
			return 0, 0, errors.New("截图选区无效")
		}
		x, y = int64(p.X)+int64(crop.X)+int64(crop.Width)/2, int64(p.Y)+int64(crop.Y)+int64(crop.Height)/2
	}
	return int32(x), int32(y), nil
}

type surfacePoint struct{ X, Y int32 }
type surfacePlacement struct {
	Length, Flags, Show uint32
	Min, Max            surfacePoint
	Normal              surfaceRect
}
type captureSurfaceGeometry struct{ X, Y, DesktopX, DesktopY, DesktopWidth, DesktopHeight int32 }
type captureSurfaceState struct {
	ID        string
	Window    uintptr
	Placement surfacePlacement
	Style     uintptr
	Topmost   bool
	Bounds    surfaceRect
	Monitor   surfaceRect
}

func (p captureSurfaceGeometry) valid(width, height int32, current surfaceRect) bool {
	right, bottom := int64(p.DesktopX)+int64(p.DesktopWidth), int64(p.DesktopY)+int64(p.DesktopHeight)
	return width > 0 && height > 0 && p.DesktopWidth > 0 && p.DesktopHeight > 0 && right <= 2147483647 && bottom <= 2147483647 &&
		p.X >= p.DesktopX && p.Y >= p.DesktopY && int64(p.X)+int64(width) <= right && int64(p.Y)+int64(height) <= bottom &&
		current == (surfaceRect{p.DesktopX, p.DesktopY, int32(right), int32(bottom)})
}

var surfaceUser32 = desktopUser32

func virtualDesktopBounds() surfaceRect {
	metric := func(index uintptr) int32 {
		value, _, _ := surfaceUser32.NewProc("GetSystemMetrics").Call(index)
		return int32(value)
	}
	x, y, w, h := metric(76), metric(77), metric(78), metric(79)
	return surfaceRect{x, y, x + w, y + h}
}

func readSurfaceWindow(window uintptr) (*captureSurfaceState, error) {
	state := &captureSurfaceState{Window: window}
	state.Placement.Length = uint32(unsafe.Sizeof(state.Placement))
	if ok, _, err := surfaceUser32.NewProc("GetWindowPlacement").Call(window, uintptr(unsafe.Pointer(&state.Placement))); ok == 0 {
		return nil, err
	}
	styleIndex, extendedIndex := int32(-16), int32(-20)
	state.Style, _, _ = surfaceUser32.NewProc("GetWindowLongPtrW").Call(window, uintptr(styleIndex))
	if state.Style == 0 {
		return nil, errors.New("无法读取 WinOne 窗口样式")
	}
	extended, _, err := surfaceUser32.NewProc("GetWindowLongPtrW").Call(window, uintptr(extendedIndex))
	if extended == 0 && err != windows.ERROR_SUCCESS {
		return nil, err
	}
	state.Topmost = extended&8 != 0
	return state, nil
}

func (state *captureSurfaceState) restore() error {
	index := int32(-16)
	surfaceUser32.NewProc("ShowWindow").Call(state.Window, 0)
	if previous, _, err := surfaceUser32.NewProc("SetWindowLongPtrW").Call(state.Window, uintptr(index), state.Style&^uintptr(0x10000000)); previous == 0 && err != windows.ERROR_SUCCESS {
		return err
	}
	top := ^uintptr(1)
	if state.Topmost {
		top = ^uintptr(0)
	}
	if ok, _, err := surfaceUser32.NewProc("SetWindowPos").Call(state.Window, top, 0, 0, 0, 0, 0x33); ok == 0 {
		return err
	}
	placement := state.Placement
	if state.Style&0x10000000 == 0 {
		placement.Show = 0
	}
	if ok, _, err := surfaceUser32.NewProc("SetWindowPlacement").Call(state.Window, uintptr(unsafe.Pointer(&placement))); ok == 0 {
		return err
	}
	return nil
}

// Entry and restoration operate only on the calling process's named window.
// A hidden transition lets React load both images before revealing the surface.
func (a *App) BeginCaptureSurface(id string) (bool, error) {
	if !a.dedicatedCapture {
		return false, errors.New("请在截图窗口中打开截图")
	}
	if err := capturehandoff.ValidateDraftID(id); err != nil {
		return false, err
	}
	if err := a.prepareDedicatedCaptureWindow(); err != nil {
		return false, err
	}
	var response struct {
		Draft *struct {
			ID                string
			Width, Height     int32
			Placement         *captureSurfaceGeometry
			DesktopPreviewID  string
			SelectionRequired bool
			Document          struct{ Crop *surfaceCrop }
		}
	}
	if err := a.request("capture.draft.get", map[string]string{"expectedId": id}, &response); err != nil {
		return false, err
	}
	if response.Draft == nil || response.Draft.ID != id {
		return false, errors.New("截图草稿不可用")
	}
	draft := response.Draft
	if draft.Placement == nil || draft.DesktopPreviewID == "" {
		return false, nil
	}
	if err := capturehandoff.ValidateDraftID(draft.DesktopPreviewID); err != nil {
		return false, err
	}
	bounds := virtualDesktopBounds()
	if !draft.Placement.valid(draft.Width, draft.Height, bounds) {
		return false, errors.New("显示布局已变化，请在普通编辑器中打开截图")
	}
	a.frameInitMu.Lock()
	defer a.frameInitMu.Unlock()
	if a.captureSurface != nil {
		if a.captureSurface.ID == id {
			return true, nil
		}
		return false, errors.New("请先完成当前截图")
	}
	window, err := desktopWindowHandle()
	if err != nil {
		return false, err
	}
	state, err := readSurfaceWindow(window)
	if err != nil {
		return false, err
	}
	if a.captureWindowBefore != nil {
		state = a.captureWindowBefore
	}
	state.ID = id
	state.Bounds = bounds
	if draft.SelectionRequired && draft.Document.Crop == nil {
		return false, errors.New("截图选区丢失")
	}
	x, y, err := captureSurfaceFocus(*draft.Placement, draft.Width, draft.Height, draft.Document.Crop)
	if err != nil {
		return false, err
	}
	point := uint64(uint32(x)) | uint64(uint32(y))<<32
	monitor, _, _ := surfaceUser32.NewProc("MonitorFromPoint").Call(uintptr(point), 2)
	info := struct {
		Size          uint32
		Monitor, Work surfaceRect
		Flags         uint32
	}{}
	info.Size = uint32(unsafe.Sizeof(info))
	if ok, _, err := surfaceUser32.NewProc("GetMonitorInfoW").Call(monitor, uintptr(unsafe.Pointer(&info))); ok == 0 {
		return false, err
	}
	state.Monitor = info.Work
	a.captureSurface = state
	rollback := func(cause error) (bool, error) {
		if err := state.restore(); err != nil {
			return false, errors.Join(cause, err)
		}
		a.captureSurface = nil
		return false, cause
	}
	if err := state.enter(); err != nil {
		return rollback(err)
	}
	return true, nil
}

func (a *App) ShowCaptureSurface(id string, width, height, scale float64) error {
	a.frameInitMu.Lock()
	defer a.frameInitMu.Unlock()
	if a.captureSurface == nil || a.captureSurface.ID != id {
		return errors.New("截图窗口已变化")
	}
	if virtualDesktopBounds() != a.captureSurface.Bounds {
		return errors.New("显示布局已变化，请退出并重新打开截图")
	}
	if !surfaceViewportMatches(a.captureSurface.Bounds, width, height, scale) {
		return errors.New("截图画布尺寸尚未就绪")
	}
	// Wails also transfers keyboard focus to WebView2 after showing the window.
	runtime.WindowShow(a.windowContext())
	return nil
}

func (a *App) EndCaptureSurface(id string) error {
	a.frameInitMu.Lock()
	defer a.frameInitMu.Unlock()
	if a.captureSurface == nil {
		return nil
	}
	if a.captureSurface.ID != id {
		return errors.New("截图窗口已变化")
	}
	if !a.dedicatedCapture {
		return errors.New("当前窗口不是截图窗口")
	}
	// The screenshot is closed in its original geometry. Nothing is resized or
	// revealed as a settings page between the last frame and destruction.
	surfaceUser32.NewProc("ShowWindow").Call(a.captureSurface.Window, 0)
	a.captureSurface = nil
	a.captureWindowBefore = nil
	return nil
}

// Keep restoration state for a retry, but expose the error in this app's own
// window rather than leaving a failed restoration hidden.
func (a *App) ShowCaptureRecovery(id string) error {
	a.frameInitMu.Lock()
	defer a.frameInitMu.Unlock()
	if a.captureSurface == nil || a.captureSurface.ID != id {
		return errors.New("截图窗口已变化")
	}
	window, err := desktopWindowHandle()
	if err != nil {
		return err
	}
	if window != a.captureSurface.Window {
		return errors.New("截图窗口已变化")
	}
	ctx := a.windowContext()
	if ctx == nil {
		return errors.New("窗口尚未就绪")
	}
	runtime.WindowShow(ctx)
	return nil
}

func (a *App) isCaptureOnly() bool { a.ctxMu.RLock(); defer a.ctxMu.RUnlock(); return a.captureOnly }
func (a *App) CaptureSurfaceMonitor(id string) (surfaceRect, error) {
	var response struct {
		Draft *struct {
			ID            string
			Width, Height int32
			Placement     *captureSurfaceGeometry
			Document      struct{ Crop *surfaceCrop }
		}
	}
	if err := a.request("capture.draft.get", map[string]string{"expectedId": id}, &response); err != nil {
		return surfaceRect{}, err
	}
	draft := response.Draft
	if draft == nil || draft.ID != id || draft.Placement == nil {
		return surfaceRect{}, errors.New("截图草稿不可用")
	}
	x, y, err := captureSurfaceFocus(*draft.Placement, draft.Width, draft.Height, draft.Document.Crop)
	if err != nil {
		return surfaceRect{}, err
	}
	point := uint64(uint32(x)) | uint64(uint32(y))<<32
	monitor, _, _ := surfaceUser32.NewProc("MonitorFromPoint").Call(uintptr(point), 2)
	info := struct {
		Size          uint32
		Monitor, Work surfaceRect
		Flags         uint32
	}{}
	info.Size = uint32(unsafe.Sizeof(info))
	if ok, _, err := surfaceUser32.NewProc("GetMonitorInfoW").Call(monitor, uintptr(unsafe.Pointer(&info))); ok == 0 {
		return surfaceRect{}, err
	}
	a.frameInitMu.Lock()
	defer a.frameInitMu.Unlock()
	if a.captureSurface == nil || a.captureSurface.ID != id {
		return surfaceRect{}, errors.New("截图窗口已变化")
	}
	// Selecting a region can move toolbar placement to another monitor without
	// hiding, resizing or recreating the screenshot window.
	a.captureSurface.Monitor = info.Work
	return info.Work, nil
}

func (a *App) rememberCaptureWindow() error {
	a.frameInitMu.Lock()
	defer a.frameInitMu.Unlock()
	if a.captureSurface != nil || a.captureWindowBefore != nil {
		return nil
	}
	window, err := desktopWindowHandle()
	if err != nil {
		return err
	}
	a.captureWindowBefore, err = readSurfaceWindow(window)
	return err
}

// Wails 2.14 adds one native bottom pixel and removes it from WebView padding.
// The probe additionally checks the actual browser viewport.
func (state *captureSurfaceState) enter() error {
	window, bounds := state.Window, state.Bounds
	surfaceUser32.NewProc("ShowWindow").Call(window, 0)
	index := int32(-16)
	// Hide and remove maximized/minimized state without briefly showing a normal window.
	if previous, _, err := surfaceUser32.NewProc("SetWindowLongPtrW").Call(window, uintptr(index), state.Style&^uintptr(0x31C40000)); previous == 0 && err != windows.ERROR_SUCCESS {
		return err
	}
	if ok, _, err := surfaceUser32.NewProc("SetWindowPos").Call(window, ^uintptr(0), uintptr(bounds.Left), uintptr(bounds.Top), uintptr(bounds.Right-bounds.Left), uintptr(bounds.Bottom-bounds.Top), 0x30); ok == 0 {
		return err
	}
	var client surfaceRect
	if ok, _, err := surfaceUser32.NewProc("GetClientRect").Call(window, uintptr(unsafe.Pointer(&client))); ok == 0 {
		return err
	}
	if client.Right != bounds.Right-bounds.Left || client.Bottom != bounds.Bottom-bounds.Top && client.Bottom != bounds.Bottom-bounds.Top+1 {
		return errors.New("窗口尺寸与截图范围不一致")
	}
	return nil
}
