package wintray

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

// programState is whether the selected program runs and shows a window,
// read every 1.5 seconds while its actions can be used. It is separate
// from the settings: reading it never replaces the draft.
type programState struct {
	id      string
	value   *struct{ Running, HasWindow, Visible bool }
	err     string
	pending bool
	// next is when to read again, on the window's clock (c.Now); zero
	// reads at the next frame. requested is when the read started.
	next, requested time.Time
	// generation discards an answer about a program no longer selected.
	generation int
}

// refresh reads the state at the next frame, as after an action.
func (s *programState) refresh() { s.next = time.Time{} }

const statePeriod = 1500 * time.Millisecond

// poll keeps the selected program's state current while the actions are
// usable (the module runs, nothing is unsaved or running, no recovery is
// pending). The window builds no frames while hidden, so neither does it
// read then.
func (p *page) poll(c *ui.Context, app program, usable bool) {
	s := &p.state
	if s.id != app.ID {
		s.generation++
		*s = programState{id: app.ID, generation: s.generation}
	}
	if !usable {
		s.next = time.Time{}
		return
	}
	now := c.Now()
	if s.pending {
		return
	}
	if now.Before(s.next) {
		c.After(s.next.Sub(now))
		return
	}
	s.pending, s.requested = true, now
	owner, id := s.generation, app.ID
	type state = struct{ Running, HasWindow, Visible bool }
	core.Go(p.env, func() (*state, error) {
		var value struct {
			Running   *bool `json:"running"`
			HasWindow bool  `json:"hasWindow"`
			Visible   bool  `json:"visible"`
		}
		if err := p.env.Host.CallCompletingInto("wintray.program.state", map[string]string{"id": id}, &value); err != nil {
			return nil, err
		}
		if value.Running == nil {
			return nil, core.Errorf(i18n.T("wintray.unknownResult"))
		}
		return &state{*value.Running, value.HasWindow, value.Visible}, nil
	}, func(value *state, err error) {
		if owner != s.generation {
			return
		}
		s.pending, s.next = false, s.requested.Add(statePeriod)
		if err != nil {
			s.value, s.err = nil, err.Error()
			return
		}
		s.value, s.err = value, ""
	})
}

// details shows the selected program: its name and file, the actions on
// the running program, and its settings.
func (p *page) details(c *ui.Context) {
	ui.Scroll(c).Fill().Padding(16, 4, 20, 20).Children(func() {
		app := p.active()
		if app == nil {
			kit.Help(c, i18n.T("wintray.select"))
			return
		}
		if p.detailFor != app.ID {
			// Each program opens on its common settings, or on its
			// information when its name is empty.
			p.detailFor, p.detailTab = app.ID, 0
			if strings.TrimSpace(app.Name) == "" {
				p.detailTab = 1
			}
		}
		name := p.displayName(*app)
		ui.Column(c).FillWidth().Gap(16).Children(func() {
			ui.Column(c).FillWidth().Gap(4).Children(func() {
				kit.Heading(c, name).Role(ui.RoleHeading).Level(2).Selectable()
				kit.Mono(c, app.ExePath).TextColor(c.Theme().TextMuted).SingleLine().Ellipsis("…").Tooltip(app.ExePath).Selectable()
			})
			p.actions(c, *app, name)
			kit.SubTabs(c, &p.detailTab, []string{i18n.T("wintray.commonSettings"), i18n.T("wintray.moreSettings")}, p.env.RequestChange)
			ui.Column(c).FillWidth().Gap(kit.GroupGap).Disabled(p.blocked()).Children(func() {
				if p.detailTab == 0 {
					p.timing(c, app)
				} else {
					p.information(c, app)
				}
			})
		})
	})
}

// actions starts or closes the program, shows or hides its window and
// removes it from the list.
func (p *page) actions(c *ui.Context, app program, name string) {
	dirty := p.dirty()
	usable := !p.disabled && !dirty && !p.recovering()
	p.poll(c, app, usable && !p.busy)
	blocked := !usable || p.busy
	state := p.state.value
	ui.Row(c).FillWidth().Gap(8).Wrap().AlignItems(ui.Center).Role(ui.RoleGroup).Label(i18n.T("wintray.programActions")).Children(func() {
		if state != nil && state.Running {
			if kit.Button(c, i18n.T("wintray.stop"), icons.Square, p.job == "program.stop").Disabled(blocked).Clicked() {
				p.stopConfirm = true
			}
		} else if kit.Button(c, i18n.T("wintray.launch"), icons.Play, p.job == "launch").Disabled(blocked).Clicked() {
			p.runJob("launch", map[string]string{"id": app.ID})
		}
		visible := state != nil && state.Visible
		label, icon := "wintray.showWindow", icons.PanelTop
		if visible {
			label, icon = "wintray.hideWindow", icons.PanelTopClose
		}
		window := kit.Button(c, i18n.T(label), icon, p.job == "program.window").Disabled(blocked || state == nil || !state.HasWindow)
		if state == nil || !state.HasWindow {
			window.Tooltip(i18n.T("wintray.noWindow"))
		}
		if window.Clicked() {
			p.runJob("program.window", map[string]any{"id": app.ID, "visible": !visible})
		}
		if p.state.err != "" {
			ui.Row(c).Gap(4).AlignItems(ui.Center).Role(ui.RoleStatus).Children(func() {
				kit.Help(c, i18n.T("wintray.stateUnavailable")).Tooltip(p.state.err)
				if kit.IconButton(c, icons.RefreshCw, i18n.T("common.retry")).Disabled(blocked).Clicked() {
					p.state.refresh()
				}
			})
		}
		if kit.Button(c, i18n.T("wintray.remove"), icons.Trash2, false).Disabled(p.blocked()).Clicked() {
			p.removeSelected()
		}
	})
}

func (p *page) removeSelected() {
	if p.blocked() || p.draft == nil {
		return
	}
	apps := p.draft.Value.ManagedApps
	var kept []core.Preserved[program]
	for _, app := range apps {
		if app.Value.ID == p.selected {
			for key := range p.texts {
				if len(key) > len(app.Value.ID) && key[:len(app.Value.ID)+1] == app.Value.ID+"/" {
					delete(p.texts, key)
				}
			}
			continue
		}
		kept = append(kept, app)
	}
	if kept == nil {
		kept = []core.Preserved[program]{}
	}
	p.draft.Value.ManagedApps = kept
	p.selected = ""
	if len(kept) > 0 {
		p.selected = kept[0].Value.ID
	}
}

func (p *page) stopDialog(c *ui.Context) {
	app := p.active()
	if !p.stopConfirm || app == nil {
		p.stopConfirm = false
		return
	}
	open := true
	confirmed := kit.Dialog(c, &open, kit.DialogOptions{
		Title: i18n.T("wintray.stopTitle", "name", p.displayName(*app)), Message: i18n.T("wintray.stopDescription"),
		Confirm: i18n.T("wintray.confirmStop"), Cancel: i18n.T("wintray.cancelReset"), Danger: true,
		ConfirmDisabled: p.busy || p.disabled || p.dirty() || p.recovering(),
	})
	if confirmed {
		p.stopConfirm = false
		p.runJob("program.stop", map[string]any{"id": app.ID, "confirmed": true})
		return
	}
	if !open && !p.busy {
		p.stopConfirm = false
	}
}

// timing is how and when the program starts.
func (p *page) timing(c *ui.Context, app *program) {
	id := app.ID
	kit.Group(c, icons.Rocket, i18n.T("settingsDesign.launchBehavior"), "", func() {
		startup := app.RunOnStartup
		if kit.Toggle(c, &startup, i18n.T("wintray.startup"), "").Changed() {
			p.update(id, func(a *program) { a.RunOnStartup = startup })
		}
		current := mode(*app)
		values := []string{"tray", "hidden", "task", "normal"}
		if !executable(app.ExePath) {
			values = []string{"hidden"}
		}
		options := make([]kit.SelectOption, len(values))
		for i, value := range values {
			options[i] = kit.SelectOption{Value: value, Label: i18n.T("wintray." + value)}
		}
		kit.Stack(c, i18n.T("wintray.mode"), "", func() {
			// The choices keep their own width, as React's fit-content
			// track did; the box shrinks with the row, and they wrap,
			// only when they do not fit.
			ui.Row(c).FillWidth().Children(func() {
				ui.Box(c).Shrink(1).MinWidth(0).Children(func() {
					if kit.Segmented(c, &current, options, i18n.T("wintray.mode")) {
						p.update(id, func(a *program) { setMode(a, current) })
					}
				})
			})
		})
		if current == "tray" {
			kit.Indented(c, func() {
				p.numberRow(c, programKey(id, "closeDelay"), i18n.T("wintray.closeDelay"), &app.TrayBehavior.Value.CloseDelaySeconds, 0, 600, p.blocked())
			})
		}
	})
	plan := &app.Schedule.Value
	kit.Group(c, icons.CalendarClock, i18n.T("settingsDesign.schedule"), "", func() {
		enabled := plan.Enabled
		if kit.Toggle(c, &enabled, i18n.T("wintray.schedule"), "").Changed() {
			p.update(id, func(a *program) { a.Schedule.Value.Enabled = enabled })
		}
		if plan.Enabled {
			kit.Indented(c, func() {
				p.numberRow(c, programKey(id, "startDelay"), i18n.T("wintray.startDelay"), &plan.StartDelayMinutes, 0, 1440, p.blocked())
				p.numberRow(c, programKey(id, "autoExit"), i18n.T("wintray.autoExit"), &plan.AutoExitMinutes, 0, 1440, p.blocked())
			})
		}
		// Run limits belong to the same plan: available once it is on.
		limit := plan.FrequencyEnabled
		help := ""
		if !plan.Enabled {
			help = i18n.T("settingsDesign.scheduleRequired")
		}
		if kit.Toggle(c, &limit, i18n.T("wintray.limitRuns"), help).Disabled(!plan.Enabled).Changed() {
			p.update(id, func(a *program) { a.Schedule.Value.FrequencyEnabled = limit })
		}
		if plan.Enabled && plan.FrequencyEnabled {
			kit.Indented(c, func() {
				p.numberRow(c, programKey(id, "days"), i18n.T("wintray.days"), &plan.FrequencyDays, 1, 365, p.blocked())
				p.numberRow(c, programKey(id, "runs"), i18n.T("wintray.runs"), &plan.FrequencyRuns, 1, 1000, p.blocked())
			})
		}
	})
}

// information is the program's name and its startup arguments.
func (p *page) information(c *ui.Context, app *program) {
	id := app.ID
	kit.Panel(c, func() {
		nameInput := p.handle(programKey(id, "name"))
		field(c, "name:"+id, i18n.T("wintray.name"), nameInput, true, func() {
			name := app.Name
			message := ""
			if strings.TrimSpace(name) == "" {
				message = i18n.T("wintray.nameRequired", "programs", p.displayName(*app))
			}
			ui.TextInput(c, &name).FillWidth().Label(i18n.T("wintray.name")).Error(message).Bind(nameInput)
			if name != app.Name {
				p.update(id, func(a *program) { a.Name = name })
			}
		})
		argsInput := p.handle(programKey(id, "args"))
		field(c, "args:"+id, i18n.T("wintray.args"), argsInput, true, func() {
			args := app.Args
			ui.TextInput(c, &args).FillWidth().Label(i18n.T("wintray.args")).Bind(argsInput)
			if args != app.Args {
				p.update(id, func(a *program) { a.Args = args })
			}
		})
	})
}
