package aiservices

import (
	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

func homeT(key string) string { return i18n.T("aiHome." + key) }

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
	// Keep the everyday choice above connection setup at every window size.
	// Both sections use the same content width and symmetric field insets.
	ui.Column(c).FillWidth().Gap(kit.GroupGap).Children(func() {
		ui.Column(c).FillWidth().MinWidth(0).Gap(12).Children(func() {
			kit.Heading(c, homeT("default"))
			p.defaultModelView(c)
		})
		ui.Column(c).FillWidth().MinWidth(0).Children(func() {
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
		})
	})
	if p.builtinEnabled("usage") {
		kit.Row(c, i18n.T("usage.title"), homeT("usageHint"), func() {
			if kit.GhostButton(c, homeT("viewUsage"), icons.ChevronRight, false).Clicked() {
				p.navigateTo(tabUsage)
			}
		})
	}
}

type moreDestination struct {
	tab         int
	label, hint string
}

func moreDestinations() []moreDestination {
	return []moreDestination{
		{tabGateway, i18n.T("gateway.title"), homeT("gatewayHint")},
		{tabRouting, i18n.T("routing.title"), homeT("routingHint")},
		{tabUsage, i18n.T("usage.title"), homeT("usageHint")},
		{tabMasking, upstreamT("Masking"), homeT("maskingHint")},
		{tabPlugins, i18n.T("aiPlugins.title"), homeT("pluginsHint")},
		{tabAdvanced, i18n.T("ai.advanced"), homeT("advancedHint")},
	}
}
func (p *page) moreHeading(c *ui.Context) {
	ui.Row(c).FillWidth().Gap(12).Wrap().AlignItems(ui.Center).Children(func() {
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
func (p *page) moreView(c *ui.Context) {
	kit.Help(c, homeT("moreHint"))
	kit.Panel(c, func() {
		for _, entry := range moreDestinations() {
			if !p.destinationEnabled(entry.tab) {
				continue
			}
			b := ui.ButtonBase(c.Key(entry.tab)).FillWidth().Padding(14, 16).Gap(12).Radius(6).Label(entry.label).Cursor(ui.CursorPointer)
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
