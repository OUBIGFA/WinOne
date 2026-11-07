package automations

import (
	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

func (p *Page) showDeleted() {
	p.deletedOpen = true
	if p.busy["deleted"] {
		return
	}
	p.busy["deleted"] = true
	core.Go(p.env, func() ([]Schedule, error) {
		var rows []Schedule
		err := p.call("deleted", nil, &rows)
		return rows, err
	}, func(rows []Schedule, err error) {
		delete(p.busy, "deleted")
		if p.closed {
			return
		}
		if err != nil {
			p.err = err.Error()
			return
		}
		p.deleted = rows
	})
}
func (p *Page) deletedView(c *ui.Context) {
	if !p.deletedOpen {
		return
	}
	kit.Dialog(c, &p.deletedOpen, kit.DialogOptions{Title: tr("deleted"), Cancel: tr("close"), Width: 650, Body: func() {
		if p.busy["deleted"] {
			kit.Loader(c)
		} else if len(p.deleted) == 0 {
			kit.Help(c, tr("noDeleted"))
		}
		for _, s := range p.deleted {
			ui.Row(c.Key(s.ID)).FillWidth().Gap(8).Wrap().Children(func() {
				ui.Text(c, s.Name).Grow(1).MinWidth(0)
				if kit.Button(c, tr("restore"), icons.RotateCcw, p.busy["restore:"+s.ID]).Clicked() {
					p.mutate("restore", s.ID, map[string]string{"id": s.ID}, p.showDeleted)
				}
			})
		}
	}})
}

// Persistent executions are grouped by their real conversation, newest group
// first. Records without a conversation remain standalone, including checks
// that skipped before creating a task.
type historyGroup struct {
	key, sessionID string
	runs           []Run
}

func groupRuns(runs []Run, bound bool) []historyGroup {
	groups := []historyGroup{}
	bySession := map[string]int{}
	for _, run := range runs {
		if bound && run.SessionID != "" {
			if index, ok := bySession[run.SessionID]; ok {
				groups[index].runs = append(groups[index].runs, run)
				continue
			}
			bySession[run.SessionID] = len(groups)
			groups = append(groups, historyGroup{key: "session:" + run.SessionID, sessionID: run.SessionID, runs: []Run{run}})
		} else {
			groups = append(groups, historyGroup{key: "run:" + run.ID, runs: []Run{run}})
		}
	}
	return groups
}
func (p *Page) historyGroupsView(c *ui.Context, s *Schedule) {
	groups := groupRuns(p.runs, s.PersistentSession || s.TargetSessionID != "")
	current := ""
	for _, group := range groups { if group.sessionID != "" { current = group.sessionID; break } }
	for _, group := range groups {
		ui.Column(c.Key(group.key)).FillWidth().Gap(10).Children(func() {
			count := len(group.runs)
			expanded := ui.Local(ui.Box(c), "expanded", func() bool { return false })
			shown := count
			if group.sessionID != "" {
				isCurrent := group.sessionID == current
				canToggle := !isCurrent || count > 3
				if !*expanded {
					shown = 0
					if isCurrent { shown = min(3, count) }
				}
				// Cindy previews only the current conversation; older groups stay
				// folded until opened. Their keyed expansion survives new records.
				icon := icons.ChevronDown
				if *expanded { icon = icons.ChevronUp }
				label := tr("conversationRuns", "count", count)+" · "+group.sessionID[:min(8,len(group.sessionID))]
				if kit.GhostButton(c, label, icon, false).AlignSelf(ui.Start).Disabled(!canToggle).Clicked() { *expanded = !*expanded }
			}
			for _, run := range group.runs[:shown] {
				p.runView(c.Key(run.ID), run)
			}
		})
	}
}
