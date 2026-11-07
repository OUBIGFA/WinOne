package chat

import (
	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/kit"
)

// Transcript actions stay subordinate to prose; other app toolbars retain
// their normal size. Focus and hover strengthen the icon without moving it.
func messageAction(c *ui.Context, icon *ui.SVG, label string) ui.Element {
	t := c.Theme()
	b := ui.ButtonBase(c).Size(24, 24).Radius(t.Radius).Justify(ui.Center).AlignItems(ui.Center).Label(label).Tooltip(label)
	color := t.TextMuted
	if b.Hovered() || b.FocusVisible() {
		b.Background(t.SurfaceHover)
		color = t.Text
	}
	if b.Pressed() {
		b.Background(t.SurfacePressed)
	}
	b.Children(func() { ui.Icon(c, icon).FontSize(13).TextColor(color) })
	return b
}

// Soften the light-theme bubble; lift the dark-theme bubble toward its text
// by a small amount so it no longer reads as a dense block.
func messageBackground(c *ui.Context) ui.Color {
	pal, t := kit.Colors(c), c.Theme()
	if t.Dark {
		return pal.Selected.Mix(t.Text, 0.06)
	}
	return pal.Selected.Mix(t.Background, 0.55)
}
