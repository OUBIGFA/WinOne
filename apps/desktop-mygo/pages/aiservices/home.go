package aiservices

import (
	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
)

func homeT(key string) string { return i18n.T("aiHome." + key) }

func (p *page) homeView(c *ui.Context) {
	if p.draft == nil {
		return
	}
	// Home holds the everyday model and result behavior. Connections remain
	// directly reachable through the provider tabs and the compact preview.
	ui.Column(c).FillWidth().Gap(kit.GroupGap).Children(func() {
		ui.Column(c).FillWidth().MinWidth(0).Gap(12).Children(func() {
			kit.Heading(c, homeT("default"))
			p.defaultModelView(c)
		})
		p.responseBehaviorView(c)
	})
}
