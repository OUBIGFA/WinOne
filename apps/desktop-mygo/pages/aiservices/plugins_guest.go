package aiservices

import (
	"errors"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/web"
)

// Only the authenticated WinOne wrapper can send these bridge messages. The
// author's sandbox retains its original options/reads/action protocol; native
// provider navigation and folder picking must not disappear with the manager.
func (x *pluginsPanel) receiveGuest(m web.Message) {
	switch m.String("type") {
	case "winone-plugin-changed":
		x.changed()
	case "winone-plugin-status":
		if message := m.String("message"); message != "" && x.p.env.Toast != nil {
			x.p.env.Toast(message)
		}
	case "winone-plugin-native":
		id := m.String("id")
		if id == "" {
			return
		}
		guest := x.guest
		reply := func(result any, err error) {
			message := map[string]any{"type": "winone-plugin-native-result", "id": id, "result": result}
			if err != nil {
				message["error"] = err.Error()
			}
			if guest != nil && x.guest == guest {
				guest.Post(message)
			}
		}
		if x.p.locked() {
			reply(nil, errors.New(i18n.T("aiPlugins.busyAction")))
			return
		}
		switch m.String("method") {
		case "choose":
			core.Go(x.p.env, func() (string, error) { return x.p.env.Dialogs.PickFolder(i18n.T("aiPlugins.title")) }, func(path string, err error) { reply(path, err) })
		case "provider":
			id := m.String("provider")
			if id == "" {
				reply(nil, errors.New(i18n.T("aiPlugins.unsupportedAction")))
				return
			}
			name := m.String("name")
			if name == "" {
				name = id
			}
			signIn, _ := m["signIn"].(bool)
			reply(nil, x.p.openPluginProvider(agent{ID: id, Name: name, Backend: "plugin", Plugin: m.String("plugin")}, signIn))
		default:
			reply(nil, errors.New(i18n.T("aiPlugins.unsupportedAction")))
		}
	}
}
