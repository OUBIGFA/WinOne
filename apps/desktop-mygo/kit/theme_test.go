package kit

import (
	"math"
	"testing"

	"github.com/egoist/mygo/ui"
)

func luminance(c ui.Color) float64 {
	channel := func(v uint8) float64 {
		x := float64(v) / 255
		if x <= 0.04045 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	return .2126*channel(c.R) + .7152*channel(c.G) + .0722*channel(c.B)
}

// Navigation must recede from the reading surface, while all ordinary
// and secondary text remains readable on every shared opaque surface.
func TestWorkspaceContrastAndHierarchy(t *testing.T) {
	for _, dark := range []bool{false, true} {
		theme := Theme(ui.Preferences{}, dark)
		palette := lightPalette
		if dark {
			palette = darkPalette
		}
		if dark && luminance(palette.Sidebar) >= luminance(theme.Background) {
			t.Error("dark navigation competes with the reading canvas")
		}
		for _, surface := range []ui.Color{theme.Background, palette.Panel, palette.Sidebar, palette.Selected} {
			for _, ink := range []ui.Color{theme.Text, theme.TextMuted} {
				a, b := luminance(surface), luminance(ink)
				ratio := (math.Max(a, b) + .05) / (math.Min(a, b) + .05)
				if ratio < 4.5 {
					t.Errorf("dark=%v contrast %.2f is below 4.5", dark, ratio)
				}
			}
		}
	}
}
