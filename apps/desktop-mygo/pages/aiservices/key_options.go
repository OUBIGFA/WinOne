package aiservices

import (
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
			numberRow(c, nativeT("keyPriceRate"), rate, .001, 1000, false, nativeT("providerPriceRate"), locked, func(v *float64) {
				if v == nil {
					delete(pr.KeyPriceRates, id)
				} else {
					if pr.KeyPriceRates == nil {
						pr.KeyPriceRates = map[string]float64{}
					}
					pr.KeyPriceRates[id] = *v
				}
			})
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
		if *slot != before {
			pr.Keys = &slots
		}
		return
	}
}
