package onebot

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"time"
	"winone/desktopmygo/chat/mathview"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"winone/desktopmygo/chat/automations"
	"winone/desktopmygo/chat/pluginchecks"
	"winone/desktopmygo/core"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

type RoutineFilter struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    string `json:"value"`
}
type RoutineTrigger struct {
	ID         string          `json:"id"`
	Kind       string          `json:"kind"`
	Expression string          `json:"expression,omitempty"`
	Timezone   string          `json:"timezone,omitempty"`
	IntervalMs int64           `json:"intervalMs,omitempty"`
	AnchorMs   *int64          `json:"anchorMs,omitempty"`
	At         int64           `json:"at,omitempty"`
	SourceID   string          `json:"sourceId,omitempty"`
	EventType  string          `json:"eventType,omitempty"`
	Filters    []RoutineFilter `json:"filters,omitempty"`
}
type RoutineModel struct {
	AgentKind  string          `json:"agentKind"`
	ProviderID *string         `json:"providerId"`
	Model      string          `json:"model"`
	Effort     string          `json:"effort"`
	FastMode   json.RawMessage `json:"fastMode"`
}
type RoutineDelivery struct {
	Kind     string `json:"kind"`
	Mode     string `json:"mode,omitempty"`
	TargetID string `json:"targetId,omitempty"`
}
type Routine struct {
	ID             string                   `json:"id"`
	BotID          string                   `json:"botId"`
	Name           string                   `json:"name"`
	Prompt         string                   `json:"prompt"`
	Enabled        bool                     `json:"enabled"`
	Triggers       []RoutineTrigger         `json:"triggers"`
	SilentWhenIdle *bool                    `json:"silentWhenIdle,omitempty"`
	PreRunHook     *automations.Hook        `json:"preRunHook"`
	PreRunPlugin   *automations.PluginCheck `json:"preRunPlugin"`
	ModelOverride  *RoutineModel            `json:"modelOverride"`
	SessionMode    string                   `json:"sessionMode,omitempty"`
	Delivery       *RoutineDelivery         `json:"delivery,omitempty"`
	Builtin        string                   `json:"builtin,omitempty"`
	Revision       int                      `json:"revision"`
	Activity       string                   `json:"activity,omitempty"`
}
type RoutineSource struct {
	ID, Name, Status string
	Events           []struct {
		Type, Name string
		Fields     []string
	}
}
type RoutineDeliveryTarget struct{ ID, Channel, Name, Recipient string }
type RoutineRun struct {
	ID, Status, Error, SessionID, ResultText string
	CreatedAt                                int64
	Delivery                                 *struct{ Status, Error string }
}
type routineEditor struct {
	value, baseline             Routine
	fresh                       bool
	cron                        map[string]*automations.CronEditor
	hookDescription, hookResult string
}
type routinesPane struct {
	settings                                                       *profileSettings
	rows                                                           []Routine
	sources                                                        []RoutineSource
	targets                                                        []RoutineDeliveryTarget
	zone                                                           string
	loading, busy, closed, stale, deleteOpen, consent, nightlyOpen bool
	error                                                          string
	editor                                                         *routineEditor
	runs                                                           []RoutineRun
	nightly                                                        *nightlyPane
	autonomy                                                       struct {
		SchedulerBotID *string `json:"schedulerBotId"`
		IsCustomized   bool    `json:"isCustomized"`
	}
	plugins pluginchecks.Picker
}

func newRoutinesPane(d *profileSettings) *routinesPane {
	p := &routinesPane{settings: d, zone: "UTC"}
	p.Refresh()
	return p
}
func (p *routinesPane) Dirty() bool {
	return p.editor != nil && !sameJSON(routineInput(p.editor.value), routineInput(p.editor.baseline))
}
func (p *routinesPane) Busy() bool { return p.busy || p.nightly != nil && p.nightly.Busy() }
func (p *routinesPane) Discard()   { p.editor = nil; p.error = "" }
func (p *routinesPane) Close() {
	p.plugins.Close()
	p.closed = true
	if p.nightly != nil {
		p.nightly.Close()
	}
}
func (p *routinesPane) Invalidate(kind string) {
	if kind == "routines-changed" || kind == "autonomy-changed" {
		p.stale = true
		if p.nightly != nil {
			p.nightly.stale = true
		}
	}
}
func (p *routinesPane) Refresh() {
	if p.closed || p.loading {
		return
	}
	p.loading = true
	p.stale = false
	type result struct {
		rows      []Routine
		sources   []RoutineSource
		targets   []RoutineDeliveryTarget
		scheduler *string
		custom    bool
		zone      string
	}
	core.Go(p.settings.parent.env, func() (result, error) {
		var r result
		if err := p.settings.parent.call("routines.list", map[string]string{"botId": p.settings.botID}, &r.rows); err != nil {
			return r, err
		}
		if err := p.settings.parent.call("routines.sources", nil, &r.sources); err != nil {
			return r, err
		}
		if err := p.settings.parent.call("routines.deliveryTargets", map[string]string{"botId": p.settings.botID}, &r.targets); err != nil {
			return r, err
		}
		var autonomy struct {
			SchedulerBotID *string `json:"schedulerBotId"`
			IsCustomized   bool    `json:"isCustomized"`
		}
		if err := p.settings.parent.call("autonomy.settings", nil, &autonomy); err != nil {
			return r, err
		}
		r.scheduler, r.custom = autonomy.SchedulerBotID, autonomy.IsCustomized
		var runtime struct {
			Timezone string `json:"timezone"`
		}
		if err := p.settings.parent.env.Host.CallInto("chat.automations.runtime", nil, &runtime); err != nil {
			return r, err
		}
		r.zone = runtime.Timezone
		return r, nil
	}, func(r result, err error) {
		if p.closed {
			return
		}
		p.loading = false
		if err != nil {
			p.error = err.Error()
			return
		}
		p.rows, p.sources, p.targets = r.rows, r.sources, r.targets
		p.autonomy.SchedulerBotID, p.autonomy.IsCustomized = r.scheduler, r.custom
		if r.zone != "" {
			p.zone = r.zone
		}
		p.error = ""
	})
}
func routineInput(r Routine) map[string]any {
	value := map[string]any{"name": r.Name, "prompt": r.Prompt, "enabled": r.Enabled, "triggers": r.Triggers, "preRunHook": r.PreRunHook, "preRunPlugin": r.PreRunPlugin, "modelOverride": r.ModelOverride}
	if r.SilentWhenIdle != nil {
		value["silentWhenIdle"] = *r.SilentWhenIdle
	}
	if r.SessionMode != "" {
		value["sessionMode"] = r.SessionMode
	}
	if r.Delivery != nil {
		value["delivery"] = r.Delivery
	}
	return value
}
func (p *routinesPane) edit(row Routine, fresh bool) {
	p.settings.parent.env.RequestChange(func() {
		p.editor = &routineEditor{value: copyValue(row), baseline: copyValue(row), fresh: fresh, cron: map[string]*automations.CronEditor{}}
		p.runs = nil
		if !fresh {
			oneBotCall(p.settings.parent, "routines.history", map[string]string{"botId": p.settings.botID, "id": row.ID}, func(runs []RoutineRun, err error) {
				if p.closed {
					return
				}
				if err != nil {
					p.error = err.Error()
				} else {
					p.runs = runs
				}
			})
		}
	})
}
func (p *routinesPane) action(method string, row Routine, input any) {
	if p.busy {
		return
	}
	p.busy = true
	params := map[string]any{"botId": p.settings.botID, "id": row.ID, "expectedRevision": row.Revision}
	if input != nil {
		params["input"] = input
	}
	if row.ID == "" {
		delete(params, "id")
		delete(params, "expectedRevision")
	}
	oneBotCall(p.settings.parent, "routines."+method, params, func(_ json.RawMessage, err error) {
		if p.closed {
			return
		}
		p.busy = false
		if err != nil {
			p.error = err.Error()
			return
		}
		if method == "save" || method == "remove" {
			p.editor = nil
			p.deleteOpen = false
		}
		p.Refresh()
	})
}
func (p *routinesPane) View(c *ui.Context) {
	defer p.plugins.Dialogs(c)
	if p.closed {
		return
	}
	if p.stale && p.editor == nil {
		p.Refresh()
	}
	p.autonomyFields(c)
	if p.nightlyOpen {
		if p.nightly == nil {
			p.nightly = newNightlyPane(p.settings)
		}
		if kit.Button(c, nt("back"), icons.ArrowLeft, false).Clicked() {
			p.nightlyOpen = false
		}
		p.nightly.View(c)
		return
	}
	ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
		if kit.PrimaryButton(c, bt("routines.add"), icons.Plus, false).Clicked() {
			p.edit(Routine{Enabled: true, Triggers: []RoutineTrigger{}, SilentWhenIdle: boolPtr(false), SessionMode: "persistent", Delivery: &RoutineDelivery{Kind: "companion", Mode: "direct"}}, true)
		}
		if kit.Button(c, nt("nightly"), icons.BookOpen, false).Clicked() {
			p.nightlyOpen = true
		}
		if kit.IconButton(c, icons.RefreshCw, nt("refresh")).Clicked() {
			p.Refresh()
		}
	})
	if p.loading {
		kit.Loader(c)
	}
	kit.InlineError(c, p.error)
	if p.editor != nil {
		p.editorView(c)
		return
	}
	for _, row := range p.rows {
		ui.Column(c.Key(row.ID)).FillWidth().Gap(4).Padding(8, 0).Children(func() {
			if kit.Button(c, row.Name, icons.Timer, false).FillWidth().MinWidth(0).Justify(ui.Start).Clicked() {
				p.edit(row, false)
			}
			// A built-in's prompt is host wording for the model, not the owner's text.
			if row.Builtin == "" {
				kit.Help(c, row.Prompt)
			}
			if row.Activity != "" {
				kit.Caption(c, row.Activity)
			}
			ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
				enabled := row.Enabled
				if kit.Toggle(c, &enabled, nt("enabled"), "").Disabled(p.busy).Changed() {
					row.Enabled = enabled
					p.action("save", row, routineInput(row))
				}
				if kit.Button(c, bt("routines.runNow"), icons.Play, false).Disabled(p.busy).Clicked() {
					p.action("runNow", row, nil)
				}
			})
		})
	}
}
func boolPtr(value bool) *bool { return &value }
func (p *routinesPane) editorView(c *ui.Context) {
	e := p.editor
	r := &e.value
	fixed := r.Builtin != ""
	ui.Field(c, nt("name"), func() { kit.TextInput(c, &r.Name).FillWidth().Label(nt("name")).Disabled(fixed) })
	ui.Field(c, nt("instruction"), func() { ui.TextArea(c, &r.Prompt).FillWidth().Height(160).Label(nt("instruction")).Disabled(fixed) })
	kit.Toggle(c, &r.Enabled, nt("enabled"), "")
	if fixed {
		kit.Help(c, nt("builtinRoutine"))
	} else {
		for index := range r.Triggers {
			if index < len(r.Triggers) {
				p.triggerView(c, e, index)
			}
		}
		ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
			for _, kind := range []string{"cron", "interval", "once", "event"} {
				if kit.Button(c, nt("trigger."+kind), icons.Plus, false).Disabled(len(r.Triggers) >= 32).Clicked() {
					trigger := RoutineTrigger{ID: rand.Text(), Kind: kind, Filters: []RoutineFilter{}}
					switch kind {
					case "cron":
						trigger.Expression, trigger.Timezone = "0 9 * * *", p.zone
					case "interval":
						trigger.IntervalMs = 60 * 60 * 1000
					case "once":
						trigger.At = time.Now().Add(time.Hour).UnixMilli()
					case "event":
						if len(p.sources) > 0 {
							trigger.SourceID = p.sources[0].ID
							if len(p.sources[0].Events) > 0 {
								trigger.EventType = p.sources[0].Events[0].Type
							}
						}
					}
					r.Triggers = append(r.Triggers, trigger)
				}
			}
		})
	}
	inherit := r.ModelOverride == nil
	if kit.Toggle(c, &inherit, bt("bots.model.inheritPrimary"), "").Changed() {
		if inherit {
			r.ModelOverride = nil
		} else {
			route := ModelRoute{Harness: "pi", FastMode: json.RawMessage("false")}
			chain := p.settings.effectiveChain()
			if len(chain) > 0 {
				route = chain[0]
			}
			r.ModelOverride = &RoutineModel{AgentKind: "pi", ProviderID: route.ProviderID, Model: route.Model, Effort: route.Effort, FastMode: route.FastMode}
		}
	}
	if r.ModelOverride != nil {
		route := ModelRoute{Harness: "pi", ProviderID: r.ModelOverride.ProviderID, Model: r.ModelOverride.Model, Effort: r.ModelOverride.Effort, FastMode: r.ModelOverride.FastMode}
		p.settings.routeFields(c, &route, func() {
			r.ModelOverride = &RoutineModel{AgentKind: "pi", ProviderID: route.ProviderID, Model: route.Model, Effort: route.Effort, FastMode: route.FastMode}
		})
	}
	ui.Field(c, bt("routines.sessionMode"), func() {
		kit.Select(c, &r.SessionMode, []kit.SelectOption{{Value: "persistent", Label: bt("routines.sessionModePersistent")}, {Value: "fresh", Label: bt("routines.sessionModeFresh")}})
	})
	silent := r.SilentWhenIdle != nil && *r.SilentWhenIdle
	if kit.Toggle(c, &silent, bt("routines.quiet"), "").Changed() {
		r.SilentWhenIdle = &silent
	}
	if !fixed {
		p.deliveryFields(c, r)
		p.checkFields(c, e)
	}
	ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
		if kit.Button(c, nt("cancel"), icons.ArrowLeft, false).Disabled(p.busy).Clicked() {
			p.settings.parent.env.RequestChange(func() { p.editor = nil })
		}
		if kit.PrimaryButton(c, nt("save"), icons.Save, p.busy).Disabled(p.busy || !p.Dirty()).Clicked() {
			p.action("save", *r, routineInput(*r))
		}
		if !e.fresh && !fixed && kit.Button(c, nt("delete"), icons.Trash2, false).Clicked() {
			p.deleteOpen = true
		}
		if !e.fresh && kit.Button(c, bt("routines.runNow"), icons.Play, false).Disabled(p.busy).Clicked() {
			p.action("runNow", e.baseline, nil)
		}
	})
	for _, run := range p.runs {
		ui.Column(c.Key(run.ID)).FillWidth().Gap(4).Padding(8, 0).Children(func() {
			kit.Caption(c, run.Status+" · "+time.UnixMilli(run.CreatedAt).Format("2006-01-02 15:04"))
			kit.InlineError(c, run.Error)
			if run.Delivery != nil {
				kit.Status(c, nt("deliveryStatus", "status", run.Delivery.Status))
				kit.InlineError(c, run.Delivery.Error)
			}
			if run.ResultText != "" {
				markdown.View(c, run.ResultText, mathview.For(p.settings.parent.env).Apply(&markdown.Options{})).FillWidth()
			}
			if run.SessionID != "" && kit.Button(c, nt("openTask"), icons.ArrowUpRight, false).Clicked() {
				p.settings.parent.openTask(run.SessionID)
			}
		})
	}
	if p.deleteOpen {
		open := true
		if kit.Dialog(c, &open, kit.DialogOptions{Title: nt("delete"), Message: e.baseline.Name, Confirm: nt("delete"), Cancel: nt("cancel"), Danger: true, Busy: p.busy, Error: p.error}) {
			p.action("remove", e.baseline, nil)
		}
		if !open {
			p.deleteOpen = false
		}
	}
}
func (p *routinesPane) triggerView(c *ui.Context, e *routineEditor, index int) {
	t := &e.value.Triggers[index]
	ui.Column(c.Key(t.ID)).FillWidth().Gap(6).Padding(8, 0).Children(func() {
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			kit.Strong(c, nt("trigger."+t.Kind)).Grow(1)
			if kit.IconButton(c, icons.X, nt("removeTrigger")).Clicked() {
				e.value.Triggers = slices.Delete(e.value.Triggers, index, index+1)
			}
		})
		switch t.Kind {
		case "cron":
			if e.cron[t.ID] == nil {
				e.cron[t.ID] = automations.NewCronEditor(t.Expression)
			}
			e.cron[t.ID].View(c, &t.Expression)
			ui.Field(c, nt("timezone"), func() { kit.TextInput(c, &t.Timezone).FillWidth().Label(nt("timezone")) })
		case "interval":
			minutes := float64(t.IntervalMs) / 60000
			if ui.NumberInput(c, &minutes, 1, 1e10, 1).Label(nt("intervalMinutes")).Changed() {
				t.IntervalMs = int64(minutes * 60000)
			}
		case "once":
			tm := time.UnixMilli(t.At)
			ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
				changed := ui.DateInput(c, &tm).Label(nt("date")).Changed()
				changed = ui.TimeInput(c, &tm).Label(nt("time")).Changed() || changed
				if changed {
					t.At = tm.UnixMilli()
				}
			})
		case "event":
			options := []kit.SelectOption{}
			for _, source := range p.sources {
				options = append(options, kit.SelectOption{Value: source.ID, Label: source.Name})
			}
			ui.Field(c, nt("eventSource"), func() { kit.Select(c, &t.SourceID, options) })
			events := []kit.SelectOption{}
			for _, source := range p.sources {
				if source.ID == t.SourceID {
					for _, event := range source.Events {
						events = append(events, kit.SelectOption{Value: event.Type, Label: event.Name})
					}
				}
			}
			ui.Field(c, nt("eventType"), func() { kit.Select(c, &t.EventType, events) })
			for i := range t.Filters {
				if i >= len(t.Filters) {
					continue
				}
				filter := &t.Filters[i]
				ui.Column(c.Key(i)).FillWidth().Gap(4).Children(func() {
					kit.TextInput(c, &filter.Field).FillWidth().Label(nt("eventField"))
					kit.Select(c, &filter.Operator, []kit.SelectOption{{Value: "equals", Label: nt("equals")}, {Value: "contains", Label: nt("contains")}, {Value: "not-equals", Label: nt("notEquals")}})
					kit.TextInput(c, &filter.Value).FillWidth().Label(nt("eventValue"))
					if kit.Button(c, nt("removeFilter"), icons.X, false).Clicked() {
						t.Filters = slices.Delete(t.Filters, i, i+1)
					}
				})
			}
			if kit.Button(c, nt("addFilter"), icons.Plus, false).Disabled(len(t.Filters) >= 16).Clicked() {
				t.Filters = append(t.Filters, RoutineFilter{Operator: "equals"})
			}
		}
	})
}
func (p *routinesPane) deliveryFields(c *ui.Context, r *Routine) {
	kind := "direct"
	if r.Delivery != nil {
		kind = r.Delivery.Mode
		if r.Delivery.Kind != "companion" {
			kind = r.Delivery.Kind
		}
	}
	ui.Field(c, bt("routines.deliveryLabel"), func() {
		if changed, _ := kit.Select(c, &kind, []kit.SelectOption{{Value: "direct", Label: bt("routines.deliveryDirect")}, {Value: "process", Label: bt("routines.deliveryProcess")}, {Value: "im", Label: bt("routines.deliveryIm")}, {Value: "none", Label: bt("routines.deliveryNone")}}); changed {
			r.Delivery = &RoutineDelivery{Kind: "companion", Mode: kind}
			if kind == "im" || kind == "none" {
				r.Delivery.Kind, r.Delivery.Mode = kind, ""
			}
		}
	})
	if r.Delivery != nil && r.Delivery.Kind == "im" {
		options := []kit.SelectOption{}
		for _, target := range p.targets {
			options = append(options, kit.SelectOption{Value: target.ID, Label: target.Name + " · " + target.Recipient})
		}
		ui.Field(c, bt("routines.deliveryRecipient"), func() { kit.Select(c, &r.Delivery.TargetID, options) })
		if len(p.targets) == 0 {
			kit.Notice(c, kit.Caution, "", bt("routines.deliveryUnavailable"), nil)
		}
	}
}
func (p *routinesPane) checkFields(c *ui.Context, e *routineEditor) {
	r := &e.value
	hook := r.PreRunHook != nil
	if kit.Toggle(c, &hook, nt("preRunHook"), "").Changed() {
		if hook {
			r.PreRunHook = &automations.Hook{}
		} else {
			r.PreRunHook = nil
		}
	}
	if r.PreRunHook != nil {
		ui.TextArea(c, &r.PreRunHook.Command).FillWidth().Height(100).Label(nt("command"))
		timeout := float64(30)
		if r.PreRunHook.TimeoutMs != nil {
			timeout = float64(*r.PreRunHook.TimeoutMs) / 1000
		}
		if ui.NumberInput(c, &timeout, 1, 86400, 1).Label(nt("timeout")).Changed() {
			value := int64(timeout * 1000)
			r.PreRunHook.TimeoutMs = &value
		}
		kit.TextInput(c, &e.hookDescription).FillWidth().Label(nt("hookDescription"))
		ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
			if kit.Button(c, nt("testHook"), icons.Play, p.busy).Clicked() {
				p.hookRequest(e, false)
			}
			if kit.Button(c, nt("generateHook"), icons.Sparkles, p.busy).Clicked() {
				p.hookRequest(e, true)
			}
		})
		if e.hookResult != "" {
			kit.Mono(c, e.hookResult).FillWidth().Selectable()
		}
	}
	p.plugins.View(c, p.settings.parent.env, "will-routine-run", &r.PreRunPlugin)
}
func (p *routinesPane) hookRequest(e *routineEditor, generate bool) {
	if p.busy || e.value.PreRunHook == nil {
		return
	}
	p.busy = true
	params := map[string]any{"command": e.value.PreRunHook.Command, "workingDir": filepath.Join(p.settings.draft.HomeDir, "workspace"), "targetSessionId": canonical(p.settings.draft), "scheduleName": e.value.Name}
	if e.value.PreRunHook.TimeoutMs != nil {
		params["timeoutMs"] = *e.value.PreRunHook.TimeoutMs
	}
	method := "hook.test"
	if generate {
		method = "hook.generate"
		params["description"] = e.hookDescription
		params["currentCommand"] = e.value.PreRunHook.Command
	}
	core.Go(p.settings.parent.env, func() (map[string]any, error) {
		var result map[string]any
		err := p.settings.parent.env.Host.CallInto("chat.automations."+method, params, &result)
		return result, err
	}, func(result map[string]any, err error) {
		if p.closed {
			return
		}
		p.busy = false
		if err != nil {
			p.error = err.Error()
			return
		}
		if p.editor != e {
			return
		}
		if command, ok := result["command"].(string); ok && generate {
			e.value.PreRunHook.Command = command
		}
		e.hookResult = fmt.Sprint(result)
	})
}
func (p *routinesPane) autonomyFields(c *ui.Context) {
	holder := p.autonomy.SchedulerBotID
	on := holder != nil && *holder == p.settings.botID
	off := slices.Contains(p.settings.draft.Capabilities.Value.SystemSkillsOff, "follow")
	if kit.Toggle(c, &on, bt("bots.autonomy.title"), bt("bots.autonomy.routineScopeHint")).Disabled(p.busy || off).Changed() {
		if on {
			p.consent = true
		} else {
			p.setScheduler(nil)
		}
	}
	if holder != nil && *holder != p.settings.botID {
		caps := &p.settings.draft.Capabilities.Value
		if kit.Toggle(c, &caps.AllowSchedulerFollow, bt("bots.autonomy.allowSchedulerFollow.title", "name", *holder), bt("bots.autonomy.allowSchedulerFollow.description", "name", p.settings.draft.Name, "scheduler", *holder)).Changed() {
			p.settings.changed()
		}
	}
	if p.autonomy.IsCustomized && kit.Button(c, bt("settings.defaults.restore"), icons.RotateCcw, false).Clicked() {
		p.busy = true
		oneBotCall(p.settings.parent, "autonomy.resetSettings", nil, func(value struct {
			SchedulerBotID *string `json:"schedulerBotId"`
			IsCustomized   bool    `json:"isCustomized"`
		}, err error) {
			if p.closed {
				return
			}
			p.busy = false
			if err != nil {
				p.error = err.Error()
				return
			}
			p.autonomy = value
		})
	}
	if p.consent {
		if kit.Dialog(c, &p.consent, kit.DialogOptions{Title: bt("bots.autonomy.invitation.title"), Message: bt("bots.autonomy.consent"), Confirm: bt("bots.autonomy.invitation.start"), Cancel: bt("bots.autonomy.invitation.notNow")}) {
			id := p.settings.botID
			p.consent = false
			p.setScheduler(&id)
		}
	}
}
func (p *routinesPane) setScheduler(id *string) {
	p.busy = true
	oneBotCall(p.settings.parent, "autonomy.setSettings", map[string]any{"patch": map[string]any{"schedulerBotId": id}}, func(value struct {
		SchedulerBotID *string `json:"schedulerBotId"`
		IsCustomized   bool    `json:"isCustomized"`
	}, err error) {
		if p.closed {
			return
		}
		p.busy = false
		if err != nil {
			p.error = err.Error()
			return
		}
		p.autonomy = value
	})
}
