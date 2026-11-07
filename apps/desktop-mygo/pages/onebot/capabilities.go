package onebot

import (
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"winone/desktopmygo/core"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

type CapabilityEntry struct {
	ID, Name, Description string
	Available, Personal   bool
}
type SkillDetail struct {
	Slug, Name, Description, Body, UpdatedAt string
	Enabled                                  *bool
}
type capabilitiesPane struct {
	settings                                          *profileSettings
	catalog                                           map[string][]CapabilityEntry
	query, view, systemID                             string
	detail, baseline                                  *SkillDetail
	loading, saving, editing, closed, stale, deleting bool
	error                                             string
	generation                                        uint64
}

func newCapabilitiesPane(d *profileSettings) *capabilitiesPane {
	p := &capabilitiesPane{settings: d, catalog: map[string][]CapabilityEntry{}}
	p.Refresh()
	return p
}
func (p *capabilitiesPane) Dirty() bool {
	return p.editing && p.detail != nil && p.baseline != nil && (p.detail.Name != p.baseline.Name || p.detail.Description != p.baseline.Description || p.detail.Body != p.baseline.Body)
}
func (p *capabilitiesPane) Busy() bool { return p.saving }
func (p *capabilitiesPane) Close()     { p.closed = true; p.generation++ }
func (p *capabilitiesPane) Discard()   { p.detail = copyValue(p.baseline); p.error = "" }
func (p *capabilitiesPane) Invalidate(kind string) {
	if kind == "capabilities-changed" || kind == "profile-changed" {
		p.stale = true
	}
}
func (p *capabilitiesPane) Refresh() {
	if p.closed {
		return
	}
	p.loading = true
	p.stale = false
	p.generation++
	generation := p.generation
	oneBotCall(p.settings.parent, "capabilities.get", map[string]string{"botId": p.settings.botID}, func(value struct {
		Entries map[string][]CapabilityEntry `json:"entries"`
	}, err error) {
		if p.closed || generation != p.generation {
			return
		}
		p.loading = false
		if err != nil {
			p.error = err.Error()
			return
		}
		p.catalog = value.Entries
		p.error = ""
	})
}
func (p *capabilitiesPane) selected(kind string) []string {
	caps := p.settings.draft.Capabilities.Value
	if kind == "skill" {
		return p.settings.draft.Skills
	}
	mode, stored := caps.ToolsetMode, caps.Toolsets
	if kind == "mcp" {
		mode, stored = caps.MCPMode, caps.MCPServers
	}
	if mode == "inherit" {
		stored = append([]string(nil), stored...)
		for _, entry := range p.catalog[kind] {
			if entry.Available && !slices.Contains(stored, entry.ID) {
				stored = append(stored, entry.ID)
			}
		}
	}
	return stored
}
func (p *capabilitiesPane) set(kind string, selected []string) {
	caps := &p.settings.draft.Capabilities.Value
	switch kind {
	case "skill":
		p.settings.draft.Skills = selected
		caps.SkillMode = "allowlist"
	case "mcp":
		caps.MCPServers = selected
		caps.MCPMode = "allowlist"
	default:
		caps.Toolsets = selected
		caps.ToolsetMode = "allowlist"
	}
	p.settings.changed()
}
func (p *capabilitiesPane) toggle(kind, id string, on bool) {
	selected := append([]string(nil), p.selected(kind)...)
	if on && !slices.Contains(selected, id) {
		selected = append(selected, id)
	} else if !on {
		selected = slices.DeleteFunc(selected, func(value string) bool { return value == id })
	}
	p.set(kind, selected)
}
func (p *capabilitiesPane) loadSkill(slug string) {
	p.loading = true
	p.generation++
	generation := p.generation
	oneBotCall(p.settings.parent, "skills.read", map[string]string{"botId": p.settings.botID, "slug": slug}, func(detail *SkillDetail, err error) {
		if p.closed || generation != p.generation {
			return
		}
		p.loading = false
		if err != nil {
			p.error = err.Error()
			return
		}
		if detail == nil {
			p.error = bt("bots.capabilities.learned.missing")
			return
		}
		p.detail, p.baseline = copyValue(detail), copyValue(detail)
		p.view = "learned"
		p.editing = false
		p.error = ""
	})
}
func (p *capabilitiesPane) Save() {
	if p.closed || p.saving || !p.Dirty() {
		return
	}
	capture := copyValue(*p.detail)
	p.saving = true
	oneBotCall(p.settings.parent, "skills.save", map[string]any{"botId": p.settings.botID, "input": map[string]string{"slug": capture.Slug, "name": capture.Name, "description": capture.Description, "body": capture.Body, "expectedUpdatedAt": p.baseline.UpdatedAt}}, func(result struct {
		Record SkillDetail `json:"record"`
	}, err error) {
		if p.closed {
			return
		}
		p.saving = false
		if err != nil {
			p.error = err.Error()
			return
		}
		if p.detail.Name == capture.Name {
			p.detail.Name = result.Record.Name
		}
		if p.detail.Description == capture.Description {
			p.detail.Description = result.Record.Description
		}
		if p.detail.Body == capture.Body {
			p.detail.Body = result.Record.Body
		}
		p.detail.UpdatedAt = result.Record.UpdatedAt
		p.baseline = copyValue(&result.Record)
		p.error = ""
		if !p.Dirty() {
			p.editing = false
		}
		p.stale = true
	})
}
func (p *capabilitiesPane) View(c *ui.Context) {
	if p.closed {
		return
	}
	if p.stale && p.view == "" {
		p.Refresh()
	}
	if p.view != "" {
		if kit.Button(c, nt("back"), icons.ArrowLeft, false).Clicked() {
			p.settings.parent.env.RequestChange(func() { p.view = ""; p.detail, p.baseline = nil, nil; p.editing = false })
		}
		switch p.view {
		case "core":
			kit.Heading(c, bt("bots.capabilities.core.name"))
			kit.Help(c, bt("bots.capabilities.core.note"))
			for _, item := range []string{"tasks", "teammates", "models", "memory", "self"} {
				kit.Strong(c, bt("bots.capabilities.core.items."+item+".title"))
				kit.Help(c, bt("bots.capabilities.core.items."+item+".body"))
			}
		case "system":
			p.systemPage(c, p.systemID)
		case "learned":
			p.learnedPage(c)
		}
	} else {
		kit.Help(c, bt("bots.capabilities.intro"))
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			ui.TextInput(c, &p.query).Grow(1).MinWidth(0).Label(bt("bots.capabilities.search")).Placeholder(bt("bots.capabilities.search"))
			if kit.IconButton(c, icons.RefreshCw, nt("refresh")).Clicked() {
				p.Refresh()
			}
		})
		if p.loading {
			kit.Loader(c)
		}
		if kit.Button(c, bt("bots.capabilities.core.name"), icons.Layers, false).FillWidth().Justify(ui.Start).Clicked() {
			p.view = "core"
		}
		for _, id := range []string{"todo", "routine", "follow"} {
			if p.query != "" && !contains(bt("bots.capabilities.system.items."+id+".name"), p.query) {
				continue
			}
			ui.Row(c.Key(id)).FillWidth().Gap(8).Children(func() {
				if kit.Button(c, bt("bots.capabilities.system.items."+id+".name"), icons.Zap, false).Grow(1).MinWidth(0).Justify(ui.Start).Clicked() {
					p.systemID, p.view = id, "system"
				}
				on := !slices.Contains(p.settings.draft.Capabilities.Value.SystemSkillsOff, id)
				if kit.Toggle(c, &on, bt("bots.capabilities.system.use"), "").Changed() {
					p.setSystem(id, on)
				}
			})
		}
		for _, kind := range []string{"skill", "toolset", "mcp"} {
			p.grants(c, kind)
		}
		caps := &p.settings.draft.Capabilities.Value
		on := caps.InputHandoff == nil || *caps.InputHandoff
		if kit.Toggle(c, &on, bt("bots.capabilities.inputHandoff.title"), bt("bots.capabilities.inputHandoff.description")).Changed() {
			caps.InputHandoff = &on
			p.settings.changed()
		}
	}
	kit.InlineError(c, p.error)
}
func (p *capabilitiesPane) setSystem(id string, on bool) {
	caps := &p.settings.draft.Capabilities.Value
	caps.SystemSkillsOff = slices.DeleteFunc(caps.SystemSkillsOff, func(value string) bool { return value == id })
	if !on {
		caps.SystemSkillsOff = append(caps.SystemSkillsOff, id)
	}
	p.settings.changed()
}
func (p *capabilitiesPane) systemPage(c *ui.Context, id string) {
	kit.Heading(c, bt("bots.capabilities.system.items."+id+".name"))
	on := !slices.Contains(p.settings.draft.Capabilities.Value.SystemSkillsOff, id)
	if kit.Toggle(c, &on, bt("bots.capabilities.system.use"), bt("bots.capabilities.system.items."+id+".hint")).Changed() {
		p.setSystem(id, on)
	}
	kit.Help(c, bt("bots.capabilities.system.items."+id+".off"))
}
func (p *capabilitiesPane) grants(c *ui.Context, kind string) {
	selected := p.selected(kind)
	rows := append([]CapabilityEntry(nil), p.catalog[kind]...)
	for _, id := range selected {
		if !slices.ContainsFunc(rows, func(row CapabilityEntry) bool { return row.ID == id }) {
			rows = append(rows, CapabilityEntry{ID: id, Name: id})
		}
	}
	kit.Heading(c, nt("capability."+kind))
	ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
		if kit.GhostButton(c, nt("selectAll"), nil, false).Disabled(p.loading || p.error != "").Clicked() {
			values := append([]string(nil), selected...)
			for _, row := range rows {
				if row.Available && !slices.Contains(values, row.ID) {
					values = append(values, row.ID)
				}
			}
			p.set(kind, values)
		}
		if kit.GhostButton(c, nt("selectNone"), nil, false).Disabled(p.loading).Clicked() {
			p.set(kind, []string{})
		}
		if kit.GhostButton(c, bt("bots.capabilities.manage"), icons.Settings2, false).Clicked() {
			p.settings.parent.env.RequestChange(func() { p.settings.parent.env.Open("chat", core.Intent{ChatSettings: true}) })
		}
	})
	for _, row := range rows {
		if p.query != "" && !contains(row.Name+" "+row.Description, p.query) {
			continue
		}
		checked := slices.Contains(selected, row.ID)
		disabled := p.loading || p.error != "" || !row.Available && !checked
		ui.Column(c.Key(row.ID)).FillWidth().Gap(4).Padding(6, 0).Children(func() {
			ui.Row(c).FillWidth().MinWidth(0).Gap(6).Children(func() {
				if kit.Toggle(c, &checked, row.Name, row.Description).Grow(1).MinWidth(0).Disabled(disabled).Changed() {
					p.toggle(kind, row.ID, checked)
				}
				if row.Personal && kit.IconButton(c, icons.ChevronRight, nt("details")).Clicked() {
					p.settings.parent.env.RequestChange(func() { p.loadSkill(strings.TrimPrefix(row.ID, "personal:")) })
				}
			})
			if !row.Available {
				kit.Status(c, bt("bots.capabilities.notOnThisComputer"))
			}
		})
	}
}
func (p *capabilitiesPane) learnedPage(c *ui.Context) {
	if p.loading {
		kit.Loader(c)
	}
	if p.detail == nil {
		return
	}
	d := p.detail
	kit.Heading(c, d.Name)
	kit.Caption(c, d.UpdatedAt)
	enabled := d.Enabled == nil || *d.Enabled
	if kit.Toggle(c, &enabled, bt("bots.capabilities.learned.use"), bt("bots.capabilities.learned.useHint")).Disabled(p.saving).Changed() {
		p.saving = true
		oneBotCall(p.settings.parent, "skills.setEnabled", map[string]any{"botId": p.settings.botID, "slug": d.Slug, "enabled": enabled}, func(_ any, err error) {
			if p.closed {
				return
			}
			p.saving = false
			if err != nil {
				p.error = err.Error()
				return
			}
			d.Enabled = &enabled
			p.baseline.Enabled = &enabled
			p.stale = true
		})
	}
	if p.editing {
		ui.Field(c, nt("name"), func() { ui.TextInput(c, &d.Name).FillWidth().Label(nt("name")) })
		ui.Field(c, nt("description"), func() { ui.TextInput(c, &d.Description).FillWidth().Label(nt("description")) })
		ui.TextArea(c, &d.Body).FillWidth().Height(280).Label(nt("skillBody"))
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			if kit.Button(c, nt("cancel"), nil, false).Disabled(p.saving).Clicked() {
				p.settings.parent.env.RequestChange(func() { p.Discard(); p.editing = false })
			}
			if kit.PrimaryButton(c, nt("save"), icons.Save, p.saving).Disabled(!p.Dirty() || p.saving).Clicked() {
				p.Save()
			}
		})
	} else {
		kit.Help(c, d.Description)
		markdown.View(c, d.Body, nil).FillWidth()
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			if kit.Button(c, bt("bots.capabilities.learned.edit"), icons.SquarePen, false).Clicked() {
				p.editing = true
			}
			if kit.Button(c, bt("bots.capabilities.learned.delete"), icons.Trash2, false).Clicked() {
				p.deleting = true
			}
		})
	}
	if p.deleting {
		open := true
		slug := d.Slug
		if kit.Dialog(c, &open, kit.DialogOptions{Title: bt("bots.capabilities.learned.deleteTitle", "name", d.Name), Message: bt("bots.capabilities.learned.deleteBody"), Confirm: bt("bots.capabilities.learned.delete"), Cancel: nt("cancel"), Danger: true, Busy: p.saving, Error: p.error}) {
			p.saving = true
			oneBotCall(p.settings.parent, "skills.delete", map[string]string{"botId": p.settings.botID, "slug": slug}, func(_ any, err error) {
				if p.closed {
					return
				}
				p.saving = false
				if err != nil {
					p.error = err.Error()
					return
				}
				p.deleting = false
				p.detail, p.baseline = nil, nil
				p.view = ""
				p.Refresh()
			})
		}
		if !open {
			p.deleting = false
		}
	}
}
