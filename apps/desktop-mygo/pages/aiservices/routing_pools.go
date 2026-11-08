package aiservices

import (
	"encoding/json"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

func (r *routingPanel) routingChoices(saved string) []kit.SelectOption {
	choices := routingChoices()
	if r.capabilities != nil {
		for _, strategy := range r.capabilities.JSRoutings {
			choices = append(choices, kit.SelectOption{Value: strategy.ID, Label: strategy.Name + " · " + strategy.Plugin})
		}
	}
	for _, choice := range choices {
		if choice.Value == saved {
			return choices
		}
	}
	if saved != "" {
		choices = append(choices, kit.SelectOption{Value: saved, Label: saved})
	}
	return choices
}

// A provider pool has the same durable routing preferences as before the
// native migration. Only that policy is written; endpoints and keys keep coming
// from the Host snapshot through winOneRoutingProvider.
func (r *routingPanel) setPool(action string, body any) {
	r.api.request(r.p, "POST", "/api/provider/"+action, body, func(json.RawMessage) { r.load() })
}

func (r *routingPanel) poolsView(c *ui.Context) {
	mask := r.accountMasker()
	if len(r.state.Pools) == 0 {
		kit.Empty(c, icons.Users, i18n.T("aiRouting.noPools"), i18n.T("aiRouting.noPoolsHint"), nil)
		return
	}
	kit.Section(c, icons.Users, nativeT("accountPools"), "", func() {
		for _, pool := range r.state.Pools {
			ui.Column(c.Key(pool.Provider+"/"+pool.Protocol)).FillWidth().Gap(8).Padding(10, 0).Children(func() {
				kit.Strong(c, pool.Name)
				for _, who := range pool.Who {
					kit.Help(c, mask(who)).Selectable()
				}
				if pool.Protocol != "" {
					kit.Help(c, pool.Protocol)
				}
				strategy := pool.Routing
				choices := r.routingChoices(strategy)
				var allowed []kit.SelectOption
				for _, choice := range choices {
					if choice.Value != "cheap" && choice.Value != "expiring" && choice.Value != "manual" {
						allowed = append(allowed, choice)
					}
				}
				if nativeSelect(c, nativeT("strategy"), &strategy, allowed, r.api.busy || r.p.locked()) {
					r.setPool("route", map[string]any{"id": pool.Provider, "routing": strategy})
				}
				affinity := pool.Affinity
				if nativeSelect(c, nativeT("affinity"), &affinity, options("", nativeT("auto"), "session", nativeT("affinitySession"), "turn", nativeT("affinityTurn"), "off", nativeT("off"), "strict", nativeT("affinityStrict")), r.api.busy || r.p.locked()) {
					r.setPool("affinity", map[string]any{"id": pool.Provider, "affinity": affinity})
				}
				sink := pool.Sink
				if kit.Toggle(c, &sink, nativeT("sink"), nativeT("sinkHint")).Disabled(r.api.busy || r.p.locked()).Changed() {
					r.setPool("sink", map[string]any{"id": pool.Provider, "sink": sink})
				}
			})
		}
	})
}
