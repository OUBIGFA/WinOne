package aiservices

import (
	"math"
	"strconv"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
)

// API keys carry their own protocol, share and plan. Account-only maps do
// not configure these upstream fields and must not stand in for them.
func (e *editor) keyOptions(c *ui.Context, pr *provider, id string, locked bool) {
	slots := append([]keySlot{}, pr.slots()...)
	for i := range slots {
		slot := &slots[i]
		if slot.ID != id {
			continue
		}
		before := *slot
		kit.Group(c, nil, i18n.T("aiComplete.keyOptions"), "", func() {
			kit.Stack(c, "", "", func() { nativeText(c, i18n.T("aiComplete.keyName"), &slot.Name, locked) })
			kit.Stack(c, "", "", func() {
				nativeSelect(c, i18n.T("ai.protocol"), &slot.Protocol, options("", i18n.T("ai.automatic"), "chat", "OpenAI Chat Completions", "responses", "OpenAI Responses", "anthropic", "Anthropic Messages", "gemini", "Google Gemini"), locked)
			})
			weight := float64(slot.Weight)
			numberRow(c, i18n.T("aiComplete.keyWeight"), &weight, 0, 1000, true, "1", locked, func(v *float64) { slot.Weight = int(value(v)) })
			var rate *float64
			if current, ok := pr.KeyPriceRates[id]; ok {
				rate = &current
			}
			placeholder := nativeT("providerPriceRate")
			if group := e.keyRests.groupRates[id]; group != nil && pr.TopUpRate > 0 {
				// Match upstream's six-decimal effective-rate hint, including free groups.
				placeholder = strconv.FormatFloat(math.Round(*group*pr.TopUpRate*1e6)/1e6, 'f', -1, 64)
			}
			numberRow(c, nativeT("keyPriceRate"), rate, .001, 1000, false, placeholder, locked, func(v *float64) {
				if v == nil {
					delete(pr.KeyPriceRates, id)
				} else {
					if pr.KeyPriceRates == nil {
						pr.KeyPriceRates = map[string]float64{}
					}
					pr.KeyPriceRates[id] = *v
				}
			})
			if pr.TopUpRate > 0 {
				// Upstream multiplies the read group rate by this factor; none is 1.
				var factor *float64
				if current, ok := pr.KeyRateFactors[id]; ok {
					factor = &current
				}
				numberRow(c, upstreamT("Cost factor"), factor, .0001, 1000, false, "1", locked, func(v *float64) {
					if v == nil {
						delete(pr.KeyRateFactors, id)
					} else {
						if pr.KeyRateFactors == nil {
							pr.KeyRateFactors = map[string]float64{}
						}
						pr.KeyRateFactors[id] = *v
					}
				})
			}
			plans := options("", i18n.T("aiComplete.providerPlan"))
			if preset := e.p.catalog.preset(pr.Preset); preset != nil && preset.KeyPlans {
				for _, region := range preset.Regions {
					plans = append(plans, kit.SelectOption{Value: region.ID, Label: region.Name})
				}
			}
			if len(plans) > 1 {
				kit.Stack(c, "", "", func() { nativeSelect(c, i18n.T("ai.keyPlan"), &slot.Plan, plans, locked) })
			}
		})
		e.keyBalancePreferences(c, pr, id, locked)
		if *slot != before {
			pr.Keys = &slots
		}
		return
	}
}
