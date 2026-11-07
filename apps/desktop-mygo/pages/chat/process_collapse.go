package chat

import (
	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

// Collecting a group restores compact rows as well as the latest-four window;
// otherwise a single expanded thought could remain pages long with no button.
func (p *Chat) collapseProcess(key string) {
	p.ui.collapseAnchor = key
	p.ui.follow, p.ui.pinNext = false, false
	p.ui.items.jump = nil
	p.ui.expanded["process:"+key] = false
	for _, entry := range p.ui.processEntries[key] {
		delete(p.ui.expanded, entry.Kind+":"+entry.Key)
	}
}

// processCollapseControls live in the viewport, not in the scrolling body.
// Each long expanded process reserves its own right gutter; the button stays
// inside that process's visible intersection, above (never over) the composer.
func (p *Chat) processCollapseControls(c *ui.Context) {
	viewport := p.ui.transcript.Bounds(c)
	for _, key := range p.ui.openProcesses {
		body := p.ui.processBodies[key].Bounds(c)
		if body.H < 240 {
			continue
		}
		top, bottom := max(viewport.Y, body.Y), min(viewport.Y+viewport.H, body.Y+body.H)
		if bottom-top < 44 {
			continue
		}
		x := body.X + body.W - viewport.X - 36
		y := bottom - viewport.Y - 36
		button := kit.IconButton(c.Key("collapse-process-"+key), icons.ChevronsUp, i18n.T("chat.collapseProcess")).
			Attach(ui.AnchorTopLeft, ui.AnchorTopLeft).Left(x).Size(28, 28).
			Border(1, c.Theme().Border).Background(kit.Colors(c).Panel)
		button.Top(y)
		if button.Clicked() {
			p.collapseProcess(key)
			p.ui.follow, p.ui.pinNext = false, false
		}
	}
}
