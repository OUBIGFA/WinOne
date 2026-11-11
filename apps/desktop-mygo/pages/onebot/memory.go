package onebot

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

type MemorySummary struct{ Filename, Type, Title, Preview, UpdatedAt string }
type MemoryDetail struct{ Filename, Type, Title, Body, UpdatedAt string }
type memoryPane struct {
	settings                                *profileSettings
	rows                                    []MemorySummary
	query, kind                             string
	detail, baseline                        *MemoryDetail
	latest                                  *MemoryDetail
	deleting                                *MemoryDetail
	editing, loading, saving, closed, stale bool
	error                                   string
	generation                              uint64
	cancel                                  func()
}

func newMemoryPane(d *profileSettings) *memoryPane {
	p := &memoryPane{settings: d, kind: "all"}
	p.Refresh()
	return p
}
func (p *memoryPane) Dirty() bool {
	return p.editing && p.detail != nil && p.baseline != nil && (p.detail.Title != p.baseline.Title || p.detail.Body != p.baseline.Body)
}
func (p *memoryPane) Busy() bool { return p.saving }
func (p *memoryPane) Close() {
	p.closed = true
	p.generation++
	if p.cancel != nil {
		p.cancel()
	}
}
func (p *memoryPane) Discard() {
	if p.cancel != nil {
		p.cancel()
	}
	p.detail = copyValue(p.baseline)
	p.latest = nil
	p.error = ""
}
func (p *memoryPane) Invalidate(kind string) {
	if strings.Contains(kind, "profile") || strings.Contains(kind, "message") {
		p.stale = true
	}
}
func (p *memoryPane) Refresh() {
	if p.closed {
		return
	}
	p.loading = true
	p.generation++
	generation := p.generation
	p.stale = false
	oneBotCall(p.settings.parent, "memory.list", map[string]string{"botId": p.settings.botID, "query": p.query}, func(rows []MemorySummary, err error) {
		if p.closed || p.generation != generation {
			return
		}
		p.loading = false
		if err != nil {
			p.error = err.Error()
			return
		}
		p.rows = rows
		p.error = ""
	})
}
func (p *memoryPane) open(filename string) {
	p.settings.parent.env.RequestChange(func() {
		p.loading = true
		p.generation++
		generation := p.generation
		oneBotCall(p.settings.parent, "memory.read", map[string]string{"botId": p.settings.botID, "filename": filename}, func(detail MemoryDetail, err error) {
			if p.closed || generation != p.generation {
				return
			}
			p.loading = false
			if err != nil {
				p.error = err.Error()
				return
			}
			p.detail, p.baseline = copyValue(&detail), copyValue(&detail)
			p.editing = false
			p.error = ""
		})
	})
}
func (p *memoryPane) changed() {
	if p.cancel != nil {
		p.cancel()
	}
	p.cancel = p.settings.parent.after(650*time.Millisecond, func() { p.cancel = nil; p.Save() })
}
func (p *memoryPane) Save() {
	if p.closed || p.saving || !p.Dirty() || p.latest != nil {
		return
	}
	if strings.TrimSpace(p.detail.Title) == "" {
		p.error = bt("bots.memory.titleRequired")
		return
	}
	capture := copyValue(*p.detail)
	p.saving = true
	oneBotCall(p.settings.parent, "memory.update", map[string]string{"botId": p.settings.botID, "filename": capture.Filename, "title": capture.Title, "body": capture.Body, "expectedUpdatedAt": p.baseline.UpdatedAt}, func(saved MemoryDetail, err error) {
		if p.closed {
			return
		}
		p.saving = false
		if err != nil {
			p.error = err.Error()
			if strings.Contains(err.Error(), "bot-memory-changed") {
				p.readLatest()
			}
			return
		}
		if p.detail.Title == capture.Title {
			p.detail.Title = saved.Title
		}
		if p.detail.Body == capture.Body {
			p.detail.Body = saved.Body
		}
		p.detail.UpdatedAt = saved.UpdatedAt
		p.baseline = copyValue(&saved)
		p.error = ""
		p.Refresh()
		if p.Dirty() {
			p.changed()
		}
	})
}
func (p *memoryPane) readLatest() {
	if p.detail == nil {
		return
	}
	oneBotCall(p.settings.parent, "memory.read", map[string]string{"botId": p.settings.botID, "filename": p.detail.Filename}, func(latest MemoryDetail, err error) {
		if p.closed {
			return
		}
		if err != nil {
			p.error = err.Error()
			return
		}
		p.latest = &latest
	})
}
func (p *memoryPane) View(c *ui.Context) {
	if p.closed {
		return
	}
	if p.stale && p.detail == nil {
		p.Refresh()
	}
	if p.detail != nil {
		ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
			if kit.Button(c, nt("back"), icons.ArrowLeft, false).Clicked() {
				p.settings.parent.env.RequestChange(func() { p.detail, p.baseline = nil, nil; p.editing = false; p.Refresh() })
			}
			if kit.Button(c, bt("bots.memory.edit"), icons.SquarePen, false).Clicked() {
				p.editing = true
			}
			if kit.Button(c, bt("bots.memory.delete"), icons.Trash2, false).Disabled(p.saving).Clicked() {
				p.deleting = copyValue(p.baseline)
			}
			if p.editing && kit.PrimaryButton(c, nt("save"), icons.Save, p.saving).Disabled(!p.Dirty() || p.saving).Clicked() {
				p.Save()
			}
		})
		kit.Caption(c, bt("bots.memory.types."+p.detail.Type)+" · "+p.detail.UpdatedAt)
		if p.latest != nil {
			kit.Notice(c, kit.Caution, "", bt("bots.memory.changedTitle", "name", p.settings.draft.Name), nil)
			markdown.View(c, p.latest.Body, nil).FillWidth()
			ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
				if kit.Button(c, bt("bots.memory.useLatest"), icons.RefreshCw, false).Clicked() {
					p.detail, p.baseline = copyValue(p.latest), copyValue(p.latest)
					p.latest = nil
					p.error = ""
				}
				if kit.Button(c, bt("bots.memory.keepMine"), icons.Save, false).Clicked() {
					p.baseline = copyValue(p.latest)
					p.latest = nil
					p.Save()
				}
			})
		}
		if p.editing {
			ui.Field(c, bt("bots.memory.titleLabel"), func() {
				if kit.TextInput(c, &p.detail.Title).FillWidth().Label(bt("bots.memory.titleLabel")).Changed() {
					p.changed()
				}
			})
			ui.Field(c, bt("bots.memory.bodyLabel"), func() {
				if ui.TextArea(c, &p.detail.Body).FillWidth().Height(280).Label(bt("bots.memory.bodyLabel")).Changed() {
					p.changed()
				}
			})
		} else {
			kit.Heading(c, p.detail.Title)
			markdown.View(c, p.detail.Body, nil).FillWidth()
		}
	} else {
		caps := &p.settings.draft.Capabilities.Value
		// The upstream disabled explanation is a state, not an always-on
		// description; showing it while enabled contradicts the saved behavior.
		hint := ""
		if !caps.Memory {
			hint = bt("bots.memory.disabledHint")
		}
		if kit.Toggle(c, &caps.Memory, bt("bots.memory.enabled"), hint).Changed() {
			p.settings.changed()
		}
		p.settings.textField(c, bt("bots.memory.aboutMe"), &p.settings.draft.UserContextSource, 120)
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			if kit.TextInput(c, &p.query).Grow(1).MinWidth(0).Placeholder(bt("bots.memory.search")).Label(bt("bots.memory.search")).Changed() {
				p.Refresh()
			}
			if kit.IconButton(c, icons.RefreshCw, nt("refresh")).Clicked() {
				p.Refresh()
			}
		})
		options := []kit.SelectOption{{Value: "all", Label: nt("all")}}
		for _, kind := range []string{"user", "feedback", "project", "reference"} {
			options = append(options, kit.SelectOption{Value: kind, Label: bt("bots.memory.types." + kind)})
		}
		kit.Segmented(c, &p.kind, options, nt("memoryTypes"))
		if p.loading {
			kit.Loader(c)
		}
		shown := 0
		for _, row := range p.rows {
			if p.kind != "all" && row.Type != p.kind {
				continue
			}
			shown++
			ui.Column(c.Key(row.Filename)).FillWidth().Gap(4).Padding(8, 0).BorderWidth(0, 0, 1, 0).BorderColor(kit.Colors(c).Hairline).Children(func() {
				if kit.GhostButton(c, row.Title, nil, false).FillWidth().MinWidth(0).Justify(ui.Start).Clicked() {
					p.open(row.Filename)
				}
				kit.Help(c, row.Preview).FillWidth()
				kit.Caption(c, bt("bots.memory.types."+row.Type)+" · "+row.UpdatedAt)
			})
		}
		if !p.loading && shown == 0 {
			kit.Help(c, bt("bots.memory.empty"))
		}
	}
	kit.InlineError(c, p.error)
	p.deleteView(c)
}
func (p *memoryPane) deleteView(c *ui.Context) {
	capture := p.deleting
	if capture == nil {
		return
	}
	open := true
	if kit.Dialog(c, &open, kit.DialogOptions{Title: bt("bots.memory.deleteTitle"), Message: bt("bots.memory.deleteDescription", "name", p.settings.draft.Name, "title", capture.Title), Confirm: bt("bots.memory.delete"), Cancel: nt("cancel"), Danger: true, Busy: p.saving, Error: p.error}) {
		p.saving = true
		oneBotCall(p.settings.parent, "memory.delete", map[string]string{"botId": p.settings.botID, "filename": capture.Filename, "expectedUpdatedAt": capture.UpdatedAt}, func(_ any, err error) {
			if p.closed {
				return
			}
			p.saving = false
			if err != nil {
				p.error = err.Error()
				return
			}
			p.deleting = nil
			p.detail, p.baseline = nil, nil
			p.editing = false
			p.Refresh()
		})
	}
	if !open {
		p.deleting = nil
	}
}
