package onebot

import (
	"time"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

type NightlyItem struct {
	ID, Kind, Action, Target, Title, Reason string
	Undone                                  bool
}
type NightlyRun struct {
	ID, Outcome, ErrorCode string
	At                     int64
	Turns                  int
	Items                  []NightlyItem
}
type NightlyCandidate struct{ ID, Title, Summary string }
type NightlyView struct {
	Enabled                *bool
	Frequency              string
	PendingTurns           int
	Running, MemoryEnabled bool
	Runs                   []NightlyRun
	Candidates             []NightlyCandidate
}
type nightlyPane struct {
	settings                     *profileSettings
	data                         NightlyView
	loading, busy, closed, stale bool
	error                        string
}

func newNightlyPane(d *profileSettings) *nightlyPane {
	p := &nightlyPane{settings: d}
	p.Refresh()
	return p
}
func (p *nightlyPane) Busy() bool { return p.busy }
func (p *nightlyPane) Close()     { p.closed = true }
func (p *nightlyPane) Refresh() {
	if p.closed || p.loading {
		return
	}
	p.loading = true
	p.stale = false
	oneBotCall(p.settings.parent, "routines.nightly.get", map[string]string{"botId": p.settings.botID}, func(data NightlyView, err error) {
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
func (p *nightlyPane) action(method string, params map[string]any) {
	if p.closed || p.busy {
		return
	}
	p.busy = true
	params["botId"] = p.settings.botID
	oneBotCall(p.settings.parent, "routines.nightly."+method, params, func(data NightlyView, err error) {
		if p.closed {
			return
		}
		p.busy = false
		if err != nil {
			p.error = err.Error()
			return
		}
		p.data = data
		p.error = ""
	})
}
func (p *nightlyPane) View(c *ui.Context) {
	if p.closed {
		return
	}
	if p.stale {
		p.Refresh()
	}
	enabled := p.data.Enabled != nil && *p.data.Enabled
	if kit.Toggle(c, &enabled, nt("nightly"), nt("nightlyHint")).Disabled(p.data.Enabled == nil || p.busy).Changed() {
		p.action("setEnabled", map[string]any{"enabled": enabled})
	}
	frequency := p.data.Frequency
	ui.Field(c, nt("frequency"), func() {
		if changed, _ := kit.Select(c, &frequency, []kit.SelectOption{{Value: "daily", Label: nt("daily")}, {Value: "weekly", Label: nt("weekly")}}); changed {
			p.action("setFrequency", map[string]any{"frequency": frequency})
		}
	})
	ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
		if kit.PrimaryButton(c, nt("learnNow"), icons.BookOpen, p.busy).Disabled(p.busy || p.data.Running).Clicked() {
			p.action("runNow", map[string]any{})
		}
		if kit.Button(c, nt("refresh"), icons.RefreshCw, p.loading).Clicked() {
			p.Refresh()
		}
	})
	kit.Help(c, nt("pendingTurns", "count", p.data.PendingTurns))
	if p.data.Running {
		kit.Status(c, nt("learning"))
	}
	kit.InlineError(c, p.error)
	for _, candidate := range p.data.Candidates {
		ui.Column(c.Key(candidate.ID)).FillWidth().Gap(4).Padding(8, 0).Children(func() {
			kit.Strong(c, candidate.Title)
			kit.Help(c, candidate.Summary)
			ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
				if kit.Button(c, nt("promoteSkill"), icons.Check, false).Disabled(p.busy).Clicked() {
					p.action("candidate", map[string]any{"candidateId": candidate.ID, "action": "promote"})
				}
				if kit.Button(c, nt("declineCandidate"), icons.X, false).Disabled(p.busy).Clicked() {
					p.action("candidate", map[string]any{"candidateId": candidate.ID, "action": "decline"})
				}
			})
		})
	}
	for _, run := range p.data.Runs {
		ui.Column(c.Key(run.ID)).FillWidth().Gap(6).Padding(10, 0).BorderWidth(0, 0, 1, 0).BorderColor(kit.Colors(c).Hairline).Children(func() {
			kit.Caption(c, time.UnixMilli(run.At).Format("2006-01-02 15:04")+" · "+nt("learningOutcome."+run.Outcome))
			if run.ErrorCode != "" {
				kit.InlineError(c, nt("learningError."+run.ErrorCode))
			}
			for _, item := range run.Items {
				ui.Column(c.Key(item.ID)).FillWidth().Gap(4).Children(func() {
					kit.Strong(c, item.Title)
					kit.Help(c, item.Reason)
					label := nt("undo")
					if item.Undone {
						label = nt("reapply")
					}
					if kit.Button(c, label, icons.Undo2, false).Disabled(p.busy).Clicked() {
						p.action("revert", map[string]any{"runId": run.ID, "itemId": item.ID, "redo": item.Undone})
					}
				})
			}
		})
	}
}
