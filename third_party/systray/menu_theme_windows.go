//go:build windows

package systray

import (
	"errors"
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var popupTheme atomic.Value

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
