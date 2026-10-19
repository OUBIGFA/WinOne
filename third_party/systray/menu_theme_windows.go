//go:build windows

package systray

import (
	"errors"
	"fmt"
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var popupTheme atomic.Value

var menuWindowFromDC = u32.NewProc("WindowFromDC")
var menuDwmAttribute = windows.NewLazySystemDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute")
var menuEnumThreadWindows = u32.NewProc("EnumThreadWindows")
var menuClassName = u32.NewProc("GetClassNameW")

// One shared callback avoids retaining a Go callback for every popup. The
// pointer is valid only during the synchronous EnumThreadWindows call.
var menuFrameCallback = windows.NewCallback(func(window, parameter uintptr) uintptr {
	var name [32]uint16
	menuClassName.Call(window, uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)))
	if windows.UTF16ToString(name[:]) == "#32768" {
		(*nativeMenuStyle)(unsafe.Pointer(parameter)).prepareWindowFrame(window)
	}
	return 1
})

// Menus normally receive Windows' smaller corner preference. Use the same
// DWMWCP_ROUND policy as WinOne's other native windows; DWM keeps ownership of
// clipping, DPI, the outer frame and shadow. No custom window region or hook.
// https://learn.microsoft.com/windows/win32/api/dwmapi/ne-dwmapi-dwm_window_corner_preference
func (s *nativeMenuStyle) preparePopupFrame(dc uintptr) {
	window, _, _ := menuWindowFromDC.Call(dc)
	if window != 0 {
		s.prepareWindowFrame(window)
		return
	}
	// Windows buffers owner-drawn menus, so their DC need not belong to an HWND.
	// During this owned menu's draw callback, only the current thread's native
	// popup chain is relevant. Never enumerate or modify other threads' menus.
	menuEnumThreadWindows.Call(uintptr(windows.GetCurrentThreadId()), menuFrameCallback, uintptr(unsafe.Pointer(s)))
}

func (s *nativeMenuStyle) prepareWindowFrame(window uintptr) {
	if s.frames[window] {
		return
	}
	s.frames[window] = true
	corner := uint32(2) // DWMWCP_ROUND, never DWMWCP_ROUNDSMALL.
	result, _, _ := menuDwmAttribute.Call(window, 33, uintptr(unsafe.Pointer(&corner)), unsafe.Sizeof(corner))
	if int32(result) < 0 && s.frameError == nil {
		s.frameError = fmt.Errorf("设置托盘菜单圆角失败：0x%08x", uint32(result))
	}
}

// SetMenuTheme sets the next popup's appearance; an open menu retains its
// snapshot. Other systray users retain system menus unless they opt in.
func SetMenuTheme(theme string) error {
	if theme != "system" && theme != "light" && theme != "dark" {
		return errors.New("invalid menu theme")
	}
	popupTheme.Store(theme)
	return nil
}

func styledPopup(source, owner windows.Handle) (*nativeMenuStyle, error) {
	preference := popupTheme.Load()
	if preference == nil {
		return nil, nil
	}
	// Preserve the system high-contrast menu rather than overriding accessibility.
	contrast := struct {
		Size, Flags uint32
		Scheme      uintptr
	}{}
	contrast.Size = uint32(unsafe.Sizeof(contrast))
	if ok, _, err := u32.NewProc("SystemParametersInfoW").Call(0x42, uintptr(contrast.Size), uintptr(unsafe.Pointer(&contrast)), 0); ok == 0 {
		return nil, err
	}
	if contrast.Flags&1 != 0 {
		return nil, nil
	}
	theme := preference.(string)
	dark := theme == "dark"
	if theme == "system" {
		key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, registry.QUERY_VALUE)
		if err != nil && !errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
			return nil, err
		}
		if err == nil {
			value, _, readErr := key.GetIntegerValue("AppsUseLightTheme")
			key.Close()
			if readErr != nil && !errors.Is(readErr, windows.ERROR_FILE_NOT_FOUND) {
				return nil, readErr
			}
			dark = readErr == nil && value == 0
		}
	}
	dpi, _, err := u32.NewProc("GetDpiForWindow").Call(uintptr(owner))
	if dpi == 0 {
		return nil, err
	}
	return newNativeMenuStyle(source, dark, uint32(dpi))
}

func (s *nativeMenuStyle) handleMessage(message uint32, wParam, lParam uintptr) (uintptr, bool) {
	switch message {
	case 0x2c:
		if lParam != 0 && s.measure((*menuMeasure)(unsafe.Pointer(lParam))) {
			return 1, true
		}
	case 0x2b:
		if lParam != 0 && s.draw((*menuDraw)(unsafe.Pointer(lParam))) {
			return 1, true
		}
	case 0x120:
		if _, ok := s.menus[windows.Handle(lParam)]; ok {
			return s.mnemonic(windows.Handle(lParam), rune(wParam&0xffff)), true
		}
	}
	return 0, false
}
