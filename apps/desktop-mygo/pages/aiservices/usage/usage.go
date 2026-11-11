// Package usage is the Usage statistics tab of the AI services page: the
// ledger of AI requests (overview, request details, the request heatmap,
// allowances and balances), the context analysis, the registered Chat
// sessions and the model aliases, as React's AIUsage
// (apps/desktop/frontend/src/features/ai-usage.tsx and its parts) showed
// them. Magpie keeps the records, prices and sums; this package reads them
// from the Host and draws Magpie's charts with MyGo (see ledger.go and the
// other ports, each under its upstream MIT notice).
//
// The page owns the tab: it builds View inside its scrolling column and
// calls Close when it closes. Model aliases are part of the page's AI
// draft, which Options.Change writes; the aliases' unfinished input is
// reported as "ai-usage-aliases".
package usage

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/widgets"
)

// Provider is the public part of a saved provider (an element of the
// providers capture.ai.configuration.get returns).
type Provider struct {
	widgets.AIProvider                 // name, icon, models, model names and metadata
	AuthMode           string          `json:"authMode"`
	Agent              string          `json:"agent,omitempty"`
	AgentBackend       string          `json:"agentBackend,omitempty"`
	HasKey             bool            `json:"hasKey"`
	BaseURL            string          `json:"baseUrl,omitempty"`
	Keys               json.RawMessage `json:"keys,omitempty"`
}

// Preferences are the parts of the AI draft's magpieSettings the
// statistics read and write.
type Preferences struct {
	Order, HiddenCards, HiddenAgents []string
	HideAccounts                     bool
	QuotaLeft                        bool
	Aliases                          map[string]string
}

// Options are what the page gives the tab each frame.
type Options struct {
	// Active is whether the tab shows. While false, View draws nothing,
	// reads nothing and counts no refresh, and a drill-down into the
	// requests gives back the filters it replaced, as React's inactive
	// tab did. A frame that does not build View counts as inactive too.
	Active bool
	// Providers are the saved providers.
	Providers []Provider
	// Preferences are nil until the configuration is read.
	Preferences *Preferences
	// Revision is the saved configuration's revision: the ledger is read
	// again when it changes.
	Revision int64
	// Disabled locks the page's draft, as while it saves.
	Disabled bool
	// Change writes changed preferences into the page's draft, which is
	// then unsaved. nil hides the model aliases.
	Change func(Preferences)
}

// The views of the tab, in their order.
const (
	viewOverview = iota
	viewRequests
	viewContext
	viewSessions
	viewAliases
)

// Usage is the statistics tab's state. Make it once per page with New.
type Usage struct {
	env    *core.Env
	opts   Options
	closed bool

	view   int
	filter filter
	// origin is the filter a chart's drill-down into the requests
	// replaced; leaving the requests gives it back.
	origin *filter
	// revision counts refreshes: the button, a retry and the timer.
	revision int
	every    string
	timer    timer

	ledger           ledgerRead
	export           exportState
	period           periodPicker
	chart            *chartView
	folded           map[string]bool // the breakdowns folded to their top seven
	agentDisplayOpen bool
	rows             requestList
	heat             heatView
	quota            *allowances
	sessions         *sessionsView
	context          *contextView
	aliases          aliasEditor
	prefs            *viewPrefs
}

// New makes the statistics tab of a page.
func New(env *core.Env) *Usage {
	u := &Usage{env: env, filter: initialFilter, every: "0", folded: map[string]bool{}}
	u.prefs = loadViewPrefs(env.Directory)
	u.chart = newChartView(u.prefs)
	u.quota = newAllowances(env)
	u.sessions = newSessionsView(env, u.prefs)
	u.context = newContextView(env)
	u.period.sync(u.filter.Period, time.Now())
	return u
}

// Close forgets the aliases' unfinished input and stops the reads in
// flight from changing anything, as the page closes.
func (u *Usage) Close() {
	u.closed = true
	u.env.Activity.Report(aliasSource, false, false, nil)
}

// ledgerRead is the ledger read for the overview and the requests.
type ledgerRead struct {
	page    *ledgerPage
	loading bool
	err     string
	// generation tells the current read from those it replaced, whose
	// late answers must not show.
	generation int
	// reading is whether a read is wanted, for key; a change of either
	// starts another.
	reading bool
	key     ledgerKey
}

type ledgerKey struct {
	filter   filter
	revision int
	settings int64
}

// timer is the auto refresh: a period, counted while the tab shows a view
// that refreshes.
type timer struct {
	next time.Time
	key  string
}

// View builds the tab inside the page's column.
func (u *Usage) View(c *ui.Context, o Options) {
	u.opts = o
	if o.Change == nil && u.view == viewAliases {
		u.view = viewOverview
	}
	root := ui.Column(c.Key(u)).FillWidth().Gap(20)
	// The tab's elements keep this mark while they are built in every
	// frame; a frame without them (the page showed another tab) drops it.
	shown := ui.Local(root, "shown", func() bool { return false })
	active := o.Active && !u.closed
	if !*shown || !active {
		u.deactivate()
	}
	*shown = active
	u.reportAliases()
	if !active {
		return
	}
	u.sync(c)
	root.Children(func() {
		kit.Help(c, i18n.T("usage.hint"))
		u.tabs(c)
		if u.view != viewAliases {
			u.controls(c)
		}
		switch u.view {
		case viewOverview, viewRequests:
			u.ledgerView(c)
		case viewContext:
			u.context.view(c, u.filter.Period, u.revision, o.Providers, u.env)
		case viewSessions:
			u.sessions.view(c, u.filter.Period, u.revision, o.Providers, u.env)
		case viewAliases:
			u.aliasesView(c)
		}
	})
}

// deactivate is React's inactive tab: no reads, no timer, and a
// drill-down gives back the filters it replaced.
func (u *Usage) deactivate() {
	if u.origin != nil {
		u.filter, u.origin = *u.origin, nil
	}
	u.cancelLedger()
	u.timer = timer{}
	u.quota.deactivate()
	u.sessions.deactivate()
	u.context.deactivate()
}

// sync starts the reads and counts the refreshes the shown view needs.
func (u *Usage) sync(c *ui.Context) {
	ledger := u.view == viewOverview || u.view == viewRequests
	if ledger {
		u.readLedger()
	} else {
		u.cancelLedger()
	}
	if u.view != viewOverview {
		u.quota.deactivate()
	}
	if u.view != viewSessions {
		u.sessions.deactivate()
	}
	if u.view != viewContext {
		u.context.deactivate()
	}
	// The timer runs while a view that refreshes shows; it starts over
	// when the period or the view changes, as React's interval did. MyGo
	// draws no frames while the window is out of sight, so a hidden
	// window counts no refreshes, as React skipped hidden documents.
	if u.view == viewAliases || u.every == "0" {
		u.timer = timer{}
		return
	}
	seconds := map[string]time.Duration{"15": 15, "30": 30, "60": 60, "300": 300, "900": 900, "1800": 1800}[u.every] * time.Second
	now := c.Now()
	key := u.every + "/" + strconv.Itoa(u.view)
	if u.timer.key != key || u.timer.next.IsZero() {
		u.timer = timer{next: now.Add(seconds), key: key}
	}
	if !now.Before(u.timer.next) {
		u.revision++
		u.timer.next = now.Add(seconds)
		if ledger {
			u.readLedger()
		}
	}
	c.After(u.timer.next.Sub(now))
}

// readLedger reads the ledger when its query changed since the last read.
func (u *Usage) readLedger() {
	key := ledgerKey{filter: u.filter, revision: u.revision, settings: u.opts.Revision}
	r := &u.ledger
	if r.reading && r.key == key {
		return
	}
	r.reading, r.key = true, key
	r.generation++
	generation := r.generation
	r.loading, r.err = true, ""
	query := u.filter
	core.Go(u.env, func() (ledgerPage, error) {
		var page ledgerPage
		err := u.env.Host.CallCompletingInto("capture.ai.usage.get", query, &page)
		return page, err
	}, func(page ledgerPage, err error) {
		if u.closed || generation != r.generation {
			return
		}
		r.loading = false
		if err != nil {
			r.err = err.Error()
			return
		}
		r.page = &page
	})
}

// cancelLedger drops the read in flight: its answer will not show.
func (u *Usage) cancelLedger() {
	r := &u.ledger
	if r.reading {
		r.reading = false
		r.generation++
		r.loading = false
	}
}

// change sets filters and goes back to the first page, as React's change.
func (u *Usage) change(edit func(f *filter)) {
	edit(&u.filter)
	u.filter.Offset = 0
}

// pick is a chart's or a ranking's drill-down: only the requests of the
// group, in the requests view. Leaving the requests gives back the
// filters it replaced, explicit edits made before included.
func (u *Usage) pick(dimension, id string) {
	if u.origin == nil {
		before := u.filter
		u.origin = &before
	}
	u.change(func(f *filter) {
		switch dimension {
		case "modelAt":
			provider, model, _ := strings.Cut(id, "/")
			f.Provider, f.Model = provider, model
		case "agent":
			f.Source = id
		case "provider":
			f.Provider = id
		case "model":
			f.Model = id
		case "account":
			f.Account = id
		case "callerKey":
			f.CallerKey = id
		case "purpose":
			f.Purpose = id
		}
	})
	u.view = viewRequests
}

// pickDay filters the requests by a day of the chart, or clears it.
func (u *Usage) pickDay(day string) {
	u.change(func(f *filter) {
		if f.Day == day {
			f.Day = ""
		} else {
			f.Day = day
		}
	})
}

// tabs are the views; switching goes through the page's unsaved-change
// guard, as every tab of the settings does.
func (u *Usage) tabs(c *ui.Context) {
	labels := []string{i18n.T("usage.overview"), i18n.T("usage.requests"), i18n.T("usage.context"), i18n.T("usage.sessions")}
	if u.opts.Change != nil {
		labels = append(labels, i18n.T("usage.aliases"))
	}
	ui.Column(c).FillWidth().Role(ui.RoleGroup).Label(i18n.T("usage.views")).Children(func() {
		kit.SubTabs(c, &u.view, labels, func(apply func()) {
			u.env.RequestChange(func() {
				before := u.view
				apply()
				u.leave(before)
			})
		})
	})
}

// leave is what switching away from a view does: the requests give back
// the filters a drill-down replaced; the aliases forget their form, as
// React unmounted it.
func (u *Usage) leave(before int) {
	if before == u.view {
		return
	}
	if u.view != viewRequests && u.origin != nil {
		u.filter, u.origin = *u.origin, nil
	}
	if before == viewAliases {
		u.aliases.reset()
		u.reportAliases()
	}
}

// controls are the period, the automatic refresh and the refresh button,
// shared by every view but the aliases.
func (u *Usage) controls(c *ui.Context) {
	ui.Column(c).FillWidth().Gap(12).Children(func() {
		ui.Row(c).FillWidth().GapX(16).GapY(12).Wrap().AlignItems(ui.End).Children(func() {
			u.period.view(c, u.filter.Period, func(period string) {
				u.change(func(f *filter) { f.Period, f.Day = period, "" })
			})
			field(c, i18n.T("usage.autoRefresh"), func() {
				options := []kit.SelectOption{{Value: "0", Label: i18n.T("usage.manual")}}
				for _, s := range []string{"15", "30", "60", "300", "900", "1800"} {
					label := i18n.T("usage.everySeconds", "seconds", s)
					if seconds, _ := strconv.Atoi(s); seconds >= 300 {
						label = i18n.T("usage.everyMinutes", "minutes", seconds/60)
					}
					options = append(options, kit.SelectOption{Value: s, Label: label})
				}
				choose(c, &u.every, options)
			})
			loading := u.ledger.loading
			if kit.Button(c, i18n.T("usage.refresh"), icons.RefreshCw, loading).Disabled(loading).Shrink(0).Clicked() {
				u.revision++
			}
		})
		u.period.dates(c, func(period string) {
			u.change(func(f *filter) { f.Period, f.Day = period, "" })
		})
	})
}

// ledgerView is the overview and the requests: filters, totals, the
// chart, and the view's own part.
func (u *Usage) ledgerView(c *ui.Context) {
	f := &u.filter
	providers := u.opts.Providers
	ui.Row(c).FillWidth().GapX(16).GapY(12).Wrap().AlignItems(ui.End).Children(func() {
		field(c, i18n.T("usage.provider"), func() {
			items := []pickItem{{value: "", label: i18n.T("usage.allProviders")}}
			for i := range providers {
				p := &providers[i]
				items = append(items, pickItem{value: p.ID, label: p.Name, icon: providerMark(p)})
			}
			if pickSelect(c, &f.Provider, items, u.env) {
				u.change(func(*filter) {})
			}
		})
		field(c, i18n.T("usage.source"), func() {
			choices := []kit.SelectOption{{Value: "", Label: i18n.T("usage.allSources")}, {Value: "winone", Label: i18n.T("usage.winone")}, {Value: "gateway", Label: i18n.T("usage.gateway")}}
			if u.ledger.page != nil {
				for _, client := range u.ledger.page.Agents {
					if client != "winone" && client != "gateway" {
						choices = append(choices, kit.SelectOption{Value: client, Label: client})
					}
				}
			}
			if choose(c, &f.Source, choices) {
				u.change(func(*filter) {})
			}
		})
		field(c, i18n.T("usage.search"), func() {
			if kit.TextInput(c.Key("query"), &f.Query).FillWidth().Changed() {
				u.change(func(*filter) {})
			}
		})
	})
	ui.Row(c).FillWidth().Gap(12).Wrap().AlignItems(ui.Center).Justify(ui.SpaceBetween).Children(func() {
		if ui.Checkbox(c, &f.Failed, i18n.T("usage.failedOnly")).Changed() {
			u.change(func(*filter) {})
		}
		if f.narrowed() && kit.Button(c, i18n.T("usage.clearFilters"), nil, false).Clicked() {
			cleared := initialFilter
			cleared.Period = f.Period
			u.filter = cleared
		}
	})
	if f.Day != "" || f.Model != "" {
		kit.Help(c, strings.TrimSpace(f.Day+" "+modelName(providers, f.Model)))
	}
	r := &u.ledger
	if r.loading {
		kit.Status(c, i18n.T("usage.loading"))
	}
	if u.export.saved != "" {
		kit.Notice(c, kit.Info, "", u.export.saved, nil)
	}
	if r.err != "" {
		kit.Notice(c, kit.Failure, "", r.err, func() {
			if kit.Button(c, i18n.T("common.retry"), icons.RefreshCw, false).Clicked() {
				u.revision++
			}
		})
	}
	if r.page == nil {
		return
	}
	page := u.named(r.page)
	if page.PriceWarning {
		kit.Notice(c, kit.Info, "", i18n.T("usage.priceWarning"), nil)
	}
	usageTotals(c, page.Sum)
	u.chart.view(c, chartInput{page: page, filter: f, pick: u.pick, day: u.pickDay})
	if u.view == viewOverview {
		u.breakdowns(c, page)
		u.quota.view(c, providers, u.opts.Preferences, u.opts.Change, u.revision)
		if len(page.Rows) == 0 {
			kit.Help(c, i18n.T("usage.empty"))
		}
		return
	}
	u.requestsView(c, page)
}

// named is the page with its groups named as the page shows them:
// providers and accounts by the saved providers, sources in words, models
// by their display names, gateway keys by theirs. It is made for every
// frame, as React named them as it drew, so a change of language or of
// the providers shows at once; the page as read stays as it came.
func (u *Usage) named(read *ledgerPage) *ledgerPage {
	page := *read
	providers := u.opts.Providers
	label := func(kind, id string) string {
		switch kind {
		case "provider", "account":
			if p := findProvider(providers, id); p != nil && p.Name != "" {
				return p.Name
			}
			return id
		case "agent":
			if id == "gateway" {
				return i18n.T("usage.gateway")
			}
			return i18n.T("usage.winone")
		case "modelAt":
			provider, model, ok := strings.Cut(id, "/")
			if !ok {
				return id
			}
			name := model
			if p := findProvider(providers, provider); p != nil {
				if text := p.ModelNames[model]; text != "" {
					name = text
				}
				if p.Name != "" {
					provider = p.Name
				}
			}
			return name + " · " + provider
		case "callerKey":
			if id == "none" {
				return i18n.T("usage.noCallerKey")
			}
			for _, k := range page.CallerKeys {
				if k.ID == id && k.CallerKeyName != "" {
					return k.CallerKeyName
				}
			}
			return id
		}
		return ""
	}
	rename := func(by map[string][]share) map[string][]share {
		if by == nil {
			return nil
		}
		out := make(map[string][]share, len(by))
		for kind, groups := range by {
			list := append([]share(nil), groups...)
			for i := range list {
				name := label(kind, list[i].ID)
				if name == "" {
					// A model, or a group of another dimension: its display
					// name, the name the ledger gave it, else its identifier.
					name = list[i].Name
					if shown, ok := displayModel(providers, list[i].ID); ok {
						name = shown
					}
				}
				if name == "" {
					name = list[i].ID
				}
				list[i].Name = name
			}
			out[kind] = list
		}
		return out
	}
	page.By, page.ChartBy = rename(read.By), rename(read.ChartBy)
	return &page
}

// findProvider is the saved provider of id, nil when it was removed.
func findProvider(providers []Provider, id string) *Provider {
	for i := range providers {
		if providers[i].ID == id {
			return &providers[i]
		}
	}
	return nil
}

// displayModel is a model's display name among the providers' enabled
// models, as React's modelNames map read it: the last provider that
// enables it names it; false when none does.
func displayModel(providers []Provider, model string) (string, bool) {
	name, found := "", false
	for _, p := range providers {
		for _, m := range p.Models {
			if m == model {
				name, found = p.ModelNames[m], true
				if name == "" {
					name = m
				}
			}
		}
	}
	return name, found
}

// modelName is a model's display name, or its identifier.
func modelName(providers []Provider, model string) string {
	if name, ok := displayModel(providers, model); ok {
		return name
	}
	return model
}

// providerMark is a provider's mark for a picker: its own picture, else
// its icon.
func providerMark(p *Provider) string {
	if p.Avatar != "" {
		return p.Avatar
	}
	return p.Icon
}
