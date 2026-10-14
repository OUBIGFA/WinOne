package resultui

import (
	"errors"
	"github.com/egoist/mygo/ui"
	"golang.org/x/sys/windows"
	"runtime"
	"unsafe"
)

type tooltipTool struct {
	size, flags     uint32
	window, id      uintptr
	rect            [4]int32
	instance        uintptr
	text            *uint16
	param, reserved uintptr
}
type tooltipText struct {
	value  string
	buffer []uint16
	pin    runtime.Pinner
	rect   [4]int32
}
type toolbarTooltip struct {
	window, target, font   uintptr
	tools                  map[uint64]*tooltipText
	dpi                    uintptr
	dark                   bool
	background, foreground uintptr
	fontHeight             int32
}

// A native common-control tooltip owns its own popup. It can extend beyond the
// 44-DIP toolbar, positions itself inside the monitor, and never takes focus.
func newToolbarTooltip(owner uintptr, t *ui.Theme) (*toolbarTooltip, error) {
	init := struct{ size, classes uint32 }{8, 0xff}
	if ok, _, _ := frameCommon.NewProc("InitCommonControlsEx").Call(uintptr(unsafe.Pointer(&init))); ok == 0 {
		return nil, errors.New("无法初始化工具提示")
	}
	class, _ := windows.UTF16PtrFromString("tooltips_class32")
	window, _, _ := frameUser.NewProc("CreateWindowExW").Call(0x08000088, uintptr(unsafe.Pointer(class)), 0, 0x80000033, 0, 0, 0, 0, owner, 0, 0, 0)
	if window == 0 {
		return nil, errors.New("无法创建工具提示窗口")
	}
	surfaceName, _ := windows.UTF16PtrFromString("MyGoSurface")
	target, _, _ := frameFind.Call(owner, 0, uintptr(unsafe.Pointer(surfaceName)), 0)
	if target == 0 {
		target = owner
	}
	tip := &toolbarTooltip{window: window, target: target, tools: map[uint64]*tooltipText{}}
	empty := uint16(0)
	windows.NewLazySystemDLL("uxtheme.dll").NewProc("SetWindowTheme").Call(window, uintptr(unsafe.Pointer(&empty)), uintptr(unsafe.Pointer(&empty)))
	tip.configure(owner, t)
	return tip, nil
}

func (t *toolbarTooltip) configure(owner uintptr, theme *ui.Theme) {
	dpi, _, _ := frameDPI.Call(owner)
	if dpi == 0 {
		dpi = 96
	}
	color := func(c ui.Color) uintptr { return uintptr(c.R) | uintptr(c.G)<<8 | uintptr(c.B)<<16 }
	background, foreground := color(theme.Surface), color(theme.Text)
	height := -int32(theme.FontSize * float32(dpi) / 96)
	if t.dpi == dpi && t.dark == theme.Dark && t.font != 0 && t.background == background && t.foreground == foreground && t.fontHeight == height {
		return
	}
	t.dpi, t.dark, t.background, t.foreground, t.fontHeight = dpi, theme.Dark, background, foreground, height
	frameSend.Call(t.window, 0x413, background, 0)
	frameSend.Call(t.window, 0x414, foreground, 0)
	frameSend.Call(t.window, 0x418, 0, uintptr(320*dpi/96))
	margin := [4]int32{int32(6 * dpi / 96), int32(4 * dpi / 96), int32(6 * dpi / 96), int32(4 * dpi / 96)}
	frameSend.Call(t.window, 0x41a, 0, uintptr(unsafe.Pointer(&margin)))
	var font struct {
		height, width, escapement, orientation, weight                                     int32
		italic, underline, strikeout, charset, outPrecision, clipPrecision, quality, pitch byte
		face                                                                               [32]uint16
	}
	font.height = height
	font.weight = 400
	font.quality = 5
	face, _ := windows.UTF16FromString("Segoe UI")
	copy(font.face[:], face)
	gdi := windows.NewLazySystemDLL("gdi32.dll")
	next, _, _ := gdi.NewProc("CreateFontIndirectW").Call(uintptr(unsafe.Pointer(&font)))
	if next != 0 {
		frameSend.Call(t.window, 0x30, next, 1)
		if t.font != 0 {
			gdi.NewProc("DeleteObject").Call(t.font)
		}
		t.font = next
	}
}

func (t *toolbarTooltip) bind(id uint64, rect ui.Rect, text string) error {
	if rect.W <= 0 || rect.H <= 0 || text == "" {
		return nil
	}
	scale := float32(t.dpi) / 96
	bounds := [4]int32{int32(rect.X * scale), int32(rect.Y * scale), int32((rect.X + rect.W) * scale), int32((rect.Y + rect.H) * scale)}
	entry := t.tools[id]
	if entry != nil && entry.value == text && entry.rect == bounds {
		return nil
	}
	info := tooltipTool{flags: 0x10, window: t.target, id: uintptr(id), rect: bounds}
	info.size = uint32(unsafe.Sizeof(info))
	if entry == nil || entry.value != text {
		if entry != nil {
			frameSend.Call(t.window, 0x433, 0, uintptr(unsafe.Pointer(&info)))
			entry.pin.Unpin()
		}
		buffer, err := windows.UTF16FromString(text)
		if err != nil {
			return err
		}
		entry = &tooltipText{value: text, buffer: buffer, rect: bounds}
		entry.pin.Pin(&entry.buffer[0])
		info.text = &entry.buffer[0]
		if ok, _, _ := frameSend.Call(t.window, 0x432, 0, uintptr(unsafe.Pointer(&info))); ok == 0 {
			entry.pin.Unpin()
			delete(t.tools, id)
			return errors.New("无法登记工具提示")
		}
		t.tools[id] = entry
	} else {
		entry.rect = bounds
		info.text = &entry.buffer[0]
		frameSend.Call(t.window, 0x434, 0, uintptr(unsafe.Pointer(&info)))
	}
	return nil
}

func (t *toolbarTooltip) close() {
	if t == nil {
		return
	}
	frameUser.NewProc("DestroyWindow").Call(t.window)
	for _, entry := range t.tools {
		entry.pin.Unpin()
	}
	t.tools = nil
	if t.font != 0 {
		windows.NewLazySystemDLL("gdi32.dll").NewProc("DeleteObject").Call(t.font)
		t.font = 0
	}
}

func (c *Controller) toolbarHint(button *ui.Element, label string, theme *ui.Theme) {
	if !c.Toolbar {
		return
	}
	if c.Window == nil {
		button.Tooltip(label)
		return
	}
	if c.tooltip == nil {
		var err error
		c.tooltip, err = newToolbarTooltip(c.Window.NativeHandle(), theme)
		if err != nil {
			c.Panel.Error = err.Error()
			return
		}
	}
	c.tooltip.configure(c.Window.NativeHandle(), theme)
	id := button.ID()
	button.DrawOver(func(_ *ui.Painter, rect ui.Rect) {
		if c.Toolbar && c.tooltip != nil {
			if err := c.tooltip.bind(id, rect, label); err != nil {
				c.Panel.Error = err.Error()
			}
		}
	})
}

// Show a launch failure in the existing native popup without resizing the
// compact toolbar or hiding the selected text behind a second dialog.
func (t *toolbarTooltip) showMessage(id uint64, message string) {
	entry := t.tools[id]
	if entry == nil {
		return
	}
	scale := float32(t.dpi) / 96
	rect := ui.Rect{X: float32(entry.rect[0]) / scale, Y: float32(entry.rect[1]) / scale, W: float32(entry.rect[2]-entry.rect[0]) / scale, H: float32(entry.rect[3]-entry.rect[1]) / scale}
	if t.bind(id, rect, message) == nil {
		frameSend.Call(t.window, 0x422, 0, 0)
	} // TTM_POPUP
}
