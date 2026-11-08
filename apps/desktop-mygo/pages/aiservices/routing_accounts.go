package aiservices

import (
	"encoding/json"
	"strings"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/pages/aiservices/usage"
)

type routeTally struct {
	ID       string         `json:"id"`
	Provider string         `json:"provider"`
	Name     string         `json:"name"`
	Who      string         `json:"who"`
	Tried    int            `json:"tried"`
	OK       int            `json:"ok"`
	Fails    map[string]int `json:"fails"`
	Rest     *traceRest     `json:"rest"`
}

func (t *nativeTrace) accountsView(c *ui.Context, p *page, rows []traceRoute) {
	ui.Collapsible(c, i18n.T("aiRouting.accountActivity"), &t.accountsOpen, func() {
		counts := map[string]routeTally{}
		// Today's unfiltered counters come from the complete persisted day,
		// not just the short live ring. Filtered/historic views count their rows.
		wholeDay := t.day == "" && t.replay == nil && t.query == "" && t.provider == "" && t.model == "" && t.status == "" && t.client == "" && t.caller == "" && len(t.purposes) == 0
		if wholeDay {
			for key, row := range t.tallies {
				counts[key] = row
			}
		}
		if !wholeDay || len(counts) == 0 {
			for _, r := range rows {
				var ev traceEvidence
				json.Unmarshal(r.Raw, &ev)
				for _, w := range append(ev.Order, ev.Left...) {
					if _, ok := counts[w.ID]; !ok {
						counts[w.ID] = routeTally{ID: w.ID, Name: w.Name, Who: w.Who, Provider: w.Provider}
					}
				}
				for _, tr := range r.Tries {
					row := counts[tr.ID]
					row.ID = tr.ID
					row.Tried++
					if tr.Done && tr.Status > 0 && tr.Status < 400 && tr.Error == "" && tr.Fail == "" {
						row.OK++
					} else if tr.Done && (tr.Fail != "" || tr.Error != "" || tr.Status >= 400) {
						if row.Fails == nil {
							row.Fails = map[string]int{}
						}
						failure := tr.Fail
						if failure == "" {
							failure = "other"
						}
						row.Fails[failure]++
					}
					row.Rest = tr.Rest
					counts[tr.ID] = row
				}
			}
		}
		mask := p.routing.accountMasker()
		for _, key := range sortedKeys(counts) {
			row := counts[key]
			ui.Column(c.Key(key)).FillWidth().Gap(5).Padding(10, 0).Children(func() {
				label := strings.TrimSpace(row.Name + " · " + row.Who)
				if label == "·" || label == "" {
					label = p.routing.providerLabel(row.Provider)
					if label == "" {
						label = nativeT("accounts")
					}
				}
				kit.Strong(c, mask(label))
				kit.Help(c, i18n.T("aiRouting.accountCounts", "tried", row.Tried, "answered", row.OK))
				for _, failure := range sortedKeys(row.Fails) {
					kit.Help(c, failure+" · "+kit.Itoa(row.Fails[failure]))
				}
				if p.draft != nil && p.draft.Value.find(row.Provider) != nil && kit.GhostButton(c, i18n.T("aiRouting.manageAccount"), nil, false).Disabled(p.locked()).Clicked() {
					change := func() { p.openProvider(row.Provider); p.focusModels = false }
					if p.env.RequestChange != nil {
						p.env.RequestChange(change)
					} else {
						change()
					}
				}
				if p.draft != nil {
					pr := p.draft.Value.find(row.Provider)
					if pr != nil && pr.AuthMode == "account" && kit.GhostButton(c, i18n.T("ai.signInAgain"), nil, false).Disabled(p.locked()).Clicked() {
						change := func() {
							p.openProvider(row.Provider)
							p.extensions.signIn = &signInRequest{provider: row.Provider, user: row.Who, nonce: nonce()}
						}
						if p.env.RequestChange != nil {
							p.env.RequestChange(change)
						} else {
							change()
						}
					}
				}
				t.restView(c, p, row.Rest)
			})
		}
		ui.Row(c).Gap(8).Wrap().Children(func() {
			if kit.Button(c, i18n.T("aiRouting.readAllowances"), icons.RefreshCw, false).Clicked() {
				t.quotasOpen = true
				if t.quotas != nil {
					t.quotas.Refresh()
				}
			}
			if kit.GhostButton(c, i18n.T("aiRouting.viewUsage"), nil, false).Clicked() {
				change := func() { p.switchTab(tabUsage) }
				if p.env.RequestChange != nil {
					p.env.RequestChange(change)
				} else {
					change()
				}
			}
		})
		if t.quotasOpen && p.saved != nil && p.draft != nil {
			if t.quotas == nil {
				t.quotas = usage.NewAllowances(p.env)
			}
			s := p.draft.Value.settings()
			t.quotas.View(c, usageProviders(p.saved.Value.Providers), &usage.Preferences{HideAccounts: s.HideAccounts, QuotaLeft: s.QuotaLeft}, func(next usage.Preferences) { s.HideAccounts = next.HideAccounts; s.QuotaLeft = next.QuotaLeft })
		}
	})
	if !t.accountsOpen && t.quotas != nil {
		t.quotas.Close()
	}
}
