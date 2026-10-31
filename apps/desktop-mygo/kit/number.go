package kit

import (
	"math"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
)

// numberBox edits a number from low to high in steps of step, in one field
// that shows its unit inside it, at the right and muted. What is typed
// applies once it is a number in range on a step's precision; Up and Down
// step; leaving the field shows the value again. label names the field
// for assistive technology.
func numberBox(c *ui.Context, value *float64, low, high, step float64, unit, label string) ui.Element {
	t := c.Theme()
	decimals := 0
	if s := strconv.FormatFloat(step, 'f', -1, 64); strings.Contains(s, ".") {
		decimals = len(s) - strings.IndexByte(s, '.') - 1
	}
	scale := math.Pow(10, float64(decimals))
	round := func(v float64) float64 { return math.Round(v*scale) / scale }
	format := func(v float64) string { return strconv.FormatFloat(v, 'f', decimals, 64) }
	box := ui.Row(c).MinHeight(ControlHeight).Padding(0, 10).Gap(6).AlignItems(ui.Center).
		Radius(t.Radius).Background(t.Surface).Border(1, t.Border).MinWidth(0)
	text := ui.Local(box, "text", func() string { return format(*value) })
	set := func(v float64) {
		*value = math.Max(low, math.Min(high, round(v)))
		*text = format(*value)
	}
	box.Children(func() {
		in := ui.TextInputBase(c, text).Grow(1).MinWidth(24).Label(label)
		if in.Changed() {
			if v, err := strconv.ParseFloat(strings.TrimSpace(*text), 64); err == nil && v >= low && v <= high && round(v) == v {
				*value = v
			}
		}
		if in.Shortcut(0, ui.KeyUp) {
			set(*value + step)
		}
		if in.Shortcut(0, ui.KeyDown) {
			set(*value - step)
		}
		if !in.Focused() {
			*text = format(*value)
		} else {
			box.Border(1, t.Accent)
		}
		if unit != "" {
			ui.Text(c, unit).FontSize(HelpSize(c)).TextColor(t.TextMuted).Shrink(0)
		}
	})
	return box
}

// UnitNumber edits a whole number from low to high, as timed power's
// hours, minutes and seconds; it fills the width it is given.
func UnitNumber(c *ui.Context, value *float64, low, high float64, unit, label string) ui.Element {
	return numberBox(c, value, low, high, 1, unit, label)
}

// NumberField edits a number from low to high in steps of step, typed or
// stepped with Up and Down, with its unit inside the field, as "%".
func NumberField(c *ui.Context, value *float64, low, high, step float64, unit, label string) ui.Element {
	return numberBox(c, value, low, high, step, unit, label).Width(120)
}
