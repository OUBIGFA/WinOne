package aiservices

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"time"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

// Gateway's recent calls is the upstream chronological list. Routing owns its
// separate stage and filters; both read the same service's evidence.
func (g *gatewayPanel) recentCallsView(c *ui.Context, p *page) {
	t := &g.calls
	now := c.Now()
	if p.extensions.state != nil && !t.api.busy && ((!t.loaded && t.api.err == "") || t.loaded && now.After(t.next)) {
		t.next = now.Add(2 * time.Second)
		t.load(p)
	}
	if !t.api.busy && t.api.err == "" {
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
	rows := t.routes
	for i := len(rows) - 1; i >= max(0, len(rows)-g.callsShown); i-- {
		r := rows[i]
		open := ui.Local(ui.Column(c.Key(r.ID)), "gateway-call", func() bool { return false })
		label := fmt.Sprintf("%s · %s · %s · %s", r.Time, r.Agent, p.routing.memberLabel(r.Model), nativeT(traceStatus(r)))
		ui.Collapsible(c.Key(r.ID), label, open, func() {
			kit.Help(c, t.metricText(r))
			if r.Error != "" {
				kit.InlineError(c, p.routing.accountMasker()(r.Error))
			}
			if kit.GhostButton(c, upstreamT("Details"), icons.FileText, false).Clicked() {
				t.selectRoute(p, r)
			}
		})
	}
	if t.loaded && len(rows) == 0 {
		kit.Help(c, upstreamT("No calls yet"))
	}
	if len(rows) > g.callsShown && kit.Button(c, nativeT("loadMore"), icons.ChevronDown, false).Clicked() {
		g.callsShown += 20
	}
	t.detailView(c, p)
}
