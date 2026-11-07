// Package design holds the visual vocabulary shared by every native WinOne window.
package design

import "github.com/egoist/mygo/ui"

// Palette holds the colors of the theme that ui.Theme has no slot for.
type Palette struct {
	// Panel is the face of grouped settings, one step from the canvas.
	Panel ui.Color
	// Sidebar is the navigation layer beside the content.
	Sidebar ui.Color
	// Selected fills the chosen navigation item and list row.
	Selected ui.Color
	// Hairline separates rows within a group and regions of the shell.
	Hairline ui.Color
	// DangerSoft and DangerLine tint an error notice; InfoSoft tints a
	// neutral one.
	DangerSoft, DangerLine, InfoSoft ui.Color
	// SuccessSoft and WarningSoft tint status badges.
	SuccessSoft, WarningSoft ui.Color
	// Skeleton fills placeholders while content loads.
	Skeleton ui.Color
}

var (
	lightTheme = ui.Theme{
		Background:     ui.Hex("#fafaf9"),
		Surface:        ui.Hex("#f0f0ee"),
		SurfaceHover:   ui.Hex("#e8e8e5"),
		SurfacePressed: ui.Hex("#dededa"),
		Border:         ui.Hex("#cececa"),
		Text:           ui.Hex("#292927"),
		TextMuted:      ui.Hex("#60605c"),
		Accent:         ui.Hex("#30302e"),
		AccentHover:    ui.Hex("#454542"),
		AccentPressed:  ui.Hex("#20201e"),
		AccentText:     ui.Hex("#ffffff"),
		Danger:         ui.Hex("#c42b1c"),
		Warning:        ui.Hex("#9d5d00"),
		Success:        ui.Hex("#0f7b0f"),
		Selection:      ui.RGBA(46, 46, 46, 0.18),
		Focus:          ui.RGBA(46, 46, 46, 0.6),
		Inverse:        ui.Hex("#292928"),
		InverseText:    ui.Hex("#fafafa"),
		Scrollbar:      ui.RGBA(0, 0, 0, 0.3),
		Radius:         8,
		Spacing:        4,
		FontSize:       14,
		Font:           SystemFont,
	}
	lightPalette = Palette{
		Panel:       ui.Hex("#ffffff"),
		Sidebar:     ui.Hex("#f0f0ee"),
		Selected:    ui.Hex("#e4e4e0"),
		Hairline:    ui.RGBA(0, 0, 0, 0.08),
		DangerSoft:  ui.RGBA(196, 43, 28, 0.07),
		DangerLine:  ui.RGBA(196, 43, 28, 0.32),
		InfoSoft:    ui.Hex("#f0f0ee"),
		SuccessSoft: ui.RGBA(15, 123, 15, 0.1),
		WarningSoft: ui.RGBA(157, 93, 0, 0.1),
		Skeleton:    ui.Hex("#e8e8e5"),
	}
	darkTheme = ui.Theme{
		Dark:           true,
		Background:     ui.Hex("#1c1c1b"),
		Surface:        ui.Hex("#292928"),
		SurfaceHover:   ui.Hex("#333331"),
		SurfacePressed: ui.Hex("#3b3b38"),
		Border:         ui.Hex("#454541"),
		Text:           ui.Hex("#e7e7e2"),
		TextMuted:      ui.Hex("#aaa9a3"),
		Accent:         ui.Hex("#e4e4e0"),
		AccentHover:    ui.Hex("#ffffff"),
		AccentPressed:  ui.Hex("#cccccc"),
		AccentText:     ui.Hex("#1c1c1b"),
		Danger:         ui.Hex("#ff99a4"),
		Warning:        ui.Hex("#fce100"),
		Success:        ui.Hex("#6ccb5f"),
		Selection:      ui.RGBA(233, 233, 233, 0.22),
		Focus:          ui.RGBA(233, 233, 233, 0.65),
		Inverse:        ui.Hex("#e7e7e2"),
		InverseText:    ui.Hex("#1c1c1b"),
		Scrollbar:      ui.RGBA(255, 255, 255, 0.3),
		Radius:         8,
		Spacing:        4,
		FontSize:       14,
		Font:           SystemFont,
	}
	darkPalette = Palette{
		Panel:       ui.Hex("#242423"),
		Sidebar:     ui.Hex("#171716"),
		Selected:    ui.Hex("#343432"),
		Hairline:    ui.RGBA(255, 255, 255, 0.08),
		DangerSoft:  ui.RGBA(255, 153, 164, 0.08),
		DangerLine:  ui.RGBA(255, 153, 164, 0.35),
		InfoSoft:    ui.Hex("#292927"),
		SuccessSoft: ui.RGBA(108, 203, 95, 0.12),
		WarningSoft: ui.RGBA(252, 225, 0, 0.1),
		Skeleton:    ui.Hex("#2d2d2b"),
	}
)

// Use a UI face with Chinese coverage as the primary family. The remaining
// installed Windows families cover fallback glyphs without a font download.
const SystemFont = "Microsoft YaHei UI, Segoe UI Variable Text, Segoe UI"

// MonoFont is the face of code, paths and identifiers that are compared
// character by character.
const MonoFont = "Cascadia Mono, Consolas, Microsoft YaHei UI"

// Base returns a value, so a window can apply accessibility preferences
// without mutating another window's theme.
func Base(dark bool) ui.Theme {
	if dark {
		return darkTheme
	}
	return lightTheme
}

// Colors returns the surface roles for the requested theme.
func Colors(dark bool) Palette {
	if dark {
		return darkPalette
	}
	return lightPalette
}
