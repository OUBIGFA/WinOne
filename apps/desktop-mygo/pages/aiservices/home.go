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
		p.responseBehaviorView(c)
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
}
