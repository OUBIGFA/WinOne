package resultui

import (
	"errors"
	"unsafe"
)

type selectionRect struct{ Left, Top, Right, Bottom int32 }
type selectionMonitor struct {
	Size          uint32
	Monitor, Work selectionRect
	Flags         uint32
}

// Keep larger initial results inside the current monitor's usable area. Sizes
// here are physical pixels, matching Win32; MyGo receives the normal resize
// notification and lays out the same shared content at its logical DPI size.
func fitResultWindow(window, monitor uintptr) error {
	if monitor == 0 {
		monitor, _, _ = frameUser.NewProc("MonitorFromWindow").Call(window, 2)
	}
	info := selectionMonitor{}
	info.Size = uint32(unsafe.Sizeof(info))
	if ok, _, _ := frameUser.NewProc("GetMonitorInfoW").Call(monitor, uintptr(unsafe.Pointer(&info))); ok == 0 {
		return errors.New("无法读取结果窗口屏幕范围")
	}
	var rect selectionRect
	if ok, _, _ := frameRect.Call(window, uintptr(unsafe.Pointer(&rect))); ok == 0 {
		return errors.New("无法读取结果窗口尺寸")
	}
	dpi, _, _ := frameDPI.Call(window)
	if dpi == 0 {
		dpi = 96
	}
	margin := int32(12 * dpi / 96)
	width := min(rect.Right-rect.Left, max(1, info.Work.Right-info.Work.Left-2*margin))
	height := min(rect.Bottom-rect.Top, max(1, info.Work.Bottom-info.Work.Top-2*margin))
	x := max(info.Work.Left+margin, min(rect.Left, info.Work.Right-margin-width))
	y := max(info.Work.Top+margin, min(rect.Top, info.Work.Bottom-margin-height))
	if ok, _, _ := frameUser.NewProc("SetWindowPos").Call(window, 0, uintptr(x), uintptr(y), uintptr(width), uintptr(height), 0x14); ok == 0 {
		return errors.New("无法调整结果窗口到屏幕范围")
	}
	return nil
}

// PlaceSelectionWindow uses physical screen coordinates from UIA and Win32
// monitor work areas, including negative origins. Window chrome remains native.
func PlaceSelectionWindow(window uintptr, anchor [2]int32) error {
	packed := uintptr(uint64(uint32(anchor[0])) | uint64(uint32(anchor[1]))<<32)
	monitor, _, _ := frameUser.NewProc("MonitorFromPoint").Call(packed, 2)
	info := selectionMonitor{}
	info.Size = uint32(unsafe.Sizeof(info))
	if ok, _, _ := frameUser.NewProc("GetMonitorInfoW").Call(monitor, uintptr(unsafe.Pointer(&info))); ok == 0 {
		return errors.New("无法确定划词窗口位置")
	}
	set := frameUser.NewProc("SetWindowPos")
	if ok, _, _ := set.Call(window, 0, uintptr(anchor[0]), uintptr(anchor[1]+8), 0, 0, 0x0015); ok == 0 {
		return errors.New("无法移动划词窗口")
	}
	if err := fitResultWindow(window, monitor); err != nil {
		return err
	}
	var bounds selectionRect
	if ok, _, _ := frameRect.Call(window, uintptr(unsafe.Pointer(&bounds))); ok == 0 {
		return errors.New("无法读取划词窗口尺寸")
	}
	width, height := bounds.Right-bounds.Left, bounds.Bottom-bounds.Top
	x := max(info.Work.Left, min(anchor[0], info.Work.Right-width))
	y := anchor[1] + 8
	if y+height > info.Work.Bottom {
		y = anchor[1] - height - 8
	}
	y = max(info.Work.Top, min(y, info.Work.Bottom-height))
	if ok, _, _ := set.Call(window, 0, uintptr(x), uintptr(y), 0, 0, 0x0015); ok == 0 {
		return errors.New("无法定位划词窗口")
	}
	return nil
}
func (c *Controller) DismissToolbar(point [2]int32, force bool, focusWindow uint64) {
	if !c.Toolbar {
		return
	}
	if force && focusWindow == uint64(c.Window.NativeHandle()) {
		return
	}
	if !force {
		var bounds selectionRect
		if ok, _, _ := frameRect.Call(c.Window.NativeHandle(), uintptr(unsafe.Pointer(&bounds))); ok != 0 && point[0] >= bounds.Left && point[0] < bounds.Right && point[1] >= bounds.Top && point[1] < bounds.Bottom {
			return
		}
	}
	c.Window.Close()
}
