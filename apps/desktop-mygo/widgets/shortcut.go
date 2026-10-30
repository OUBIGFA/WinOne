// Package widgets holds controls that need the Host as well as the kit:
// the shortcut recorder and the file and folder drop areas.
package widgets

import (
	"os"
	"time"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
)

// ShortcutRecorder records a global shortcut by its keys. While it records,
// the Host releases WinOne's registered shortcuts (shortcuts.record.start),
// so pressing an existing chord records it instead of running it; every
// way out (a chord, Escape, Tab, leaving the field, 20 seconds) restores
// them (shortcuts.record.stop) before the new value is committed.
//
// React source: ui/shortcut-recorder.tsx and ui/shortcut-keys.ts.
type ShortcutRecorder struct {
	env       *core.Env
	phase     string // "", "preparing", "recording", "finishing"
	token     string
	preview   string
	candidate string
	err       string
	notice    string
	clear     bool
	started   time.Time
	focused   bool
	// generation discards a start that finished before the Host answered.
	generation int
	// pendingValue carries a committed chord to the next View, which owns
	// the value pointer.
	pendingValue *string
}

// NewShortcutRecorder makes a recorder for one field.
func NewShortcutRecorder(env *core.Env) *ShortcutRecorder { return &ShortcutRecorder{env: env} }

// Recording reports whether the recorder holds WinOne's shortcuts.
func (r *ShortcutRecorder) Recording() bool { return r.phase != "" }

// View shows the field and its Clear button; it returns whether *value
// changed. hint shows the idle explanation, which repeated fields share.
func (r *ShortcutRecorder) View(c *ui.Context, label string, value *string, disabled, hint bool) bool {
	t := c.Theme()
	before := *value
	ui.Column(c).Gap(6).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			field := ui.Box(c).Width(220).MinHeight(kit.ControlHeight).Padding(5, 10).Radius(t.Radius).Background(t.Surface).
				Border(1, t.Border).Focusable().Label(label).Role(ui.RoleTextField).Cursor(ui.CursorText).Disabled(disabled)
			focused := field.Focused()
			switch {
			case r.phase == "recording" || r.phase == "preparing":
				field.Border(1, t.Accent)
			case field.Hovered() && !disabled:
				field.Background(t.SurfaceHover)
			}
			text, muted := *value, false
			switch r.phase {
			case "recording":
				text = r.preview
				// Modifiers held alone show as they are pressed.
				if r.candidate == "" {
					_, held := RecordedShortcut(ui.KeyA, ui.KeyA, c.Modifiers(), false)
					text = modifiersOnly(held)
				}
			case "preparing":
				text = ""
			}
			if text == "" {
				muted = true
				text = i18n.T("shortcuts.recordPlaceholder")
				if r.phase == "recording" {
					text = i18n.T("shortcuts.pressKeys")
				}
			}
			field.HandleInput(func(ev ui.InputEvent) bool { return r.key(ev, value) }).Children(func() {
				shown := ui.Text(c, text).SingleLine()
				if muted {
					shown.TextColor(t.TextMuted)
				}
			})
			if !disabled && (focused && !r.focused || field.Clicked()) && r.phase == "" {
				r.start()
			}
			if r.focused && !focused && r.phase != "" {
				r.finish(nil, "")
			}
			r.focused = focused
			if kit.Button(c, i18n.T("shortcuts.clear"), nil, false).Disabled(disabled || *value == "").Clicked() {
				r.clear = true
				*value = ""
				if r.phase != "" {
					r.finish(nil, "")
				}
			}
		})
		// All modifiers released after a chord: commit it, so the chord
		// cannot run the action it now names while still held.
		if r.phase == "recording" && r.candidate != "" && c.Modifiers() == 0 {
			candidate := r.candidate
			r.finish(&candidate, "")
		}
		if r.phase == "recording" && time.Since(r.started) > 20*time.Second {
			r.finish(nil, i18n.T("shortcuts.recordTimeout"))
		} else if r.phase == "recording" {
			c.After(time.Second)
		}
		message := ""
		switch {
		case r.phase == "preparing":
			message = i18n.T("shortcuts.recordPreparing")
		case r.phase == "recording":
			message = i18n.T("shortcuts.recordInstructions")
		case r.notice != "":
			message = r.notice
		case hint:
			message = i18n.T("shortcuts.recordHint")
		}
		kit.Status(c, message)
		kit.InlineError(c, r.err)
	})
	if r.pendingValue != nil {
		*value = *r.pendingValue
		r.pendingValue = nil
	}
	return *value != before
}

func (r *ShortcutRecorder) key(ev ui.InputEvent, value *string) bool {
	if ev.Kind != ui.InputKeyDown || r.phase == "" {
		return false
	}
	if ev.Key == ui.KeyTab {
		r.finish(nil, "")
		return false
	}
	if ev.Key == ui.KeyEscape {
		r.finish(nil, "")
		return true
	}
	if r.phase != "recording" {
		return true
	}
	if (ev.Key == ui.KeyDelete || ev.Key == ui.KeyBackspace) && ev.Mods == 0 {
		empty := ""
		r.finish(&empty, "")
		return true
	}
	kind, text := RecordedShortcut(ev.Key, ev.PhysicalKey, ev.Mods, ev.Repeat)
	switch kind {
	case "shortcut":
		r.candidate, r.preview, r.err = text, text, ""
	case "invalid":
		r.err = i18n.T("shortcuts.recordError." + text)
	}
	return true
}

func (r *ShortcutRecorder) start() {
	r.generation++
	owner := r.generation
	r.phase, r.err, r.notice, r.preview, r.candidate, r.clear = "preparing", "", "", "", "", false
	core.Go(r.env, func() (string, error) {
		var started struct {
			Token string `json:"token"`
		}
		err := r.env.Host.CallInto("shortcuts.record.start", map[string]int{"processId": os.Getpid()}, &started)
		return started.Token, err
	}, func(token string, err error) {
		if owner != r.generation {
			// The field finished before the Host answered: release now.
			if err == nil {
				r.stop(token)
			}
			return
		}
		if err != nil {
			r.phase, r.err = "", err.Error()
			return
		}
		r.token, r.phase, r.started = token, "recording", time.Now()
	})
}

func (r *ShortcutRecorder) finish(next *string, notice string) {
	if r.phase == "" || r.phase == "finishing" {
		return
	}
	token := r.token
	r.generation++
	if r.phase == "preparing" {
		// start's answer releases the registration when it comes.
		r.phase, r.token = "", ""
		return
	}
	r.phase = "finishing"
	clear := r.clear
	core.Do(r.env, func() error {
		return r.env.Host.CallInto("shortcuts.record.stop", map[string]string{"token": token}, nil)
	}, func(err error) {
		r.phase, r.token, r.preview, r.candidate = "", "", "", ""
		if err != nil {
			r.err = err.Error()
			return
		}
		if next != nil && !clear {
			value := *next
			r.pendingValue = &value
		}
		r.err, r.notice = "", notice
	})
}

func (r *ShortcutRecorder) stop(token string) {
	go func() { _ = r.env.Host.CallInto("shortcuts.record.stop", map[string]string{"token": token}, nil) }()
}

// Stop ends a recording when its page closes.
func (r *ShortcutRecorder) Stop() {
	if r.phase == "recording" || r.phase == "finishing" {
		r.stop(r.token)
	}
	r.generation++
	r.phase, r.token = "", ""
}

// RecordedShortcut maps a key press to WinOne's shortcut vocabulary, as the
// Host parses it: modifiers Ctrl, Alt, Shift and Win, then a letter, a
// digit, a keypad digit (Num0–Num9) or F1–F11 (F12 is reserved by
// Windows' debuggers). kind is "shortcut" with the text, "invalid" with
// the reason ("modifier", "key", "reserved"), or "ignore".
func RecordedShortcut(key, physical ui.Key, mods ui.Modifiers, repeat bool) (kind, text string) {
	if repeat {
		return "ignore", ""
	}
	var parts []string
	for _, m := range []struct {
		mod  ui.Modifiers
		name string
	}{{ui.Ctrl, "Ctrl"}, {ui.Alt, "Alt"}, {ui.Shift, "Shift"}, {ui.Super, "Win"}} {
		if mods&m.mod != 0 {
			parts = append(parts, m.name)
		}
	}
	name := ""
	switch {
	case key >= ui.KeyA && key <= ui.KeyZ:
		name = string(rune('A' + int(key-ui.KeyA)))
	case key >= ui.Key0 && key <= ui.Key9:
		digit := string(rune('0' + int(key-ui.Key0)))
		// The keypad's digits share the layout key of the row's; their
		// position is not a digit key of the row.
		if physical < ui.Key0 || physical > ui.Key9 {
			name = "Num" + digit
		} else {
			name = digit
		}
	case key == ui.KeyF12:
		return "invalid", "reserved"
	case key >= ui.KeyF1 && key <= ui.KeyF11:
		name = "F" + kit.Itoa(1+int(key-ui.KeyF1))
	default:
		return "invalid", "key"
	}
	if len(parts) == 0 {
		return "invalid", "modifier"
	}
	parts = append(parts, name)
	text = parts[0]
	for _, part := range parts[1:] {
		text += "+" + part
	}
	return "shortcut", text
}

// modifiersOnly removes the placeholder key from a recorded chord.
func modifiersOnly(chord string) string {
	if len(chord) < 2 || chord[len(chord)-2:] != "+A" {
		return ""
	}
	return chord[:len(chord)-2]
}
