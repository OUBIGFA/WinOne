package aiservices

import (
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

type traceRoute struct {
	ID            int64  `json:"id"`
	Seq           int64  `json:"seq"`
	Time          string `json:"time"`
	Agent         string `json:"agent"`
	Session       string `json:"session"`
	Conv          string `json:"conv"`
	Folder        string `json:"folder"`
	Model         string `json:"model"`
	Served        string `json:"served"`
	Provider      string `json:"provider"`
	CallerKeyName string `json:"callerKeyName"`
	CallerKey     string `json:"callerKey"`
	Done          bool   `json:"done"`
	Canceled      bool   `json:"canceled"`
	Status        int    `json:"status"`
	Error         string `json:"error"`
	Fail          string `json:"fail"`
	Millis        int64  `json:"ms"`
	TTFT          int64  `json:"ttft"`
	Tokens        int64  `json:"tokens"`
	Output        int64  `json:"out"`
	Bytes         int64  `json:"bytes"`
	Archive       string `json:"archive"`
	Held          string `json:"held"`
	Masked        *struct {
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
	api, detailAPI, titleAPI                            nativeAPI
	titles                                              map[string]string
	nextTitles                                          time.Time
	routes                                              []traceRoute
	days                                                []string
	day, query, provider, model, status, client, caller string
	loaded                                              bool
	paused                                              bool
	byKey                                               bool
	next                                                time.Time
	folded                                              map[string]bool
	selected                                            *traceRoute
	detail                                              json.RawMessage
	detailKind                                          string
}

func (t *nativeTrace) load(p *page) {
	endpoint := "/api/gateway/trace"
	if t.day != "" {
		endpoint = "/api/gateway/history?day=" + url.QueryEscape(t.day)
	}
	t.api.request(p, "GET", endpoint, nil, func(raw json.RawMessage) {
		var got struct {
			Routes []traceRoute `json:"routes"`
			Days   []string     `json:"days"`
		}
		if nativeDecode(&t.api, raw, &got) {
			t.routes = got.Routes
			if time.Now().After(t.nextTitles) {
				t.nextTitles = time.Now().Add(30 * time.Second)
				var keys []string
				for _, row := range got.Routes {
					if row.Session != "" {
						key := row.Agent + ":" + row.Session
						if !slices.Contains(keys, key) {
							keys = append(keys, key)
						}
					}
				}
				if len(keys) > 0 {
					t.titleAPI.request(p, "POST", "/api/gateway/session-titles", map[string]any{"sessions": keys}, func(raw json.RawMessage) {
						var value struct {
							Titles map[string]string `json:"titles"`
						}
						if nativeDecode(&t.titleAPI, raw, &value) {
							t.titles = value.Titles
						}
					})
				}
			}
			t.loaded = true
			if got.Days != nil {
				t.days = got.Days
			}
		}
	})
}
func (t *nativeTrace) View(c *ui.Context, p *page) {
	if t.folded == nil {
		t.folded = map[string]bool{}
	}
	now := c.Now()
	if !t.api.busy && p.extensions.state != nil && (!t.loaded || !t.paused && now.After(t.next)) {
		t.next = now.Add(2 * time.Second)
		t.load(p)
	}
	if !t.paused {
		c.After(max(time.Millisecond, t.next.Sub(now)))
	}
	ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
		ui.TextInput(c, &t.query).Grow(1).MinWidth(180).Label(nativeT("searchRequests")).Placeholder(nativeT("searchRequests"))
		label := nativeT("pauseRefresh")
		if t.paused {
			label = nativeT("resumeRefresh")
		}
		if kit.Button(c, label, nil, false).Clicked() {
			t.paused = !t.paused
		}
		if kit.IconButton(c, icons.RefreshCw, i18n.T("common.refresh")).Disabled(t.api.busy).Clicked() {
			t.load(p)
		}
	})
	ui.Row(c).FillWidth().Gap(12).Wrap().Children(func() {
		ui.Column(c).Basis(180).Grow(1).Children(func() { nativeText(c, nativeT("day"), &t.day, t.api.busy) })
		if kit.Button(c, nativeT("loadDay"), icons.Calendar, false).Disabled(t.api.busy).Clicked() {
			t.load(p)
		}
		if kit.GhostButton(c, nativeT("today"), nil, false).Clicked() {
			t.day = ""
			t.load(p)
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
	choiceRow(c, nativeT("status"), "", &t.status, options("", nativeT("all"), "running", nativeT("running"), "success", nativeT("succeeded"), "failed", nativeT("failed"), "canceled", nativeT("canceled")), false)
	kit.Toggle(c, &t.byKey, nativeT("groupByKey"), "")
	ui.Row(c).Gap(8).Wrap().Children(func() {
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
	if !t.loaded && t.api.busy {
		kit.Status(c, i18n.T("capture.loading"))
	}
	groups := map[string][]traceRoute{}
	var order []string
	for _, r := range t.routes {
		state := traceStatus(r)
		if t.status != "" && t.status != state {
			continue
		}
		if !strings.Contains(strings.ToLower(r.Provider), strings.ToLower(t.provider)) || !strings.Contains(strings.ToLower(r.Model), strings.ToLower(t.model)) || !strings.Contains(strings.ToLower(r.Agent), strings.ToLower(t.client)) || !strings.Contains(strings.ToLower(r.CallerKeyName+" "+r.CallerKey), strings.ToLower(t.caller)) {
			continue
		}
		if t.query != "" && !strings.Contains(strings.ToLower(string(r.Raw)), strings.ToLower(t.query)) {
			continue
		}
		key := t.groupKey(r)
		if _, ok := groups[key]; !ok {
			order = append(order, key)
		}
		groups[key] = append(groups[key], r)
	}
	for _, key := range order {
		open := !t.folded[key]
		ui.Collapsible(c.Key(key), t.groupLabel(groups[key][0])+" · "+kit.Itoa(len(groups[key])), &open, func() {
			for _, r := range groups[key] {
				label := r.Model
				if r.Served != "" && r.Served != r.Model {
					label += " → " + r.Served
				}
				button := ui.ButtonBase(c.Key(r.ID)).FillWidth().Padding(10, 8).BorderWidth(0, 0, 1, 0).BorderColor(kit.Colors(c).Hairline).Label(nativeT("request") + " " + strconv.FormatInt(r.ID, 10))
				button.Children(func() {
					ui.Column(c).FillWidth().Gap(4).Children(func() {
						kit.Strong(c, label).Selectable()
						kit.Help(c, r.Agent+" · "+r.Time+" · "+nativeT(traceStatus(r)))
						kit.Help(c, fmt.Sprintf("%s %d ms · %s %d ms · %d tokens · %d bytes", nativeT("duration"), r.Millis, nativeT("timeToFirst"), r.TTFT, r.Tokens, r.Bytes))
						if r.Error != "" {
							kit.Help(c, r.Error).Selectable()
						}
					})
				})
				if button.Clicked() {
					copy := r
					t.selected = &copy
					t.detail = nil
					t.detailKind = "route"
				}
			}
		})
		t.folded[key] = !open
	}
	if t.loaded && len(order) == 0 {
		kit.Empty(c, icons.Waypoints, nativeT("noRequests"), "", nil)
	}
	t.detailView(c, p)
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
	if r.Error != "" || r.Status >= 400 {
		return "failed"
	}
	return "success"
}
func (t *nativeTrace) groupLabel(r traceRoute) string {
	if !t.byKey {
		if title := t.titles[r.Agent+":"+r.Session]; title != "" {
			return strings.Join(strings.Fields(title), " ")
		}
	}
	return t.groupKey(r)
}

func (t *nativeTrace) groupKey(r traceRoute) string {
	if t.byKey {
		if r.CallerKeyName != "" {
			return r.CallerKeyName
		}
		return nativeT("noKey")
	}
	if r.Session != "" {
		return r.Agent + " · " + r.Session
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
			if kit.Button(c, nativeT("routeDetails"), icons.Waypoints, false).Clicked() {
				t.detailKind = "route"
				t.detail = nil
			}
			if kit.Button(c, nativeT("requestResponse"), icons.FileText, false).Disabled(t.detailAPI.busy).Clicked() {
				t.detailRead(p, "body", "/api/gateway/call?route="+strconv.FormatInt(r.ID, 10))
			}
			if r.Archive != "" && kit.Button(c, nativeT("archive"), icons.Archive, false).Disabled(t.detailAPI.busy).Clicked() {
				t.detailRead(p, "archive", archiveEndpoint(r.Archive))
			}
			if r.Held != "" && kit.Button(c, nativeT("heldReport"), icons.FileText, false).Disabled(t.detailAPI.busy).Clicked() {
				t.detailRead(p, "held", "/api/gateway/held/"+url.PathEscape(r.Held))
			}
			if r.Masked != nil && kit.Button(c, nativeT("maskingRecords"), icons.ShieldCheck, false).Clicked() {
				p.maskRecord = r.Masked.Key
				p.advancedTab = advancedPrivacy
				p.switchTab(tabAdvanced)
				open = false
			}
			if kit.Button(c, nativeT("copy"), icons.Copy, false).Clicked() {
				data := r.Raw
				if t.detail != nil {
					data = t.detail
				}
				c.Services().WriteClipboard(string(data))
			}
		})
		t.detailAPI.notice(c, nil)
		if t.detailKind == "route" {
			t.evidenceView(c, p, r)
		} else if t.detail != nil {
			nativeJSON(c, t.detail)
		}
	}})
	if !open {
		t.selected = nil
		t.detailAPI.close()
	}
}
