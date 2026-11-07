// Package kit is WinOne's design system on MyGo's native UI: the theme,
// text roles, layout of settings pages and the few compositions every page
// shares. Pages use these instead of their own colors and sizes, so one
// change here restyles the whole window.
//
// Direction (docs/MyGo前端迁移.md, "设计系统"): an Operate surface in
// WinOne's neutral grays, Windows' system font, MyGo's own controls, a
// fixed type ramp, tight rows inside groups and generous space between
// groups. Semantic colors appear only where they carry meaning.
package kit

import (
	"github.com/egoist/mygo/ui"
	"winone/modules/nativeui/design"
)

// Shared surface roles are also used by the independent result window.
type Palette = design.Palette

var (
	lightTheme   = design.Base(false)
	darkTheme    = design.Base(true)
	lightPalette = design.Colors(false)
	darkPalette  = design.Colors(true)
)

const MonoFont = design.MonoFont

// Theme returns the light or dark theme for the desktop's preferences:
// Windows' text size scales every role of the type ramp, and high
// contrast darkens borders and secondary text as MyGo's own theme does.
// The result is shared; do not change it.
func Theme(p ui.Preferences, dark bool) *ui.Theme {
	key := themeKey{dark: dark, scale: p.TextScale, contrast: p.HighContrast}
	if t, ok := themes[key]; ok {
		return t
	}
	t := lightTheme
	if dark {
		t = darkTheme
	}
	if p.TextScale > 0 {
		t.FontSize = 14 * p.TextScale
	}
	if p.HighContrast {
		t.Border = t.Border.Mix(t.Text, 0.45)
		t.TextMuted = t.TextMuted.Mix(t.Text, 0.4)
		t.Focus = t.Accent
	}
	themes[key] = &t
	return &t
}

type themeKey struct {
	dark, contrast bool
	scale          float32
}

// themes is touched on the UI thread only.
var themes = map[themeKey]*ui.Theme{}

// Colors returns the palette matching the context's theme.
func Colors(c *ui.Context) Palette {
	if c.Theme().Dark {
		return darkPalette
	}
	return lightPalette
}
