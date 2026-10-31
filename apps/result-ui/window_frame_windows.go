package resultui

import "golang.org/x/sys/windows"

// Win32 calls for placing result windows in physical pixels and for the
// selection toolbar's native tooltip. MyGo v0.7.3 owns the frame itself:
// rounded corners (WindowOptions.CornerRadius), the dark frame
// (mygo.Theme), the top resize edge of frameless windows and tool windows
// kept out of the taskbar (WindowOptions.Panel).
var (
	frameUser   = windows.NewLazySystemDLL("user32.dll")
	frameCommon = windows.NewLazySystemDLL("comctl32.dll")
	frameRect   = frameUser.NewProc("GetWindowRect")
	frameDPI    = frameUser.NewProc("GetDpiForWindow")
	frameFind   = frameUser.NewProc("FindWindowExW")
	frameSend   = frameUser.NewProc("SendMessageW")
)
