package aiservices

import (
	"slices"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/internal/ai"
)

func (e *editor) billingPreferences(c *ui.Context, pr *provider, locked bool) {
	s := &pr.ProviderPreferences
	kit.Group(c, nil, i18n.T("ai.requestLimits"), i18n.T("ai.requestLimitsHint"), func() {
		for _, field := range []struct {
			key string
			v   *float64
			max float64
		}{{"maxConcurrency", &pr.MaxConcurrency, 1000}, {"maxRPM", &pr.MaxRPM, 10000}, {"queueLimit", &pr.QueueLimit, 10000}, {"queueWait", &pr.QueueWait, 3600}} {
			numberRow(c, i18n.T("ai."+field.key), field.v, 0, field.max, true, i18n.T("ai.noRequestLimit"), locked, func(v *float64) { *field.v = value(v) })
		}
		var max *float64
		if s.MaxAccounts != nil {
			n := float64(*s.MaxAccounts)
			max = &n
		}
		numberRow(c, i18n.T("ai.maxAccounts"), max, 0, 1024, true, i18n.T("aiOptions.default"), locked, func(v *float64) {
			s.MaxAccounts = nil
			if v != nil {
				n := int(*v)
				s.MaxAccounts = &n
			}
		})
		kit.Toggle(c, &s.CapStop, i18n.T("ai.capStop"), "").Disabled(locked)
		kit.Toggle(c, &s.CapCareful, i18n.T("ai.capCareful"), "").Disabled(locked)
		kit.Toggle(c, &s.CountFree, i18n.T("ai.countFree"), i18n.T("ai.countFreeHint")).Disabled(locked)
	})
	kit.Group(c, nil, i18n.T("aiComplete.rest"), i18n.T("aiComplete.restHint"), func() {
		quota, cap := float64(s.QuotaRest), float64(s.RestCap)
		numberRow(c, i18n.T("aiComplete.quotaRest"), &quota, 0, 525600, true, "0", locked, func(v *float64) { s.QuotaRest = int(value(v)) })
		numberRow(c, i18n.T("aiComplete.restCap"), &cap, -1, 525600, true, "0", locked, func(v *float64) { s.RestCap = int(value(v)) })
		kit.Stack(c, "", i18n.T("aiComplete.quotaResetAtHint"), func() { nativeText(c, i18n.T("aiComplete.quotaResetAt"), &s.QuotaResetAt, locked) })
	})
	kit.Group(c, nil, i18n.T("aiOptions.price"), "", func() {
		var rate *float64
		if s.PriceRate != 0 {
			rate = &s.PriceRate
		}
		// Zero means the regular price, not free requests.
		numberRow(c, i18n.T("ai.priceRate"), rate, 0, 1000, false, "1", locked, func(v *float64) { s.PriceRate = value(v) })
		if pr.AuthMode == "api-key" {
			kit.Stack(c, i18n.T("ai.keyPlan"), "", func() { ui.TextInput(c, &s.KeyPlan).FillWidth().Label(i18n.T("ai.keyPlan")).Disabled(locked) })
		}
	})
	kit.Section(c, nil, i18n.T("ai.rateWindows"), i18n.T("ai.rateWindowsHint"), func() {
		if len(s.RateWindows) == 0 {
			kit.Help(c, i18n.T("aiOptions.noWindows"))
		}
		remove := -1
		for i := range s.RateWindows {
			w := &s.RateWindows[i]
			kit.Panel(c.Key(i), func() {
				kit.Row(c, kit.Itoa(i+1), "", func() {
					if kit.IconButton(c, icons.Trash2, i18n.T("aiOptions.removeWindow")).Disabled(locked).Clicked() {
						remove = i
					}
				})
				kit.Stack(c, "", "", func() {
					ui.Row(c).FillWidth().Gap(16).Wrap().Children(func() {
						ui.Column(c).Basis(180).Grow(1).Children(func() { nativeText(c, i18n.T("aiOptions.start"), &w.Start, locked) })
						ui.Column(c).Basis(180).Grow(1).Children(func() { nativeText(c, i18n.T("aiOptions.end"), &w.End, locked) })
					})
				})
				numberRow(c, i18n.T("ai.priceRate"), &w.Rate, .001, 1000, false, "1", locked, func(v *float64) { w.Rate = value(v) })
				kit.Stack(c, i18n.T("aiOptions.days"), i18n.T("aiOptions.allDays"), func() {
					ui.Row(c).Gap(12).Wrap().Children(func() {
						for day, name := range []string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"} {
							on := slices.Contains(w.Days, day)
							if ui.Checkbox(c.Key(day), &on, i18n.T("aiNative.day."+name)).Disabled(locked).Changed() {
								if on {
									w.Days = append(w.Days, day)
								} else {
									w.Days = slices.DeleteFunc(w.Days, func(v int) bool { return v == day })
								}
							}
						}
					})
				})
			})
		}
		if remove >= 0 {
			s.RateWindows = slices.Delete(s.RateWindows, remove, remove+1)
		}
		if kit.Button(c, i18n.T("aiOptions.addWindow"), icons.Plus, false).Disabled(locked || len(s.RateWindows) >= 8).Clicked() {
			s.RateWindows = append(s.RateWindows, ai.RateWindow{Start: "09:00", End: "18:00", Rate: 1})
		}
	})
}
