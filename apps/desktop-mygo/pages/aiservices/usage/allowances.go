package usage

import (
	"encoding/json"
	"net/url"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/widgets"
)

// allowances are React's UsageAllowances: the windows and balances of the
// configured accounts and API keys, read from their vendors when the
// overview shows, again on each refresh, and never invented on a failure.
type allowances struct {
	env *core.Env
	// flights share one read in flight per account among the reads that
	// ask for it while it runs; a refresh after it ends reads anew, and a
	// failed read is never kept as a reading.
	flights map[string][]func(quotaResult)
	data    []quotaResult
	loading bool
	active  bool
	key     string
	// generation tells the latest round of reads from older ones.
	generation int
	// masked, left: the accounts hidden, the windows as what is left;
	// seen are the preferences they last followed.
	masked, left bool
	seen         *[2]bool
	trend        string
	zoomed       map[string]*bool
}

// quotaResult is one provider's readings, or why there are none.
type quotaResult struct {
	provider Provider
	quotas   []allowance
	err      string
}

func newAllowances(env *core.Env) *allowances {
	return &allowances{env: env, flights: map[string][]func(quotaResult){}, masked: true, left: true, trend: "cycle", zoomed: map[string]*bool{}}
}

// deactivate stops the round in flight from showing; showing again reads.
func (a *allowances) deactivate() {
	if a.active {
		a.active = false
		a.generation++
		a.loading = false
	}
}

// configured are the providers an allowance can be read for: accounts
// signed in with an agent, and API keys that have a key.
func configured(providers []Provider) []Provider {
	var out []Provider
	for _, p := range providers {
		if p.AuthMode == "account" && p.Agent != "" || p.AuthMode == "api-key" && p.HasKey {
			out = append(out, p)
		}
	}
	return out
}

// read reads every configured provider's allowance once, all together.
func (a *allowances) read(providers []Provider) {
	a.generation++
	generation := a.generation
	a.loading = true
	results := make([]quotaResult, len(providers))
	left := len(providers)
	if left == 0 {
		a.loading, a.data = false, nil
		return
	}
	for i, p := range providers {
		a.readOne(p, func(r quotaResult) {
			results[i] = r
			left--
			if left == 0 && generation == a.generation {
				a.loading, a.data = false, results
			}
		})
	}
}

// readOne reads a provider's allowance, joining a read of the same
// account still in flight.
func (a *allowances) readOne(p Provider, done func(quotaResult)) {
	identity, _ := json.Marshal([]string{p.ID, p.Agent, p.AgentBackend})
	key := string(identity)
	waiting, running := a.flights[key]
	a.flights[key] = append(waiting, done)
	if running {
		return
	}
	params := map[string]string{"provider": p.ID}
	if p.Agent != "" {
		params["agent"] = p.Agent
	}
	core.Go(a.env, func() ([]allowance, error) {
		var reply struct {
			Quotas []allowance `json:"quotas"`
		}
		err := a.env.Host.CallCompletingInto("capture.ai.accounts.quota", params, &reply)
		return reply.Quotas, err
	}, func(quotas []allowance, err error) {
		waiting := a.flights[key]
		delete(a.flights, key)
		r := quotaResult{provider: p, quotas: quotas}
		if err != nil {
			r = quotaResult{provider: p, err: err.Error()}
		}
		for _, fn := range waiting {
			fn(r)
		}
	})
}

// view shows the allowances; revision is the tab's refresh count.
func (a *allowances) view(c *ui.Context, providers []Provider, prefs *Preferences, change func(Preferences), revision int) {
	if prefs != nil && (a.seen == nil || *a.seen != [2]bool{prefs.HideAccounts, prefs.QuotaLeft}) {
		a.seen = &[2]bool{prefs.HideAccounts, prefs.QuotaLeft}
		a.masked, a.left = prefs.HideAccounts, prefs.QuotaLeft
	}
	list := configured(providers)
	identity := make([][]any, len(list))
	for i, p := range list {
		identity[i] = []any{p.ID, p.Agent, p.BaseURL, p.Keys}
	}
	encoded, _ := json.Marshal(identity)
	key := string(encoded) + "/" + strconv.Itoa(revision)
	if !a.active || key != a.key {
		a.active, a.key = true, key
		a.read(list)
	}
	t, pal := c.Theme(), kit.Colors(c)
	ui.Column(c).FillWidth().Gap(14).Role(ui.RoleGroup).Label(tr("usage.allowances")).Children(func() {
		ui.Row(c).FillWidth().GapX(12).GapY(10).Wrap().AlignItems(ui.Center).Justify(ui.SpaceBetween).Children(func() {
			kit.Heading(c, tr("usage.allowances")).Role(ui.RoleHeading).Level(3)
			ui.Row(c).Gap(8).Wrap().Children(func() {
				icon, label := icons.Eye, "usage.hideAccounts"
				if a.masked {
					icon, label = icons.EyeOff, "usage.showAccounts"
				}
				if kit.Button(c, tr(label), icon, false).Clicked() {
					a.masked = !a.masked
					if change != nil && prefs != nil {
						next := *prefs
						next.HideAccounts = a.masked
						change(next)
					}
				}
				label = "usage.showLeft"
				if a.left {
					label = "usage.showUsed"
				}
				if kit.Button(c, tr(label), nil, false).Clicked() {
					a.left = !a.left
					if change != nil && prefs != nil {
						next := *prefs
						next.QuotaLeft = a.left
						change(next)
					}
				}
			})
		})
		ui.Row(c).FillWidth().Children(func() {
			field(c, tr("usage.quotaTrend"), func() {
				choose(c, &a.trend, []kit.SelectOption{{Value: "off", Label: tr("usage.trendOff")}, {Value: "2d", Label: tr("usage.twoDays")},
					{Value: "cycle", Label: tr("usage.cycle")}})
			}).Grow(0).Basis(240)
		})
		if a.loading {
			kit.Status(c, tr("usage.quotaLoading"))
		}
		if len(list) == 0 {
			kit.Help(c, tr("usage.noAccounts"))
		}
		ui.Row(c).FillWidth().GapX(24).GapY(24).Wrap().AlignItems(ui.Start).Children(func() {
			for _, r := range a.data {
				ui.Column(c.Key(r.provider.ID)).Basis(280).Grow(1).Shrink(1).MinWidth(0).Gap(12).Padding(16, 0, 0, 0).
					BorderWidth(1, 0, 0, 0).BorderColor(pal.Hairline).Role(ui.RoleGroup).Label(r.provider.Name).Children(func() {
					ui.Row(c).Gap(8).AlignItems(ui.Center).MinWidth(0).Children(func() {
						widgets.ProviderIcon(c, a.env, providerMark(&r.provider), 16)
						kit.Strong(c, r.provider.Name).Role(ui.RoleHeading).Level(4).MinWidth(0)
					})
					if r.err != "" {
						kit.Notice(c, kit.Failure, "", r.err, nil)
					}
					if r.err == "" && len(r.quotas) == 0 {
						kit.Help(c, tr("usage.quotaUnavailable"))
					}
					for i, q := range r.quotas {
						a.quotaView(c, r.provider, i, q, t)
					}
				})
			}
		})
	})
}

// quotaView is one account's reading: who, its plan, its windows and
// balances, its curve and when it was read.
func (a *allowances) quotaView(c *ui.Context, provider Provider, index int, q allowance, t *ui.Theme) {
	key := strconv.Itoa(index) + "/" + q.User
	ui.Column(c.Key(key)).FillWidth().Gap(10).MinWidth(0).Children(func() {
		who := q.User
		if who == "" {
			who = provider.Name
		}
		if a.masked {
			who = tr("usage.maskedAccount", "number", index+1)
		}
		if q.Plan != "" {
			who += " · " + q.Plan
		}
		kit.Help(c, who)
		if q.Error != "" || q.AsOf != "" {
			allowanceMessage(c, q)
		}
		if q.Balance != "" {
			ui.Text(c, tr("usage.balance")+" · "+q.Balance).Selectable()
		}
		for _, parts := range [][]balancePart{q.BalanceParts, q.BalanceSplit} {
			if len(parts) > 0 {
				balanceDetails(c, parts)
			}
		}
		for i, w := range q.Windows {
			ui.Column(c.Key(i)).FillWidth().Gap(6).Margin(8, 0, 0, 0).Children(func() {
				ui.Row(c).FillWidth().GapX(16).GapY(4).Wrap().Justify(ui.SpaceBetween).Children(func() {
					ui.Text(c, w.Name).MinWidth(0)
					amount := tr("usage.unlimited")
					if !w.Unlimited {
						share, word := w.Used, "usage.used"
						if a.left {
							share, word = max(0, 100-w.Used), "usage.remaining"
						}
						amount = toFixed(share, 1) + "% " + tr(word)
					}
					ui.Text(c, amount).NoWrap().FontFeatures("tnum")
				})
				if !w.Unlimited {
					shown := w.Used
					if a.left {
						shown = 100 - w.Used
					}
					ui.Progress(c, max(0, min(100, shown))/100).FillWidth().Label(w.Name)
				}
				var detail []string
				if w.Display != "" {
					detail = append(detail, w.Display)
				}
				if w.ResetsAt != "" {
					detail = append(detail, tr("usage.resetsAt", "time", dateTime(w.ResetsAt)))
				}
				if len(detail) > 0 {
					kit.Help(c, strings.Join(detail, " · "))
				}
			})
		}
		zoom := a.zoomed[provider.ID+"\x00"+key]
		if zoom == nil {
			zoom = new(bool)
			a.zoomed[provider.ID+"\x00"+key] = zoom
		}
		// Each reading's history is its own: the curve finds its lines
		// in it, as React passed it to Magpie's reader.
		quotaCurve(c, q, a.trend, zoom)
		read := q.AsOf
		if read == "" {
			read = q.ReadAt
		}
		if read != "" {
			kit.Help(c, tr("usage.readAt", "time", dateTime(read)))
		}
	})
}

// allowanceMessage is React's AllowanceMessage: a vendor that tells no
// balance is not a failed or zero reading, and a reading kept from
// before is never shown as a new one. Its console is offered only as a
// plain https address.
func allowanceMessage(c *ui.Context, q allowance) {
	console := ""
	if u, err := url.Parse(q.SeeAt); err == nil && u.Scheme == "https" && u.User == nil && u.Host != "" {
		console = u.String()
	}
	unavailable := strings.HasPrefix(q.Error, "OpenCode tells no API key its balance")
	message := tr("usage.quotaUnavailable")
	switch {
	case q.AsOf != "":
		message = tr("usage.quotaStale", "time", dateTime(q.AsOf))
	case unavailable:
		message = tr("usage.openCodeNoBalance")
	}
	ui.Column(c).FillWidth().Gap(6).Children(func() {
		if unavailable {
			kit.Help(c, message)
		} else {
			kit.Notice(c, kit.Failure, "", message, nil)
		}
		if console != "" {
			ui.Link(c, tr("usage.openConsole"), console)
		}
	})
}

// balanceDetails are React's BalanceDetails: each amount as the vendor
// gave it, zero included, its percentage as a bar, a part that failed
// said so. Only the two split labels are translated.
func balanceDetails(c *ui.Context, parts []balancePart) {
	ui.Column(c).FillWidth().Gap(8).MinWidth(0).Children(func() {
		for i, part := range parts {
			label := part.Label
			switch label {
			case "Granted":
				label = tr("usage.granted")
			case "Topped up":
				label = tr("usage.toppedUp")
			case "":
				label = tr("usage.balance")
			}
			ui.Column(c.Key(i)).FillWidth().Gap(4).MinWidth(0).Children(func() {
				ui.Row(c).FillWidth().GapX(8).GapY(2).Wrap().Justify(ui.SpaceBetween).Children(func() {
					ui.Text(c, label).MinWidth(0)
					ui.Text(c, part.Text).Selectable().FontFeatures("tnum")
				})
				switch {
				case part.Error != "":
					kit.Help(c, tr("usage.quotaUnavailable"))
				case part.Percent != nil:
					ui.Progress(c, max(0, min(100, *part.Percent))/100).FillWidth().Label(label)
				}
			})
		}
	})
}
