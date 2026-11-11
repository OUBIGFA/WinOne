package onebot

import (
	"crypto/rand"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

type TodoNext struct {
	Label       string `json:"label"`
	Instruction string `json:"instruction"`
	Kind        string `json:"kind"`
}
type TodoSource struct{ Kind, ID, Label, Ref string }
type TodoAssociation struct{ Kind, ID, Label string }
type TodoAction struct {
	RequestID, State, Error string
	Revision                int
}
type TodoDecision struct{ Kind, Until string }
type Todo struct {
	ID, Key, Title, Progress, Outcome, Value, Status, UpdatedAt string
	Revision                                                    int
	Next                                                        *TodoNext
	Sources                                                     []TodoSource
	Associations                                                []TodoAssociation
	Action                                                      *TodoAction
	Decision                                                    *TodoDecision
	DeadlineCandidate                                           json.RawMessage
	DeadlineOverride                                            json.RawMessage
	SourceDeadline                                              json.RawMessage
}
type todoEditor struct {
	item                                                             Todo
	original                                                         Todo
	fresh                                                            bool
	nextLabel, nextInstruction, nextKind, deadlineDate, deadlineZone string
}
type todoDecision struct {
	item                      Todo
	operation, summary, until string
}
type todosPane struct {
	settings                            *profileSettings
	rows                                []Todo
	filter, query                       string
	loading, busy, closed, stale, daily bool
	error                               string
	editor                              *todoEditor
	decision                            *todoDecision
}

func newTodosPane(d *profileSettings) *todosPane {
	p := &todosPane{settings: d, filter: "open"}
	p.Refresh()
	return p
}
func (p *todosPane) Dirty() bool {
	e := p.editor
	if e == nil {
		return false
	}
	return e.item.Title != e.original.Title || e.item.Progress != e.original.Progress || e.item.Outcome != e.original.Outcome || e.item.Value != e.original.Value || e.nextLabel != nextLabel(e.original.Next) || e.nextInstruction != nextInstruction(e.original.Next) || e.deadlineDate != ""
}
func nextLabel(n *TodoNext) string {
	if n == nil {
		return ""
	}
	return n.Label
}
func nextInstruction(n *TodoNext) string {
	if n == nil {
		return ""
	}
	return n.Instruction
}
func (p *todosPane) Busy() bool { return p.busy }
func (p *todosPane) Close()     { p.closed = true }
func (p *todosPane) Discard()   { p.editor = nil; p.error = "" }
func (p *todosPane) Invalidate(kind string) {
	if kind == "workbench-changed" || kind == "routines-changed" {
		p.stale = true
	}
}
func (p *todosPane) Refresh() {
	if p.closed || p.loading {
		return
	}
	p.loading = true
	p.stale = false
	oneBotCall(p.settings.parent, "todos.read", map[string]string{"botId": p.settings.botID}, func(value struct {
		Items []Todo `json:"items"`
	}, err error) {
		if p.closed {
			return
		}
		p.loading = false
		if err != nil {
			p.error = err.Error()
			return
		}
		p.rows = value.Items
		p.error = ""
	})
	oneBotCall(p.settings.parent, "todos.dailyCheck", map[string]string{"botId": p.settings.botID}, func(on bool, err error) {
		if p.closed {
			return
		}
		if err != nil {
			p.error = err.Error()
			return
		}
		p.daily = on
	})
}
func (p *todosPane) open(item Todo, fresh bool) {
	p.settings.parent.env.RequestChange(func() {
		p.editor = &todoEditor{item: copyValue(item), original: copyValue(item), fresh: fresh, nextLabel: nextLabel(item.Next), nextInstruction: nextInstruction(item.Next), nextKind: "advance", deadlineZone: "UTC"}
		if item.Next != nil {
			p.editor.nextKind = item.Next.Kind
		}
	})
}
func (p *todosPane) patch(item Todo, changes map[string]any) {
	if p.busy {
		return
	}
	p.busy = true
	changes["id"] = item.ID
	changes["expectedRevision"] = item.Revision
	oneBotCall(p.settings.parent, "todos.patch", map[string]any{"botId": p.settings.botID, "patch": changes}, func(_ Todo, err error) {
		if p.closed {
			return
		}
		p.busy = false
		if err != nil {
			p.error = err.Error()
			return
		}
		p.decision = nil
		p.Refresh()
	})
}
func (p *todosPane) saveEditor() {
	e := p.editor
	if e == nil || p.busy {
		return
	}
	if strings.TrimSpace(e.item.Title) == "" {
		p.error = nt("titleRequired")
		return
	}
	patch := map[string]any{"title": e.item.Title, "progress": e.item.Progress, "outcome": e.item.Outcome, "value": e.item.Value}
	if e.nextLabel != "" {
		patch["next"] = TodoNext{Label: e.nextLabel, Instruction: e.nextInstruction, Kind: e.nextKind}
	} else {
		patch["next"] = nil
	}
	if e.fresh {
		patch["key"] = "owner:" + rand.Text()
		patch["origin"] = "assigned"
	} else {
		patch["id"] = e.item.ID
		patch["expectedRevision"] = e.item.Revision
	}
	if e.deadlineDate != "" {
		patch["deadlineOverride"] = map[string]any{"value": map[string]string{"kind": "date", "date": e.deadlineDate, "timeZone": e.deadlineZone}}
	}
	p.busy = true
	oneBotCall(p.settings.parent, "todos.patch", map[string]any{"botId": p.settings.botID, "patch": patch}, func(_ Todo, err error) {
		if p.closed {
			return
		}
		p.busy = false
		if err != nil {
			p.error = err.Error()
			return
		}
		p.editor = nil
		p.Refresh()
	})
}
func (p *todosPane) advance(item Todo) {
	if p.busy {
		return
	}
	p.busy = true
	oneBotCall(p.settings.parent, "todos.act", map[string]any{"botId": p.settings.botID, "id": item.ID, "revision": item.Revision, "requestId": rand.Text(), "locale": botLocale()}, func(_ *Todo, err error) {
		if p.closed {
			return
		}
		p.busy = false
		if err != nil {
			p.error = err.Error()
			return
		}
		p.Refresh()
	})
}
func (p *todosPane) View(c *ui.Context) {
	if p.closed {
		return
	}
	if p.stale && p.editor == nil {
		p.Refresh()
	}
	if p.editor != nil {
		p.editorView(c)
		kit.InlineError(c, p.error)
		return
	}
	ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
		if kit.PrimaryButton(c, nt("addTodo"), icons.Plus, false).Clicked() {
			p.open(Todo{Status: "open"}, true)
		}
		if kit.Button(c, nt("refresh"), icons.RefreshCw, p.loading).Clicked() {
			p.Refresh()
		}
	})
	daily := p.daily
	off := slices.Contains(p.settings.draft.Capabilities.Value.SystemSkillsOff, "todo")
	if kit.Toggle(c, &daily, nt("dailyCheck"), nt("dailyCheckHint")).Disabled(p.busy || off).Changed() {
		p.busy = true
		oneBotCall(p.settings.parent, "todos.dailyCheck", map[string]any{"botId": p.settings.botID, "enabled": daily}, func(on bool, err error) {
			if p.closed {
				return
			}
			p.busy = false
			if err != nil {
				p.error = err.Error()
				return
			}
			p.daily = on
		})
	}
	kit.TextInput(c, &p.query).FillWidth().Label(nt("search")).Placeholder(nt("search"))
	kit.Segmented(c, &p.filter, []kit.SelectOption{{Value: "open", Label: nt("todoOpen")}, {Value: "done", Label: nt("todoDone")}, {Value: "all", Label: nt("all")}}, nt("todoFilter"))
	if p.loading {
		kit.Loader(c)
	}
	kit.InlineError(c, p.error)
	for _, item := range p.rows {
		if p.filter != "all" && (item.Status != p.filter || item.Decision != nil) {
			continue
		}
		if !contains(item.Title+" "+item.Progress, p.query) {
			continue
		}
		ui.Column(c.Key(item.ID)).FillWidth().Gap(5).Padding(10, 0).BorderWidth(0, 0, 1, 0).BorderColor(kit.Colors(c).Hairline).Children(func() {
			kit.Strong(c, item.Title)
			if item.Progress != "" {
				kit.Help(c, item.Progress)
			}
			if item.Outcome != "" {
				kit.Help(c, item.Outcome)
			}
			if item.Next != nil && kit.Button(c, item.Next.Label, icons.ArrowRight, false).Disabled(p.busy || item.Status != "open").Clicked() {
				p.advance(item)
			}
			if item.Action != nil {
				kit.Caption(c, nt("todoAction."+item.Action.State))
				kit.InlineError(c, item.Action.Error)
			}
			for _, source := range item.Sources {
				if kit.GhostButton(c, source.Label, icons.Link, false).FillWidth().MinWidth(0).Justify(ui.Start).Clicked() {
					if source.Kind == "task" || source.Kind == "conversation" {
						p.settings.parent.openTask(source.ID)
					} else if strings.HasPrefix(source.Ref, "https://") || strings.HasPrefix(source.Ref, "http://") {
						p.settings.parent.openExternal(source.Ref)
					}
				}
			}
			for _, association := range item.Associations {
				if kit.GhostButton(c, association.Label, icons.GitPullRequest, false).Clicked() {
					if association.Kind == "task" {
						p.settings.parent.openTask(association.ID)
					} else {
						p.settings.parent.openExternal(association.ID)
					}
				}
			}
			kit.IconButton(c, icons.Ellipsis, nt("actions")).Menu(func(m *ui.Menu) {
				if m.Item(nt("edit")).Chosen() {
					p.open(item, false)
				}
				for _, op := range []string{"complete", "reopen", "delete", "mute", "later", "restore"} {
					if m.Item(nt("todoOperation." + op)).Chosen() {
						p.decision = &todoDecision{item: copyValue(item), operation: op, until: time.Now().Add(24 * time.Hour).Format(time.RFC3339)}
					}
				}
				if len(item.DeadlineCandidate) > 0 && string(item.DeadlineCandidate) != "null" && m.Item(nt("confirmDeadline")).Chosen() {
					p.patch(item, map[string]any{"operation": "confirm-deadline"})
				}
			})
		})
	}
	p.decisionView(c)
}
func (p *todosPane) editorView(c *ui.Context) {
	e := p.editor
	ui.Field(c, nt("todoTitle"), func() { kit.TextInput(c, &e.item.Title).FillWidth().Label(nt("todoTitle")) })
	for _, field := range []struct {
		label string
		value *string
	}{{"todoProgress", &e.item.Progress}, {"todoOutcome", &e.item.Outcome}, {"todoValue", &e.item.Value}, {"nextLabel", &e.nextLabel}, {"nextInstruction", &e.nextInstruction}} {
		ui.Field(c, nt(field.label), func() { ui.TextArea(c, field.value).FillWidth().Height(80).Label(nt(field.label)) })
	}
	kit.Select(c, &e.nextKind, []kit.SelectOption{{Value: "advance", Label: nt("advance")}, {Value: "view", Label: nt("view")}, {Value: "decide", Label: nt("decide")}})
	ui.Field(c, nt("deadlineDate"), func() {
		kit.TextInput(c, &e.deadlineDate).FillWidth().Placeholder("YYYY-MM-DD").Label(nt("deadlineDate"))
	})
	ui.Field(c, nt("timezone"), func() { kit.TextInput(c, &e.deadlineZone).FillWidth().Label(nt("timezone")) })
	ui.Row(c).FillWidth().Gap(8).Children(func() {
		if kit.Button(c, nt("cancel"), nil, false).Disabled(p.busy).Clicked() {
			p.settings.parent.env.RequestChange(func() { p.editor = nil })
		}
		if kit.PrimaryButton(c, nt("save"), icons.Save, p.busy).Disabled(p.busy).Clicked() {
			p.saveEditor()
		}
	})
}
func (p *todosPane) decisionView(c *ui.Context) {
	d := p.decision
	if d == nil {
		return
	}
	open := true
	if kit.Dialog(c, &open, kit.DialogOptions{Title: nt("todoOperation." + d.operation), Message: d.item.Title, Confirm: nt("confirm"), Cancel: nt("cancel"), Danger: d.operation == "delete", Busy: p.busy, Error: p.error, Body: func() {
		if d.operation == "complete" {
			ui.TextArea(c, &d.summary).FillWidth().Height(100).Label(nt("completionSummary"))
		}
		if d.operation == "later" {
			kit.TextInput(c, &d.until).FillWidth().Label(nt("until"))
		}
	}}) {
		changes := map[string]any{"operation": d.operation}
		if d.operation == "complete" {
			changes["completion"] = map[string]string{"summary": d.summary}
		}
		if d.operation == "later" {
			changes["until"] = d.until
		}
		p.patch(d.item, changes)
	}
	if !open {
		p.decision = nil
	}
}
