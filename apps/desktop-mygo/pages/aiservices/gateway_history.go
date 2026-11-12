package aiservices

import (
	"encoding/json"
	"github.com/egoist/mygo/ui"
	"strings"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

type gatewayHistoryState struct {
	Recording bool                               `json:"recording"`
	Recorded  bool                               `json:"recorded"`
	Sessions  []struct{ ID, Agent, Last string } `json:"sessions"`
}
type gatewayHistoryPanel struct {
	api         nativeAPI
	source      string
	state       *gatewayHistoryState
	query, text string
	confirm     bool
	page        int
	cut         bool
}

func (g *gatewayHistoryPanel) accept(raw json.RawMessage) {
	var state gatewayHistoryState
	if nativeDecode(&g.api, raw, &state) {
		g.state = &state
		g.confirm = false
	}
}
func (g *gatewayHistoryPanel) load(p *page) {
	g.api.request(p, "GET", "/api/winone/gateway/conversations", nil, g.accept)
}

// This page lists only calls recorded by WinOne's gateway. Recording consent
// is an explicit local operation, independent of external-client history.
func (g *gatewayHistoryPanel) View(c *ui.Context, p *page) {
	if p.gateway.runtimeURL == "" {
		return
	}
	if g.source != p.gateway.runtimeURL {
		g.api.close()
		g.api.err = ""
		g.source = p.gateway.runtimeURL
		g.state = nil
		g.text = ""
	}
	if g.state == nil && !g.api.busy && g.api.err == "" {
		g.load(p)
	}
	g.api.notice(c, func() { g.load(p) })
	kit.Help(c, accessT("historyHint"))
	if g.state == nil {
		return
	}
	ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
		label := accessT("startRecording")
		if g.state.Recording {
			label = accessT("stopRecording")
		}
		if kit.Button(c, label, icons.Activity, false).Disabled(g.api.busy).Clicked() {
			g.api.request(p, "POST", "/api/winone/gateway/conversations/recording", map[string]bool{"on": !g.state.Recording}, g.accept)
		}
		if kit.IconButton(c, icons.RefreshCw, i18n.T("common.refresh")).Disabled(g.api.busy).Clicked() {
			g.load(p)
		}
		if kit.GhostButton(c, accessT("clearRecording"), icons.Trash2, false).Disabled(g.api.busy || !g.state.Recorded).Clicked() {
			g.confirm = true
		}
	})
	if g.confirm {
		kit.Notice(c, kit.Caution, "", accessT("clearRecordingHint"), func() {
			if kit.Button(c, accessT("confirm"), icons.Check, false).Disabled(g.api.busy).Clicked() {
				g.api.request(p, "POST", "/api/winone/gateway/conversations/recording", map[string]bool{"clear": true}, func(raw json.RawMessage) {
					g.accept(raw)
					if g.api.err == "" {
						g.text = ""
					}
				})
			}
			if kit.GhostButton(c, i18n.T("common.cancel"), nil, false).Clicked() {
				g.confirm = false
			}
		})
	}
	kit.TextInput(c, &g.query).FillWidth().Label(accessT("findSession"))
	count := 0
	for _, session := range g.state.Sessions {
		if !strings.Contains(strings.ToLower(session.Agent+" "+session.ID), strings.ToLower(g.query)) {
			continue
		}
		index := count
		count++
		if index < g.page*20 || index >= (g.page+1)*20 {
			continue
		}
		kit.Row(c.Key(session.Agent+session.ID), session.Agent+" / "+session.ID, session.Last, func() {
			if kit.Button(c, accessT("readSession"), icons.BookOpen, false).Disabled(g.api.busy).Clicked() {
				g.api.request(p, "POST", "/api/winone/gateway/conversations/read", map[string]string{"agent": session.Agent, "id": session.ID}, func(raw json.RawMessage) {
					var out struct {
						Text string
						Cut  bool
					}
					if nativeDecode(&g.api, raw, &out) {
						g.text, g.cut = out.Text, out.Cut
					}
				})
			}
		})
	}
	if g.page > 0 && g.page*20 >= count {
		g.page = 0
	}
	ui.Row(c).Gap(8).Children(func() {
		if kit.IconButton(c, icons.ChevronLeft, accessT("previousSessions")).Disabled(g.page == 0).Clicked() {
			g.page--
		}
		if kit.IconButton(c, icons.ChevronRight, accessT("nextSessions")).Disabled((g.page+1)*20 >= count).Clicked() {
			g.page++
		}
	})
	if count == 0 {
		kit.Help(c, accessT("noSessions"))
	}
	if g.text != "" {
		if g.cut {
			kit.Help(c, accessT("sessionCut"))
		}
		if kit.Button(c, accessT("copySession"), icons.Copy, false).Clicked() {
			c.Services().WriteClipboard(g.text)
		}
		ui.Text(c, g.text).FillWidth().Wrap()
	}
}
