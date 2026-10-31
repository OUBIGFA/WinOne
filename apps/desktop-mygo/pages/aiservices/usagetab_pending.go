//go:build !usage_ready

package aiservices

import (
	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
)

// usageTab stands for the usage statistics until package usage is in
// place. TODO(mygo-ai-usage): remove with the usage_ready constraint.
type usageTab struct{}

func (*usageTab) Close() {}

func (p *page) usageView(c *ui.Context) {
	if page := p.extensions.replacement("usage"); page != nil {
		p.extensions.Page(c, *page, nil, p.locked())
		return
	}
	kit.Status(c, i18n.T("capture.loading"))
	p.extensions.Slot(c, "usage", nil, p.locked())
}
