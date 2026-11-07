package chat

import (
	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/chat/model"
	"winone/desktopmygo/i18n"
)

type messageJump struct {
	sessionID string
	ids       []string
	before    string
	attempted bool
}

// JumpToMessage keeps search results on their owning conversation. Older
// pages are read until the exact host/client identity is available; one failed
// cursor is never polled again automatically.
func (p *Chat) JumpToMessage(clientID, messageID string) {
	ids := []string{}
	for _, id := range []string{clientID, messageID} {
		if id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return
	}
	id := p.activeID
	if p.requested != nil {
		id = p.requested.Conversation
	}
	p.messageJump = &messageJump{sessionID: id, ids: ids}
	p.ui.follow, p.ui.pinNext = false, false
}
func (p *Chat) matchesMessageJump(m model.Message) bool {
	jump := p.messageJump
	if jump == nil || jump.sessionID != p.activeID {
		return false
	}
	for _, id := range jump.ids {
		if id == m.ID || id == m.RequestID {
			return true
		}
	}
	return false
}
func (p *Chat) revealMessage(element ui.Element, m model.Message) {
	if p.matchesMessageJump(m) {
		element.ScrollIntoView()
		p.messageJump = nil
		p.ui.follow, p.ui.pinNext = false, false
	}
}
func (p *Chat) advanceMessageJump() {
	jump := p.messageJump
	if jump == nil || jump.sessionID != p.activeID || p.opening || p.historyLoading {
		return
	}
	for _, m := range p.view.Messages {
		if p.matchesMessageJump(m) {
			return
		}
	}
	if !p.view.HasOlder {
		p.messageJump = nil
		p.notice = i18n.T("chat.requestedMessageMissing")
		return
	}
	if jump.attempted && jump.before == p.view.Before {
		return
	}
	jump.attempted = true
	jump.before = p.view.Before
	p.loadEarlier()
}
