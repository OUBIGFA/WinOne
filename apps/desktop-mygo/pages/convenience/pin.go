package convenience

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/widgets"
)

// pinPreferences are the pinning tool's settings
// (modules/always-on-top/engine.Settings); fields this page does not
// edit, as the legacy "enabled", are kept by core.Preserved.
type pinPreferences struct {
	SchemaVersion   int      `json:"schemaVersion"`
	Revision        uint64   `json:"revision"`
	Frame           bool     `json:"frame"`
	FrameThickness  float64  `json:"frameThickness"`
	FrameColor      string   `json:"frameColor"`
	FrameOpacity    float64  `json:"frameOpacity"`
	UseAccent       bool     `json:"useAccent"`
	RoundCorners    bool     `json:"roundCorners"`
	BlockInGameMode bool     `json:"blockInGameMode"`
	Sound           bool     `json:"sound"`
	ExcludedApps    []string `json:"excludedApps"`
}

// The pin tab's own tabs.
const (
	pinGeneral = iota
	pinAdvanced
	pinShortcuts
)

// pinTab is the pinning tool: its switch, then its frame, feedback and
// limits, and its shortcut, each a tab of its own.
//
// React source: features/always-on-top.tsx.
type pinTab struct {
	env     *core.Env
	modules *widgets.Modules
	tab     int
	// draft is the form; saved is what the Host stored, to tell edits.
	draft, saved *core.Preserved[pinPreferences]
	// exclusions is the excluded programs as typed, one per line.
	exclusions string
	working    bool
	loading    bool
	err        string
	shortcuts  *widgets.Shortcuts
}

func newPinTab(env *core.Env) *pinTab {
	t := &pinTab{env: env, modules: widgets.NewModules(env)}
	t.reload()
	return t
}

// reload reads the settings again, dropping the draft. A turned off tool
// only answers settings.get.
func (t *pinTab) reload() {
	t.loading, t.err = true, ""
	disabled := moduleDisabled(t.env, "always-on-top")
	core.Go(t.env, func() (core.Preserved[pinPreferences], error) {
		var settings core.Preserved[pinPreferences]
		if disabled {
			err := t.env.Host.CallCompletingInto("always-on-top.settings.get", nil, &settings)
			return settings, err
		}
		var snapshot struct {
			Settings *core.Preserved[pinPreferences] `json:"settings"`
		}
		snapshot.Settings = &settings
		err := t.env.Host.CallCompletingInto("always-on-top.snapshot", nil, &snapshot)
		return settings, err
	}, func(settings core.Preserved[pinPreferences], err error) {
		t.loading = false
		if err != nil {
			t.err = err.Error()
			return
		}
		t.load(settings)
	})
}

func (t *pinTab) load(settings core.Preserved[pinPreferences]) {
	draft, saved := settings, settings
	draft.Value.ExcludedApps = slices.Clone(settings.Value.ExcludedApps)
	t.draft, t.saved = &draft, &saved
	t.exclusions = strings.Join(settings.Value.ExcludedApps, "\n")
}

// excluded is the list the text saves: lines trimmed, blank ones dropped.
func (t *pinTab) excluded() []string {
	apps := []string{}
	for _, line := range strings.Split(t.exclusions, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			apps = append(apps, line)
		}
	}
	return apps
}

// value is the document a save sends.
func (t *pinTab) value() core.Preserved[pinPreferences] {
	value := *t.draft
	value.Value.ExcludedApps = t.excluded()
	return value
}

// dirty compares what a save would send with what the Host stored, so
// undoing an edit by hand also clears it.
func (t *pinTab) dirty() bool {
	if t.draft == nil || t.saved == nil {
		return false
	}
	a, errA := json.Marshal(t.value())
	b, errB := json.Marshal(*t.saved)
	return errA != nil || errB != nil || !bytes.Equal(a, b)
}

func wholeIn(value, low, high float64) bool {
	return value == float64(int(value)) && value >= low && value <= high
}

func (t *pinTab) validNumbers() bool {
	v := t.draft.Value
	return wholeIn(v.FrameThickness, 1, 15) && wholeIn(v.FrameOpacity, 10, 100)
}

func (t *pinTab) save(c *ui.Context) {
	if t.draft == nil || t.working {
		return
	}
	t.working, t.err = true, ""
	value := t.value()
	core.Go(t.env, func() (core.Preserved[pinPreferences], error) {
		var saved core.Preserved[pinPreferences]
		err := t.env.Host.CallCompletingInto("always-on-top.settings.save", value, &saved)
		return saved, err
	}, func(saved core.Preserved[pinPreferences], err error) {
		t.working = false
		if err != nil {
			t.err = err.Error()
			return
		}
		t.load(saved)
		t.env.Toast(i18n.T("common.saved"))
	})
}

func (t *pinTab) Close() {
	t.env.Activity.Report("always-on-top", false, false, nil)
	t.env.Activity.Report("module-availability:always-on-top", false, false, nil)
	if t.shortcuts != nil {
		t.shortcuts.Close()
	}
}

func (t *pinTab) View(c *ui.Context, locked bool) {
	busy := t.working || t.loading
	dirty := t.dirty()
	// Reading is safe to leave; only a save holds the guard.
	t.env.Activity.Report("always-on-top", dirty, t.working, t.reload)
	t.modules.Switch(c, "always-on-top", locked)
	ui.Column(c).FillWidth().Gap(kit.GroupGap).Children(func() {
		// Switching asks about unsaved changes as every settings tab does,
		// though the drafts would stay: the user decides before moving on.
		kit.Tabs(c, &t.tab, []kit.Tab{
			{Label: i18n.T("aot.general"), Icon: icons.Settings},
			{Label: i18n.T("aot.options"), Icon: icons.SlidersHorizontal},
			{Label: i18n.T("shortcuts.title"), Icon: icons.Keyboard},
		}, t.env.RequestChange)
		if t.err != "" && t.draft != nil {
			kit.Notice(c, kit.Failure, "", t.err, nil)
		}
		if moduleDisabled(t.env, "always-on-top") {
			kit.Status(c, i18n.T("aot.disabled"))
		}
		if t.tab == pinShortcuts {
			if t.shortcuts == nil {
				t.shortcuts = widgets.NewShortcuts(t.env, "always-on-top")
			}
			t.shortcuts.View(c)
			return
		}
		// A shortcut edit made on its tab stays guarded while another shows.
		if t.shortcuts != nil {
			t.shortcuts.Report()
		}
		if t.draft == nil {
			if t.err != "" {
				kit.Notice(c, kit.Failure, "", t.err, func() {
					if kit.Button(c, i18n.T("common.retry"), icons.RefreshCw, t.loading).Disabled(busy).Clicked() {
						t.reload()
					}
				})
			} else {
				kit.Status(c, i18n.T("aot.loading"))
			}
			return
		}
		valid := t.validNumbers()
		t.env.Rail.Set(
			core.Action{Label: i18n.T("aot.save"), Disabled: !dirty || !valid || busy, Busy: t.working, Run: func() { t.save(c) }},
			core.Action{Disabled: !dirty || busy, Run: t.reload},
		)
		if t.tab == pinGeneral {
			t.general(c, busy, valid)
		} else {
			t.advanced(c, busy)
		}
	})
}

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func (t *pinTab) general(c *ui.Context, busy, valid bool) {
	v := &t.draft.Value
	kit.Group(c, icons.Frame, i18n.T("settingsDesign.borderAppearance"), "", func() {
		kit.Toggle(c, &v.Frame, i18n.T("aot.frame"), "").Disabled(busy)
		// The frame's options stay reachable while their numbers are out
		// of range, so a turned off frame cannot hide what blocks saving.
		if !v.Frame && valid {
			return
		}
		source := "custom"
		if v.UseAccent {
			source = "system"
		}
		kit.Row(c, i18n.T("settingsDesign.accentSource"), "", func() {
			if kit.Segmented(c, &source, []kit.SelectOption{
				{Value: "system", Label: i18n.T("settingsDesign.accentSystem")},
				{Value: "custom", Label: i18n.T("settingsDesign.accentCustom")},
			}, i18n.T("settingsDesign.accentSource")) && !busy {
				v.UseAccent = source == "system"
			}
		})
		// A system color is never changed by an edit below it: the custom
		// parameters keep their draft and wait.
		custom := busy || v.UseAccent
		kit.Indented(c, func() {
			kit.Row(c, i18n.T("aot.color"), "", func() {
				color := ui.RGB(128, 128, 128)
				if hexColor.MatchString(v.FrameColor) {
					color = ui.Hex(v.FrameColor)
				}
				if ui.ColorWell(c, &color).Label(i18n.T("aot.color")).Disabled(custom).Changed() {
					v.FrameColor = fmt.Sprintf("#%02x%02x%02x", color.R, color.G, color.B)
				}
			})
			kit.Row(c, i18n.T("aot.thickness"), "", func() {
				kit.NumberField(c, &v.FrameThickness, 1, 15, 1, "px", i18n.T("aot.thickness")).Disabled(custom)
			})
			kit.Row(c, i18n.T("aot.opacity"), "", func() {
				kit.NumberField(c, &v.FrameOpacity, 10, 100, 1, "%", i18n.T("aot.opacity")).Disabled(custom)
			})
			kit.Toggle(c, &v.RoundCorners, i18n.T("aot.rounded"), "").Disabled(custom)
		})
		kit.Stack(c, "", i18n.T("settingsDesign.customBorderHint"), func() {
			if !valid {
				kit.InlineError(c, i18n.T("aot.numberRange"))
			}
		})
	})
	kit.Group(c, icons.BellRing, i18n.T("settingsDesign.feedback"), "", func() {
		kit.Toggle(c, &v.Sound, i18n.T("aot.sound"), "").Disabled(busy)
	})
}

func (t *pinTab) advanced(c *ui.Context, busy bool) {
	v := &t.draft.Value
	kit.Group(c, icons.Gamepad2, i18n.T("settingsDesign.usageLimits"), "", func() {
		kit.Toggle(c, &v.BlockInGameMode, i18n.T("aot.game"), "").Disabled(busy)
		kit.Stack(c, i18n.T("aot.exclusions"), "", func() {
			area := ui.TextArea(c, &t.exclusions).FillWidth().MinHeight(96).Font(kit.MonoFont).
				Label(i18n.T("aot.exclusions")).Disabled(busy)
			// Dropped programs join the list once each.
			widgets.DropArea(c, t.env, area, widgets.DropFiles, true, busy || t.draft == nil, func(paths []string, err error) {
				if err != nil {
					t.err = err.Error()
					return
				}
				t.err = ""
				lines := []string{}
				for _, line := range strings.Split(t.exclusions, "\n") {
					if line != "" {
						lines = append(lines, line)
					}
				}
				for _, path := range paths {
					if !slices.Contains(lines, path) {
						lines = append(lines, path)
					}
				}
				t.exclusions = strings.Join(lines, "\n")
			})
		})
	})
}
