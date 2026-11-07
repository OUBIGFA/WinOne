package chat

import (
	"encoding/json"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/chat/pluginhost"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

// pluginRecommendations are the tasks enabled plugins suggest on a new conversation.
// Choosing one fills the draft; the user still sends it.
func (p *Chat) pluginRecommendations(c *ui.Context) {
	if !p.ui.recommendationsAsked {
		p.ui.recommendationsAsked = true
		host := pluginhost.For(p.env)
		host.OnPreview = func(payload json.RawMessage) {
			var preview struct {
				URL string `json:"url"`
			}
			if json.Unmarshal(payload, &preview) == nil && preview.URL != "" {
				p.openLink(preview.URL, "plugin")
			}
		}
		host.Recommendations(func(rows []pluginhost.Recommendation) { p.ui.recommendations = rows })
	}
	if len(p.ui.recommendations) == 0 {
		return
	}
	ui.Row(c).FillWidth().Gap(6).Wrap().Children(func() {
		for _, row := range p.ui.recommendations {
			if kit.GhostButton(c.Key(row.PluginID+"/"+row.ID), row.Label, icons.Sparkles, false).Clicked() {
				p.updateText(joinNonEmpty("\n\n", p.newDraft, row.Prompt), p.newFiles, "")
			}
		}
	})
}
