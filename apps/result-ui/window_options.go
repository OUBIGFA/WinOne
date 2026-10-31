package resultui

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// WindowOptions returns how a controller's window opens: frameless with
// Windows 11's rounded corners, hidden until placed, at its content size.
// A capture result floats above the capture it replaces; the Quick
// assistant follows its setting. The selection toolbar, and the result it
// becomes in the same window, is a panel: a tool window that never gets a
// taskbar button or an Alt+Tab entry, even before its first show.
func (c *Controller) WindowOptions() mygo.WindowOptions {
	width, height, minWidth, minHeight := DefaultResultWidth, DefaultResultHeight, 320, 300
	if c.Toolbar {
		width, height = c.ToolbarDimensions()
		minWidth, minHeight = 0, 0
	}
	return mygo.WindowOptions{
		Title:           "WinOne",
		Width:           width,
		Height:          height,
		UseContentSize:  true,
		MinWidth:        minWidth,
		MinHeight:       minHeight,
		Frameless:       true,
		CornerRadius:    8,
		Hidden:          true,
		AlwaysOnTop:     c.Initial.Source != "quick-assistant" || c.Panel.Pinned,
		Panel:           c.Toolbar || c.Initial.Source == "selection",
		DisableResize:   c.Toolbar,
		DisableMaximize: true,
		Content:         ui.View(c.View),
	}
}

// PrepareFrame keeps a result inside its monitor's work area before its
// first visible frame. The toolbar is placed by its anchor instead.
func (c *Controller) PrepareFrame() error {
	if c.Toolbar {
		return nil
	}
	return fitResultWindow(c.Window.NativeHandle(), 0)
}

// ThemeSource is MyGo's appearance for WinOne's theme setting.
func ThemeSource(theme string) mygo.ThemeSource {
	switch theme {
	case "light":
		return mygo.ThemeLight
	case "dark":
		return mygo.ThemeDark
	}
	return mygo.ThemeSystem
}

// LocaleTag is MyGo's language for WinOne's interface language.
func LocaleTag(language string) string {
	if language == "en-US" {
		return "en"
	}
	return "zh-Hans"
}
