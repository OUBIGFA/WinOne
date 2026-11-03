// Package onebot is the OneBot page: Cindy's teammates (their list, home,
// roster, market, direct and group chats, sessions, history, IM, voice,
// memory and routines), which the user chose to keep in React (the
// hybrid plan). The page shows features/onebot.tsx from chat-embed.html
// in a web view of the window (package webui); a teammate's conversation
// there is the React Chat surface in the OneBot transport scope, as in
// the React window.
package onebot

import (
	"encoding/json"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/shell"
	"winone/desktopmygo/webui"
)

func init() {
	shell.Register("onebot", core.Conversation, func(env *core.Env) core.Page { return newPage(env) })
}

type page struct {
	env     *core.Env
	surface *webui.Surface
	browser *webui.Browser
}

func newPage(env *core.Env) *page {
	p := &page{env: env, browser: webui.NewBrowser(env)}
	p.surface = webui.New(env, webui.Options{View: "onebot", Label: i18n.T("onebot.title"),
		Receive: p.receive, Browser: p.browser})
	return p
}

// state is what the OneBot view is told: the window's language and
// theme. The page reads the AI services and Chat's catalog itself.
type state struct {
	Language string   `json:"language"`
	Theme    string   `json:"theme"`
	Host     struct{} `json:"host"`
}

// View shows the page. A change it asked of the Host and that is not
// answered yet keeps the window open, as in the Rust window.
func (p *page) View(c *ui.Context) {
	p.env.Activity.Report("onebot", false, p.surface.Busy(), nil)
	s := state{Language: i18n.Locale(), Theme: "light"}
	if c.Theme().Dark {
		s.Theme = "dark"
	}
	ui.Column(c).Fill().Children(func() {
		p.surface.View(c, s).Fill().Children(func() { p.browser.Overlay(c) })
	})
}

// receive answers the page's requests of the window: its links.
func (p *page) receive(kind string, payload json.RawMessage, reply func(any, error)) {
	switch kind {
	case "navigate":
		webui.Navigate(p.env, payload)
	case "drop":
		// A teammate's Chat takes its own drops.
	default:
		reply(nil, core.Errorf(i18n.T("chat.unsupportedOperation")))
		return
	}
	reply(nil, nil)
}

// Close destroys the web views.
func (p *page) Close() {
	p.browser.Close()
	p.surface.Close()
}
