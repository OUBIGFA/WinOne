package chat

import (
	"winone/desktopmygo/chat/pluginhost"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
)

// Browser editors can still have sanitized events in transit after the native
// controls look idle. Await the original mail delivery barrier without clearing
// either the source form or Chat's draft when synchronization fails.
func (p *Chat) flushMailEdits(session string, done func(error)) {
	pluginhost.For(p.env).FlushMailEdits(session, func(ok bool) {
		if !ok {
			done(core.Errorf(i18n.T("notices.mailSyncFailed")))
			return
		}
		done(nil)
	})
}

func (p *Chat) mailNavigation(action func()) {
	id, revision := p.activeID, p.selectionRevision
	p.flushMailEdits(id, func(err error) {
		if p.closed() || p.activeID != id || p.selectionRevision != revision {
			return
		}
		if err != nil {
			p.err = err.Error()
			return
		}
		action()
	})
}

func (p *Chat) bindMailBrowser() {
	plugins := pluginhost.For(p.env)
	plugins.SetMailBrowserOpener(func(session, address string) {
		if !p.closed() && p.activeID == session {
			p.mailNavigation(func() { p.openBrowserURL(address) })
			return
		}
		// Multiple native conversation controllers share this Host. Preserve
		// the reader's actual destination when the last-bound one is not it.
		plugins.QueueMailBrowser(session, address)
	})
}
