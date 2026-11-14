package aiservices

import (
	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

// The retained tasks are the entire navigation. A saved extension setting
// cannot add another page or prevent the local gateway from being opened.
func (p *page) upstreamNavigation(c *ui.Context) {
	type destination struct {
		label string
		tab   int
		icon  *ui.SVG
	}
	// Icons as on every other page's tabs; the custom tab keeps LayersPlus.
	entries := []destination{
		{homeT("home"), tabHome, icons.Star},
		{i18n.T("ai.customProviders"), tabCustom, icons.LayersPlus}, {i18n.T("ai.addProvidersTab"), tabAccounts, icons.UserRound},
		{i18n.T("gateway.title"), tabGateway, icons.Server},
	}
	current := 0
	tabs := make([]kit.Tab, len(entries))
	for i, e := range entries {
		tabs[i] = kit.Tab{Label: e.label, Icon: e.icon}
		if p.tab == e.tab {
			current = i
		}
	}
	ui.Column(c).FillWidth().Gap(kit.HeadingGap).Disabled(p.navigationLocked()).Children(func() {
		kit.Tabs(c, &current, tabs, func(apply func()) {
			change := func() {
				apply()
				e := entries[current]
				if p.tab != e.tab {
					p.switchTab(e.tab)
				}
			}
			if p.env.RequestChange != nil {
				p.env.RequestChange(change)
			} else {
				change()
			}
		})
	})
}
