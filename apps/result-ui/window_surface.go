package resultui

import "github.com/egoist/mygo/ui"

const DefaultResultWidth = 620
const DefaultResultHeight = 660

// Windows owns the outer border and corner clipping for every shared window,
// including the selection toolbar. Client content must fill that surface without
// drawing a second frame. Interior controls and popovers keep their own borders.
func windowSurface(element *ui.Element, theme *ui.Theme) *ui.Element {
	return element.Fill().Border(0, ui.Transparent).Radius(0).Background(theme.Background)
}
