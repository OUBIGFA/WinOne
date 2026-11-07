package automations

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

type viewPrefs struct {
	Filter      string          `json:"filter"`
	ListPercent float32         `json:"listPercent"`
	Collapsed   map[string]bool `json:"collapsed"`
}

const prefsFile = "desktop-automations-view.json"

func (p *Page) loadPrefs() {
	p.collapsed = map[string]bool{}
	if p.env.Directory == "" {
		return
	}
	file := filepath.Join(p.env.Directory, prefsFile)
	info, err := os.Stat(file)
	if err != nil || info.Size() > 64<<10 {
		return
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		return
	}
	var saved viewPrefs
	if json.Unmarshal(raw, &saved) != nil {
		return
	}
	if saved.Filter == "all" || saved.Filter == "active" || saved.Filter == "paused" {
		p.filter = saved.Filter
	}
	if saved.ListPercent >= 22 && saved.ListPercent <= 65 {
		p.split.Percent = saved.ListPercent
	}
	if saved.Collapsed != nil {
		p.collapsed = saved.Collapsed
	}
}
func (p *Page) savePrefs() {
	if p.env.Directory == "" {
		return
	}
	raw, err := json.Marshal(viewPrefs{Filter: p.filter, ListPercent: p.split.Percent, Collapsed: p.collapsed})
	if err != nil {
		return
	}
	file, err := os.CreateTemp(p.env.Directory, ".automations-view-*.tmp")
	if err != nil {
		p.err = err.Error()
		return
	}
	name := file.Name()
	defer os.Remove(name)
	_, err = file.Write(raw)
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(name, filepath.Join(p.env.Directory, prefsFile))
	}
	if err != nil {
		p.err = err.Error()
	}
}

type scheduleGroup struct {
	key, title, directory string
	items                 []Schedule
}

func scheduleGroups(items []Schedule) []scheduleGroup {
	groups := []scheduleGroup{}
	index := map[string]int{}
	for _, s := range items {
		key := strings.ToLower(strings.TrimRight(s.WorkingDir, "/\\"))
		title := filepath.Base(s.WorkingDir)
		if s.WorkspaceKind == "dialogue" {
			key = "dialogue"
			title = tr("dialogue")
		} else if key == "" {
			key = "other"
			title = tr("other")
		}
		i, ok := index[key]
		if !ok {
			i = len(groups)
			index[key] = i
			groups = append(groups, scheduleGroup{key: key, title: title, directory: s.WorkingDir})
		}
		groups[i].items = append(groups[i].items, s)
	}
	return groups
}
func (p *Page) groupedList(c *ui.Context, items []Schedule) {
	for _, group := range scheduleGroups(items) {
		ui.Column(c.Key(group.key)).FillWidth().Gap(3).Children(func() {
			ui.Row(c).FillWidth().Gap(4).AlignItems(ui.Center).Children(func() {
				icon := icons.ChevronDown
				if p.collapsed[group.key] {
					icon = icons.ChevronRight
				}
				if kit.GhostButton(c, group.title, icon, false).Grow(1).MinWidth(0).Justify(ui.Start).Tooltip(group.directory).Clicked() {
					p.collapsed[group.key] = !p.collapsed[group.key]
					p.savePrefs()
				}
				if group.directory != "" && kit.IconButton(c, icons.Plus, tr("createInProject")).Clicked() {
					p.openEditor(nil)
					if p.draft != nil && !p.Dirty() {
						p.draft.Value.WorkspaceKind = "project"
						p.draft.Value.WorkingDir = group.directory
					}
				}
			})
			if p.collapsed[group.key] {
				return
			}
			for _, s := range group.items {
				p.listItem(c.Key(s.ID), s)
			}
		})
	}
}
func (p *Page) listItem(c *ui.Context, s Schedule) {
	b := ui.ButtonBase(c).FillWidth().Padding(10).MinWidth(0).Radius(kit.PanelRadius).TextAlign(ui.Start).Label(s.Name)
	if s.ID == p.selected || b.Hovered() {
		b.Background(c.Theme().SurfaceHover)
	}
	b.Children(func() {
		ui.Column(c).MinWidth(0).Grow(1).Gap(4).Children(func() {
			ui.Text(c, s.Name).SingleLine().Ellipsis("…").FillWidth().Tooltip(s.Name)
			state := tr("status." + s.Status)
			for _, run := range p.runtime.InFlightRuns {
				if run.ScheduleID == s.ID {
					state = tr("phase." + run.Phase)
					break
				}
			}
			for _, waiting := range p.runtime.WaitingSchedules {
				if waiting.ScheduleID == s.ID {
					state = tr("waitingSlot")
					break
				}
			}
			kit.Help(c, state+" · "+dateText(s.NextFireAt))
			if s.UnreadCount > 0 {
				kit.Help(c, tr("unread", "count", s.UnreadCount))
			}
		})
	})
	if b.Clicked() {
		p.selectSchedule(s.ID)
	}
	if b.DoubleClicked() {
		p.openEditor(&s)
	}
}
