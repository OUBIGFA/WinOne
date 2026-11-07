package automations

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"winone/desktopmygo/chat/mathview"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"winone/desktopmygo/chat/pluginchecks"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/widgets"
)

// Options keeps navigation in the owning native Chat page, not in a second
// window or transport. No schedule action runs until a native control asks.
type Options struct {
	ConfigureConnections func()
	OpenSession          func(string)
	Back                 func()
}
type Page struct {
	env                                                  *core.Env
	options                                              Options
	items                                                []Schedule
	runs                                                 []Run
	runtime                                              Runtime
	selected, runsID, filter, search, directory          string
	loading, loaded, runsLoading, shown, closed, polling bool
	refreshAgain                                         bool
	err                                                  string
	generation, runsGeneration                           uint64
	cursor                                               uint64
	timer                                                *time.Timer
	busy                                                 map[string]bool
	area                                                 ui.Handle
	narrowDetail                                         bool
	collapsed                                            map[string]bool
	deletedOpen                                          bool
	deleted                                              []Schedule
	markdown                                             markdown.Options
	split                                                kit.SplitState
	draft                                                *Draft
	pluginDraftIDs                                       map[string]bool
	pluginDraft                                          json.RawMessage
	discard                                              bool
	confirmation                                         *confirmation
	templates                                            []Template
	templateOpen                                         bool
	templateID                                           string
	templateParams                                       map[string]string
	templateSearch                                       string
	templateErr                                          string
	importsRevision                                      uint64
	imports                                              []PendingImport
	importOpen                                           bool
	support                                              supportState
	plugins                                              pluginchecks.Picker
	supportLoaded, supportLoading, supportAgain          bool
	supportError                                         string
	providers                                            []widgets.AIProvider
	sessions                                             []struct {
		ID        string `json:"id"`
		Title     string `json:"title"`
		Directory string `json:"directory"`
		Archived  bool   `json:"archived"`
	}
}
type confirmation struct {
	previewReady, recordDone   bool
	title, message, action, id string
	schedule                   *Schedule
	disposition                string
	sessionIDs                 []string
	busy                       bool
	err                        string
}

func New(env *core.Env, o Options) *Page {
	p := &Page{env: env, options: o, filter: "all", cursor: 9007199254740991, busy: map[string]bool{}, split: kit.SplitState{Percent: 34}, templateParams: map[string]string{}}
	p.loadPrefs()
	return p
}
func (p *Page) Close() {
	p.plugins.Close()
	p.savePrefs()
	p.closed = true
	p.shown = false
	p.generation++
	p.runsGeneration++
	if p.timer != nil {
		p.timer.Stop()
	}
}
func (p *Page) SetVisible(shown bool) {
	if p.closed || p.shown == shown {
		return
	}
	p.shown = shown
	if !shown {
		p.savePrefs()
		if p.timer != nil {
			p.timer.Stop()
		}
		return
	}
	if !p.loaded && !p.loading {
		p.refresh()
	}
	p.poll()
}
func (p *Page) Dirty() bool { return p.draft != nil && p.draft.changed() }
func (p *Page) Busy() bool {
	for _, busy := range p.busy {
		if busy {
			return true
		}
	}
	return p.confirmation != nil && p.confirmation.busy
}
func (p *Page) Discard() { p.draft = nil; p.discard = false }
func (p *Page) call(method string, params, out any) error {
	return p.env.Host.CallInto("chat.automations."+method, params, out)
}
func (p *Page) refresh() {
	if p.closed {
		return
	}
	if p.loading {
		p.refreshAgain = true
		return
	}
	p.loading = true
	p.generation++
	generation := p.generation
	loadImports, importsRevision := !p.loaded, p.importsRevision
	type result struct {
		items   []Schedule
		runtime Runtime
		imports []PendingImport
	}
	core.Go(p.env, func() (result, error) {
		var r result
		if err := p.call("list", nil, &r.items); err != nil {
			return r, err
		}
		if err := p.call("runtime", nil, &r.runtime); err != nil {
			return r, err
		}
		if loadImports {
			if err := p.call("project.pendingImports", nil, &r.imports); err != nil {
				return r, err
			}
		}
		return r, nil
	}, func(r result, err error) {
		if p.closed || generation != p.generation {
			return
		}
		p.loading = false
		defer func() {
			if p.refreshAgain {
				p.refreshAgain = false
				p.refresh()
			}
		}()
		if err != nil {
			p.err = err.Error()
			return
		}
		p.err = ""
		p.loaded = true
		p.items = r.items
		p.runtime = r.runtime
		if p.pluginDraft != nil {
			draft := p.pluginDraft
			p.pluginDraft = nil
			p.OpenPluginDraft(draft)
		}
		if loadImports && importsRevision == p.importsRevision {
			p.imports = r.imports
		}
		rows := visible(p.items, p.filter, p.search, p.directory)
		if !containsSchedule(rows, p.selected) {
			p.selected = ""
			if len(rows) > 0 {
				p.selected = rows[0].ID
			}
		}
		p.readRuns()
	})
}
func containsSchedule(items []Schedule, id string) bool {
	for _, s := range items {
		if s.ID == id {
			return true
		}
	}
	return false
}
func (p *Page) selectedSchedule() *Schedule {
	for i := range p.items {
		if p.items[i].ID == p.selected {
			return &p.items[i]
		}
	}
	return nil
}
func (p *Page) selectSchedule(id string) {
	// The first list row is already selected by refresh. A narrow-window
	// press still reveals that owner's details without reading its runs twice.
	p.narrowDetail = true
	if p.selected == id {
		return
	}
	p.selected = id
	p.readRuns()
}
func (p *Page) readRuns() {
	id := p.selected
	// A cleared or newly selected schedule invalidates every older receipt and
	// its run actions; an error must not expose another schedule's history.
	p.runsGeneration++
	generation := p.runsGeneration
	if id == "" || p.runsID != id {
		p.runs = nil
		p.runsID = ""
	}
	if id == "" {
		p.runsLoading = false
		return
	}
	p.runsLoading = true
	core.Go(p.env, func() ([]Run, error) {
		var rows []Run
		err := p.call("runs", map[string]any{"id": id, "limit": 50}, &rows)
		return rows, err
	}, func(rows []Run, err error) {
		if p.closed || generation != p.runsGeneration {
			return
		}
		p.runsLoading = false
		if err != nil {
			p.err = err.Error()
			return
		}
		p.runs = rows
		p.runsID = id
	})
}

// One Host long poll owns refreshes; hidden pages stop scheduling more reads.
// An immediate empty response backs off, matching Cindy's event adapter.
func (p *Page) poll() {
	if !p.shown || p.closed || p.polling {
		return
	}
	p.polling = true
	cursor := p.cursor
	started := time.Now()
	type events struct {
		Cursor uint64            `json:"cursor"`
		Events []json.RawMessage `json:"events"`
	}
	core.Go(p.env, func() (events, error) {
		var e events
		err := p.call("events", map[string]any{"cursor": cursor}, &e)
		return e, err
	}, func(e events, err error) {
		p.polling = false
		if p.closed || !p.shown {
			return
		}
		delay := time.Duration(0)
		if err != nil {
			p.err = err.Error()
			delay = time.Second
			p.cursor = 9007199254740991
		} else {
			p.cursor = e.Cursor
			changed := p.receiveEvents(e.Events)
			if e.Cursor < cursor {
				p.loaded = false
			}
			if changed || e.Cursor < cursor {
				p.refresh()
			}
			if len(e.Events) == 0 && time.Since(started) < 200*time.Millisecond {
				delay = time.Second
			}
		}
		p.timer = time.AfterFunc(delay, func() { p.env.Post(p.poll) })
	})
}
func (p *Page) mutate(action, id string, params any, done func()) {
	key := action + ":" + id
	if p.busy[key] {
		return
	}
	p.busy[key] = true
	p.err = ""
	core.Do(p.env, func() error { return p.call(action, params, nil) }, func(err error) {
		delete(p.busy, key)
		if p.closed {
			return
		}
		if err != nil {
			p.err = err.Error()
			return
		}
		if done != nil {
			done()
		}
		p.refresh()
	})
}
func (p *Page) openEditor(s *Schedule) {
	if p.draft != nil && p.draft.changed() {
		p.discard = true
		return
	}
	p.draft = newDraft(s, p.runtime.Timezone)
	p.loadFormResources()
}
func (p *Page) loadFormResources() {
	p.RefreshSupport()
	type resources struct {
		Providers []widgets.AIProvider `json:"providers"`
	}
	core.Go(p.env, func() (resources, error) {
		var r resources
		err := p.env.Host.CallInto("capture.ai.configuration.get", nil, &r)
		return r, err
	}, func(r resources, err error) {
		if p.closed {
			return
		}
		if err != nil {
			p.err = err.Error()
		} else {
			p.providers = r.Providers
		}
	})
	core.Go(p.env, func() (json.RawMessage, error) { return p.env.Host.Call("chat.catalog", nil) }, func(raw json.RawMessage, err error) {
		if p.closed {
			return
		}
		if err != nil {
			p.err = err.Error()
			return
		}
		var data struct {
			Conversations json.RawMessage `json:"conversations"`
		}
		if err = json.Unmarshal(raw, &data); err == nil {
			err = json.Unmarshal(data.Conversations, &p.sessions)
		}
		if err != nil {
			p.err = err.Error()
		}
	})
}
func (p *Page) save() {
	d := p.draft
	if d == nil || p.busy["save"] {
		return
	}
	if err := d.validate(); err != nil {
		d.err = err.Error()
		return
	}
	p.busy["save"] = true
	core.Go(p.env, func() (Schedule, error) {
		var s Schedule
		method, params := "create", map[string]any{"input": d.Value}
		if d.ID != "" {
			method = "update"
			params = map[string]any{"id": d.ID, "patch": d.patch()}
		}
		err := p.call(method, params, &s)
		return s, err
	}, func(s Schedule, err error) {
		delete(p.busy, "save")
		if p.closed || p.draft != d {
			return
		}
		if err != nil {
			d.err = err.Error()
			return
		}
		p.draft = nil
		p.selected = s.ID
		if p.filter == "paused" {
			p.filter = "active"
		}
		p.refresh()
	})
}
func (p *Page) pause(s Schedule) {
	if s.Status == "paused" {
		p.mutate("resume", s.ID, map[string]string{"id": s.ID}, nil)
		return
	}
	if p.busy["pause-check:"+s.ID] {
		return
	}
	p.busy["pause-check:"+s.ID] = true
	core.Go(p.env, func() (int, error) {
		var count int
		err := p.call("inflightCount", map[string]string{"id": s.ID}, &count)
		return count, err
	}, func(count int, err error) {
		delete(p.busy, "pause-check:"+s.ID)
		if p.closed {
			return
		}
		if err != nil {
			p.err = err.Error()
			return
		}
		if count == 0 {
			p.mutate("pause", s.ID, map[string]string{"id": s.ID}, nil)
			return
		}
		p.confirmation = &confirmation{title: tr("pause"), message: tr("pauseWarning", "count", count), action: "pause", id: s.ID}
	})
}
func (p *Page) deleteSchedule(s Schedule) {
	c := &confirmation{title: tr("delete"), message: tr("deleteWarning", "name", s.Name), action: "delete", id: s.ID, schedule: &s, disposition: "keep", busy: true}
	p.confirmation = c
	core.Go(p.env, func() ([]string, error) {
		var runs []Run
		err := p.call("runs", map[string]any{"id": s.ID, "limit": 10000}, &runs)
		return generatedSessionIDs(s, runs), err
	}, func(ids []string, err error) {
		if p.closed || p.confirmation != c {
			return
		}
		c.busy = false
		if err != nil {
			c.err = err.Error()
		} else {
			c.sessionIDs = ids
			c.previewReady = true
		}
	})
}
func (p *Page) confirm() {
	c := p.confirmation
	if c == nil || c.busy || c.schedule != nil && !c.previewReady {
		return
	}
	c.busy = true
	c.err = ""
	type outcome struct {
		recordDone bool
		remaining  []string
	}
	action, id, disposition, recordDone := c.action, c.id, c.disposition, c.recordDone
	sessionIDs := append([]string(nil), c.sessionIDs...)
	core.Go(p.env, func() (outcome, error) {
		result := outcome{recordDone: recordDone, remaining: sessionIDs}
		if !recordDone {
			if err := p.call(action, map[string]string{"id": id, "runId": id}, nil); err != nil {
				return result, err
			}
			result.recordDone = true
		}
		result.remaining = nil
		if action == "delete" && disposition != "keep" {
			var failures []string
			for _, sessionID := range sessionIDs {
				method, params := "chat.conversation.update", map[string]any{"id": sessionID, "archived": true}
				if disposition == "delete" {
					method = "chat.conversation.delete"
				}
				if err := p.env.Host.CallInto(method, params, nil); err != nil {
					result.remaining = append(result.remaining, sessionID)
					failures = append(failures, sessionID+": "+err.Error())
				}
			}
			if len(failures) > 0 {
				return result, fmt.Errorf("%s\n%s", tr("partialDelete"), strings.Join(failures, "\n"))
			}
		}
		return result, nil
	}, func(result outcome, err error) {
		if p.closed || p.confirmation != c {
			return
		}
		c.busy = false
		c.recordDone = result.recordDone
		c.sessionIDs = result.remaining
		if err != nil {
			c.err = err.Error()
			p.refresh()
			return
		}
		p.confirmation = nil
		if action == "template.delete" {
			p.showTemplates()
		}
		p.refresh()
	})
}
func (p *Page) View(c *ui.Context) {
	defer p.plugins.Dialogs(c)
	p.SetVisible(true)
	p.env.Activity.Report("chat-automations", p.Dirty(), p.Busy(), p.Discard)
	width := p.area.Bounds(c).W
	narrow := width > 0 && width < 650
	ui.Column(c).Fill().MinWidth(0).Gap(8).Bind(&p.area).Children(func() {
		ui.Row(c).FillWidth().Gap(8).Padding(12).Wrap().AlignItems(ui.Center).Children(func() {
			if p.options.Back != nil && kit.IconButton(c, icons.ArrowLeft, tr("back")).Clicked() {
				p.env.RequestChange(p.options.Back)
			}
			kit.Heading(c, tr("title")).Grow(1)
			if kit.Button(c, tr("templates"), icons.LayoutTemplate, false).Clicked() {
				p.showTemplates()
			}
			if len(p.imports) > 0 && kit.Button(c, tr("imports", "count", len(p.imports)), icons.FolderOpen, false).Clicked() {
				p.importOpen = true
			}
			if kit.IconButton(c, icons.History, tr("deleted")).Clicked() {
				p.showDeleted()
			}
			if kit.IconButton(c, icons.RefreshCw, tr("refresh")).Disabled(p.loading).Clicked() {
				p.refresh()
			}
			if kit.PrimaryButton(c, tr("create"), icons.Plus, false).Disabled(!p.loaded).Clicked() {
				p.openEditor(nil)
			}
		})
		kit.InlineError(c, p.err)
		if p.loading && !p.loaded {
			kit.Loader(c)
		}
		if narrow {
			if p.narrowDetail {
				if kit.Button(c, tr("all"), icons.ChevronLeft, false).Clicked() {
					p.narrowDetail = false
				}
				p.historyView(c)
			} else {
				p.listView(c)
			}
		} else {
			kit.Split(c, &p.split, kit.SplitOptions{Label: tr("resize"), MinFirst: 22, MinSecond: 35}, func() { p.listView(c) }, func() { p.historyView(c) }).Grow(1).MinHeight(0)
		}
	})
	p.editorView(c)
	p.confirmView(c)
	p.templatesView(c)
	p.importsView(c)
	p.deletedView(c)
}
func (p *Page) listView(c *ui.Context) {
	ui.Column(c).Fill().Padding(12).Gap(8).Children(func() {
		previousSearch, previousFilter := p.search, p.filter
		ui.TextInput(c, &p.search).Placeholder(tr("search")).FillWidth().Label(tr("search"))
		kit.Segmented(c, &p.filter, []kit.SelectOption{{Value: "all", Label: tr("all")}, {Value: "active", Label: tr("active")}, {Value: "paused", Label: tr("paused")}}, tr("filter"))
		if previousSearch != p.search || previousFilter != p.filter {
			if previousFilter != p.filter {
				p.savePrefs()
			}
			rows := visible(p.items, p.filter, p.search, p.directory)
			if !containsSchedule(rows, p.selected) {
				p.selected = ""
				if len(rows) > 0 {
					p.selected = rows[0].ID
				}
				p.readRuns()
			}
		}
		if p.runtime.MaxConcurrentRuns > 0 {
			kit.Help(c, tr("slots", "count", p.runtime.SlotsInUse, "max", p.runtime.MaxConcurrentRuns))
		}
		ui.Scroll(c).Grow(1).MinHeight(0).FillWidth().Gap(4).Children(func() {
			rows := visible(p.items, p.filter, p.search, p.directory)
			if len(rows) == 0 {
				kit.Help(c, tr("empty"))
			}
			p.groupedList(c, rows)
		})
	})
}
func (p *Page) historyView(c *ui.Context) {
	s := p.selectedSchedule()
	if s == nil {
		kit.Help(c, tr("choose"))
		return
	}
	ui.Column(c).Fill().Padding(16).Gap(12).Children(func() {
		kit.Heading(c, s.Name)
		kit.Help(c, s.CronExpr+" · "+s.Timezone)
		ui.Row(c).FillWidth().Gap(6).Wrap().Children(func() {
			if kit.PrimaryButton(c, tr("run"), icons.Play, p.busy["run:"+s.ID]).Clicked() {
				p.mutate("run", s.ID, map[string]string{"id": s.ID}, nil)
			}
			if kit.Button(c, tr("edit"), icons.Pencil, false).Clicked() {
				p.openEditor(s)
			}
			label := tr("pause")
			if s.Status == "paused" {
				label = tr("resume")
			}
			if kit.Button(c, label, icons.Pause, false).Clicked() {
				p.pause(*s)
			}
			if kit.Button(c, tr("delete"), icons.Trash2, false).Clicked() {
				p.deleteSchedule(*s)
			}
			if kit.Button(c, tr("markAll"), icons.Check, false).Clicked() {
				p.mutate("markAllRead", "", nil, nil)
			}
		})
		ui.Row(c).FillWidth().Gap(6).Wrap().Children(func() {
			if s.WorkingDir != "" {
				if kit.Button(c, tr("openFolder"), icons.FolderOpen, false).Clicked() {
					p.mutate("open", s.ID, map[string]string{"path": s.WorkingDir}, nil)
				}
				if kit.Button(c, tr("reloadProject"), icons.RefreshCw, false).Clicked() {
					p.mutate("project.reconcile", s.ID, map[string]string{"workingDir": s.WorkingDir}, nil)
				}
			}
			if s.Source == "project" {
				if kit.Button(c, tr("clone"), icons.Copy, false).Clicked() {
					p.mutate("project.clone", s.ID, map[string]string{"id": s.ID}, nil)
				}
				if kit.Button(c, tr("demote"), icons.Folder, false).Clicked() {
					p.confirmation = &confirmation{title: tr("demote"), message: tr("projectChange"), action: "project.demote", id: s.ID}
				}
			} else if s.WorkingDir != "" {
				if kit.Button(c, tr("promote"), icons.Folder, false).Clicked() {
					p.confirmation = &confirmation{title: tr("promote"), message: tr("projectChange"), action: "project.promote", id: s.ID}
				}
			}
		})
		if p.runsLoading {
			kit.Loader(c)
		}
		ui.Scroll(c).Grow(1).MinHeight(0).FillWidth().Gap(16).Children(func() {
			if !p.runsLoading && len(p.runs) == 0 {
				kit.Help(c, tr("noRuns"))
			}
			p.historyGroupsView(c, s)
		})
	})
}
func (p *Page) openRun(run Run) {
	key := "open-run:" + run.ID
	if p.closed || p.busy[key] || p.options.OpenSession == nil { return }
	p.busy[key] = true
	core.Do(p.env, func() error {
		// Match Cindy's get-session-before-mark-read order: an unavailable
		// conversation must not clear an unread failure the user cannot open.
		var catalog struct { Conversations []struct { ID string `json:"id"` } `json:"conversations"` }
		if err := p.env.Host.CallInto("chat.catalog", nil, &catalog); err != nil { return err }
		found := false
		for _, session := range catalog.Conversations { found = found || session.ID == run.SessionID }
		if !found { return fmt.Errorf("%s", i18n.T("chat.requestedConversationMissing")) }
		if run.ReadAt == 0 && run.Status != "running" { return p.call("markRead", map[string]string{"runId":run.ID}, nil) }
		return nil
	}, func(err error) {
		delete(p.busy,key)
		if p.closed { return }
		if err != nil { p.err = err.Error(); return }
		p.refresh()
		p.options.OpenSession(run.SessionID)
	})
}

func (p *Page) runView(c *ui.Context, r Run) {
	ui.Column(c).FillWidth().MinWidth(0).Gap(6).BorderWidth(0, 0, 1, 0).BorderColor(kit.Colors(c).Hairline).Padding(0, 0, 12, 0).Children(func() {
		ui.Row(c).FillWidth().Gap(6).Wrap().Children(func() {
			kit.Label(c, tr("runStatus."+r.Status))
			kit.Help(c, dateText(r.FiredAt)).Grow(1)
			if r.SessionID != "" && kit.Button(c, tr("openSession"), icons.MessageSquare, false).Disabled(p.busy["open-run:"+r.ID]).Clicked() {
				p.openRun(r)
			}
			if r.Status != "running" && kit.IconButton(c, icons.Trash2, tr("deleteRun")).Clicked() {
				p.confirmation = &confirmation{title: tr("deleteRun"), message: tr("deleteRunWarning"), action: "deleteRun", id: r.ID}
			}
		})
		if r.ErrorMsg != "" {
			kit.InlineError(c, r.ErrorMsg)
		}
		if r.NotificationError != "" {
			kit.InlineError(c, r.NotificationError)
		}
		if r.ResultText != "" {
			markdown.View(c, r.ResultText, mathview.For(p.env).Apply(&p.markdown)).FillWidth()
		}
		if len(r.PreRunHookResult) > 0 {
			var formatted any
			_ = json.Unmarshal(r.PreRunHookResult, &formatted)
			raw, _ := json.MarshalIndent(formatted, "", "  ")
			ui.Text(c, string(raw)).Font(kit.MonoFont).Selectable().FillWidth()
		}
		if r.TotalTokens > 0 {
			kit.Help(c, fmt.Sprintf("%d tokens", r.TotalTokens))
		}
		if r.CostMoney != nil {
			kit.Help(c, tr("cost")+fmt.Sprintf(" %s %.4f", r.CostMoney.Currency, r.CostMoney.Amount))
		}
		if r.EstimatedValueMoney != nil {
			kit.Help(c, tr("estimate")+fmt.Sprintf(" %s %.4f", r.EstimatedValueMoney.Currency, r.EstimatedValueMoney.Amount))
		}
		if r.ReadAt == 0 && r.Status != "running" && kit.Button(c, tr("markRead"), icons.Check, false).AlignSelf(ui.Start).Clicked() {
			p.mutate("markRead", r.ID, map[string]string{"runId": r.ID}, nil)
		}
	})
}
func (p *Page) confirmView(c *ui.Context) {
	target := p.confirmation
	if target == nil {
		return
	}
	open := true
	if kit.Dialog(c, &open, kit.DialogOptions{Title: target.title, Message: target.message, Error: target.err, Confirm: tr("confirm"), Cancel: tr("cancel"), Danger: target.action == "delete" || target.action == "deleteRun", Busy: target.busy, ConfirmDisabled: target.schedule != nil && !target.previewReady, Body: func() {
		if target.schedule != nil {
			kit.Help(c, tr("generated", "count", len(target.sessionIDs)))
			kit.Select(c, &target.disposition, []kit.SelectOption{{Value: "keep", Label: tr("keepSessions")}, {Value: "archive", Label: tr("archiveSessions")}, {Value: "delete", Label: tr("deleteSessions")}})
		}
	}}) {
		p.confirm()
	}
	if !open {
		p.confirmation = nil
	}
}
