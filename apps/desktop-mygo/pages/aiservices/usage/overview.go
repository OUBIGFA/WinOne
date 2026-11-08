package usage

import (
	"github.com/egoist/mygo/ui"
	"slices"

	"winone/desktopmygo/kit"
)

// breakdowns are React's UsageBreakdowns: Magpie's ranking of the period
// by source, model, account and provider, with shares, costs and failed
// counts. A ranking shows every group until folded to its top seven and
// "Other", so each stays one click from its requests.
func (u *Usage) breakdowns(c *ui.Context, page *ledgerPage) {
	p := kit.Colors(c)
	ui.Row(c).FillWidth().GapX(32).GapY(28).Wrap().AlignItems(ui.Start).Role(ui.RoleGroup).Label(tr("usage.breakdowns")).Children(func() {
		for _, kind := range []string{"agent", "model", "account", "provider"} {
			list := page.By[kind]
			if kind == "agent" && u.opts.Preferences != nil {
				list = slices.DeleteFunc(slices.Clone(list), func(row share) bool { return slices.Contains(u.opts.Preferences.HiddenAgents, row.ID) })
			}
			// Two rankings to a row, as React's grid had them, one in a
			// narrow window.
			ui.Column(c.Key(kind)).Basis(360).Grow(1).Shrink(1).MinWidth(0).Gap(8).Role(ui.RoleGroup).Label(tr("usage.group" + kind)).
				Children(func() {
					if kind == "agent" && u.opts.Change != nil && u.opts.Preferences != nil {
						ui.Collapsible(c, tr("aiComplete.shownClients"), &u.agentDisplayOpen, func() {
							for _, row := range page.By[kind] {
								shown := !slices.Contains(u.opts.Preferences.HiddenAgents, row.ID)
								if kit.Toggle(c.Key(row.ID), &shown, row.Name, "").Changed() {
									next := *u.opts.Preferences
									next.HiddenAgents = changeHidden(next.HiddenAgents, row.ID, !shown)
									u.opts.Change(next)
								}
							}
						})
					}
					ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Baseline).Padding(0, 0, 10, 0).BorderWidth(0, 0, 1, 0).
						BorderColor(p.Hairline).Children(func() {
						kit.Strong(c, tr("usage.group"+kind)).Role(ui.RoleHeading).Level(3).Grow(1).MinWidth(0)
						kit.Help(c, tr("usage.groupCount", "count", len(list))).Shrink(0)
					})
					if len(list) == 0 {
						kit.Help(c, tr("usage.empty")).Padding(18, 0)
						return
					}
					folded := u.folded[kind]
					shown := len(list)
					if folded {
						shown = ledShown
					}
					rankList(c, rankRows(list, nil, "tokens", shown, true), "", true, func(id string) { u.pick(kind, id) })
					if len(list) > ledShown {
						key := "usage.showTopGroups"
						if folded {
							key = "usage.showAllGroups"
						}
						if kit.Button(c, tr(key, "count", len(list)), nil, false).FillWidth().Clicked() {
							u.folded[kind] = !folded
						}
					}
				})
		}
	})
}
