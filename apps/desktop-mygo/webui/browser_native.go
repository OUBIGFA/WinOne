package webui

import (
	"encoding/json"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
)

// NativeRequest uses the identical isolated page/session/guest script and
// navigation checks for every tab. The native panel supplies the chrome.
func (b *Browser) NativeRequest(method string, values map[string]any) (any, error) {
	raw, err := json.Marshal(values)
	if err != nil {
		return nil, err
	}
	return b.request(method, raw)
}
func (b *Browser) NativeView(c *ui.Context, sessionID, tabID string, visible bool) ui.Element {
	key, err := tabKey(browserPayload{SessionID: sessionID, TabID: tabID})
	root := ui.Column(c).Fill().MinWidth(0).MinHeight(0).Children(func() {
		if err != nil {
			ui.Text(c, err.Error())
			return
		}
		tab := b.tabs[key]
		if tab == nil || tab.view == nil {
			ui.Text(c, i18n.T("chat.browser.notReady"))
			return
		}
		tab.state.Visible = visible
		if !visible {
			return
		}
		b.pump(tab)
		ui.WebView(c, tab.view).Fill().Label(first(tab.state.Title, tab.state.URL))
	})
	return root
}
func (b *Browser) SetNativeVisible(sessionID, tabID string, visible bool) {
	key, err := tabKey(browserPayload{SessionID: sessionID, TabID: tabID})
	if err != nil {
		return
	}
	if tab := b.tabs[key]; tab != nil {
		tab.state.Visible = visible
		if visible {
			b.pump(tab)
		}
	}
}
