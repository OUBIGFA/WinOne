package aiservices

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

type platformSession struct {
	Kind        string `json:"kind"`
	SignedIn    bool   `json:"signedIn"`
	Account     string `json:"account"`
	Ended       bool   `json:"ended"`
	URL         string `json:"url"`
	Bookmarklet string `json:"bookmarklet"`
}

type allowanceWindow struct {
	Name, Display, Unit, Pool, Family, Lane string
	Used, Amount, Limit, RunsOut            float64
	Text, Unlimited, Credits, Extra         bool
	Balance, Spent                          *float64
	ResetsAt, RunsOutAt, SpentSince         *time.Time
}

type allowanceCard struct {
	Name, User, Plan, Balance, Error, ReadError, SharedBalance, Renew string
	ReadDeferred, NeedsAccessKey                                      bool
	ReadAt, AsOf, Until                                               *time.Time
	Windows                                                           []allowanceWindow
	BalanceParts, BalanceSplit                                        []struct {
		Label, Text, Error string
		Percent            *float64
		Expires            *time.Time
	}
}

type planPanel struct {
	source               string
	read                 int
	quotaGeneration      int
	busy                 bool
	err                  string
	state                *platformSession
	auth                 *accountPanel
	quotaBusy, quotaRead bool
	quotas               []allowanceCard
}

func planT(key string, args ...any) string { return i18n.T("aiPlan."+key, args...) }

func (e *editor) loadPlan(pr *provider, action string) {
	m := &e.plan
	if m.busy {
		return
	}
	m.read++
	// A login change invalidates only old allowance work. Session status
	// reads and allowance reads otherwise retain independent lifetimes.
	if action == "signout" {
		m.quotaGeneration++
		m.quotaBusy = false
	}
	owner := m.read
	m.busy, m.err = true, ""
	id := pr.ID
	core.Go(e.p.env, func() (platformSession, error) {
		var state platformSession
		err := e.p.env.Host.CallCompletingInto("capture.ai.plans."+action, map[string]string{"provider": id}, &state)
		return state, err
	}, func(state platformSession, err error) {
		if e.closed || owner != m.read {
			return
		}
		m.busy = false
		if err != nil {
			m.err = err.Error()
			return
		}
		m.state = &state
		if action == "signout" {
			m.quotas = nil
			m.quotaRead = false
		}
	})
}

func (e *editor) readAllowance(pr *provider) {
	m := &e.plan
	if m.quotaBusy {
		return
	}
	m.quotaBusy, m.err = true, ""
	m.quotaGeneration++
	owner := m.quotaGeneration
	params := map[string]string{"provider": pr.ID}
	if pr.AuthMode == "account" {
		params["agent"] = pr.Agent
	}
	core.Go(e.p.env, func() ([]allowanceCard, error) {
		var result struct {
			Quotas []allowanceCard `json:"quotas"`
		}
		err := e.p.env.Host.CallCompletingInto("capture.ai.accounts.quota", params, &result)
		return result.Quotas, err
	}, func(cards []allowanceCard, err error) {
		if e.closed || owner != m.quotaGeneration {
			return
		}
		m.quotaBusy = false
		if err != nil {
			m.err = err.Error()
			return
		}
		m.quotas, m.quotaRead = cards, true
	})
}

// Platform login augments an API-key provider. It never becomes another
// provider or replaces the key used for model calls.
func (e *editor) planView(c *ui.Context, pr *provider, locked bool) {
	m, p := &e.plan, e.p
	saved := p.connectionSaved(pr) && p.keys[pr.ID] == nil
	if pr.AuthMode == "api-key" && saved {
		source := fmt.Sprintf("%d:%s:%s:%s", p.savedGen, pr.ID, pr.BaseURL, pr.PresetRegion)
		if source != m.source && !m.busy && !m.quotaBusy {
			m.source = source
			m.state = nil
			e.loadPlan(pr, "get")
		}
	}
	kit.Group(c, icons.ChartColumn, planT("title"), planT("hint"), func() {
		kit.Stack(c, "", "", func() {
			if !saved {
				kit.Help(c, planT("saveFirst"))
			}
			if state := m.state; state != nil && state.Kind != "" && saved {
				name := map[string]string{"qianwen": "Qwen AI", "qianfan": "Baidu Qianfan", "stepfun": "StepFun"}[state.Kind]
				label := planT("signIn", "name", name)
				if state.SignedIn {
					label = planT("signInAgain", "name", name)
				}
				if state.Account != "" {
					kit.Help(c, state.Account)
				}
				if state.Ended {
					kit.Help(c, planT("ended"))
				}
				if state.Bookmarklet != "" {
					kit.Help(c, planT("pasteHint"))
					if kit.Button(c, planT("copyBookmarklet"), icons.Copy, false).Disabled(locked).Clicked() {
						c.Services().WriteClipboard(state.Bookmarklet)
					}
				}
				ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
					if kit.Button(c, label, icons.LogIn, false).Disabled(locked || m.busy || m.quotaBusy).Clicked() {
						if m.auth != nil {
							m.auth.Close()
						}
						m.auth = newAccountPanel(p, pr)
						m.auth.plan = true
						m.auth.completed = func() { e.loadPlan(pr, "get"); m.quotaRead = false; m.quotas = nil }
						m.auth.start(accountRequest{Agent: "platform-plan", Provider: pr.ID})
					}
					if state.SignedIn && kit.GhostButton(c, planT("signOut"), icons.LogOut, false).Disabled(locked || m.busy || m.quotaBusy).Clicked() {
						e.loadPlan(pr, "signout")
					}
				})
			}
			if m.auth != nil {
				m.auth.report()
				kit.InlineError(c, m.auth.err)
				if m.auth.job != nil {
					m.auth.jobView(c)
				}
			}
			if kit.Button(c, planT("refresh"), icons.RefreshCw, m.quotaBusy).Disabled(locked || m.busy || m.quotaBusy || !saved).Clicked() {
				e.readAllowance(pr)
			}
			kit.InlineError(c, m.err)
			if m.quotaRead && len(m.quotas) == 0 {
				kit.Help(c, planT("empty"))
			}
			for i, card := range m.quotas {
				ui.Column(c.Key(i)).FillWidth().Gap(4).Children(func() {
					kit.Label(c, strings.Trim(strings.Join([]string{card.Name, card.User, card.Plan}, " · "), " ·"))
					if card.Balance != "" {
						kit.Help(c, planT("balance", "value", card.Balance))
					}
					if card.SharedBalance != "" {
						kit.Help(c, planT("shared", "name", card.SharedBalance))
					}
					for _, part := range append(card.BalanceParts, card.BalanceSplit...) {
						text := strings.Join([]string{part.Label, part.Text, part.Error}, " ")
						if part.Expires != nil {
							text += " · " + planT("until", "time", part.Expires.Local().Format("2006-01-02 15:04"), "renew", "")
						}
						kit.Help(c, text)
					}
					for _, w := range card.Windows {
						kit.Help(c, allowanceWindowText(w))
					}
					if card.Until != nil {
						kit.Help(c, planT("until", "time", card.Until.Local().Format("2006-01-02 15:04"), "renew", card.Renew))
					}
					if card.AsOf != nil {
						kit.Help(c, planT("cached", "time", card.AsOf.Local().Format("2006-01-02 15:04")))
					} else if card.ReadAt != nil {
						kit.Help(c, planT("readAt", "time", card.ReadAt.Local().Format("2006-01-02 15:04")))
					}
					if card.ReadDeferred {
						kit.Help(c, planT("deferred"))
					}
					kit.InlineError(c, card.ReadError)
					kit.InlineError(c, card.Error)
				})
			}
		})
	})
}

func allowanceWindowText(w allowanceWindow) string {
	text := w.Display
	if text == "" && !w.Text {
		switch {
		case w.Unlimited:
			text = planT("unlimited")
		case w.Balance != nil:
			text = planT("balance", "value", formatNumber(*w.Balance)+" "+w.Unit)
		case w.Limit > 0:
			text = formatNumber(w.Amount) + " / " + formatNumber(w.Limit) + " " + w.Unit
		default:
			text = planT("used", "value", formatNumber(w.Used))
		}
	}
	line := strings.TrimSpace(w.Name + " · " + text)
	if w.ResetsAt != nil {
		line += " · " + planT("reset", "time", w.ResetsAt.Local().Format("2006-01-02 15:04"))
	}
	if w.RunsOutAt != nil {
		line += " · " + planT("expires", "value", formatNumber(w.RunsOut)+" "+w.Unit, "time", w.RunsOutAt.Local().Format("2006-01-02 15:04"))
	}
	if w.Spent != nil {
		line += " · " + planT("spent", "value", formatNumber(*w.Spent)+" "+w.Unit)
	}
	return line
}
