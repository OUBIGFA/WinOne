package chat

import (
	"encoding/json"
	"net/url"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
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

func init() {
	shell.Register("chat", core.Conversation, func(env *core.Env) core.Page { return New(env) })
}
