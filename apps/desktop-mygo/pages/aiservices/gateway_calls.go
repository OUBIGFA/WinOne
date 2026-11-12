package aiservices

import (
	"encoding/json"
	"fmt"
	"github.com/egoist/mygo/ui"
	"strconv"
	"time"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

// Recent calls belong to the gateway. Route-group editing, replay and global
// usage analysis have no lifecycle or controls in this retained page.
type gatewayCall struct {
	ID                        int64 `json:"id"`
	Time, Agent, Model, Error string
	Done, Canceled            bool
	Status                    int
	Millis                    int64   `json:"ms"`
	Tokens                    int64   `json:"tokens"`
	Output                    int64   `json:"out"`
	Cost                      float64 `json:"cost"`
	raw                       json.RawMessage
}

func (r *gatewayCall) UnmarshalJSON(raw []byte) error {
	type plain gatewayCall
	if err := json.Unmarshal(raw, (*plain)(r)); err != nil {
		return err
	}
	r.raw = append(json.RawMessage(nil), raw...)
	return nil
}

type gatewayCalls struct {
	api, detailAPI nativeAPI
	routes         []gatewayCall
	loaded         bool
	next           time.Time
	selected       int64
	detail         json.RawMessage
}

func (t *gatewayCalls) close() {
	t.api.close()
	t.detailAPI.close()
	t.loaded = false
	t.routes = nil
	t.selected = 0
	t.detail = nil
}
func (t *gatewayCalls) load(p *page) {
	t.api.request(p, "GET", "/api/gateway/trace", nil, func(raw json.RawMessage) {
		var out struct {
			Routes []gatewayCall `json:"routes"`
		}
		if nativeDecode(&t.api, raw, &out) {
			t.routes = out.Routes
			t.loaded = true
			t.next = time.Now().Add(2 * time.Second)
		}
	})
}
func (g *gatewayPanel) recentCallsView(c *ui.Context, p *page) {
	t := &g.calls
	now := c.Now()
	if p.gateway.runtimeURL != "" && !t.api.busy && t.api.err == "" && (!t.loaded || now.After(t.next)) {
		t.next = now.Add(2 * time.Second)
		t.load(p)
	}
	if !t.api.busy && t.api.err == "" && p.gateway.runtimeURL != "" {
		c.After(max(time.Millisecond, t.next.Sub(now)))
	}
	ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
		kit.Heading(c, upstreamT("Recent calls"))
		ui.Spacer(c).Grow(1)
		if kit.IconButton(c, icons.RefreshCw, i18n.T("common.refresh")).Disabled(t.api.busy).Clicked() {
			t.load(p)
		}
	})
	t.api.notice(c, func() { t.load(p) })
	if g.callsShown == 0 {
		g.callsShown = 20
	}
	for i := len(t.routes) - 1; i >= max(0, len(t.routes)-g.callsShown); i-- {
		r := t.routes[i]
		open := ui.Local(ui.Column(c.Key(r.ID)), "gateway-call", func() bool { return false })
		status := "running"
		if r.Canceled {
			status = "canceled"
		} else if r.Error != "" || r.Status >= 400 {
			status = "failed"
		} else if r.Done {
			status = "succeeded"
		}
		label := fmt.Sprintf("%s · %s · %s · %s", r.Time, r.Agent, r.Model, nativeT(status))
		ui.Collapsible(c.Key(r.ID), label, open, func() {
			kit.Help(c, fmt.Sprintf("%d ms · %d → %d tokens · $%.6f", r.Millis, r.Tokens, r.Output, r.Cost))
			if r.Error != "" {
				kit.InlineError(c, r.Error)
			}
			if kit.GhostButton(c, upstreamT("Details"), icons.FileText, false).Clicked() {
				t.selected = r.ID
				t.detail = r.raw
			}
			if t.selected == r.ID {
				if kit.Button(c, upstreamT("Request"), icons.FileText, false).Disabled(t.detailAPI.busy).Clicked() {
					t.detailAPI.request(p, "GET", "/api/gateway/call?route="+strconv.FormatInt(r.ID, 10), nil, func(raw json.RawMessage) { t.detail = raw })
				}
				t.detailAPI.notice(c, nil)
				if t.detail != nil {
					nativeJSON(c, t.detail)
				}
			}
		})
	}
	if t.loaded && len(t.routes) == 0 {
		kit.Help(c, upstreamT("No calls yet"))
	}
	if len(t.routes) > g.callsShown && kit.Button(c, nativeT("loadMore"), icons.ChevronDown, false).Clicked() {
		g.callsShown += 20
	}
}
