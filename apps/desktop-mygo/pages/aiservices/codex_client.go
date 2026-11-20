package aiservices

import (
	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
)

// The upstream setting is shared by ChatGPT account signing, including model
// lists and quota reads. It is not a provider header preset that signing replaces.
func (e *editor) codexClientView(c *ui.Context, locked bool) {
	kit.Group(c, nil, i18n.T("aiSync.codexClient"), i18n.T("aiSync.codexClientHint"), func() {
		kit.Stack(c, "", "", func() {
			_, trigger := kit.Select(c, &e.p.draft.Value.settings().CodexClient,
				options("", "Codex CLI", "desktop", i18n.T("aiSync.codexDesktop")))
			trigger.FillWidth().Label(i18n.T("aiSync.codexClient")).Disabled(locked)
		})
	})
}
