package aiservices

import (
	"encoding/json"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/pages/aiservices/usage"
)

type traceRoute struct {
	ID              int64          `json:"id"`
	Seq             int64          `json:"seq"`
	Time            string         `json:"time"`
	Agent           string         `json:"agent"`
	Session         string         `json:"session"`
	ParentSession   string         `json:"parentSession"`
	ParentMatched   bool           `json:"parentMatched"`
	NativeSession   string         `json:"native_session"`
	Conv            string         `json:"conv"`
	Folder          string         `json:"folder"`
	Model           string         `json:"model"`
	Served          string         `json:"served"`
	Provider        string         `json:"provider"`
	CallerKeyName   string         `json:"callerKeyName"`
	CallerKey       string         `json:"callerKey"`
	Done            bool           `json:"done"`
	Canceled        bool           `json:"canceled"`
	Status          int            `json:"status"`
	Error           string         `json:"error"`
	Fail            string         `json:"fail"`
	Millis          int64          `json:"ms"`
	TTFT            int64          `json:"ttft"`
	Tokens          int64          `json:"tokens"`
	Output          int64          `json:"out"`
	Bytes           int64          `json:"bytes"`
	Archive         string         `json:"archive"`
	Held            string         `json:"held"`
	Kind            string         `json:"kind"`
	SessionTitle    string         `json:"sessionTitle"`
	Reasoning       int64          `json:"reasoning"`
	FirstText       int64          `json:"firstText"`
	FirstReasoning  int64          `json:"firstReasoning"`
	ReasoningHidden bool           `json:"reasoningHidden"`
	Flow            int64          `json:"flow"`
	Phase           string         `json:"phase"`
	Cost            float64        `json:"cost"`
	Priced          bool           `json:"priced"`
	Unpriced        int            `json:"unpriced"`
	Usage           []traceUsage   `json:"usage"`
	Tries           []traceAttempt `json:"tries"`
	Masked          *struct {
		Key string `json:"key"`
	} `json:"masked"`
	Raw json.RawMessage `json:"-"`
}

func (r *traceRoute) UnmarshalJSON(data []byte) error {
	type plain traceRoute
	if err := json.Unmarshal(data, (*plain)(r)); err != nil {
		return err
	}
	r.Raw = slices.Clone(data)
	return nil
}

type nativeTrace struct {
	prefsPath, prefsError                               string
	accountsOpen, quotasOpen                            bool
	quotas                                              *usage.Allowances
	tallies                                             map[string]routeTally
	nextDays                                            time.Time
	api, detailAPI, titleAPI                            nativeAPI
	routeAPI, daysAPI                                   nativeAPI
	replayAPI                                           nativeAPI
	replay                                              *routeReplay
	replayRows                                          []traceRoute
	replayRead                                          int
	metrics                                             map[string]bool
	groupBy                                             string
	purposes                                            map[string]bool
	filtersOpen, metricsOpen                            bool
	seq                                                 int64
	titles                                              map[string]string
	nextTitles                                          time.Time
	routes                                              []traceRoute
	days                                                []traceDay
	daysLoaded, cut                                     bool
	day, query, provider, model, status, client, caller string
	loaded                                              bool
	paused                                              bool
	byKey                                               bool
	next                                                time.Time
	folded                                              map[string]bool
	selected, focused                                   *traceRoute
	detail                                              json.RawMessage
	detailKind                                          string
	archive                                             *usage.Archive
}

type traceDay struct {
	Day      string `json:"day"`
	Requests int    `json:"requests"`
}

func (t *nativeTrace) loadDays(p *page) {
	t.daysAPI.request(p, "GET", "/api/gateway/history", nil, func(raw json.RawMessage) {
		var got struct {
			Days    []traceDay            `json:"days"`
			Tallies map[string]routeTally `json:"tallies"`
		}
		if nativeDecode(&t.daysAPI, raw, &got) {
			t.days, t.daysLoaded = got.Days, true
			t.tallies = got.Tallies
			t.nextDays = time.Now().Add(30 * time.Second)
		}
	})
}

func (t *nativeTrace) load(p *page) {
	endpoint := "/api/gateway/trace"
	delta := t.day == "" && t.loaded && t.seq > 0
	if t.day != "" {
		endpoint = "/api/gateway/history?day=" + url.QueryEscape(t.day)
	} else if delta {
		endpoint += "?wait=1&after=" + strconv.FormatInt(t.seq, 10)
	}
	t.api.request(p, "GET", endpoint, nil, func(raw json.RawMessage) {
		var got struct {
			Routes []traceRoute `json:"routes"`
			Days   []traceDay   `json:"days"`
			Cut    bool         `json:"cut"`
			Seq    int64        `json:"seq"`
			IDs    []int64      `json:"ids"`
		}
		if nativeDecode(&t.api, raw, &got) {
			if delta {
				merged := map[int64]traceRoute{}
				for _, row := range t.routes {
					if slices.Contains(got.IDs, row.ID) {
						merged[row.ID] = row
					}
				}
				for _, row := range got.Routes {
					merged[row.ID] = row
				}
				t.routes = nil
				for _, row := range merged {
					t.routes = append(t.routes, row)
				}
				slices.SortFunc(t.routes, func(a, b traceRoute) int {
					if a.ID < b.ID {
						return -1
					}
					if a.ID > b.ID {
						return 1
					}
					return 0
				})
			} else {
				t.routes = got.Routes
			}
			t.seq = got.Seq
			t.next = time.Now().Add(100 * time.Millisecond)
			if t.selected != nil && t.day == "" && t.replay == nil {
				for _, row := range t.routes {
					if row.ID == t.selected.ID {
						copy := row
						t.selected = &copy
						break
					}
				}
			}
			t.cut = got.Cut
			if time.Now().After(t.nextTitles) && !t.titleAPI.busy {
				t.nextTitles = time.Now().Add(30 * time.Second)
				t.loadTitles(p, slices.Clone(t.routes), false)
			}
			t.loaded = true
			if got.Days != nil {
				t.days = got.Days
				t.daysLoaded = true
			}
		}
	})
}
func (t *nativeTrace) View(c *ui.Context, p *page) {
	if t.folded == nil {
		t.folded = map[string]bool{}
	}
	if (!t.daysLoaded || t.day == "" && !t.paused && c.Now().After(t.nextDays)) && !t.daysAPI.busy && t.daysAPI.err == "" {
		t.loadDays(p)
	}
	now := c.Now()
	if !t.api.busy && p.extensions.state != nil && (!t.loaded || !t.paused && t.day == "" && now.After(t.next)) {
		t.next = now.Add(2 * time.Second)
		t.load(p)
	}
	if !t.paused && t.day == "" && !t.api.busy {
		c.After(max(time.Millisecond, t.next.Sub(now)))
	}
	rows := t.replayView(c, p)
	filtered := t.filterRoutes(rows)
	t.overviewView(c, p, filtered)
	ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
		ui.TextInput(c, &t.query).Grow(1).MinWidth(180).Label(nativeT("searchRequests")).Placeholder(nativeT("searchRequests"))
		label := nativeT("pauseRefresh")
		if t.paused {
			label = nativeT("resumeRefresh")
		}
		if kit.Button(c, label, nil, false).Clicked() {
			t.paused = !t.paused
			if t.paused {
				t.api.close()
			}
		}
		if kit.IconButton(c, icons.RefreshCw, i18n.T("common.refresh")).Clicked() {
			t.refresh(p)
		}
	})
	ui.Collapsible(c, i18n.T("aiRouting.filters"), &t.filtersOpen, func() {
		ui.Row(c).FillWidth().Gap(12).Wrap().Children(func() {
			ui.Column(c).Basis(180).Grow(1).Children(func() {
				choices := []kit.SelectOption{{Value: "", Label: nativeT("today")}}
				for _, d := range t.days {
					choices = append(choices, kit.SelectOption{Value: d.Day, Label: d.Day + " · " + kit.Itoa(d.Requests)})
				}
				if nativeSelect(c, nativeT("day"), &t.day, choices, false) {
					t.stopReplay()
					t.refresh(p)
				}
			})
			if kit.Button(c, nativeT("loadDay"), icons.Calendar, false).Clicked() {
				t.refresh(p)
			}
			if kit.GhostButton(c, nativeT("today"), nil, false).Clicked() {
				t.day = ""
				t.stopReplay()
				t.refresh(p)
			}
		})
		ui.Row(c).FillWidth().Gap(12).Wrap().Children(func() {
			for _, field := range []struct {
				key   string
				value *string
			}{{"provider", &t.provider}, {"model", &t.model}, {"client", &t.client}, {"callerKey", &t.caller}} {
				ui.Column(c.Key(field.key)).Basis(160).Grow(1).Children(func() { nativeText(c, nativeT(field.key), field.value, false) })
			}
		})
		t.purposeView(c)
		choiceRow(c, nativeT("status"), "", &t.status, options("", nativeT("all"), "running", nativeT("running"), "success", nativeT("succeeded"), "failed", nativeT("failed"), "canceled", nativeT("canceled")), false)
	})
	if nativeSelect(c, i18n.T("aiRouting.groupBy"), &t.groupBy, options("", i18n.T("aiRouting.byRequest"), "session", i18n.T("aiRouting.bySession"), "key", nativeT("groupByKey")), false) {
		t.saveViewPrefs()
	}
	if t.prefsError != "" {
		kit.Notice(c, kit.Caution, "", t.prefsError, nil)
	}
	t.metricsView(c)
	ui.Row(c).Gap(8).Wrap().Children(func() {
		if kit.GhostButton(c, i18n.T("aiRouting.replayAll"), nil, false).Disabled(t.replayAPI.busy || len(filtered) == 0).Clicked() {
			t.startReplay(p, filtered)
		}
		if kit.GhostButton(c, nativeT("expandAll"), icons.ChevronsDown, false).Clicked() {
			clear(t.folded)
		}
		if kit.GhostButton(c, nativeT("collapseAll"), icons.ChevronsUp, false).Clicked() {
			for _, r := range t.routes {
				t.folded[t.groupKey(r)] = true
			}
		}
	})
	t.api.notice(c, func() { t.load(p) })
	t.daysAPI.notice(c, func() { t.loadDays(p) })
	t.titleAPI.notice(c, func() { t.loadTitles(p, slices.Clone(t.routes), false) })
	if t.cut {
		kit.Notice(c, kit.Info, "", i18n.T("aiRouting.historyCut"), nil)
	}
	if !t.loaded && t.api.busy {
		kit.Status(c, i18n.T("capture.loading"))
	}
	groups := map[string][]traceRoute{}
	var order []string
	mask := p.routing.accountMasker()
	for _, r := range filtered {
		key := t.groupKey(r)
		if _, ok := groups[key]; !ok {
			order = append(order, key)
		}
		groups[key] = append(groups[key], r)
	}
	for _, key := range order {
		open := !t.folded[key]
		ui.Collapsible(c.Key(key), mask(t.groupLabel(groups[key][0]))+" · "+kit.Itoa(len(groups[key])), &open, func() {
			if t.groupBy != "" || t.byKey {
				kit.Help(c, traceTotals(groups[key]))
				if kit.GhostButton(c, i18n.T("aiRouting.replayGroup"), nil, false).Disabled(t.replayAPI.busy).Clicked() {
					t.startReplay(p, groups[key])
				}
			}
			for _, r := range groups[key] {
				label := p.routing.memberLabel(r.Model)
				if r.Served != "" && r.Served != r.Model {
					label += " → " + p.routing.memberLabel(r.Served)
				}
				button := ui.ButtonBase(c.Key(r.ID)).FillWidth().Padding(10, 8).BorderWidth(0, 0, 1, 0).BorderColor(kit.Colors(c).Hairline).Label(nativeT("request") + " " + strconv.FormatInt(r.ID, 10))
				button.Children(func() {
					ui.Column(c).FillWidth().Gap(4).Children(func() {
						kit.Strong(c, label).Selectable()
						kit.Help(c, r.Agent+" · "+r.Time+" · "+nativeT(traceStatus(r)))
						kit.Help(c, t.metricText(r))
						if r.Error != "" {
							kit.Help(c, mask(r.Error)).Selectable()
						}
					})
				})
				if button.Clicked() {
					t.selectRoute(p, r)
				}
			}
		})
		t.folded[key] = !open
	}
	if t.loaded && len(order) == 0 {
		kit.Empty(c, icons.Waypoints, nativeT("noRequests"), "", nil)
	}
	t.accountsView(c, p, filtered)
	t.detailView(c, p)
}

func (t *nativeTrace) refresh(p *page) {
	t.stopReplay()
	t.focused = nil
	t.api.close()
	t.titleAPI.close()
	t.nextTitles = time.Time{}
	t.seq = 0
	t.loaded = false
	t.routeAPI.close()
	t.selected = nil
	t.detailAPI.close()
	t.load(p)
}

// History rows are summaries. Read the original route before presenting its
// candidates, decisions and timing; summary absence is not empty evidence.
func (t *nativeTrace) selectRoute(p *page, r traceRoute) {
	t.routeAPI.close()
	t.detailAPI.close()
	t.selected, t.detail, t.detailKind = &r, nil, "route"
	t.focused = &r
	t.archive = nil
	if t.replay != nil {
		return
	}
	q := url.Values{"id": {strconv.FormatInt(r.ID, 10)}, "day": {strings.Split(r.Time, "T")[0]}}
	t.routeAPI.request(p, "GET", "/api/gateway/route?"+q.Encode(), nil, func(raw json.RawMessage) {
		var full traceRoute
		if nativeDecode(&t.routeAPI, raw, &full) {
			if full.ID != r.ID {
				t.routeAPI.err = nativeT("invalidResponse")
				return
			}
			t.selected = &full
			t.focused = &full
		}
	})
}
func archiveEndpoint(ref string) string {
	day, id, _ := strings.Cut(ref, "/")
	return "/api/archive?" + url.Values{"date": {day}, "id": {id}}.Encode()
}

func traceStatus(r traceRoute) string {
	if r.Canceled {
		return "canceled"
	}
	if !r.Done {
		return "running"
	}
	if r.Error != "" || r.Fail != "" || r.Status >= 400 {
		return "failed"
	}
	return "success"
}
func (t *nativeTrace) groupLabel(r traceRoute) string {
	if (t.byKey || t.groupBy == "key") && r.CallerKeyName != "" {
		return r.CallerKeyName
	}
	if !t.byKey && t.groupBy == "session" {
		if r.SessionTitle != "" && !r.ParentMatched {
			return r.SessionTitle
		}
		if title := t.titles[routeTitleKey(r)]; title != "" {
			return strings.Join(strings.Fields(title), " ")
		}
	}
	return t.groupKey(r)
}

func (t *nativeTrace) groupKey(r traceRoute) string {
	if t.byKey || t.groupBy == "key" {
		if r.CallerKey != "" {
			return r.CallerKey
		}
		return nativeT("noKey")
	}
	if t.groupBy == "" {
		return nativeT("request") + " " + strconv.FormatInt(r.ID, 10)
	}
	if session := routeSession(r); session != "" {
		return r.Agent + " · " + session
	}
	if r.Conv != "" {
		return r.Agent + " · " + r.Conv
	}
	return nativeT("noSession")
}
func (t *nativeTrace) detailRead(p *page, kind, endpoint string) {
	t.detailKind = kind
	t.detail = nil
	t.detailAPI.request(p, "GET", endpoint, nil, func(raw json.RawMessage) { t.detail = raw })
}
func (t *nativeTrace) detailView(c *ui.Context, p *page) {
	if t.selected == nil {
		return
	}
	r := t.selected
	open := true
	kit.Dialog(c, &open, kit.DialogOptions{Title: nativeT("request") + " " + strconv.FormatInt(r.ID, 10), Cancel: i18n.T("common.close"), Width: 880, Body: func() {
		ui.Row(c).Gap(8).Wrap().Children(func() {
			if kit.Button(c, i18n.T("aiRouting.replayOne"), nil, false).Disabled(!r.Done || t.routeAPI.busy || t.replayAPI.busy).Clicked() {
				t.startReplay(p, []traceRoute{*r})
				open = false
			}
			if kit.Button(c, i18n.T("aiRouting.replayFrom"), nil, false).Disabled(!r.Done || t.routeAPI.busy || t.replayAPI.busy).Clicked() {
				var rows []traceRoute
				for _, row := range t.filterRoutes(t.routes) {
					if row.ID >= r.ID {
						rows = append(rows, row)
					}
				}
				t.startReplay(p, rows)
				open = false
			}
			if kit.Button(c, nativeT("routeDetails"), icons.Waypoints, false).Clicked() {
				t.detailKind = "route"
				t.detail = nil
			}
			if kit.Button(c, nativeT("requestResponse"), icons.FileText, false).Disabled(t.detailAPI.busy).Clicked() {
				t.detailRead(p, "body", "/api/gateway/call?route="+strconv.FormatInt(r.ID, 10))
			}
			if r.Archive != "" && kit.Button(c, nativeT("archive"), icons.Archive, false).Disabled(t.detailAPI.busy).Clicked() {
				t.detailKind = "archive"
				if t.archive == nil {
					t.archive = usage.NewArchive(p.env, r.Archive)
				}
			}
			if r.Held != "" && kit.Button(c, nativeT("heldReport"), icons.FileText, false).Disabled(t.detailAPI.busy).Clicked() {
				t.detailRead(p, "held", "/api/gateway/held/"+url.PathEscape(r.Held))
			}
			if r.Masked != nil && kit.Button(c, nativeT("maskingRecords"), icons.ShieldCheck, false).Clicked() {
				change := func() {
					p.maskRecord = r.Masked.Key
					p.advancedTab = advancedPrivacy
					p.switchTab(tabAdvanced)
					t.selected = nil
				}
				if p.env.RequestChange != nil {
					p.env.RequestChange(change)
				} else {
					change()
				}
			}
			if kit.Button(c, nativeT("copy"), icons.Copy, false).Clicked() {
				data := r.Raw
				if t.detail != nil {
					data = t.detail
				}
				c.Services().WriteClipboard(p.routing.accountMasker()(string(data)))
			}
		})
		t.detailAPI.notice(c, nil)
		t.routeAPI.notice(c, func() { t.selectRoute(p, *r) })
		if t.routeAPI.busy {
			kit.Status(c, i18n.T("capture.loading"))
		}
		if t.detailKind == "route" {
			t.evidenceView(c, p, r)
		} else if t.detailKind == "archive" && t.archive != nil {
			t.archive.View(c)
		} else if t.detail != nil {
			t.bodyView(c, p)
		}
	}})
	if !open {
		t.selected = nil
		t.detailAPI.close()
		t.routeAPI.close()
	}
}
