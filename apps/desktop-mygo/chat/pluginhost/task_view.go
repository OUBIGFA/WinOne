package pluginhost

import (
	"encoding/json"
	"winone/desktopmygo/chat/pluginhost/globalpages"
	"winone/desktopmygo/chat/workspace"
	"winone/desktopmygo/core"
)

// Focus updates serialize; a slow earlier RPC must not win over the newest
// active task. Calling View repeatedly does not send repeated Host requests.
func (h *Host) FocusSession(id string) {
	h.focusedSession = id
	if !h.closed {
		h.pages.Session(id)
	}
	if h.closed || h.focusSending || h.focusSent == id {
		return
	}
	h.focusSending = true
	h.call("session-focus", map[string]string{"sessionId": id}, func(_ json.RawMessage, err error) {
		h.focusSending = false
		if err == nil {
			h.focusSent = id
		}
		if h.focusedSession != id {
			h.FocusSession(h.focusedSession)
		}
	})
}
func (h *Host) HasFocusedSession() bool { return h.focusedSession != "" }
func (h *Host) BlurSession(id string) {
	if h.focusedSession == id {
		h.FocusSession("")
	}
}
func (h *Host) TakeTaskViews(session string) []string {
	rows := h.taskViews[session]
	delete(h.taskViews, session)
	return rows
}

func NewTaskPane(env *core.Env, instance, session string) workspace.DocumentBody {
	return globalpages.NewTaskBody(env, instance, session)
}
