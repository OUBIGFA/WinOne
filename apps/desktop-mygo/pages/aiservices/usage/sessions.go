package usage

import (
	"sort"
	"strconv"
	"strings"
	"time"

	cldr "github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

// sessionsView is React's UsageSessions: the registered Chat sessions of
// the period, from WinOne's own histories. Their sums stay apart from the
// gateway's, which may count the same call.
type sessionsView struct {
	env   *core.Env
	query sessionQuery
	page  *sessionReport
	err   string
	// loading is a read in flight; generation tells it from those it
	// replaced; key is what the shown or wanted report is of.
	loading    bool
	active     bool
	generation int
	key        sessionKey
	retry      int
	projects   []project
	open       string
	// detail is the open session's page of context history.
	detail   int
	activity string
	shape    string
	chart    *chartView
}

type project struct{ id, name string }

// sessionKey is what a report is of: the query, the period, the refresh
// count and the retries.
type sessionKey struct {
	query           sessionQuery
	period          string
	revision, retry int
}

func newSessionsView(env *core.Env, prefs *viewPrefs) *sessionsView {
	return &sessionsView{env: env, query: sessionQuery{TopBy: "tokens"}, activity: "tokens", shape: "messages", chart: newChartView(prefs)}
}

// deactivate drops the reads in flight; showing again reads.
func (s *sessionsView) deactivate() {
	if s.active {
		s.active = false
		s.generation++
		s.loading = false
	}
}

// sync reads the sessions when the query, the period or the refresh
// count changed, and the projects' names as the view shows.
func (s *sessionsView) sync(period string, revision int) {
	query := s.query
	key := sessionKey{query: query, period: period, revision: revision, retry: s.retry}
	if !s.active {
		s.active = true
		// The projects name the filter and the groups; the statistics
		// stay without them.
		core.Go(s.env, func() (catalog, error) {
			var c catalog
			err := s.env.Host.CallInto("chat.catalog", map[string]any{}, &c)
			return c, err
		}, func(c catalog, err error) {
			if err != nil {
				return
			}
			s.projects = s.projects[:0]
			for _, p := range c.Projects {
				s.projects = append(s.projects, project{p.ID, p.Name})
			}
		})
	} else if key == s.key {
		return
	}
	s.key = key
	s.generation++
	generation := s.generation
	s.loading, s.err = true, ""
	params := struct {
		sessionQuery
		Period string `json:"period"`
	}{query, period}
	core.Go(s.env, func() (sessionReport, error) {
		var report sessionReport
		err := s.env.Host.CallInto("chat.usage", params, &report)
		return report, err
	}, func(report sessionReport, err error) {
		if generation != s.generation {
			return
		}
		s.loading = false
		if err != nil {
			s.err = err.Error()
			return
		}
		s.page = &report
	})
}

// change sets a filter and goes back to the first page.
func (s *sessionsView) change(edit func(q *sessionQuery)) {
	edit(&s.query)
	s.query.Offset = 0
}

// summary is a session's or a group's tokens, messages and minutes.
func summary(x sessionCounts) string {
	return num(x.tokens()) + " Token · " + num(x.Messages) + " " + tr("usage.messages") + " · " +
		toFixed(round(x.ActiveMs/60000), 0) + " " + tr("usage.minutes")
}

func (s *sessionsView) projectName(id string) string {
	for _, p := range s.projects {
		if p.id == id {
			return p.name
		}
	}
	return tr("usage.noProject")
}

func (s *sessionsView) view(c *ui.Context, period string, revision int, providers []Provider, env *core.Env) {
	s.sync(period, revision)
	q := &s.query
	kit.Help(c, tr("usage.sessionsHint"))
	ui.Row(c).FillWidth().GapX(16).GapY(12).Wrap().AlignItems(ui.End).Children(func() {
		field(c, tr("usage.provider"), func() {
			options := []kit.SelectOption{{Value: "", Label: tr("usage.allProviders")}}
			for _, p := range providers {
				options = append(options, kit.SelectOption{Value: p.ID, Label: p.Name})
			}
			if choose(c, &q.Provider, options) {
				s.change(func(*sessionQuery) {})
			}
		})
		field(c, tr("usage.project"), func() {
			options := []kit.SelectOption{{Value: "", Label: tr("usage.allProjects")}}
			for _, p := range s.projects {
				options = append(options, kit.SelectOption{Value: p.id, Label: p.name})
			}
			if choose(c, &q.Project, options) {
				s.change(func(*sessionQuery) {})
			}
		})
		field(c, tr("usage.searchSessions"), func() {
			if kit.TextInput(c.Key("query"), &q.Query).FillWidth().Changed() {
				s.change(func(*sessionQuery) {})
			}
		})
	})
	if s.loading {
		kit.Status(c, tr("usage.loading"))
	}
	if s.err != "" {
		kit.Notice(c, kit.Failure, "", s.err, func() {
			if kit.Button(c, tr("common.retry"), icons.RefreshCw, false).Clicked() {
				s.retry++
			}
		})
	}
	page := s.page
	if page == nil {
		return
	}
	for i, w := range page.Warnings {
		ui.Column(c.Key(i)).FillWidth().Children(func() { kit.Notice(c, kit.Failure, "", w, nil) })
	}
	sum := page.Sum
	tiles(c, []tile{
		{label: tr("usage.sessions"), value: num(sum.Sessions)},
		{label: tr("usage.tokenTotal"), value: num(sum.tokens())},
		{label: tr("usage.output"), value: num(sum.Output)},
		{label: tr("usage.cost"), value: "$" + toFixed(sum.Cost, 6)},
		{label: tr("usage.messages"), value: num(sum.Messages)},
		{label: tr("usage.activeTime"), value: toFixed(round(sum.ActiveMs/60000), 0) + " " + tr("usage.minutes")},
		{label: tr("usage.median"), value: num(page.Median) + " Token"},
		{label: tr("usage.p90"), value: num(page.P90) + " Token"},
	})
	if sum.Unpriced > 0 {
		kit.Help(c, tr("usage.unpriced", "count", sum.Unpriced))
	}
	s.activityView(c, page, period, c.Now())
	s.shapeView(c, page)
	chart := sessionsChart(page, providers)
	s.chart.view(c, chartInput{page: &chart, pick: func(dimension, id string) {
		if dimension == "provider" || dimension == "model" {
			s.change(func(q *sessionQuery) {
				if dimension == "provider" {
					q.Provider = id
				} else {
					q.Model = id
				}
			})
		}
	}})
	ui.Row(c).FillWidth().GapX(24).GapY(24).Wrap().AlignItems(ui.Start).Children(func() {
		for _, kind := range []string{"project", "model", "hour"} {
			groups := page.By[kind]
			ids := make([]string, 0, len(groups))
			for id := range groups {
				ids = append(ids, id)
			}
			sort.SliceStable(ids, func(a, b int) bool {
				ta, tb := groups[ids[a]].tokens(), groups[ids[b]].tokens()
				if ta != tb {
					return ta > tb
				}
				return ids[a] < ids[b]
			})
			s.section(c, kind, tr("usage.group"+kind), func() {
				for _, id := range ids {
					label := id
					switch kind {
					case "project":
						label = s.projectName(id)
					case "hour":
						label = id + ":00"
					}
					if groupRow(c, id, label, "", summary(groups[id]), kind != "hour") && kind != "hour" {
						s.change(func(q *sessionQuery) {
							if kind == "project" {
								q.Project = id
							} else {
								q.Model = id
							}
						})
					}
				}
			})
		}
		for _, kind := range []string{"tools", "skills"} {
			counts := page.Tools
			if kind == "skills" {
				counts = page.Skills
			}
			names := make([]string, 0, len(counts))
			for name := range counts {
				names = append(names, name)
			}
			sort.SliceStable(names, func(a, b int) bool {
				if counts[names[a]] != counts[names[b]] {
					return counts[names[a]] > counts[names[b]]
				}
				return names[a] < names[b]
			})
			s.section(c, kind, tr("usage."+kind), func() {
				if len(names) == 0 {
					kit.Help(c, tr("usage.noneRecorded"))
				}
				for _, name := range names {
					groupRow(c, name, name, "", num(counts[name]), false)
				}
			})
		}
	})
	ui.Column(c).FillWidth().Gap(10).Children(func() {
		ui.Row(c).FillWidth().Children(func() {
			field(c, tr("usage.topSessions"), func() {
				options := []kit.SelectOption{{Value: "tokens", Label: tr("usage.tokenTotal")}, {Value: "cost", Label: tr("usage.cost")},
					{Value: "active", Label: tr("usage.activeTime")}}
				if choose(c, &q.TopBy, options) {
					s.change(func(*sessionQuery) {})
				}
			}).Grow(0).Basis(240)
		})
		s.sessionList(c, "top", page.Top, false, env)
	})
	ui.Column(c).FillWidth().Gap(10).Children(func() {
		kit.Heading(c, tr("usage.latestSessions")).Role(ui.RoleHeading).Level(3)
		if len(page.Rows) == 0 {
			kit.Help(c, tr("usage.noSessions"))
		}
		s.sessionList(c, "latest", page.Rows, true, env)
		pager(c, page.Total, page.Offset, len(page.Rows), s.loading, func(offset int) { s.query.Offset = offset })
	})
}

// section is a titled group of rows in the sessions' grid.
func (s *sessionsView) section(c *ui.Context, key, title string, fn func()) {
	ui.Column(c.Key(key)).Basis(280).Grow(1).Shrink(1).MinWidth(0).Gap(4).Role(ui.RoleGroup).Label(title).Children(func() {
		kit.Strong(c, title).Role(ui.RoleHeading).Level(3).Margin(0, 0, 4, 0)
		ui.Column(c).FillWidth().Role(ui.RoleList).Children(fn)
	})
}

// groupRow is a name and its figures across a line, over a hairline; a
// picking row is a button. It reports a click.
func groupRow(c *ui.Context, key, label, small, figures string, picks bool) bool {
	t, p := c.Theme(), kit.Colors(c)
	var row ui.Element
	if picks {
		row = ui.ButtonBase(c.Key(key))
		if row.Hovered() {
			row.Background(t.Surface)
		}
	} else {
		row = ui.Row(c.Key(key)).Role(ui.RoleListItem)
	}
	row.FillWidth().Padding(12, 4).GapX(16).GapY(4).Wrap().AlignItems(ui.Start).Justify(ui.SpaceBetween).
		BorderWidth(0, 0, 1, 0).BorderColor(p.Hairline).Children(func() {
		ui.Column(c).Basis(160).Grow(1).Shrink(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, label).MinWidth(0)
			if small != "" {
				kit.Help(c, small)
			}
		})
		ui.Text(c, figures).TextColor(t.TextMuted).TextAlign(ui.End).FontFeatures("tnum").Shrink(1).MinWidth(0)
	})
	return picks && row.Clicked()
}

// sessionList lists sessions, each opening its details; latest rows say
// their model, last change and archive, and their read error.
func (s *sessionsView) sessionList(c *ui.Context, key string, sessions []usageSession, latest bool, env *core.Env) {
	ui.Column(c.Key(key)).FillWidth().Role(ui.RoleList).Children(func() {
		for _, x := range sessions {
			ui.Column(c.Key(x.ID)).FillWidth().Gap(8).Children(func() {
				small := ""
				if latest {
					small = x.Model + " · " + dateTime(x.UpdatedAt)
					if x.Archived {
						small += " · " + tr("usage.archived")
					}
				}
				if groupRow(c, "row", x.Title, small, summary(x.sessionCounts), true) {
					if s.open == x.ID {
						s.open = ""
					} else {
						s.open, s.detail = x.ID, 0
					}
				}
				if latest && x.Error != "" {
					kit.Notice(c, kit.Failure, "", x.Error, nil)
				}
				if s.open == x.ID {
					s.details(c, x, env)
				}
			})
		}
	})
}

// details are a session's counts and its context history, fifteen at a
// time, and the way to it in Chat.
func (s *sessionsView) details(c *ui.Context, x usageSession, env *core.Env) {
	p := kit.Colors(c)
	ui.Column(c).FillWidth().Gap(14).Padding(12, 0, 16, 0).Children(func() {
		ui.Row(c).Children(func() {
			if kit.Button(c, tr("usage.openChat"), nil, false).Clicked() {
				openChat(env, x.ID)
			}
		})
		if x.Partial {
			kit.Notice(c, kit.Info, "", tr("usage.partialSession"), nil)
		}
		raw := func(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
		details(c, []tile{
			{label: tr("usage.calls"), value: raw(x.Calls)}, {label: tr("usage.input"), value: raw(x.Input)},
			{label: tr("usage.output"), value: raw(x.Output)}, {label: tr("usage.cacheRead"), value: raw(x.CacheRead)},
			{label: tr("usage.cacheWrite"), value: raw(x.CacheWrite)}, {label: tr("usage.reasoning"), value: raw(x.Reasoning)},
			{label: tr("usage.compactions"), value: raw(x.Compactions)}, {label: tr("usage.errors"), value: raw(x.Errors)},
		})
		ui.Column(c).FillWidth().Gap(8).Role(ui.RoleTable).Label(tr("usage.contextHistory")).Children(func() {
			kit.Strong(c, tr("usage.contextHistory"))
			end := min(len(x.Context), s.detail+pageSize)
			shown := x.Context[min(s.detail, len(x.Context)):end]
			ui.Grid(c).FillWidth().ColumnTracks(ui.Fr(2), ui.Fr(1), ui.Fr(1), ui.Fr(1)).GapX(12).Children(func() {
				header := func(label string) {
					kit.Caption(c, label).Padding(0, 0, 8, 0).BorderWidth(0, 0, 1, 0).BorderColor(p.Hairline).Role(ui.RoleColumnHeader)
				}
				for _, label := range []string{tr("usage.time"), tr("usage.input"), tr("usage.output"), tr("usage.cacheRead")} {
					header(label)
				}
				for i, point := range shown {
					when := dateTime(point.Time)
					if point.Compaction {
						when += " · " + tr("usage.compaction")
					}
					cell := func(k int, text string) {
						ui.Text(c.Key(i*4+k), text).FontFeatures("tnum").MinWidth(0).Padding(8, 0).BorderWidth(0, 0, 1, 0).BorderColor(p.Hairline)
					}
					cell(0, when)
					cell(1, raw(point.Input))
					cell(2, raw(point.Output))
					cell(3, raw(point.CacheRead))
				}
			})
			pager(c, len(x.Context), s.detail, len(shown), false, func(offset int) { s.detail = offset })
		})
	})
}

// openChat shows the session in Chat. The page cannot yet name the
// conversation to the Chat page, so it shows Chat, where the session is
// in the list.
func openChat(env *core.Env, id string) {
	_ = id
	env.Navigate("chat", nil)
}

// sessionsChart is the sessions as a ledger page, for the trend chart:
// a column a day and their groups by provider, model, project and hour.
func sessionsChart(page *sessionReport, providers []Provider) ledgerPage {
	counts := func(x sessionCounts) totals {
		return totals{Calls: x.Calls, Errors: x.Errors, Input: x.Input, Output: x.Output, CacheRead: x.CacheRead,
			CacheWrite: x.CacheWrite, Reasoning: x.Reasoning, Cost: x.Cost, Unpriced: x.Unpriced}
	}
	chart := ledgerPage{Sum: counts(page.Sum), Total: page.Total, Offset: page.Offset, Bucket: "day", By: map[string][]share{}}
	for _, p := range page.Series {
		chart.Series = append(chart.Series, point{Time: p.Time + "T00:00:00", totals: counts(p.sessionCounts)})
	}
	for kind, groups := range page.By {
		ids := make([]string, 0, len(groups))
		for id := range groups {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			name := id
			if kind == "provider" {
				if p := findProvider(providers, id); p != nil && p.Name != "" {
					name = p.Name
				}
			}
			chart.By[kind] = append(chart.By[kind], share{ID: id, Name: name, totals: counts(groups[id])})
		}
	}
	return chart
}

// activityView is React's SessionActivity: the period's days in Magpie's
// calendar, by the metric chosen.
func (s *sessionsView) activityView(c *ui.Context, page *sessionReport, period string, now time.Time) {
	ui.Column(c).FillWidth().Gap(14).Children(func() {
		ui.Row(c).FillWidth().Children(func() {
			field(c, tr("usage.activity"), func() {
				options := make([]kit.SelectOption, 0, 6)
				for _, m := range []string{"tokens", "output", "messages", "sessions", "cost", "active"} {
					key := map[string]string{"tokens": "usage.tokenTotal", "active": "usage.activeTime"}[m]
					if key == "" {
						key = "usage." + m
					}
					options = append(options, kit.SelectOption{Value: m, Label: tr(key)})
				}
				choose(c, &s.activity, options)
			}).Grow(0).Basis(240)
		})
		last := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
		first := last
		switch {
		case period == "all" && len(page.Series) > 0:
			if day, ok := localDay(page.Series[0].Time); ok {
				first = day
			}
		case period == "7d":
			first = last.AddDate(0, 0, -6)
		case period == "30d":
			first = last.AddDate(0, 0, -29)
		case strings.Contains(period, ".."):
			// A chosen range shows its own days.
			from, to, _ := strings.Cut(period, "..")
			if a, ok := localDay(from); ok {
				first = a
			}
			if b, ok := localDay(to); ok {
				last = b
			}
		}
		at := map[string]sessionCounts{}
		for _, p := range page.Series {
			at[p.Time] = p.sessionCounts
		}
		value := func(x sessionCounts) float64 {
			switch s.activity {
			case "tokens":
				return x.tokens()
			case "active":
				return x.ActiveMs / 60000
			case "cost":
				return x.Cost
			case "output":
				return x.Output
			case "messages":
				return x.Messages
			}
			return x.Sessions
		}
		var days []calDay
		for d := first; !d.After(last); d = d.AddDate(0, 0, 1) {
			days = append(days, calDay{day: d, value: value(at[isoDay(d)])})
		}
		if len(days) == 0 {
			return
		}
		ui.Column(c).FillWidth().Gap(12).Padding(8, 0).Children(func() {
			sessCalendar(c, days, func(d calDay) string {
				return formatDate(d.day, "yMd") + " · " + cldr.FormatNumber(lang(), d.value, cldr.NumberOptions{MaxFractionDigits: 6})
			})
			sessLegend(c, tr("usage.less"), tr("usage.more"))
		})
	})
}

// shapeView is React's SessionShape: Magpie's session-shape bins, each a
// column as tall as its count among the others.
func (s *sessionsView) shapeView(c *ui.Context, page *sessionReport) {
	t, p := c.Theme(), kit.Colors(c)
	ui.Column(c).FillWidth().Gap(14).Children(func() {
		ui.Row(c).FillWidth().Children(func() {
			field(c, tr("usage.shape"), func() {
				options := []kit.SelectOption{{Value: "messages", Label: tr("usage.messages")}, {Value: "minutes", Label: tr("usage.minutes")},
					{Value: "autonomy", Label: tr("usage.autonomy")}}
				choose(c, &s.shape, options)
			}).Grow(0).Basis(240)
		})
		shape, ok := page.Shape[s.shape]
		if ok && len(shape.Counts) > 0 {
			peak := 1.0
			for _, n := range shape.Counts {
				peak = max(peak, n)
			}
			const plot = 124
			ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Stretch).Children(func() {
				for i, count := range shape.Counts {
					lo := shape.Edges[min(i, len(shape.Edges)-1)]
					label := ""
					switch {
					case s.shape == "autonomy" && lo == 0:
						label = "<1"
					case i+1 >= len(shape.Edges):
						label = toString(lo) + "+"
					case shape.Edges[i+1]-1 == lo:
						label = toString(lo)
					default:
						label = toString(lo) + "–" + toString(shape.Edges[i+1]-1)
					}
					col := ui.Column(c.Key(i)).Grow(1).Basis(0).MinWidth(0).Gap(4).Tooltip(label + " · " + toString(count) + " " + tr("usage.sessions"))
					hovered := col.Hovered()
					col.Children(func() {
						ui.Column(c).FillWidth().Height(plot).Gap(4).Justify(ui.End).BorderWidth(0, 0, 1, 0).BorderColor(p.Hairline).Children(func() {
							ui.Text(c, toString(count)).FontWeight(600).FontSize(kit.HelpSize(c)).FontFeatures("tnum").TextAlign(ui.Center)
							if count == 0 {
								ui.Box(c).Height(2).WidthPercent(72).AlignSelf(ui.Center).Radius(1).Background(p.Hairline)
								return
							}
							color := t.Accent.Alpha(0.55)
							if hovered {
								color = t.Accent
							}
							height := max(4, float32(plot-20)*float32(count/peak))
							ui.Box(c).Height(height).WidthPercent(72).AlignSelf(ui.Center).Radius(5, 5, 0, 0).Background(color)
						})
						kit.Help(c, label).TextAlign(ui.Center).NoWrap().FontFeatures("tnum")
					})
				}
			})
		}
		kit.Help(c, tr("usage.shapeHint"))
	})
}
