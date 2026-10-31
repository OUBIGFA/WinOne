//go:build windows

package convenience

import "golang.org/x/sys/windows"

var getAsyncKeyState = windows.NewLazySystemDLL("user32.dll").NewProc("GetAsyncKeyState")

// xButtonHeld reports whether the side button a trigger names ("x1" or
// "x2") is held. MyGo turns those buttons into Back and Forward keys as
// they are let go, so a recording reads their state from Windows.
func xButtonHeld(trigger string) bool {
	key := uintptr(0x05) // VK_XBUTTON1
	switch trigger {
	case "x1":
	case "x2":
		key = 0x06 // VK_XBUTTON2
	default:
		return false
	}
	state, _, _ := getAsyncKeyState.Call(key)
	return state&0x8000 != 0
}
