package resultui

import (
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"unsafe"
)

var (
	frameUser             = windows.NewLazySystemDLL("user32.dll")
	frameCommon           = windows.NewLazySystemDLL("comctl32.dll")
	frameSubclass         = frameCommon.NewProc("SetWindowSubclass")
	frameRemove           = frameCommon.NewProc("RemoveWindowSubclass")
	frameDefault          = frameCommon.NewProc("DefSubclassProc")
	frameRect             = frameUser.NewProc("GetWindowRect")
	frameStyle            = frameUser.NewProc("GetWindowLongPtrW")
	frameDPI              = frameUser.NewProc("GetDpiForWindow")
	frameMetric           = frameUser.NewProc("GetSystemMetricsForDpi")
	frameZoomed           = frameUser.NewProc("IsZoomed")
	frameFind             = frameUser.NewProc("FindWindowExW")
	frameSend             = frameUser.NewProc("SendMessageW")
	frameSystemParameters = frameUser.NewProc("SystemParametersInfoW")
	frameDwmAttribute     = windows.NewLazySystemDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute")
	frameCallback         uintptr
)

func init() { frameCallback = windows.NewCallback(resultFrameMessage) }

// PrepareWindowFrame restores the native resize band removed with MyGo's
// frameless caption. Existing left/right/bottom non-client borders remain
// owned by Windows. The surface forwards top-edge gestures to that same OS
// sizing loop, so moving/resizing never waits for OCR or model requests.
func PrepareWindowFrame(window uintptr, theme string) error {
	// Windows owns the one visible border and its clipping. An additional
	// rounded UI border has a different radius and leaves doubled corners.
	// https://learn.microsoft.com/windows/apps/desktop/modernize/apply-rounded-corners
	setAttribute := func(attribute uintptr, value uint32) error {
		status, _, _ := frameDwmAttribute.Call(window, attribute, uintptr(unsafe.Pointer(&value)), 4)
		if int32(status) < 0 {
			return fmt.Errorf("result window decoration failed: 0x%x", uint32(status))
		}
		return nil
	}
	if err := setAttribute(33, 2); err != nil {
		return err
	} // DWMWCP_ROUND
	if theme == "light" || theme == "dark" {
		var contrast struct {
			Size, Flags uint32
			Scheme      uintptr
		}
		contrast.Size = uint32(unsafe.Sizeof(contrast))
		if ok, _, _ := frameSystemParameters.Call(0x0042, uintptr(contrast.Size), uintptr(unsafe.Pointer(&contrast)), 0); ok != 0 && contrast.Flags&1 == 0 {
			dark := uint32(0)
			if theme == "dark" {
				dark = 1
			}
			if err := setAttribute(20, dark); err != nil {
				return err
			}
		}
	}
	class, _ := windows.UTF16PtrFromString("MyGoSurface")
	surface, _, _ := frameFind.Call(window, 0, uintptr(unsafe.Pointer(class)), 0)
	if surface == 0 {
		return errors.New("result input surface is unavailable")
	}
	if ok, _, _ := frameSubclass.Call(window, frameCallback, 1, window); ok == 0 {
		return errors.New("result frame initialization failed")
	}
	if ok, _, _ := frameSubclass.Call(surface, frameCallback, 1, window); ok == 0 {
		frameRemove.Call(window, frameCallback, 1)
		return errors.New("result resize surface initialization failed")
	}
	return nil
}

func resultFrameMessage(window uintptr, message uint32, wparam, lparam, id, root uintptr) uintptr {
	switch message {
	case 0x0082: // WM_NCDESTROY
		frameRemove.Call(window, frameCallback, id)
	case 0x0084: // WM_NCHITTEST
		if hit := topResizeHit(root, lparam); hit != 0 {
			return hit
		}
	case 0x00A1, 0x00A3: // WM_NCLBUTTONDOWN / WM_NCLBUTTONDBLCLK
		if window != root && (wparam == 12 || wparam == 13 || wparam == 14) {
			value, _, _ := frameSend.Call(root, uintptr(message), wparam, lparam)
			return value
		}
	case 0x0020: // WM_SETCURSOR: use Windows' directional resize pointer.
		hit := lparam & 0xffff
		if window != root && (hit == 12 || hit == 13 || hit == 14) {
			value, _, _ := frameSend.Call(root, uintptr(message), root, lparam)
			return value
		}
	}
	value, _, _ := frameDefault.Call(window, uintptr(message), wparam, lparam)
	return value
}

func topResizeHit(window, lparam uintptr) uintptr {
	if maximized, _, _ := frameZoomed.Call(window); maximized != 0 {
		return 0
	}
	// GWL_STYLE is -16; only resizable windows expose a resize band.
	style, _, _ := frameStyle.Call(window, ^uintptr(15))
	if style&0x00040000 == 0 {
		return 0
	} // WS_THICKFRAME
	var rect struct{ Left, Top, Right, Bottom int32 }
	if ok, _, _ := frameRect.Call(window, uintptr(unsafe.Pointer(&rect))); ok == 0 {
		return 0
	}
	dpi, _, _ := frameDPI.Call(window)
	border, _, _ := frameMetric.Call(33, dpi)  // SM_CYSIZEFRAME
	padding, _, _ := frameMetric.Call(92, dpi) // SM_CXPADDEDBORDER
	band := int32(border + padding)
	x, y := int32(int16(lparam&0xffff)), int32(int16((lparam>>16)&0xffff))
	if band <= 0 || x < rect.Left || x >= rect.Right || y < rect.Top || y >= rect.Top+band {
		return 0
	}
	if x < rect.Left+band {
		return 13
	} // HTTOPLEFT
	if x >= rect.Right-band {
		return 14
	} // HTTOPRIGHT
	return 12 // HTTOP
}
