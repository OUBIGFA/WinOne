package selection

import (
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/internal/resultwindow"
)

// open shows the Quick assistant and brings its window to the front. The
// Host starts or shows it and returns its window; only this window, which
// the user just clicked, may hand the foreground to it, as the previous
// window's bridge did after quick-assistant.show.
func (p *page) open() {
	if p.opening {
		return
	}
	p.opening, p.err = true, ""
	core.Do(p.env, func() error {
		var target resultwindow.WindowTarget
		if err := p.env.Host.CallCompletingInto("quick-assistant.show", nil, &target); err != nil {
			return err
		}
		if err := activate(target); err != nil {
			return core.Errorf(i18n.T("quickAssistant.activateFailed"))
		}
		return nil
	}, func(err error) {
		p.opening = false
		if err != nil {
			p.err = err.Error()
		}
	})
}

// activate brings the Quick assistant's window to the front; tests
// replace it.
var activate = func(target resultwindow.WindowTarget) error { return target.Activate() }
