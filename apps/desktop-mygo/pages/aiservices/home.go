package aiservices

import (
	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

func homeT(key string) string { return i18n.T("aiHome." + key) }

// Three task areas replace eight competing top-level settings tabs. The
// original editors and their shared drafts remain the only editing paths.
func (p *page) navigationView(c *ui.Context) {
	area := 2
	if p.tab == tabHome {
		area = 0
	} else if p.tab == tabCustom || p.tab == tabAccounts {
		area = 1
	}
	kit.Tabs(c, &area, []kit.Tab{{Label: homeT("home")}, {Label: homeT("connections")}, {Label: homeT("more")}}, func(apply func()) {
		change := func() { apply(); p.switchTab([]int{tabHome, tabCustom, tabMore}[area]) }
		if p.env.RequestChange != nil {
			p.env.RequestChange(change)
		} else {
			change()
		}
	})
	if area == 1 {
		sub := 0
		if p.tab == tabAccounts {
			sub = 1
		}
		kit.SubTabs(c, &sub, []string{i18n.T("ai.customProviders"), i18n.T("ai.addProvidersTab")}, func(apply func()) {
			change := func() { apply(); p.switchTab([]int{tabCustom, tabAccounts}[sub]) }
			if p.env.RequestChange != nil {
				p.env.RequestChange(change)
			} else {
				change()
			}
		})
	} else if area == 2 && p.tab != tabMore {
		ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
			if kit.GhostButton(c, homeT("back"), icons.ChevronLeft, false).Clicked() {
				p.navigateTo(tabMore)
			}
			for _, entry := range moreDestinations() {
				if entry.tab == p.tab {
					kit.Heading(c, entry.label)
					break
				}
			}
		})
	}
}
func (p *page) navigateTo(tab int) {
	change := func() { p.switchTab(tab) }
	if p.env.RequestChange != nil {
		p.env.RequestChange(change)
	} else {
		change()
	}
}
func (p *page) homeView(c *ui.Context) {
	if p.draft == nil {
		return
	}
	ui.Column(c).FillWidth().Gap(12).Children(func() { kit.Heading(c, homeT("default")); p.defaultModelView(c) })
	kit.Group(c, nil, homeT("connect"), homeT("connectHint"), func() {
		ui.Row(c).FillWidth().Wrap().Gap(8).Padding(12, 16).Children(func() {
			if kit.Button(c, homeT("api"), icons.Plus, false).Clicked() {
				p.navigateTo(tabCustom)
			}
			if kit.GhostButton(c, homeT("account"), icons.UserPlus, false).Clicked() {
				p.navigateTo(tabAccounts)
			}
		})
	})
	kit.Row(c, i18n.T("usage.title"), homeT("usageHint"), func() {
		if kit.GhostButton(c, homeT("viewUsage"), icons.ChevronRight, false).Clicked() {
			p.navigateTo(tabUsage)
		}
	})
}

type moreDestination struct {
	tab         int
	label, hint string
}

func moreDestinations() []moreDestination {
	return []moreDestination{
		{tabDefaults, homeT("preferences"), homeT("preferencesHint")},
		{tabGateway, i18n.T("gateway.title"), homeT("gatewayHint")},
		{tabRouting, i18n.T("routing.title"), homeT("routingHint")},
		{tabUsage, i18n.T("usage.title"), homeT("usageHint")},
		{tabPlugins, i18n.T("aiPlugins.title"), homeT("pluginsHint")},
		{tabAdvanced, i18n.T("ai.advanced"), homeT("advancedHint")},
	}
}
func (p *page) moreView(c *ui.Context) {
	kit.Help(c, homeT("moreHint"))
	kit.Panel(c, func() {
		for _, entry := range moreDestinations() {
			b := ui.ButtonBase(c.Key(entry.tab)).FillWidth().Padding(12, 8).Gap(12).Radius(6).Label(entry.label).Cursor(ui.CursorPointer)
			if b.Hovered() {
				b.Background(c.Theme().SurfaceHover)
			}
			if b.Clicked() {
				p.navigateTo(entry.tab)
			}
			b.Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() { ui.Text(c, entry.label).FontWeight(500); kit.Help(c, entry.hint) })
				ui.Icon(c, icons.ChevronRight).Size(16, 16).AlignSelf(ui.Center).TextColor(c.Theme().TextMuted)
			})
		}
	})
}
