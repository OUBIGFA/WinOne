package automations

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/widgets"
)

func field(c *ui.Context, label string, value *string, multiline bool) {
	ui.Column(c).FillWidth().Gap(5).Children(func() {
		kit.Label(c, label)
		if multiline {
			ui.TextArea(c, value).FillWidth().MinHeight(100).Label(label)
		} else {
			ui.TextInput(c, value).FillWidth().Label(label)
		}
	})
}
func choice(c *ui.Context, label string, value *string, options ...kit.SelectOption) bool {
	changed := false
	ui.Column(c).FillWidth().Gap(5).Children(func() { kit.Label(c, label); changed, _ = kit.Select(c, value, options) })
	return changed
}
func option(value, key string) kit.SelectOption {
	return kit.SelectOption{Value: value, Label: tr(key)}
}
func (p *Page) editorView(c *ui.Context) {
	d := p.draft
	if d == nil {
		return
	}
	open := true
	title := tr("create")
	if d.ID != "" {
		title = tr("edit")
	}
	busy := p.busy["save"] || p.busy["hook"]
	confirmed := kit.Dialog(c, &open, kit.DialogOptions{Title: title, Error: d.err, Confirm: tr("save"), Cancel: tr("cancel"), Busy: busy, Width: 760, ConfirmDisabled: !d.changed() && d.ID != "", Actions: func() {
		if kit.Button(c, tr("saveTemplate"), icons.LayoutTemplate, p.busy["template.save:"]).Disabled(busy).Clicked() {
			p.saveTemplate()
		}
	}, Body: func() {
		ui.Column(c).FillWidth().Gap(18).Disabled(busy).Children(func() {
			field(c, tr("name"), &d.Value.Name, false)
			mode := d.Value.ExecutionMode
			if mode == "" {
				mode = "agent"
			}
			if choice(c, tr("execution"), &mode, option("agent", "agent"), option("script", "script")) {
				d.Value.ExecutionMode = mode
				if mode == "script" && d.Value.ScriptConfig == nil {
					d.Value.ScriptConfig = &ScriptConfig{Capabilities: []string{}}
				}
			}
			if mode == "script" {
				p.scriptFields(c, d)
			} else {
				field(c, tr("prompt"), &d.Value.Prompt, true)
			}
			p.timingFields(c, d)
			p.sessionFields(c, d)
			if mode != "script" {
				p.modelFields(c, d)
			}
			p.hookFields(c, d)
			p.notificationsFields(c, d)
		})
	}})
	if confirmed {
		p.save()
	}
	if !open {
		if d.changed() {
			p.discard = true
		} else {
			p.draft = nil
		}
	}
	if p.discard {
		discard := true
		if kit.Dialog(c, &discard, kit.DialogOptions{Title: tr("unsaved"), Message: tr("unsavedHint"), Confirm: tr("discard"), Cancel: tr("continueEditing"), Danger: true}) {
			p.Discard()
		}
		if !discard {
			p.discard = false
		}
	}
}
func (p *Page) timingFields(c *ui.Context, d *Draft) {
	ui.Column(c).FillWidth().Gap(10).Children(func() {
		if choice(c, tr("timing"), &d.timing, option("cron", "fixedTime"), option("interval", "interval"), option("manual", "manual"), option("date", "specificDate")) {
			d.Value.Manual = d.timing == "manual"
			if d.timing != "interval" {
				d.Value.IntervalMs = nil
			} else if d.Value.IntervalMs == nil {
				v := int64(5 * 60000)
				d.Value.IntervalMs = &v
				d.interval = "5"
				d.intervalUnit = "minutes"
			}
			if d.timing != "date" {
				d.Value.OnceAt = nil
				d.Value.ExpireAt = nil
			} else if d.Value.OnceAt == nil {
				v := time.Now().Add(time.Hour).UnixMilli()
				d.Value.OnceAt = &v
				d.Value.Recurring = false
			}
		}
		switch d.timing {
		case "cron":
			p.cronFields(c, d)
			field(c, tr("timezone"), &d.Value.Timezone, false)
			if kit.Button(c, tr("preview"), icons.Clock, p.busy["preview"]).Clicked() {
				p.preview(d)
			}
			if d.preview != "" {
				kit.Help(c, d.preview)
			}
			kit.Toggle(c, &d.Value.Recurring, tr("recurring"), tr("onceHint"))
		case "interval":
			ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
				input := ui.TextInput(c, &d.interval).Label(tr("intervalValue")).Grow(1).Basis(120)
				changed, _ := kit.Select(c, &d.intervalUnit, []kit.SelectOption{option("seconds", "seconds"), option("minutes", "minutes"), option("hours", "hours"), option("days", "days")})
				if input.Changed() || changed {
					value, err := strconv.ParseFloat(d.interval, 64)
					unit := map[string]float64{"seconds": 1000, "minutes": 60000, "hours": 3600000, "days": 86400000}[d.intervalUnit]
					if err == nil && value > 0 && value*unit <= float64(math.MaxInt64) {
						ms := int64(value * unit)
						d.Value.IntervalMs = &ms
						d.err = ""
					} else {
						d.err = tr("invalidInterval")
					}
				}
			})
			kit.Help(c, tr("intervalHint"))
			kit.Toggle(c, &d.Value.Recurring, tr("recurring"), tr("onceHint"))
		case "date":
			tm := time.Now().Add(time.Hour)
			if d.Value.OnceAt != nil {
				tm = time.UnixMilli(*d.Value.OnceAt).Local()
			}
			ui.Row(c).Gap(8).Wrap().Children(func() {
				changed := ui.DateInput(c, &tm).Label(tr("date")).Changed()
				changed = ui.TimeInput(c, &tm).Label(tr("time")).Changed() || changed
				if changed {
					value := tm.UnixMilli()
					d.Value.OnceAt = &value
					d.Value.Recurring = false
				}
			})
		case "manual":
			kit.Help(c, tr("manualHint"))
		}
	})
}
func (p *Page) preview(d *Draft) {
	if p.busy["preview"] {
		return
	}
	p.busy["preview"] = true
	expr, zone := d.Value.CronExpr, d.Value.Timezone
	var result struct {
		NextFireAt  int64  `json:"nextFireAt"`
		Description string `json:"description"`
	}
	core.Do(p.env, func() error { return p.call("preview", map[string]string{"cronExpr": expr, "timezone": zone}, &result) }, func(err error) {
		delete(p.busy, "preview")
		if p.closed || p.draft != d || d.Value.CronExpr != expr || d.Value.Timezone != zone {
			return
		}
		if err != nil {
			d.err = err.Error()
			return
		}
		d.err = ""
		d.preview = tr("nextRun", "time", dateText(result.NextFireAt)) + " · " + result.Description
	})
}
func (p *Page) sessionFields(c *ui.Context, d *Draft) {
	v := &d.Value
	mode := "fresh"
	if v.PersistentSession {
		mode = "persistent"
	} else if v.TargetSessionID != "" {
		mode = "bound"
	}
	if choice(c, tr("sessionMode"), &mode, option("fresh", "fresh"), option("persistent", "persistent"), option("bound", "bound")) {
		v.PersistentSession = mode == "persistent"
		v.TargetSessionID = ""
		if mode == "bound" {
			v.TargetSessionID = "__pending__"
		}
	}
	if mode == "bound" {
		options := []kit.SelectOption{option("__pending__", "chooseSession")}
		for _, s := range p.sessions {
			if !s.Archived {
				options = append(options, kit.SelectOption{Value: s.ID, Label: s.Title})
			}
		}
		choice(c, tr("session"), &v.TargetSessionID, options...)
		return
	}
	if choice(c, tr("workspace"), &v.WorkspaceKind, option("dialogue", "dialogue"), option("project", "project")) {
		if v.WorkspaceKind == "dialogue" {
			v.WorkingDir = ""
			v.UseWorktree = false
		}
	}
	if v.WorkspaceKind == "project" {
		row := ui.Column(c).FillWidth().Gap(8).Children(func() {
			field(c, tr("directory"), &v.WorkingDir, false)
			if kit.Button(c, tr("browse"), icons.FolderOpen, false).Clicked() {
				core.Go(p.env, func() (string, error) { return p.env.Dialogs.PickFolder(tr("directory")) }, func(value string, err error) {
					if p.draft != d {
						return
					}
					if err != nil {
						d.err = err.Error()
					} else if value != "" {
						v.WorkingDir = value
					}
				})
			}
		})
		widgets.DropArea(c, p.env, row, widgets.DropFolders, false, false, func(paths []string, err error) {
			if p.draft != d {
				return
			}
			if err != nil {
				d.err = err.Error()
			} else if len(paths) > 0 {
				v.WorkingDir = paths[0]
			}
		})
		kit.Toggle(c, &v.UseWorktree, tr("worktree"), tr("worktreeHint"))
	}
}
func (p *Page) modelFields(c *ui.Context, d *Draft) {
	value := ""
	if d.Value.Model != "" {
		value = widgets.SelectionValue(widgets.AISelection{Provider: d.Value.ProviderID, Model: d.Value.Model})
	}
	ui.Column(c).FillWidth().Gap(8).Children(func() {
		kit.Label(c, tr("model"))
		selected, changed := widgets.ModelField(c, widgets.ModelFieldOptions{ModelMenuOptions: widgets.ModelMenuOptions{Env: p.env, Groups: widgets.ProviderGroups(p.providers), Selected: value, Label: tr("model"), Empty: tr("noModels"), Choices: []widgets.ModelChoice{{Value: "", Label: tr("followModel")}}}, Placeholder: tr("followModel")})
		if changed {
			if model, ok := widgets.ParseSelectionValue(selected); ok {
				d.Value.ProviderID = model.Provider
				d.Value.Model = model.Model
				d.Value.ModelAgentKind = "pi"
			} else {
				d.Value.ProviderID = ""
				d.Value.Model = ""
				d.Value.ModelAgentKind = ""
				d.Value.Effort = ""
				d.Value.FastMode = false
			}
		}
		// Effort is a provider word, not a hard-coded tier list. The native shared
		// model menu remains the only model chooser on this page.
		options := []kit.SelectOption{{Value: "", Label: tr("followModel")}}
		found := d.Value.Effort == ""
		for _, provider := range p.providers {
			if provider.ID != d.Value.ProviderID {
				continue
			}
			metadata := provider.ModelMetadata[d.Value.Model]
			efforts := metadata.Efforts
			if len(efforts) == 0 {
				efforts = metadata.Levels
			}
			for _, effort := range efforts {
				options = append(options, kit.SelectOption{Value: effort, Label: effort})
				found = found || effort == d.Value.Effort
			}
		}
		if !found {
			options = append(options, kit.SelectOption{Value: d.Value.Effort, Label: d.Value.Effort})
		}
		choice(c, tr("effort"), &d.Value.Effort, options...)
		kit.Toggle(c, &d.Value.FastMode, tr("fast"), "")
	})
}
func (p *Page) scriptFields(c *ui.Context, d *Draft) {
	script := d.Value.ScriptConfig
	if script == nil {
		return
	}
	field(c, tr("command"), &script.Command, true)
	timeoutField(c, tr("timeout"), &d.scriptTimeout, &script.TimeoutMs, &d.err)
	kit.Help(c, tr("capabilitiesHint"))
	for _, capability := range p.runtime.ScriptCapabilities {
		enabled := false
		for _, v := range script.Capabilities {
			if v == capability {
				enabled = true
			}
		}
		state := p.support.Capabilities[capability]
		hint := ""
		if state == "ghost-missing" {
			hint = tr("connectorUnconfigured")
		} else if state == "ghost-asleep" {
			hint = tr("connectorDisabled")
		}
		disabled := (state == "ghost-missing" || state == "ghost-asleep") && !enabled
		if kit.Toggle(c, &enabled, capability, hint).Disabled(disabled).Changed() {
			next := []string{}
			for _, v := range script.Capabilities {
				if v != capability {
					next = append(next, v)
				}
			}
			if enabled {
				next = append(next, capability)
			}
			script.Capabilities = next
		}
	}
}
func timeoutField(c *ui.Context, label string, text *string, value **int64, message *string) {
	ui.Column(c).FillWidth().Gap(5).Children(func() {
		kit.Label(c, label)
		if ui.TextInput(c, text).FillWidth().Placeholder(tr("unlimited")).Label(label).Changed() {
			if strings.TrimSpace(*text) == "" {
				*value = nil
				*message = ""
				return
			}
			n, err := strconv.ParseFloat(*text, 64)
			if err != nil || n <= 0 || n > float64(math.MaxInt64)/1000 {
				*message = tr("invalidTimeout")
				return
			}
			ms := int64(n * 1000)
			*value = &ms
			*message = ""
		}
	})
}
func (p *Page) hookFields(c *ui.Context, d *Draft) {
	enabled := d.Value.PreRunHook != nil
	if kit.Toggle(c, &enabled, tr("hook"), tr("hookHint")).Changed() {
		if enabled {
			d.Value.PreRunHook = &Hook{}
		} else {
			d.Value.PreRunHook = nil
			d.hookTimeout = ""
		}
	}
	if enabled {
		field(c, tr("command"), &d.Value.PreRunHook.Command, true)
		timeoutField(c, tr("timeout"), &d.hookTimeout, &d.Value.PreRunHook.TimeoutMs, &d.err)
		field(c, tr("hookDescription"), &d.hookDescription, true)
		ui.Row(c).Gap(8).Wrap().Children(func() {
			if kit.Button(c, tr("testHook"), icons.Play, p.busy["hook"]).Clicked() {
				p.hookRequest(d, false)
			}
			if kit.Button(c, tr("generateHook"), icons.Sparkles, p.busy["hook"]).Disabled(strings.TrimSpace(d.hookDescription) == "").Clicked() {
				p.hookRequest(d, true)
			}
		})
		if d.hookResult != "" {
			ui.Text(c, d.hookResult).Selectable().Font(kit.MonoFont).FillWidth()
		}
	}
	p.plugins.View(c, p.env, "will-schedule-run", &d.Value.PreRunPlugin)
}
func (p *Page) hookRequest(d *Draft, generate bool) {
	if p.busy["hook"] || d.Value.PreRunHook == nil {
		return
	}
	p.busy["hook"] = true
	params := map[string]any{"command": d.Value.PreRunHook.Command, "workingDir": d.Value.WorkingDir, "targetSessionId": d.Value.TargetSessionID, "scheduleName": d.Value.Name}
	method := "hook.test"
	if d.Value.PreRunHook.TimeoutMs != nil {
		params["timeoutMs"] = *d.Value.PreRunHook.TimeoutMs
	}
	if generate {
		method = "hook.generate"
		params["description"] = d.hookDescription
		params["currentCommand"] = d.Value.PreRunHook.Command
		params["model"] = d.Value.Model
		params["providerId"] = d.Value.ProviderID
	}
	core.Go(p.env, func() (map[string]any, error) {
		var result map[string]any
		err := p.call(method, params, &result)
		return result, err
	}, func(result map[string]any, err error) {
		delete(p.busy, "hook")
		if p.closed || p.draft != d {
			return
		}
		if err != nil {
			d.err = err.Error()
			return
		}
		if result["ok"] == false {
			d.err = fmt.Sprint(result)
			return
		}
		if command, ok := result["command"].(string); ok && d.Value.PreRunHook != nil {
			d.Value.PreRunHook.Command = command
		}
		d.hookResult = fmt.Sprint(result)
	})
}
