package chat

import "winone/desktopmygo/chat/pluginhost"

// panelRequest is what the side panel opens, as React's openDetails family
// in chat.tsx set it: a tab of the panel (home, the file library, the
// resources, a review), a file at a line, a web page, or a turn's changes.
// The native workspace controller receives each numbered request once.
type panelRequest struct {
	// Tab is "home", "files", "resources" or "review".
	Tab       string
	Directory string
	File      string
	Line      int
	URL       string
	Review    *reviewTarget
}

// reviewTarget is a turn's changes, optionally at one of its files.
type reviewTarget struct {
	TurnID string
	Path   string
}

// openPanel shows the side panel and asks it to open r. Each request
// counts, as React's detailsActivation did, so asking twice for the same
// tab brings it forward again.
func (p *Chat) openPanel(r panelRequest) {
	if r.Tab == "" {
		r.Tab = "home"
	}
	p.ui.details = true
	p.ui.detailsTab = r.Tab
	p.ui.panelRequests++
	p.ui.panelRequest = &r
}

// openTools opens a side panel tab, as the composer's + menu asks.
func (p *Chat) openTools(tab string) { p.openPanel(panelRequest{Tab: tab}) }

// openFile shows a file of the conversation in the side panel, at line
// when it is above zero.
func (p *Chat) openFile(path string, line int) {
	p.openPanel(panelRequest{Tab: "home", File: path, Line: line})
}

// openReview shows a turn's changes in the side panel.
func (p *Chat) openReview(turnID, path string) {
	p.openPanel(panelRequest{Tab: "review", Review: &reviewTarget{TurnID: turnID, Path: path}})
}

// openURL gives ordinary links the original read:false mail probe, after the
// current editor's real delivery barrier. Reader links use openBrowserURL below
// and cannot recursively probe the same mail address.
func (p *Chat) openURL(address string) {
	p.mailNavigation(func() {
		id, revision := p.activeID, p.selectionRevision
		pluginhost.For(p.env).OpenMailIfSupported(id, address, func(handled bool) {
			if !handled && !p.closed() && p.activeID == id && p.selectionRevision == revision {
				p.openBrowserURL(address)
			}
		})
	})
}

func (p *Chat) openBrowserURL(address string) { p.openPanel(panelRequest{Tab: "home", URL: address}) }
