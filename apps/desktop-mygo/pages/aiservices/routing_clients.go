package aiservices

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
)

// Suggestions come from the same gateway ledger as routing. Reading metadata
// here never scans client files; arbitrary client names remain editable above.
func (r *routingPanel) clientChoices(c *ui.Context, rule *routeRule, locked bool) {
	if !r.clientsLoaded && !r.clientsAPI.busy && r.clientsAPI.err == "" {
		r.clientsAPI.request(r.p, "GET", "/api/winone/routing/clients", nil, func(raw json.RawMessage) {
			var result struct {
				Clients []named `json:"clients"`
				Agents  []struct {
					ID string `json:"id"`
				} `json:"agents"`
			}
			if !nativeDecode(&r.clientsAPI, raw, &result) {
				return
			}
			r.clientsLoaded = true
			r.clientNames = map[string]string{}
			for _, client := range result.Clients {
				r.recentClients = toggleString(r.recentClients, client.ID, true)
				r.clientNames[client.ID] = client.Name
			}
			for _, agent := range result.Agents {
				parts := strings.FieldsFunc(strings.ToLower(strings.TrimSpace(agent.ID)), func(ch rune) bool { return ch == '/' || ch == ',' || ch == ' ' || ch == '\t' })
				if len(parts) > 0 && parts[0] != "other" && parts[0] != "magpie" {
					r.recentClients = toggleString(r.recentClients, parts[0], true)
				}
			}
		})
	}
	r.clientsAPI.notice(c, func() { r.clientsAPI.err = ""; r.clientsLoaded = false })
	if len(r.recentClients) == 0 {
		return
	}
	open := ui.Local(ui.Column(c), "client-choices", func() bool { return false })
	ui.Collapsible(c, nativeT("select")+" "+nativeT("clients"), open, func() {
		ui.Row(c).FillWidth().Wrap().Gap(8).Children(func() {
			for _, client := range r.recentClients {
				label := r.clientNames[client]
				if label == "" {
					label = client
				}
				on := slices.Contains(rule.Agents, client)
				if ui.Checkbox(c.Key(client), &on, label).Disabled(locked).Changed() {
					rule.Agents = toggleString(rule.Agents, client, on)
				}
			}
		})
	})
}
