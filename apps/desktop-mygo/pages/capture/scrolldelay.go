package capture

import (
	"math"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

// The scroll interval's range, in milliseconds.
const (
	minScrollDelay = 100
	maxScrollDelay = 2000
)

// scrollDelay reads the scroll interval as typed: a whole number of
// milliseconds from 100 to 2000, as React's validNumber accepted it.
func scrollDelay(text string) (int, bool) {
	value, ok := parseNumber(text)
	if !ok || value != math.Trunc(value) || value < minScrollDelay || value > maxScrollDelay {
		return 0, false
	}
	return int(value), true
}

func parseNumber(text string) (float64, bool) {
	value, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false
	}
	return value, true
}

// stepScrollDelay steps the typed interval by one millisecond as a number
// field's arrows do: to the next whole number, within the range; an empty
// or unreadable field steps from zero, so to the minimum.
func stepScrollDelay(text string, up bool) string {
	value, _ := parseNumber(text)
	next := math.Ceil(value) - 1
	if up {
		next = math.Floor(value) + 1
	}
	next = math.Max(minScrollDelay, math.Min(maxScrollDelay, next))
	return strconv.Itoa(int(next))
}

// scrollDelayRow edits the scroll interval. What is typed stays as typed;
// an interval that is not a whole number from 100 to 2000 is marked, and
// the page cannot be saved until it is corrected. Up and Down, and the
// buttons beside the field, step it by one millisecond.
func (p *page) scrollDelayRow(c *ui.Context) {
	label := i18n.T("capture.delay")
	value, valid := scrollDelay(p.scrollText)
	if valid {
		p.draft.Value.ScrollDelay = value
	}
	message := ""
	if !valid {
		message = i18n.T("common.numberRange", "min", minScrollDelay, "max", maxScrollDelay)
	}
	ui.Row(c).FillWidth().MinHeight(kit.RowMinHeight).Padding(10, 16).GapX(16).GapY(8).AlignItems(ui.Center).Wrap().Children(func() {
		ui.Column(c).Basis(220).Grow(1).Shrink(1).MinWidth(0).Gap(4).Children(func() {
			kit.Label(c, label)
			kit.InlineError(c, message)
		})
		ui.Row(c).Shrink(0).Gap(4).AlignItems(ui.Center).Children(func() {
			in := ui.TextInput(c, &p.scrollText).Width(96).Label(label).Bind(&p.scrollField).
				OnTextKey(func(k ui.TextKey) bool {
					if k.Mods != 0 || k.Key != ui.KeyUp && k.Key != ui.KeyDown {
						return false
					}
					k.SetText(stepScrollDelay(k.Text(), k.Key == ui.KeyUp))
					return true
				})
			in.Error(message)
			current, _ := parseNumber(p.scrollText)
			if kit.IconButton(c, icons.Minus, i18n.T("capture.scrollDelayDown")).Disabled(valid && current <= minScrollDelay).Clicked() {
				p.scrollText = stepScrollDelay(p.scrollText, false)
			}
			if kit.IconButton(c, icons.Plus, i18n.T("capture.scrollDelayUp")).Disabled(valid && current >= maxScrollDelay).Clicked() {
				p.scrollText = stepScrollDelay(p.scrollText, true)
			}
		})
	})
}
