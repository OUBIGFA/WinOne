package webui

import (
	"encoding/json"

	"winone/desktopmygo/core"
)

// navigation is a link of a React page into the window, as main.tsx took
// the winone-open-chat and winone-open-page events: a Chat conversation
// (at a file of it), Chat settings, or another page.
type navigation struct {
	To      string           `json:"to"` // "conversation", "new-chat" or "page"
	ID      string           `json:"id"`
	Text    string           `json:"text"` // a new chat's prefill
	Section string           `json:"section"`
	File    *core.IntentFile `json:"file"`
}

// linkedPages are the pages the React pages link to: AI services from
// Cindy's provider prompts, OneBot's IM settings, Chat.
var linkedPages = map[string]bool{"ai-services": true, "onebot": true, "chat": true}

// Navigate follows a page's link through the shell's unsaved-change
// guard. A file must be a plain path, as main.tsx checked it; links to
// other pages are ignored.
func Navigate(env *core.Env, payload json.RawMessage) {
	var n navigation
	if json.Unmarshal(payload, &n) != nil {
		return
	}
	switch {
	case n.To == "conversation" && n.ID != "":
		intent := core.Intent{Conversation: n.ID}
		if f := n.File; f != nil && f.Path != "" && len(f.Path) <= 32768 && !containsNUL(f.Path) && f.Line >= 0 {
			intent.File = f
		}
		env.Open("chat", intent)
	case n.To == "new-chat":
		env.Open("chat", core.Intent{NewChat: true, Prefill: n.Text})
	case n.To == "page" && n.ID == "chat" && n.Section == "settings":
		env.Open("chat", core.Intent{ChatSettings: true})
	case n.To == "page" && linkedPages[n.ID]:
		env.Navigate(n.ID, nil)
	}
}

func containsNUL(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == 0 {
			return true
		}
	}
	return false
}
