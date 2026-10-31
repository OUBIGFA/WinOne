package wintray

import (
	"strconv"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
)

// handle returns the persistent identity of the input named key, which a
// click on its label focuses.
func (p *page) handle(key string) *ui.Handle {
	if h := p.handles[key]; h != nil {
		return h
	}
	h := new(ui.Handle)
	p.handles[key] = h
	return h
}

// field lays out an input with its label, which focuses the input when
// clicked, as an HTML label did for its field: the label left and the
// input right, or, stacked, the input below for one that needs the row's
// width.
func field(c *ui.Context, key, label string, input *ui.Handle, stacked bool, control func()) {
	var container ui.Element
	if stacked {
		container = ui.Column(c.Key(key)).FillWidth().Padding(12, 16).Gap(8)
	} else {
		container = ui.Row(c.Key(key)).FillWidth().MinHeight(kit.RowMinHeight).Padding(10, 16).GapX(16).GapY(8).AlignItems(ui.Center).Wrap()
	}
	container.Children(func() {
		text := kit.Label(c, label)
		if !stacked {
			text.Basis(220).Grow(1).Shrink(1)
		}
		if text.Clicked() {
			input.Focus()
		}
		control()
	})
}

// numberRow is a setting row editing a whole number within bounds. What is
// typed stays as typed: an unfinished or out-of-range entry marks the field,
// says which numbers it takes and blocks saving (even once the field is
// hidden), rather than being clamped or replaced, as React's NumberField
// kept such input. Up and Down step the number. key names the field in
// p.texts.
func (p *page) numberRow(c *ui.Context, key, label string, value *int, minimum, maximum int, disabled bool) {
	text, editing := p.texts[key]
	if !editing {
		text = strconv.Itoa(*value)
	}
	_, valid := parseWhole(text, minimum, maximum)
	message := ""
	if !valid {
		message = i18n.T("common.numberRange", "min", minimum, "max", maximum)
	}
	handle := p.handle(key)
	field(c, key, label, handle, false, func() {
		ui.Column(c).Gap(4).AlignItems(ui.End).Children(func() {
			before := text
			in := ui.TextInput(c, &text).Width(120).Label(label).Error(message).Disabled(disabled).Bind(handle)
			if text != before {
				p.texts[key] = text
				if n, ok := parseWhole(text, minimum, maximum); ok {
					*value = n
				}
			}
			step := func(delta int) {
				n, ok := parseWhole(text, minimum, maximum)
				if !ok {
					n = *value
				}
				n = min(max(n+delta, minimum), maximum)
				*value = n
				p.texts[key] = strconv.Itoa(n)
			}
			if in.Shortcut(0, ui.KeyUp) {
				step(1)
			}
			if in.Shortcut(0, ui.KeyDown) {
				step(-1)
			}
			// A valid number shows as the draft holds it once the field
			// is left; an invalid one stays for the user to correct.
			if !in.Focused() {
				if _, ok := parseWhole(p.texts[key], minimum, maximum); ok {
					delete(p.texts, key)
				}
			}
			kit.InlineError(c, message)
		})
	})
}
