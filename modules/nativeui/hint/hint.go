// Package hint owns the app's hover popup. C++ uses the same style.json through
// the capture build adapter; native common-control popups use Colors as well.
package hint

import (
	_ "embed"
	"encoding/json"
	"github.com/egoist/mygo/ui"
)

//go:embed style.json
var source []byte

type palette struct{ Background, Foreground, Border string }
type style struct {
	Colors                                         palette
	FontSize, Radius, PaddingX, PaddingY, MaxWidth float32
}

var shared = func() style {
	var value style
	if err := json.Unmarshal(source, &value); err != nil {
		panic(err)
	}
	return value
}()

// Colors returns a complete pair: no caller mixes a dark surface with the
// current window's ordinary foreground.
func Colors(_ bool) (background, foreground, border ui.Color) {
	colors := shared.Colors
	return ui.Hex(colors.Background), ui.Hex(colors.Foreground), ui.Hex(colors.Border)
}

// Attach uses MyGo's existing hover/focus timing, placement and accessibility.
// Setting the text node explicitly also prevents a parent button's color from
// leaking into the floating label.
func Attach(c *ui.Context, anchor ui.Element, text string) ui.Element {
	if text == "" {
		return anchor
	}
	background, foreground, border := Colors(c.Theme().Dark)
	fontSize := shared.FontSize * c.Theme().FontSize / 14
	anchor.Description(text)
	ui.TooltipBase(c, anchor, func(tip ui.Element) {
		tip.MaxWidth(shared.MaxWidth).Padding(shared.PaddingY, shared.PaddingX).
			Radius(shared.Radius).Background(background).Border(1, border).
			TextColor(foreground).FontSize(fontSize).
			AttachTo(anchor, ui.AnchorTop, ui.AnchorBottom).Margin(0, 0, 8, 0).
			Children(func() { ui.Text(c, text).TextColor(foreground).FontSize(fontSize) })
	})
	return anchor
}
