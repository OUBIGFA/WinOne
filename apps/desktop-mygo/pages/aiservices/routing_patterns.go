package aiservices

import (
	"encoding/json"
	"slices"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

type patternMatch struct {
	Pattern string `json:"pattern"`
	Models  int    `json:"models"`
}

func (r *routingPanel) patternsView(c *ui.Context, g *routeGroup, locked bool) {
	if kit.Button(c, nativeT("previewPatterns"), icons.Search, false).Disabled(locked || r.patternAPI.busy).Clicked() {
		patterns := slices.Clone(g.Match)
		r.patternAPI.request(r.p, "POST", "/api/groups/preview", r.edit, func(raw json.RawMessage) {
			var value struct {
				Patterns []patternMatch `json:"patterns"`
			}
			if nativeDecode(&r.patternAPI, raw, &value) {
				r.previewPatterns, r.previewFor = value.Patterns, patterns
			}
		})
	}
	r.patternAPI.notice(c, nil)
	if slices.Equal(g.Match, r.previewFor) {
		for _, hit := range r.previewPatterns {
			kit.Help(c, i18n.T("aiNative.patternMatches", "pattern", hit.Pattern, "count", hit.Models))
		}
	}
}
