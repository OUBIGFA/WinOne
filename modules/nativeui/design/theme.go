// Package design holds the visual vocabulary shared by every native WinOne window.
package design

import "github.com/egoist/mygo/ui"

// Shared corner radius scale in DIPs, 8 to 40 by size and layer. Nested
// surfaces step concentrically (outer radius = inner radius + padding) so
// corners align cleanly.
const (
	// RadiusMark rounds only a check box glyph: at 16 DIPs a larger radius
	// would turn it into a radio dot.
	RadiusMark float32 = 4
	RadiusXS   float32 = 8   // micro badges, compact tags, placeholders
	RadiusSM   float32 = 8   // items inside panels/popovers, segmented tabs, compact icon buttons
	RadiusMD   float32 = 12  // standard controls: buttons, inputs, selects, nav items, tabs
	RadiusLG   float32 = 16  // panels, disclosures, popovers, notices, result regions
	RadiusXL   float32 = 24  // dialogs and other large modal panels
	RadiusFull float32 = 999 // pills, circular buttons, switch tracks, radio dots
)

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
		// One step below the white Panel, so grouped settings read as
		// blocks on the canvas without a heavier border.
		Background:     ui.Hex("#f5f5f7"),
		Surface:        ui.Hex("#ececee"),
		SurfaceHover:   ui.Hex("#e8e8eb"),
		SurfacePressed: ui.Hex("#dcdce0"),
		Border:         ui.Hex("#c6c6cb"),
		Text:           ui.Hex("#181818"),
		TextMuted:      ui.Hex("#666666"),
		Accent:         ui.Hex("#181818"),
		AccentHover:    ui.Hex("#333333"),
		AccentPressed:  ui.Hex("#000000"),
		AccentText:     ui.Hex("#ffffff"),
		Danger:         ui.Hex("#c42b1c"),
		Warning:        ui.Hex("#9d5d00"),
		Success:        ui.Hex("#0f7b0f"),
		Selection:      ui.RGBA(24, 24, 24, 0.16),
		Focus:          ui.RGBA(24, 24, 24, 0.6),
		Inverse:        ui.Hex("#181818"),
		InverseText:    ui.Hex("#fafafa"),
		Scrollbar:      ui.RGBA(0, 0, 0, 0.3),
		Radius:         RadiusMD,
		Spacing:        4,
		FontSize:       14,
		Font:           SystemFont,
	}
	lightPalette = Palette{
		Panel:       ui.Hex("#ffffff"),
		Sidebar:     ui.Hex("#ececee"),
		Selected:    ui.Hex("#e4e4e7"),
		Hairline:    ui.RGBA(0, 0, 0, 0.13),
		DangerSoft:  ui.RGBA(196, 43, 28, 0.07),
		DangerLine:  ui.RGBA(196, 43, 28, 0.32),
		InfoSoft:    ui.Hex("#ececee"),
		SuccessSoft: ui.RGBA(15, 123, 15, 0.1),
		WarningSoft: ui.RGBA(157, 93, 0, 0.1),
		Skeleton:    ui.Hex("#e8e8eb"),
	}
	darkTheme = ui.Theme{
		Dark:           true,
		Background:     ui.Hex("#161616"),
		Surface:        ui.Hex("#242424"),
		SurfaceHover:   ui.Hex("#2e2e2e"),
		SurfacePressed: ui.Hex("#383838"),
		Border:         ui.Hex("#4a4a4a"),
		Text:           ui.Hex("#ececec"),
		TextMuted:      ui.Hex("#a0a0a0"),
		Accent:         ui.Hex("#ececec"),
		AccentHover:    ui.Hex("#ffffff"),
		AccentPressed:  ui.Hex("#cccccc"),
		AccentText:     ui.Hex("#161616"),
		Danger:         ui.Hex("#ff99a4"),
		Warning:        ui.Hex("#fce100"),
		Success:        ui.Hex("#6ccb5f"),
		Selection:      ui.RGBA(233, 233, 233, 0.22),
		Focus:          ui.RGBA(233, 233, 233, 0.65),
		Inverse:        ui.Hex("#ececec"),
		InverseText:    ui.Hex("#161616"),
		Scrollbar:      ui.RGBA(255, 255, 255, 0.3),
		Radius:         RadiusMD,
		Spacing:        4,
		FontSize:       14,
		Font:           SystemFont,
	}
	darkPalette = Palette{
		Panel:       ui.Hex("#1f1f1f"),
		Sidebar:     ui.Hex("#121212"),
		Selected:    ui.Hex("#303030"),
		Hairline:    ui.RGBA(255, 255, 255, 0.12),
		DangerSoft:  ui.RGBA(255, 153, 164, 0.08),
		DangerLine:  ui.RGBA(255, 153, 164, 0.35),
		InfoSoft:    ui.Hex("#262626"),
		SuccessSoft: ui.RGBA(108, 203, 95, 0.12),
		WarningSoft: ui.RGBA(252, 225, 0, 0.1),
		Skeleton:    ui.Hex("#2b2b2b"),
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
