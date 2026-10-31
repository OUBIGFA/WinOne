package aiservices

import (
	"math"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/kit"
)

// numberText edits a number that may be left empty, as React's number
// inputs did: an empty field shows the placeholder ("default", "no
// limit") and means the setting is not set. What is typed applies once it
// is a number in range (a whole one when whole is set); until then the
// field is marked, the setting keeps its value, and leaving the field
// shows that value again. It returns the
// new value, nil for an empty field, and whether it changed this frame.
func numberText(c *ui.Context, value *float64, low, high float64, whole bool, placeholder, label string) (*float64, bool) {
	t := c.Theme()
	format := func(v *float64) string {
		if v == nil {
			return ""
		}
		return strconv.FormatFloat(*v, 'f', -1, 64)
	}
	box := ui.Row(c).MinHeight(kit.ControlHeight).Padding(0, 10).Radius(t.Radius).Background(t.Surface).
		Border(1, t.Border).AlignItems(ui.Center).MinWidth(0).Width(160)
	text := ui.Local(box, "text", func() string { return format(value) })
	var next *float64
	changed := false
	box.Children(func() {
		in := ui.TextInputBase(c, text).Grow(1).MinWidth(24).Label(label).Placeholder(placeholder)
		typed := strings.TrimSpace(*text)
		v, err := strconv.ParseFloat(typed, 64)
		valid := typed == "" || (err == nil && v >= low && v <= high && !math.IsNaN(v) && (!whole || v == math.Trunc(v)))
		if in.Changed() && valid {
			if typed != "" {
				next = &v
			}
			changed = true
		}
		switch {
		case !in.Focused():
			// Not being edited, the field follows the setting: text left
			// invalid goes back to the value in force.
			*text = format(value)
		case !valid:
			box.Border(1, t.Danger)
		default:
			box.Border(1, t.Accent)
		}
	})
	return next, changed
}

// syncedText is a text field's string kept between frames for a value
// that lives elsewhere: a map entry, a field of a draft that a save
// replaces. A field must edit a string that outlives the frame; this one
// takes what the user typed since the last frame as an edit, and
// otherwise shows the value.
type syncedText struct{ text, shown string }

// sync returns the typed text and true after an edit; otherwise it shows
// current.
func (s *syncedText) sync(current string) (string, bool) {
	if s.text != s.shown {
		s.shown = s.text
		return s.text, true
	}
	s.text, s.shown = current, current
	return "", false
}

// fields are the synced texts of one form, by name.
type fields map[string]*syncedText

func (f fields) get(name string) *syncedText {
	if f[name] == nil {
		f[name] = &syncedText{}
	}
	return f[name]
}
