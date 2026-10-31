package convenience

import (
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/widgets"
	"winone/internal/timedpower"
)

var powerActions = []string{"shutdown", "restart", "hibernate", "sleep", "lock", "logoff"}

// closing actions end the session's programs.
func closing(action string) bool { return action == "shutdown" || action == "restart" || action == "logoff" }

// clock is hours, minutes and seconds, edited apart; they become seconds
// or a time of day only at the Host boundary, never while a digit is typed.
type clock [3]float64

func secondsClock(seconds int) clock {
	return clock{float64(seconds / 3600), float64(seconds / 60 % 60), float64(seconds % 60)}
}

func timeClock(value string) clock {
	var parts clock
	for i, part := range strings.SplitN(value, ":", 3) {
		n, _ := strconv.Atoi(part)
		parts[i] = float64(n)
	}
	return parts
}

// powerDraft is the form: the saved template with its times in parts.
type powerDraft struct {
	preferences timedpower.Preferences
	delay, at   clock
	reminder    float64
}

func toDraft(p timedpower.Preferences) powerDraft {
	p = p.Clone()
	return powerDraft{preferences: p, delay: secondsClock(p.DelaySeconds), at: timeClock(p.TimeOfDay), reminder: float64(p.ReminderSeconds)}
}

func (d powerDraft) toPreferences() timedpower.Preferences {
	p := d.preferences.Clone()
	p.DelaySeconds = int(d.delay[0])*3600 + int(d.delay[1])*60 + int(d.delay[2])
	p.TimeOfDay = fmt.Sprintf("%02d:%02d:%02d", int(d.at[0]), int(d.at[1]), int(d.at[2]))
	p.ReminderSeconds = int(d.reminder)
	return p
}

// remaining formats the time left before deadline as hh:mm:ss.
func remaining(deadline, now time.Time) string {
	seconds := int(max(0, deadline.Sub(now).Seconds()+0.999))
	return fmt.Sprintf("%02d:%02d:%02d", seconds/3600, seconds/60%60, seconds%60)
}

// powerTab is timed power: the running task, from the Host's state, and
// the saved template that starts one. Live state never replaces an
// unfinished template.
//
// React source: features/timed-power.tsx.
type powerTab struct {
	env     *core.Env
	modules *widgets.Modules
	draft   *powerDraft
	saved   *powerDraft
	status  *timedpower.Snapshot
	now     time.Time
	busy    bool
	err     string
	// generation discards a poll that a newer operation overtook.
	generation int
	polling    bool
	closed     bool
	confirm    *timedpower.Preferences
	confirmOn  bool
}

func newPowerTab(env *core.Env) *powerTab {
	t := &powerTab{env: env, modules: widgets.NewModules(env), now: time.Now()}
	t.load()
	if !moduleDisabled(env, "timed-power") {
		t.readStatus(false)
	}
	return t
}

func (t *powerTab) load() {
	t.err = ""
	core.Go(t.env, func() (timedpower.Preferences, error) {
		var p timedpower.Preferences
		err := t.env.Host.CallCompletingInto("timed-power.settings.get", map[string]any{}, &p)
		return p, err
	}, func(p timedpower.Preferences, err error) {
		if t.closed {
			return
		}
		if err != nil {
			t.err = err.Error()
			return
		}
		draft, saved := toDraft(p), toDraft(p)
		t.draft, t.saved = &draft, &saved
	})
}

// readStatus reads the running task; poll marks the once-a-second read
// while a task runs.
func (t *powerTab) readStatus(poll bool) {
	version := t.generation
	core.Go(t.env, func() (timedpower.Snapshot, error) {
		var s timedpower.Snapshot
		err := t.env.Host.CallCompletingInto("timed-power.snapshot", map[string]any{}, &s)
		return s, err
	}, func(s timedpower.Snapshot, err error) {
		if poll {
			t.polling = false
		}
		if t.closed {
			return
		}
		if err != nil {
			t.err = err.Error()
			return
		}
		if version == t.generation {
			t.status, t.now = &s, time.Now()
		}
	})
}

// run performs one operation at a time, then done applies its result on
// the UI thread; polls started before it are discarded, as their state is
// older.
func (t *powerTab) run(work func() error, done func()) {
	if t.busy {
		return
	}
	t.generation++
	t.busy, t.err = true, ""
	core.Do(t.env, work, func(err error) {
		t.generation++
		t.busy = false
		if err != nil {
			t.err = err.Error()
			return
		}
		done()
	})
}

// setStatus shows a state an operation returned.
func (t *powerTab) setStatus(s timedpower.Snapshot) { t.status, t.now = &s, time.Now() }

func (t *powerTab) start(p timedpower.Preferences, confirmed bool) {
	var s timedpower.Snapshot
	t.run(func() error {
		return t.env.Host.CallCompletingInto("timed-power.start", map[string]any{"preferences": p, "confirmed": confirmed}, &s)
	}, func() {
		t.setStatus(s)
		t.confirm, t.confirmOn = nil, false
	})
}

func (t *powerTab) dirty() bool {
	return t.draft != nil && t.saved != nil && !reflect.DeepEqual(t.draft.toPreferences(), t.saved.toPreferences())
}

func (t *powerTab) discard() {
	if t.saved != nil {
		draft := *t.saved
		draft.preferences = draft.preferences.Clone()
		t.draft = &draft
	}
	t.err = ""
}

func (t *powerTab) Close() {
	t.closed = true
	t.env.Activity.Report("timed-power", false, false, nil)
	t.env.Activity.Report("module-availability:timed-power", false, false, nil)
}

func (t *powerTab) taskLabel(p timedpower.Preferences) string {
	tag := p.Mode
	switch {
	case p.DryRun:
		tag = "dryRunTag"
	case closing(p.Action) && p.Force:
		tag = "forceTag"
	case closing(p.Action):
		tag = "normalTag"
	}
	return i18n.T("timedPower."+p.Action) + " · " + i18n.T("timedPower."+tag)
}

func (t *powerTab) View(c *ui.Context, locked bool) {
	disabled := moduleDisabled(t.env, "timed-power")
	t.env.Activity.Report("timed-power", t.dirty(), t.busy, t.discard)
	t.modules.Switch(c, "timed-power", locked)
	// The running task follows the Host once a second while it runs.
	if disabled {
		t.status = nil
	} else if t.status != nil && t.status.Active {
		c.After(time.Second)
		if !t.polling && time.Since(t.now) >= time.Second && !t.busy {
			t.polling = true
			t.readStatus(true)
		}
	}
	pageLocked := t.busy || t.confirm != nil
	ui.Column(c).FillWidth().Gap(kit.GroupGap).Children(func() {
		if t.err != "" {
			kit.Notice(c, kit.Failure, "", t.err, func() {
				if t.draft == nil && kit.Button(c, i18n.T("common.retry"), icons.RefreshCw, false).Clicked() {
					t.load()
				}
			})
		}
		if disabled {
			kit.Status(c, i18n.T("timedPower.disabled"))
		}
		t.current(c)
		if t.draft == nil {
			if t.err == "" {
				kit.Status(c, i18n.T("timedPower.loading"))
			}
			return
		}
		valid := t.draft.toPreferences().Validate() == nil
		dirty := t.dirty()
		t.env.Rail.Set(
			core.Action{Label: i18n.T("timedPower.save"), Disabled: !dirty || !valid || pageLocked, Busy: t.busy, Run: func() { t.save(c) }},
			core.Action{Disabled: !dirty || pageLocked, Run: t.discard},
		)
		ui.Column(c).FillWidth().Gap(kit.GroupGap).Disabled(pageLocked).Children(func() {
			t.schedule(c)
			t.power(c)
			t.reminder(c)
		})
		ui.Column(c).FillWidth().Gap(8).Children(func() {
			if !valid {
				kit.InlineError(c, i18n.T("timedPower.invalid"))
			}
			ui.Row(c).Children(func() {
				active := t.status != nil && t.status.Active
				if kit.Button(c, i18n.T("timedPower.start"), icons.Play, false).Disabled(pageLocked || disabled || !valid || active).Clicked() {
					p := t.draft.toPreferences()
					if p.DryRun {
						t.start(p, false)
					} else {
						t.confirm, t.confirmOn = &p, true
					}
				}
			})
			kit.Help(c, i18n.T("timedPower.lifetime"))
		})
	})
	t.confirmation(c)
}

// current shows the running task from the Host's state.
func (t *powerTab) current(c *ui.Context) {
	theme := c.Theme()
	kit.Section(c, icons.Timer, i18n.T("timedPower.current"), "", func() {
		s := t.status
		state := "idle"
		if s != nil && s.State != "" {
			state = s.State
		}
		kit.Status(c, i18n.T("timedPower."+state))
		if s != nil && s.Active && s.Task != nil {
			kit.Label(c, t.taskLabel(*s.Task)+" · "+s.Deadline.Local().Format("2006/1/2 15:04:05"))
			ui.Text(c, remaining(s.Deadline, time.Now())).FontSize(theme.Rem(1.875)).FontWeight(500).
				Label(i18n.T("timedPower.remaining")).Font(kit.MonoFont)
			ui.Row(c).Children(func() {
				if kit.Button(c, i18n.T("timedPower.cancel"), icons.Square, false).Disabled(t.busy || s.State == "executing").Clicked() {
					var next timedpower.Snapshot
					t.run(func() error {
						return t.env.Host.CallCompletingInto("timed-power.cancel", map[string]any{}, &next)
					}, func() { t.setStatus(next) })
				}
			})
			kit.Help(c, i18n.T("timedPower.activeHint"))
		}
		if s != nil && s.LastEvent == "missed" && s.Active {
			kit.Help(c, i18n.T("timedPower.missed"))
		}
		if s != nil && s.Error != "" {
			kit.Notice(c, kit.Failure, "", s.Error, nil)
		}
		if s != nil && s.NoticeError != "" {
			kit.Notice(c, kit.Failure, "", i18n.T("timedPower.noticeError", "message", s.NoticeError), nil)
		}
	})
}

// clockFields edits hours, minutes and seconds side by side; after shows
// what fills them, as presets.
func clockFields(c *ui.Context, label string, value *clock, maxHours float64, after func()) {
	kit.Stack(c, label, "", func() {
		ui.Grid(c).FillWidth().ColumnTracks(ui.Fr(1), ui.Fr(1), ui.Fr(1)).Gap(12).Role(ui.RoleGroup).Label(label).Children(func() {
			for i, part := range []string{"hours", "minutes", "seconds"} {
				high := 59.0
				if i == 0 {
					high = maxHours
				}
				kit.UnitNumber(c.Key(part), &value[i], 0, high, i18n.T("timedPower."+part+"Unit"), i18n.T("timedPower."+part))
			}
		})
		if after != nil {
			after()
		}
	})
}

func (t *powerTab) schedule(c *ui.Context) {
	d := t.draft
	kit.Group(c, icons.Timer, i18n.T("timedPower.schedule"), "", func() {
		kit.Row(c, i18n.T("timedPower.mode"), "", func() {
			kit.Segmented(c, &d.preferences.Mode, []kit.SelectOption{
				{Value: "countdown", Label: i18n.T("timedPower.countdown")},
				{Value: "time", Label: i18n.T("timedPower.time")},
				{Value: "weekly", Label: i18n.T("timedPower.weekly")},
			}, i18n.T("timedPower.mode"))
		})
		if d.preferences.Mode == "countdown" {
			clockFields(c, i18n.T("timedPower.delay"), &d.delay, 168, func() {
				// Presets are light and outlined, so they never compete
				// with the fields they fill.
				ui.Row(c).FillWidth().Gap(8).Wrap().Role(ui.RoleGroup).Label(i18n.T("timedPower.presets")).Children(func() {
					for _, minutes := range []int{15, 30, 60, 90, 120, 150} {
						if presetChip(c, i18n.T("timedPower.preset"+strconv.Itoa(minutes))).Clicked() {
							d.delay = clock{float64(minutes / 60), float64(minutes % 60), 0}
						}
					}
				})
			})
		} else {
			clockFields(c, i18n.T("timedPower.timeOfDay"), &d.at, 23, func() { kit.Help(c, i18n.T("timedPower.timeHint")) })
		}
		if d.preferences.Mode == "weekly" {
			kit.Stack(c, i18n.T("timedPower.days"), "", func() {
				ui.Row(c).Gap(20).Wrap().Role(ui.RoleGroup).Label(i18n.T("timedPower.days")).Children(func() {
					for _, day := range []int{1, 2, 3, 4, 5, 6, 0} {
						on := slices.Contains(d.preferences.Weekdays, day)
						if ui.Checkbox(c.Key(day), &on, i18n.T("timedPower.day"+strconv.Itoa(day))).Changed() {
							days := slices.DeleteFunc(slices.Clone(d.preferences.Weekdays), func(v int) bool { return v == day })
							if on {
								days = append(days, day)
								slices.Sort(days)
							}
							d.preferences.Weekdays = days
						}
					}
				})
			})
		}
	})
}

// presetChip is a light, rounded button that fills a value.
func presetChip(c *ui.Context, label string) ui.Element {
	theme, p := c.Theme(), kit.Colors(c)
	b := ui.ButtonBase(c).Padding(4, 12).Radius(999).Border(1, p.Hairline).Label(label)
	color := theme.TextMuted
	if b.Hovered() {
		b.Background(theme.SurfaceHover).Border(1, theme.Border)
		color = theme.Text
	}
	b.Children(func() { ui.Text(c, label).FontSize(kit.HelpSize(c)).TextColor(color).SingleLine() })
	return b
}

func (t *powerTab) power(c *ui.Context) {
	p := &t.draft.preferences
	kit.Group(c, icons.Power, i18n.T("timedPower.power"), "", func() {
		kit.Row(c, i18n.T("timedPower.action"), "", func() {
			options := make([]kit.SelectOption, len(powerActions))
			for i, action := range powerActions {
				options[i] = kit.SelectOption{Value: action, Label: i18n.T("timedPower." + action)}
			}
			_, trigger := kit.Select(c, &p.Action, options)
			trigger.Label(i18n.T("timedPower.action"))
		})
		if closing(p.Action) {
			hint := "timedPower.normalHint"
			if p.Force {
				hint = "timedPower.forceHint"
			}
			kit.Toggle(c, &p.Force, i18n.T("timedPower.force"), i18n.T(hint))
		}
	})
}

func (t *powerTab) reminder(c *ui.Context) {
	d := t.draft
	kit.Group(c, icons.BellRing, i18n.T("timedPower.dryRunTag"), "", func() {
		kit.Row(c, i18n.T("timedPower.reminder"), "", func() {
			kit.NumberField(c, &d.reminder, 0, 3600, 1, i18n.T("timedPower.secondsUnit"), i18n.T("timedPower.reminder"))
		})
		kit.Toggle(c, &d.preferences.DryRun, i18n.T("timedPower.dryRun"), i18n.T("timedPower.dryRunHint"))
	})
}

func (t *powerTab) save(c *ui.Context) {
	p := t.draft.toPreferences()
	var saved timedpower.Preferences
	t.run(func() error {
		return t.env.Host.CallCompletingInto("timed-power.settings.save", p, &saved)
	}, func() {
		draft, kept := toDraft(saved), toDraft(saved)
		t.draft, t.saved = &draft, &kept
		t.env.Toast(i18n.T("timedPower.saved"))
	})
}

// confirmation asks before a task that acts on the computer starts.
func (t *powerTab) confirmation(c *ui.Context) {
	if t.confirm == nil {
		return
	}
	p := *t.confirm
	description := "timedPower.confirmNormal"
	if p.Force && closing(p.Action) {
		description = "timedPower.confirmForce"
	}
	when := p.TimeOfDay
	if p.Mode == "countdown" {
		when = remaining(time.Now().Add(time.Duration(p.DelaySeconds)*time.Second), time.Now())
	}
	err := ""
	if t.confirmOn {
		err = t.err
	}
	if kit.Dialog(c, &t.confirmOn, kit.DialogOptions{
		Title: i18n.T("timedPower.confirmTitle"), Message: i18n.T(description), Error: err,
		Confirm: i18n.T("timedPower.confirm"), Cancel: i18n.T("timedPower.back"), Danger: p.Force && closing(p.Action),
		Busy: t.busy,
		Body: func() { kit.Strong(c, t.taskLabel(p)+" · "+when) },
	}) {
		t.start(p, true)
	}
	if !t.confirmOn && !t.busy {
		t.confirm = nil
	}
}
