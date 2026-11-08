package aiservices

import (
	"slices"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/internal/ai"
)

// Latest settings share the existing saved draft. Neither an editor redraw nor
// an automatic runtime refresh may opt into an operation or rewrite a value.
func (a *advancedPanel) latestSearchOptions(c *ui.Context, locked bool) {
	s := a.settings()
	a.effortField(c, "searcherEffort", &s.SearcherEffort, locked)
	kit.Toggle(c, &s.SearchAll, nativeT("searchAll"), nativeT("searchAllHint")).Disabled(locked)
	nativeList(c, a.fields, "searcherThen", nativeT("searchFallbacks"), &s.SearcherThen, locked)
}

func (a *advancedPanel) latestReplyOptions(c *ui.Context, locked bool) {
	s := a.settings()
	model := s.ReplyModel
	if model == "" && s.MemberModel {
		model = "member"
	}
	if nativeSelect(c, nativeT("replyModelName"), &model, options("", nativeT("replyAsked"), "member", nativeT("replyMember"), "vendor", nativeT("replyVendor")), locked) {
		s.ReplyModel, s.MemberModel = model, model == "member"
	}
	numberRow(c, nativeT("refusalRetries"), s.RefusalRetries, -1, 1000000, true, "", locked, func(value *float64) { s.RefusalRetries = value })
	kit.Help(c, nativeT("refusalRetriesHint"))
	kit.Toggle(c, &s.ClaudePrintMode, nativeT("claudePrintMode"), nativeT("claudePrintModeHint")).Disabled(locked)
}

func (a *advancedPanel) clientCompacts(c *ui.Context, locked bool) {
	s := a.settings()
	kit.Help(c, nativeT("agentCompactsHint"))
	for _, id := range []string{"codex", "claude", "grok"} { // settings.CompactAgents
		ui.Column(c.Key(id)).FillWidth().Gap(6).Children(func() {
			compact, own := s.AgentCompacts[id]
			if kit.Toggle(c, &own, nativeT("agentCompact")+" · "+id, "").Disabled(locked).Changed() {
				if s.AgentCompacts == nil {
					s.AgentCompacts = map[string]ai.AgentCompact{}
				}
				if own {
					compact = ai.AgentCompact{At: intNumber(s.CompactAt), Full: s.FullContext}
					s.AgentCompacts[id] = compact
				} else {
					delete(s.AgentCompacts, id)
				}
			}
			if !own {
				return
			}
			full := compact.Full
			if kit.Toggle(c, &full, nativeT("wholeWindow"), "").Disabled(locked).Changed() {
				compact.Full = full
				s.AgentCompacts[id] = compact
			}
			if !full {
				at := compact.At
				nativeInt(c, nativeT("compactTokens"), &at, 0, 100000000, locked)
				if at != compact.At {
					compact.At = at
					s.AgentCompacts[id] = compact
				}
			}
		})
	}
}

func (a *advancedPanel) mcpRouterOptions(c *ui.Context, locked bool) {
	s := a.settings()
	kit.Group(c, icons.Waypoints, nativeT("mcpRouter"), nativeT("mcpRouterHint"), func() {
		nativeText(c, nativeT("routerNote"), &s.RouterNote, locked)
		kit.Toggle(c, &s.RouterAllTools, nativeT("routerAllTools"), nativeT("routerAllToolsHint")).Disabled(locked)
	})
}

func (a *advancedPanel) creditPriority(c *ui.Context, account string, locked bool) {
	s := a.settings()
	priority := ""
	if slices.Contains(s.CodexCreditsFirst, account) {
		priority = "first"
	} else if slices.Contains(s.CodexCreditsLast, account) {
		priority = "last"
	}
	if nativeSelect(c, nativeT("creditPriority"), &priority, options("", nativeT("creditNormal"), "first", nativeT("creditFirst"), "last", nativeT("creditLast")), locked) {
		s.CodexCreditsFirst = toggleString(s.CodexCreditsFirst, account, priority == "first")
		s.CodexCreditsLast = toggleString(s.CodexCreditsLast, account, priority == "last")
	}
}
