package chat

import (
	"encoding/json"
	"net/url"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/shell"

	"winone/internal/branding"
)

// The page's image of a new conversation, decoded once.
var welcomeBitmap, _ = ui.DecodeBitmap(branding.ChatWelcome)

func welcomeLogo() *ui.Bitmap { return welcomeBitmap }

// safeLink accepts only web addresses with a host and no credentials, as
// the replies' links of the result window.
func safeLink(value string) bool {
	u, err := url.Parse(value)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != "" && u.User == nil && ui.SafeToOpen(value)
}

func jsonIndent(v any) (string, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	return string(data), err
}

// openSettings shows Chat's settings.
func (p *Chat) openSettings() { p.ui.settings = true }

// automationsView is the scheduled tasks, which Cindy's automation page
// shows in a web view.
func (p *Chat) automationsView(c *ui.Context) {
	kit.Empty(c, icons.Timer, i18n.T("chat.automations"), i18n.T("shell.unavailableHint"), func() {
		if kit.Button(c, i18n.T("common.back"), nil, false).Clicked() {
			p.ui.automations = false
		}
	})
}

func init() {
	shell.Register("chat", core.Conversation, func(env *core.Env) core.Page { return New(env) })
}
