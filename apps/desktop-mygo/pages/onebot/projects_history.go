package onebot

import (
	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

type ProjectOption struct {
	ID, Name, Path    string
	Linked, Available bool
}
type projectsPane struct {
	settings                     *profileSettings
	rows                         []ProjectOption
	loading, busy, closed, stale bool
	error                        string
}

func newProjectsPane(d *profileSettings) *projectsPane {
	p := &projectsPane{settings: d}
	p.Refresh()
	return p
}
func (p *projectsPane) Busy() bool { return p.busy }
func (p *projectsPane) Close()     { p.closed = true }
func (p *projectsPane) Invalidate(kind string) {
	if kind == "profile-changed" || kind == "workbench-changed" {
		p.stale = true
	}
}
func (p *projectsPane) Refresh() {
	if p.closed || p.loading {
		return
	}
	p.loading = true
	p.stale = false
	oneBotCall(p.settings.parent, "projects.list", map[string]string{"botId": p.settings.botID}, func(rows []ProjectOption, err error) {
		if p.closed {
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
func (p *projectsPane) View(c *ui.Context) {
	if p.closed {
		return
	}
	if p.stale {
		p.Refresh()
	}
	kit.Help(c, bt("bots.projects.description"))
	if kit.Button(c, nt("refresh"), icons.RefreshCw, p.loading).Clicked() {
		p.Refresh()
	}
	if p.loading {
		kit.Loader(c)
	}
	kit.InlineError(c, p.error)
	for _, row := range p.rows {
		ui.Column(c.Key(row.ID)).FillWidth().Gap(4).Padding(8, 0).Children(func() {
			linked := row.Linked
			if kit.Toggle(c, &linked, row.Name, row.Path).Disabled(p.busy || !row.Available && !row.Linked).Changed() {
				p.busy = true
				oneBotCall(p.settings.parent, "projects.set", map[string]any{"botId": p.settings.botID, "id": row.ID, "linked": linked}, func(rows []ProjectOption, err error) {
					if p.closed {
						return
					}
					p.busy = false
					if err != nil {
						p.error = err.Error()
						return
					}
					p.rows = rows
					p.error = ""
				})
			}
			if !row.Available {
				kit.Notice(c, kit.Caution, "", nt("projectUnavailable"), nil)
			}
		})
	}
	if !p.loading && len(p.rows) == 0 {
		kit.Help(c, bt("bots.projects.empty"))
	}
}

type HistoryHit struct {
	MessageID       string `json:"messageId"`
	MessageClientID string `json:"messageClientId"`
	Preview         string `json:"preview"`
}
type HistoryResult struct {
	Session     struct{ ID, Title, UpdatedAt string } `json:"session"`
	ContentHit  *HistoryHit                           `json:"contentHit"`
	ContentHits []HistoryHit                          `json:"contentHits"`
}
type historyPane struct {
	settings               *profileSettings
	rows                   []BotSession
	results                []HistoryResult
	query                  string
	loading, closed, stale bool
	error                  string
	generation             uint64
}

func newHistoryPane(d *profileSettings) *historyPane {
	p := &historyPane{settings: d}
	p.Refresh()
	return p
}
func (p *historyPane) Close() { p.closed = true; p.generation++ }
func (p *historyPane) Invalidate(kind string) {
	if kind == "message-created" {
		p.stale = true
	}
}
func (p *historyPane) Refresh() {
	p.loading = true
	p.stale = false
	p.generation++
	generation := p.generation
	if p.query == "" {
		oneBotCall(p.settings.parent, "bots.history", map[string]string{"botId": p.settings.botID}, func(rows []BotSession, err error) {
			if p.closed || generation != p.generation {
				return
			}
			p.loading = false
			if err != nil {
				p.error = err.Error()
				return
			}
			p.rows = rows
			p.results = nil
			p.error = ""
		})
		return
	}
	oneBotCall(p.settings.parent, "history.search", map[string]any{"botId": p.settings.botID, "query": p.query, "limit": 50}, func(value struct {
		Results []HistoryResult `json:"results"`
	}, err error) {
		if p.closed || generation != p.generation {
			return
		}
		p.loading = false
		if err != nil {
			p.error = err.Error()
			return
		}
		p.results = value.Results
		p.error = ""
	})
}
func (p *historyPane) View(c *ui.Context) {
	if p.closed {
		return
	}
	if p.stale {
		p.Refresh()
	}
	ui.Row(c).FillWidth().Gap(8).Children(func() {
		if kit.TextInput(c, &p.query).Grow(1).MinWidth(0).Label(bt("bots.historySearch.title")).Placeholder(bt("bots.historySearch.title")).Changed() {
			p.Refresh()
		}
		if kit.IconButton(c, icons.RefreshCw, nt("refresh")).Clicked() {
			p.Refresh()
		}
	})
	if p.loading {
		kit.Loader(c)
	}
	kit.InlineError(c, p.error)
	if p.query == "" {
		for _, row := range p.rows {
			if kit.Button(c, row.Title, icons.History, false).FillWidth().Justify(ui.Start).MinWidth(0).Clicked() {
				p.settings.parent.openSession(p.settings.botID, row.ID)
			}
		}
	} else {
		for _, row := range p.results {
			ui.Column(c.Key(row.Session.ID)).FillWidth().Gap(4).Padding(8, 0).Children(func() {
				if kit.GhostButton(c, row.Session.Title, icons.History, false).FillWidth().Justify(ui.Start).Clicked() {
					p.settings.parent.openSession(p.settings.botID, row.Session.ID)
				}
				hits := row.ContentHits
				if len(hits) == 0 && row.ContentHit != nil {
					hits = []HistoryHit{*row.ContentHit}
				}
				for _, hit := range hits {
					if kit.GhostButton(c, hit.Preview, nil, false).FillWidth().MinWidth(0).Justify(ui.Start).Clicked() {
						p.settings.parent.openSessionAt(p.settings.botID, row.Session.ID, hit.MessageClientID, hit.MessageID)
					}
				}
			})
		}
	}
}

type GlanceRow struct {
	SessionID, Title, Phase, Question, WaitingFor, RequestID string
	Minutes                                                  int
	Online                                                   bool
}
type Glance struct {
	Scope            string
	Waiting, Working []GlanceRow
	DoneToday        int
	OfflineDevices   []string
}
type glancePane struct {
	settings               *profileSettings
	data                   Glance
	loading, closed, stale bool
	error                  string
}

func newGlancePane(d *profileSettings) *glancePane {
	p := &glancePane{settings: d}
	p.Refresh()
	return p
}
func (p *glancePane) Close() { p.closed = true }
func (p *glancePane) Invalidate(kind string) {
	if kind == "autonomy-changed" || kind == "workbench-changed" || kind == "task-changed" {
		p.stale = true
	}
}
func (p *glancePane) Refresh() {
	if p.loading || p.closed {
		return
	}
	p.loading = true
	p.stale = false
	oneBotCall(p.settings.parent, "autonomy.glance", map[string]string{"botId": p.settings.botID}, func(data Glance, err error) {
		if p.closed {
			return
		}
		p.loading = false
		if err != nil {
			p.error = err.Error()
			return
		}
		p.data = data
		p.error = ""
	})
}
func (p *glancePane) View(c *ui.Context) {
	if p.closed {
		return
	}
	if p.stale {
		p.Refresh()
	}
	if kit.Button(c, nt("refresh"), icons.RefreshCw, p.loading).Clicked() {
		p.Refresh()
	}
	kit.InlineError(c, p.error)
	kit.Help(c, nt("doneToday", "count", p.data.DoneToday))
	for _, row := range append(append([]GlanceRow(nil), p.data.Waiting...), p.data.Working...) {
		ui.Column(c.Key(row.SessionID)).FillWidth().Gap(4).Padding(8, 0).Children(func() {
			if kit.GhostButton(c, row.Title, icons.Activity, false).FillWidth().Justify(ui.Start).Clicked() {
				p.settings.parent.openTask(row.SessionID)
			}
			kit.Caption(c, row.Phase+" · "+nt("minutes", "count", row.Minutes))
			if row.Question != "" {
				kit.Help(c, row.Question)
			}
			// Permissions and questions are answered in the task that owns the
			// captured request, never guessed from a stale summary here.
			if row.WaitingFor != "" && kit.Button(c, nt("openTask"), icons.ArrowUpRight, false).Clicked() {
				p.settings.parent.openTask(row.SessionID)
			}
		})
	}
}
