// Shared provider and gateway form controls, independent of optional AI pages.
package aiservices

import (
	"github.com/egoist/mygo/ui"
	"slices"
	"strconv"
	"strings"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
)

// Shared list choices are used by the retained provider account settings.
func stringChoices(values []string) []kit.SelectOption {
	out := make([]kit.SelectOption, 0, len(values))
	for _, v := range values {
		out = append(out, kit.SelectOption{Value: v, Label: v})
	}
	return out
}

func completeT(key string) string { return i18n.T("aiComplete." + key) }
func nativeT(key string) string   { return i18n.T("aiNative." + key) }

// choiceRow is a setting with a few choices shown at once.
func choiceRow(c *ui.Context, label, hint string, value *string, options []kit.SelectOption, disabled bool) bool {
	changed := false
	kit.Row(c, label, hint, func() {
		ui.Column(c).Disabled(disabled).Children(func() { changed = kit.Segmented(c, value, options, label) })
	})
	return changed
}

func options(pairs ...string) []kit.SelectOption {
	out := make([]kit.SelectOption, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, kit.SelectOption{Value: pairs[i], Label: pairs[i+1]})
	}
	return out
}

// numberRow is a setting that is a number, empty or zero meaning off or
// default as the setting says.
func numberRow(c *ui.Context, label string, current *float64, low, high float64, whole bool, placeholder string, disabled bool, set func(*float64)) {
	kit.Row(c, label, "", func() {
		ui.Column(c).Disabled(disabled).Children(func() {
			if v, changed := numberText(c, current, low, high, whole, placeholder, label); changed {
				set(v)
			}
		})
	})
}

func fmtNumber(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

func nativeSelect(c *ui.Context, label string, current *string, choices []kit.SelectOption, disabled bool) bool {
	changed := false
	ui.Field(c, label, func() {
		var trigger ui.Element
		changed, trigger = kit.Select(c, current, choices)
		trigger.FillWidth().MinWidth(0).Label(label).Disabled(disabled)
	})
	return changed
}

func nativeText(c *ui.Context, label string, value *string, disabled bool) {
	ui.Field(c, label, func() { kit.TextInput(c, value).FillWidth().Label(label).Disabled(disabled) })
}
func nativeInt(c *ui.Context, label string, value *int, low, high int, disabled bool) bool {
	changed := false
	n := float64(*value)
	numberRow(c, label, &n, float64(low), float64(high), true, "", disabled, func(v *float64) { *value = intNumber(v); changed = true })
	return changed
}
func intNumber(v *float64) int {
	if v == nil {
		return 0
	}
	return int(*v)
}
func nativeList(c *ui.Context, f fields, key, label string, values *[]string, disabled bool) {
	s := f.get(key)
	if value, changed := s.sync(strings.Join(*values, "\n")); changed {
		var out []string
		for _, line := range strings.Split(value, "\n") {
			if line = strings.TrimSpace(line); line != "" && !slices.Contains(out, line) {
				out = append(out, line)
			}
		}
		*values = out
	}
	ui.Field(c, label, func() { ui.TextArea(c, &s.text).FillWidth().MinHeight(64).Label(label).Disabled(disabled) })
}

func upstreamT(key string, args ...any) string {
	if i18n.Has("aiUpstream." + key) {
		return i18n.T("aiUpstream."+key, args...)
	}
	return strings.ReplaceAll(key, "magpie", "WinOne")
}
