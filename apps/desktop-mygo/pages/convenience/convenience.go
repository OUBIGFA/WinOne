// Package convenience is the Convenience page: tools grouped in one entry,
// each tab keeping its own switch, settings and lifecycle — pinning
// windows, mouse gestures, file and folder links, and timed power.
//
// React source: features/convenience.tsx. As there, a tab shows its
// content only while chosen: leaving a tab (through the unsaved-change
// guard) drops its state, and coming back reads it again.
package convenience

import (
	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/shell"
)

func init() {
	shell.Register("convenience", core.Settings, func(env *core.Env) core.Page { return newPage(env) })
}

// The tabs, in their order.
const (
	tabPin = iota
	tabGestures
	tabLinks
	tabPower
)

// part is the content of one tab.
type part interface {
	View(c *ui.Context, locked bool)
	// Close stops the tab's work and withdraws its activity.
	Close()
}

type page struct {
	env  *core.Env
	tab  int
	part part
}

func newPage(env *core.Env) *page {
	p := &page{env: env}
	p.mount(tabPin)
	return p
}

func (p *page) mount(tab int) {
	if p.part != nil {
		p.part.Close()
	}
	p.tab = tab
	switch tab {
	case tabPin:
		p.part = newPinTab(p.env)
	case tabGestures:
		p.part = newGesturesTab(p.env)
	case tabLinks:
		p.part = newLinksTab(p.env)
	default:
		p.part = newPowerTab(p.env)
	}
}

// moduleDisabled reports whether the Host's snapshot shows a module off;
// its settings stay editable and apply once it is on.
func moduleDisabled(env *core.Env, id string) bool {
	for _, module := range env.Snapshot().Modules {
		if module.ID == id {
			return module.State == "disabled"
		}
	}
	return false
}

func (p *page) View(c *ui.Context) {
	// A switch never changes under an unsaved edit or a running
	// operation of this page.
	dirty, busy := p.env.Activity.State()
	locked := dirty || busy
	// Explorer's "create link" prefills show the links tab, once nothing
	// here would be lost or interrupted.
	if !locked {
		if intents := p.env.TakeIntents("convenience"); len(intents) > 0 {
			if p.tab != tabLinks {
				p.mount(tabLinks)
			}
			for _, intent := range intents {
				p.part.(*linksTab).Queue(intent)
			}
		}
	}
	kit.Page(c, func() {
		kit.Tabs(c, &p.tab, []kit.Tab{
			{Label: i18n.T("nav.always-on-top"), Icon: icons.Pin},
			{Label: i18n.T("stroke.title"), Icon: icons.MousePointer2},
			{Label: i18n.T("links.title"), Icon: icons.FolderSymlink},
			{Label: i18n.T("timedPower.title"), Icon: icons.Timer},
		}, func(apply func()) {
			p.env.RequestChange(func() {
				apply()
				p.mount(p.tab)
			})
		})
		p.part.View(c, locked)
	})
}

// Close ends the page's work when the shell shows another page.
func (p *page) Close() { p.part.Close() }
