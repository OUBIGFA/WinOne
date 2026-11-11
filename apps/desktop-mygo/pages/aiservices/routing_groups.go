package aiservices

import (
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/widgets"
)

func nativeT(key string) string { return i18n.T("aiNative." + key) }

type draggedRouteGroup struct{ id string }
type draggedRouteMember struct{ id string }
type draggedRouteRule struct{ index int }

func (r *routingPanel) groupsView(c *ui.Context) {
	locked := r.api.busy || r.p.locked()
	// A focused editor shares the page width and draft, without nesting a
	// settings form inside a list card or pushing the other groups far down.
	if r.edit != nil {
		r.editor(c)
		return
	}
	ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
		kit.TextInput(c, &r.search).Grow(1).MinWidth(180).Placeholder(nativeT("searchGroups")).Label(nativeT("searchGroups"))
		if kit.PrimaryButton(c, nativeT("newGroup"), icons.Plus, false).Disabled(locked).Clicked() {
			r.beginNew(core.Preserved[routeGroup]{Value: routeGroup{Members: []string{}}})
		}
		if kit.Button(c, nativeT("import"), icons.Import, false).Disabled(locked).Clicked() {
			r.dialog = "import"
			r.text = ""
			r.replaceImport = false
		}
		if kit.IconButton(c, icons.RefreshCw, i18n.T("common.refresh")).Disabled(locked).Clicked() {
			r.load()
		}
	})
	if r.imported != nil {
		result := r.imported
		kit.Group(c, icons.Import, i18n.T("aiRouting.imported", "name", result.Name), "", func() {
			ui.Column(c).FillWidth().Gap(8).Padding(12, 16).Children(func() {
				kit.Help(c, i18n.T("aiRouting.importedRules", "count", result.Rules))
				if result.Replaced {
					kit.Help(c, i18n.T("aiRouting.importedReplaced"))
				}
				if len(result.Missing) > 0 {
					kit.Help(c, i18n.T("aiRouting.importedMissing", "models", r.memberNames(result.Missing)))
				}
				if len(result.Instead) > 0 {
					kit.Help(c, i18n.T("aiRouting.importedInstead", "models", r.memberNames(result.Instead)))
				}
				if kit.GhostButton(c, i18n.T("common.close"), nil, false).Clicked() {
					r.imported = nil
				}
			})
		})
	}
	ui.Row(c).FillWidth().Gap(8).Wrap().AlignItems(ui.Center).Children(func() {
		if kit.GhostButton(c, nativeT("selectAll"), nil, false).Clicked() {
			r.selectVisible()
		}
		if kit.GhostButton(c, nativeT("selectNone"), nil, false).Clicked() {
			clear(r.selected)
		}
		if len(r.selected) > 0 && kit.GhostButton(c, nativeT("deleteSelected"), icons.Trash2, false).Disabled(locked).Clicked() {
			r.dialog = "delete-selected"
		}
		ui.Spacer(c).Grow(1)
		ui.Checkbox(c, &r.showHidden, i18n.T("aiRouting.showHidden"))
	})
	// A single list surface gives peer groups one rhythm. Keeping each row
	// inside it avoids inheriting the page's large gap between settings groups.
	kit.Panel(c, func() {
		for index, doc := range r.state.Groups {
			g := doc.Value
			if !r.groupVisible(g) {
				continue
			}
			row := ui.Column(c.Key(g.ID)).FillWidth().Gap(8).Padding(14, 16).BorderWidth(0, 0, 1, 0).BorderColor(kit.Colors(c).Hairline)
			if item, ok := ui.Drop[draggedRouteGroup](row); ok && !locked {
				from := slices.IndexFunc(r.state.Groups, func(v core.Preserved[routeGroup]) bool { return v.Value.ID == item.id })
				if from >= 0 && from != index {
					r.moveGroup(from, index-from)
				}
			}
			row.Children(func() {
				ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
					handle := kit.IconButton(c, icons.GripVertical, upstreamT("Move {name}", "name", g.Name)).Disabled(locked)
					if !locked {
						handle.Drag(draggedRouteGroup{g.ID})
						if handle.Shortcut(ui.Alt, ui.KeyUp) && index > 0 {
							r.moveGroup(index, -1)
						}
						if handle.Shortcut(ui.Alt, ui.KeyDown) && index+1 < len(r.state.Groups) {
							r.moveGroup(index, 1)
						}
					}
					selected := r.selected[g.ID]
					if ui.Checkbox(c, &selected, "").Label(nativeT("select") + " " + g.Name).Changed() {
						if selected {
							r.selected[g.ID] = true
						} else {
							delete(r.selected, g.ID)
						}
					}
					ui.Row(c).Grow(1).MinWidth(0).GapX(12).GapY(4).Wrap().AlignItems(ui.Center).Children(func() {
						kit.Label(c, g.Name).FontSize(16).FontWeight(500).MinWidth(0).Selectable()
						kit.Help(c, r.strategyLabel(g.Routing))
					})
					status := "routeReady"
					if g.Disabled || g.Hidden {
						status = "routeDisabled"
					} else if !g.Ready {
						status = "routeUnavailable"
					}
					kit.Help(c, nativeT(status)).Shrink(0)
					on := !g.Disabled && !g.Hidden
					if ui.Switch(c, &on).Label(nativeT("enabled") + " " + g.Name).Disabled(locked).Changed() {
						if g.Hidden && on {
							r.act("show", map[string]string{"id": g.ID}, nil)
						} else {
							r.act("switch", map[string]any{"id": g.ID, "on": on}, nil)
						}
					}
					if kit.Button(c, nativeT("edit"), icons.Pencil, false).Disabled(locked).Clicked() {
						r.begin(doc)
					}
					kit.IconButton(c, icons.Ellipsis, nativeT("groupActions")+" "+g.Name).Disabled(locked).Menu(func(m *ui.Menu) {
						if m.Item(nativeT("copy")).Chosen() {
							r.dialog = "copy"
							r.text = g.ID
							r.name = g.Name + " copy"
						}
						if m.Item(nativeT("share")).Chosen() {
							r.act("share", map[string]string{"id": g.ID}, func() { r.dialog = "share"; r.text = r.state.Shared })
						}
						m.Separator()
						if m.Item(nativeT("moveUp")).Disabled(index == 0).Chosen() {
							r.moveGroup(index, -1)
						}
						if m.Item(nativeT("moveDown")).Disabled(index == len(r.state.Groups)-1).Chosen() {
							r.moveGroup(index, 1)
						}
						m.Separator()
						if m.Item(nativeT("delete")).Chosen() {
							r.dialog = "delete"
							r.text = g.ID
						}
					})
				})
				ui.Row(c).FillWidth().GapX(20).GapY(6).Wrap().Padding(0, 0, 0, 28).Children(func() {
					for _, member := range g.Members {
						kit.Label(c, r.memberLabel(member)).FontSize(14).TextColor(c.Theme().TextMuted).MinWidth(0).Selectable()
					}
				})
				if len(g.Match) > 0 {
					kit.Help(c, nativeT("patterns")+": "+strings.Join(g.Match, " · ")).Selectable()
				}
				for _, pattern := range g.Patterns {
					kit.Help(c, i18n.T("aiNative.patternMatches", "pattern", pattern.Pattern, "count", pattern.Models)).Selectable()
				}
				if g.Routing == "manual" {
					pick := g.Pick
					if nativeSelect(c, nativeT("manualPick"), &pick, r.memberChoices(g.Members), locked) {
						r.act("pick", map[string]string{"id": g.ID, "pick": pick}, nil)
					}
				}
			})
		}
	})
	if len(r.state.Groups) == 0 {
		kit.Empty(c, icons.Waypoints, nativeT("noGroups"), nativeT("templatesHint"), nil)
	}
	toolsOpen := ui.Local(ui.Column(c), "group-tools", func() bool { return len(r.state.Groups) == 0 })
	ui.Collapsible(c, nativeT("groupTools"), toolsOpen, func() { r.discoveryView(c, locked) })
	if r.p.draft != nil {
		defaultsOpen := ui.Local(ui.Column(c), "routing-defaults", func() bool { return false })
		ui.Collapsible(c, nativeT("groupDefaults"), defaultsOpen, func() {
			s := r.p.draft.Value.settings()
			modelNameSuffix(c, s, locked)
			kit.Help(c, nativeT("groupDefaultsHint"))
			kit.Toggle(c, &s.GroupSink, nativeT("sink"), nativeT("sinkHint")).Disabled(locked)
			nativeInt(c, nativeT("firstToken"), &s.GroupFirstToken, 0, 3600, locked)
			kit.Help(c, nativeT("defaultFirstTokenHint"))
			nativeInt(c, nativeT("patience"), &s.GroupPatience, -1, 240, locked)
			kit.Help(c, nativeT("defaultPatienceHint"))
		})
	}

}

func (r *routingPanel) groupVisible(g routeGroup) bool {
	return (r.showHidden || !g.Hidden) && strings.Contains(strings.ToLower(g.Name+" "+g.ID+" "+r.memberNames(g.Members)), strings.ToLower(r.search))
}
func (r *routingPanel) selectVisible() {
	clear(r.selected)
	for _, g := range r.state.Groups {
		if r.groupVisible(g.Value) {
			r.selected[g.Value.ID] = true
		}
	}
}

// routingSummary answers the first question on the page—whether routing is
// ready—before the editable group list. It uses the same upstream snapshot as
// the rows, so the counts never become a second source of truth.
func (r *routingPanel) routingSummary(c *ui.Context, locked bool) {
	ready := 0
	for _, group := range r.state.Groups {
		if group.Value.Ready && !group.Value.Disabled && !group.Value.Hidden {
			ready++
		}
	}
	kit.Group(c, icons.Waypoints, nativeT("overview"), "", func() {
		ui.Row(c).FillWidth().Padding(12, 16).Gap(12).Wrap().AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).MinWidth(220).Children(func() {
				kit.Strong(c, i18n.T("aiRouting.groupSummary", "count", len(r.state.Groups), "ready", ready))
				if len(r.state.Pools) > 0 {
					kit.Help(c, i18n.T("aiRouting.poolSummary", "count", len(r.state.Pools)))
				} else {
					kit.Help(c, i18n.T("aiRouting.noPoolsHint"))
				}
			})
			if kit.GhostButton(c, nativeT("requests"), icons.Waypoints, false).Disabled(locked).Clicked() {
				r.tab = 1
			}
		})
	})
}
func (r *routingPanel) moveGroup(index, delta int) {
	var order []string
	for _, g := range r.state.Groups {
		order = append(order, g.Value.ID)
	}
	order = moveRouteItem(order, index, index+delta)
	r.act("arrange", map[string]any{"order": order}, nil)
}

// Insertion, rather than swapping, preserves the intervening fallback order.
func moveRouteItem(items []string, from, to int) []string {
	if from < 0 || to < 0 || from >= len(items) || to >= len(items) || from == to {
		return items
	}
	item := items[from]
	items = slices.Delete(items, from, from+1)
	return slices.Insert(items, to, item)
}
func stringChoices(values []string) []kit.SelectOption {
	out := make([]kit.SelectOption, 0, len(values))
	for _, v := range values {
		out = append(out, kit.SelectOption{Value: v, Label: v})
	}
	return out
}

func (r *routingPanel) dialogView(c *ui.Context) {
	if r.dialog == "" {
		return
	}
	kind := r.dialog
	open := true
	confirm := nativeT("confirm")
	if kind == "share" {
		confirm = ""
	}
	if kit.Dialog(c, &open, kit.DialogOptions{Title: nativeT(kind), Confirm: confirm, Cancel: i18n.T("common.cancel"), Danger: strings.HasPrefix(kind, "delete"), Busy: r.api.busy, Error: r.api.err, Width: 640, Body: func() {
		switch kind {
		case "copy":
			nativeText(c, nativeT("groupName"), &r.name, r.api.busy)
		case "import", "share":
			ui.TextArea(c, &r.text).FillWidth().MinHeight(160).ReadOnly(kind == "share").Label(nativeT(kind)).Font(kit.MonoFont)
			if kind == "import" {
				kit.Toggle(c, &r.replaceImport, nativeT("replaceImportedGroup"), nativeT("replaceImportedGroupHint")).Disabled(r.api.busy)
			}
			if kind == "share" && kit.Button(c, nativeT("copy"), icons.Copy, false).Clicked() {
				c.Services().WriteClipboard(r.text)
			}
		case "delete", "delete-selected":
			kit.Help(c, nativeT("deleteGroupHint"))
		}
	}}) {
		done := func() { r.dialog = ""; clear(r.selected) }
		switch kind {
		case "copy":
			r.act("copy", map[string]string{"id": r.text, "name": r.name}, done)
		case "import":
			r.act("import", map[string]any{"text": r.text, "replace": r.replaceImport}, done)
		case "delete":
			r.act("delete", map[string]string{"id": r.text}, done)
		case "delete-selected":
			var ids []string
			for id := range r.selected {
				ids = append(ids, id)
			}
			r.act("delete", map[string]any{"ids": ids}, done)
		}
	}
	if !open {
		r.dialog = ""
	}
}

func (r *routingPanel) editor(c *ui.Context) {
	if r.edit == nil {
		return
	}
	g := &r.edit.Value
	mask := r.accountMasker()
	locked := r.api.busy || r.p.locked()
	ui.Row(c).FillWidth().Gap(8).Wrap().Margin(0, 0, 16, 0).Children(func() {
		kit.Strong(c, nativeT("editGroup")).Grow(1)
		if kit.PrimaryButton(c, nativeT("saveGroup"), icons.Save, r.api.busy).Disabled(locked || !r.dirty()).Clicked() {
			r.saveGroup()
		}
		if kit.Button(c, i18n.T("common.cancel"), nil, false).Disabled(locked).Clicked() {
			if r.dirty() && r.p.env.RequestChange != nil {
				r.p.env.RequestChange(r.discard)
			} else {
				r.discard()
			}
		}
	})

	// Each section edits the same draft; switching sections never saves or
	// discards it. All upstream options remain reachable without a long form.
	if r.edit == nil {
		return
	} // Cancel can discard the draft in this frame.
	kit.SubTabs(c, &r.editorTab, []string{i18n.T("aiRouting.basics"), nativeT("members"), nativeT("rules"), i18n.T("aiRouting.advanced"), nativeT("accounts")}, nil)
	ui.Column(c).FillWidth().Gap(kit.GroupGap).Children(func() {
		switch r.editorTab {
		case 0:
			kit.Section(c, nil, i18n.T("aiRouting.basics"), "", func() {
				ui.Column(c).FillWidth().Gap(16).Children(func() {
					nativeText(c, nativeT("groupName"), &g.Name, locked)
					if r.newGroup {
						kit.Help(c, upstreamT("Agents pick it as {id}", "id", "group/"+r.groupID())).Selectable()
					} else {
						nativeText(c, nativeT("groupID"), &g.ID, locked)
						if r.groupID() != r.saved.Value.ID {
							kit.Help(c, upstreamT("an agent set to {id} needs setting again", "id", r.saved.Value.Ref))
						}
					}
					nativeSelect(c, nativeT("strategy"), &g.Routing, r.routingChoices(g.Routing), locked)
					kit.Help(c, nativeT("strategyHint"))
					if strings.Contains(g.Routing, "/") {
						nativeText(c, nativeT("pluginStrategy"), &g.Routing, locked)
					}
				})
			})
			r.memberSummary(c, g)
		case 1:
			r.membersView(c, g, locked)
			ui.Column(c).FillWidth().Gap(8).Children(func() {
				nativeList(c, r.fields, "match", nativeT("patterns"), &g.Match, locked)
				r.patternsView(c, g, locked)
			})
		case 2:
			r.rulesView(c, g, locked)
		case 3:
			kit.Group(c, nil, i18n.T("aiRouting.advanced"), "", func() {
				kit.Stack(c, "", "", func() {
					nativeSelect(c, nativeT("affinity"), &g.Affinity, options("", nativeT("auto"), "session", nativeT("affinitySession"), "turn", nativeT("affinityTurn"), "off", nativeT("off"), "strict", nativeT("affinityStrict")), locked)
				})
				kit.Stack(c, "", nativeT("sinkHint"), func() {
					mode := "default"
					if g.Sink {
						mode = "sink"
					} else if g.NoSink {
						mode = "keep"
					}
					if nativeSelect(c, nativeT("sink"), &mode, options("default", nativeT("followDefaults"), "sink", nativeT("sink"), "keep", nativeT("keepPlace")), locked) {
						g.Sink, g.NoSink = mode == "sink", mode == "keep"
					}
				})
				kit.Stack(c, "", nativeT("firstTokenDefaultHint"), func() { nativeInt(c, nativeT("firstToken"), &g.FirstToken, -1, 3600, locked) })
				kit.Stack(c, "", nativeT("patienceDefaultHint"), func() { nativeInt(c, nativeT("patience"), &g.Patience, -1, 240, locked) })
				kit.Stack(c, "", nativeT("contextHint"), func() { nativeInt(c, nativeT("context"), &g.Context, -1, 10000000, locked) })
				kit.Stack(c, "", "", func() { nativeText(c, nativeT("family"), &g.Family, locked) })
				kit.Toggle(c, &g.Unlists, nativeT("unlistMembers"), "").Disabled(locked)
				kit.Stack(c, "", "", func() { nativeList(c, r.fields, "levels", nativeT("offeredLevels"), &g.Levels, locked) })
			})
		case 4:
			kit.Section(c, icons.Users, nativeT("accounts"), nativeT("accountsHint"), func() {
				for _, provider := range sortedKeys(r.state.Accounts) {
					kit.Section(c, nil, r.providerLabel(provider), "", func() {
						for _, account := range r.state.Accounts[provider] {
							id := account.ID
							on := slices.Contains(g.Accounts, id)
							if ui.Checkbox(c.Key(id), &on, mask(account.Name)).Disabled(locked).Changed() {
								g.Accounts = toggleString(g.Accounts, id, on)
							}
						}
					})
				}
			})
		}
	})
}

func (r *routingPanel) membersView(c *ui.Context, g *routeGroup, locked bool) {
	kit.Group(c, icons.Layers, nativeT("members"), nativeT("membersHint"), func() {
		moveFrom, moveTo := -1, -1
		if g.Routing == "weight" {
			if kit.GhostButton(c, upstreamT("Sort by weight"), icons.ArrowDown, false).Disabled(locked).Clicked() {
				slices.SortStableFunc(g.Members, func(a, b string) int { return max(1, g.Weights[b]) - max(1, g.Weights[a]) })
				g.Placed = slices.Clone(g.Members)
			}
		}
		for i, id := range g.Members {
			row := ui.Column(c.Key(id)).FillWidth().Gap(6).Padding(12, 16)
			if item, ok := ui.Drop[draggedRouteMember](row); ok && !locked {
				moveFrom, moveTo = slices.Index(g.Members, item.id), i
			}
			row.Children(func() {
				ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
					handle := kit.IconButton(c, icons.GripVertical, upstreamT("Move {name}", "name", r.memberLabel(id))).Disabled(locked)
					if !locked {
						handle.Drag(draggedRouteMember{id})
						if handle.Shortcut(ui.Alt, ui.KeyUp) {
							moveFrom, moveTo = i, i-1
						}
						if handle.Shortcut(ui.Alt, ui.KeyDown) {
							moveFrom, moveTo = i, i+1
						}
					}
					on := !slices.Contains(g.Off, id)
					if ui.Checkbox(c, &on, r.memberLabel(id)).Grow(1).MinWidth(0).Disabled(locked).Changed() {
						g.Off = toggleString(g.Off, id, !on)
					}
					if kit.IconButton(c, icons.ArrowUp, nativeT("moveUp")+" "+r.memberLabel(id)).Disabled(locked || i == 0).Clicked() {
						moveFrom, moveTo = i, i-1
					}
					if kit.IconButton(c, icons.ArrowDown, nativeT("moveDown")+" "+r.memberLabel(id)).Disabled(locked || i == len(g.Members)-1).Clicked() {
						moveFrom, moveTo = i, i+1
					}
					if kit.IconButton(c, icons.X, nativeT("remove")+" "+r.memberLabel(id)).Disabled(locked).Clicked() {
						removeRouteMember(g, id)
						return
					}
				})
				if !slices.Contains(g.Members, id) {
					return
				}
				r.memberEffort(c, g, id, locked)
				fast := slices.Contains(g.Fast, id)
				if kit.Toggle(c, &fast, nativeT("fastMode"), "").Disabled(locked).Changed() {
					g.Fast = toggleString(g.Fast, id, fast)
				}
				if g.Routing == "weight" {
					weight := g.Weights[id]
					if weight == 0 {
						weight = 1
					}
					if nativeInt(c, nativeT("weight"), &weight, 1, 1000, locked) {
						if g.Weights == nil {
							g.Weights = map[string]int{}
						}
						g.Weights[id] = weight
					}
				}
			})
		}
		if g.Routing == "manual" {
			kit.Stack(c, "", "", func() { nativeSelect(c, nativeT("manualPick"), &g.Pick, r.memberChoices(g.Members), locked) })
		}
		g.Members = moveRouteItem(g.Members, moveFrom, moveTo)
		if moveFrom >= 0 && moveTo >= 0 && moveTo < len(g.Members) {
			g.Placed = slices.Clone(g.Members)
		}
		kit.Stack(c, "", "", func() {
			kit.TextInput(c, &r.memberSearch).FillWidth().Placeholder(nativeT("searchModels")).Label(nativeT("searchModels"))
		})
		all := append(append([]routeModel{}, r.state.Models...), r.state.Drawers...)
		for _, other := range r.state.Groups {
			if other.Value.ID != g.ID && other.Value.Ref != "" {
				all = append(all, routeModel{ID: other.Value.Ref, Name: other.Value.Name})
			}
		}
		ui.Scroll(c).FillWidth().MaxHeight(240).Padding(8, 16).Children(func() {
			for _, model := range all {
				if r.memberSearch != "" && !strings.Contains(strings.ToLower(model.ID+" "+model.Name+" "+model.ProviderName), strings.ToLower(r.memberSearch)) {
					continue
				}
				on := slices.Contains(g.Members, model.ID)
				if ui.Checkbox(c.Key(model.ID), &on, r.memberLabel(model.ID)).Disabled(locked).Changed() {
					if on {
						g.Members = toggleString(g.Members, model.ID, true)
					} else {
						removeRouteMember(g, model.ID)
					}
				}
			}
		})
		// A literal member field keeps plugin-defined references and effort
		// suffixes usable, including subsets such as :low+high, without a host
		// whitelist. Upstream validates references and cyclic groups at save.
		manual := ui.Local(ui.Column(c), "manual-reference", func() bool { return false })
		ui.Collapsible(c, nativeT("customReference"), manual, func() {
			nativeText(c, nativeT("memberReference"), &r.newMember, locked)
			if kit.Button(c, nativeT("addMember"), icons.Plus, false).Disabled(locked || strings.TrimSpace(r.newMember) == "").Clicked() {
				g.Members = toggleString(g.Members, strings.TrimSpace(r.newMember), true)
				r.newMember = ""
			}
		}).Padding(12, 16)
		kit.Stack(c, "", "", func() { r.memberOrderView(c, nativeT("memberOrder"), &g.Placed, g.Matched, locked) })
	})
}

func (r *routingPanel) rulesView(c *ui.Context, g *routeGroup, locked bool) {
	kit.Section(c, icons.ListFilter, nativeT("rules"), nativeT("rulesHint"), func() {
		all := append(append([]routeModel{}, r.state.Models...), r.state.Deciders...)
		for _, other := range r.state.Groups {
			if other.Value.ID != g.ID {
				all = append(all, routeModel{ID: other.Value.Ref, Name: other.Value.Name, Provider: "group", ProviderName: nativeT("groups")})
			}
		}
		kit.Panel(c, func() {
			kit.Stack(c, "", "", func() {
				nativeModelField(c, r.p, nativeT("classifier"), &g.Classifier, all, []widgets.ModelChoice{{Value: "", Label: nativeT("none")}}, locked)
			})
			var n float64
			if g.IntentSure != nil {
				n = *g.IntentSure
			} else {
				n = .4
			}
			numberRow(c, nativeT("intentSure"), &n, 0, 1, false, "0.4", locked, func(v *float64) { g.IntentSure = v })
			choiceRow(c, nativeT("automaticEffort"), "", &g.Effort, options("", nativeT("off"), "auto", nativeT("auto")), locked)
		})
		remove := -1
		moveFrom, moveTo := -1, -1
		for i := range g.Rules {
			rule := &g.Rules[i].Value
			row := ui.Column(c.Key(i)).FillWidth().Gap(8).Padding(12, 0).BorderWidth(1, 0, 0, 0).BorderColor(kit.Colors(c).Hairline)
			if item, ok := ui.Drop[draggedRouteRule](row); ok && !locked {
				moveFrom, moveTo = item.index, i
			}
			row.Children(func() {
				open := ui.Local(ui.Column(c), "editing", func() bool { return len(g.Rules) == 1 })
				section := ui.Local(ui.Column(c), "section", func() int { return 0 })
				ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
					handle := kit.IconButton(c, icons.GripVertical, upstreamT("Move {name}", "name", nativeT("rule")+" "+kit.Itoa(i+1))).Disabled(locked)
					if !locked {
						handle.Drag(draggedRouteRule{i})
						if handle.Shortcut(ui.Alt, ui.KeyUp) {
							moveFrom, moveTo = i, i-1
						}
						if handle.Shortcut(ui.Alt, ui.KeyDown) {
							moveFrom, moveTo = i, i+1
						}
					}
					ui.Column(c).Grow(1).MinWidth(0).Children(func() {
						kit.Strong(c, nativeT("rule")+" "+kit.Itoa(i+1))
						kit.Help(c, strings.TrimSpace(r.memberLabel(rule.Use)+" "+rule.Intent))
					})
					if kit.IconButton(c, icons.Pencil, i18n.T("aiRouting.editRule")).Disabled(locked).Clicked() {
						*open = !*open
					}
					if kit.IconButton(c, icons.ArrowUp, nativeT("moveUp")).Disabled(locked || i == 0).Clicked() {
						moveFrom, moveTo = i, i-1
					}
					if kit.IconButton(c, icons.ArrowDown, nativeT("moveDown")).Disabled(locked || i == len(g.Rules)-1).Clicked() {
						moveFrom, moveTo = i, i+1
					}
					if kit.IconButton(c, icons.Trash2, nativeT("deleteRule")).Disabled(locked).Clicked() {
						remove = i
					}
				})
				if !*open {
					return
				}
				// A rule is one draft. Tabs expose its action, matching conditions
				// and scope without interleaving three unrelated sets of fields.
				kit.SubTabs(c, section, []string{i18n.T("aiRouting.ruleAction"), i18n.T("aiRouting.ruleConditions"), i18n.T("aiRouting.ruleScope")}, nil)
				kit.Panel(c, func() {
					prefix := "rule-" + kit.Itoa(i) + ":"
					switch *section {
					case 0:
						kit.Stack(c, "", "", func() {
							nativeSelect(c, nativeT("useMember"), &rule.Use, r.memberChoices(g.Members), locked)
						})
						if kit.Toggle(c, &rule.Pause, nativeT("pauseMember"), "").Disabled(locked).Changed() && rule.Pause {
							// Pauses have no outgoing request. Hidden routing-only
							// conditions must not make this otherwise valid rule unsaveable.
							rule.More, rule.UseEffort = nil, ""
							rule.Tokens, rule.Images, rule.Compact = 0, false, false
							rule.Effort, rule.Intent = "", ""
						}
						if !rule.Pause {
							kit.Stack(c, "", "", func() {
								r.memberOrderView(c, nativeT("thenMembers"), &rule.More, g.Members, locked)
								nativeText(c, nativeT("useEffort"), &rule.UseEffort, locked)
							})
						}
					case 1:
						if !rule.Pause {
							nativeInt(c, nativeT("minimumTokens"), &rule.Tokens, 0, 10000000, locked)
							kit.Toggle(c, &rule.Images, nativeT("hasImages"), "").Disabled(locked)
							kit.Toggle(c, &rule.Compact, nativeT("compacting"), "").Disabled(locked)
							kit.Stack(c, "", "", func() {
								nativeText(c, nativeT("minimumEffort"), &rule.Effort, locked)
								nativeText(c, nativeT("intent"), &rule.Intent, locked)
							})
						}
						kit.Stack(c, "", "", func() {
							nativeList(c, r.fields, prefix+"agents", nativeT("clients"), &rule.Agents, locked)
							r.clientChoices(c, rule, locked)
						})
					case 2:
						kit.Stack(c, "", "", func() {
							nativeFolderList(c, r.p, r.fields, prefix+"folders", nativeT("folders"), &rule.Folders, locked)
							nativeFolderList(c, r.p, r.fields, prefix+"except", nativeT("exceptFolders"), &rule.ExceptFolders, locked)
						})
						timed := rule.Time != nil
						if kit.Toggle(c, &timed, nativeT("timeWindow"), "").Disabled(locked).Changed() {
							if timed {
								rule.Time = &routeTime{From: "09:00", To: "18:00"}
							} else {
								rule.Time = nil
							}
						}
						if rule.Time != nil {
							kit.Stack(c, "", "", func() {
								nativeText(c, nativeT("from"), &rule.Time.From, locked)
								nativeText(c, nativeT("to"), &rule.Time.To, locked)
								nativeText(c, nativeT("timeZone"), &rule.Time.Zone, locked)
								nativeWeekdays(c, &rule.Time.Days, locked)
							})
						}
					}
				})
			})
		}
		if remove >= 0 {
			g.Rules = slices.Delete(g.Rules, remove, remove+1)
			r.fields = fields{}
		} else if moveFrom >= 0 && moveTo >= 0 && moveFrom < len(g.Rules) && moveTo < len(g.Rules) && moveFrom != moveTo {
			item := g.Rules[moveFrom]
			g.Rules = slices.Insert(slices.Delete(g.Rules, moveFrom, moveFrom+1), moveTo, item)
			r.fields = fields{}
		}
		if kit.Button(c, nativeT("addRule"), icons.Plus, false).Disabled(locked).Clicked() {
			rule := routeRule{}
			if len(g.Members) > 0 {
				rule.Use = g.Members[len(g.Members)-1]
			}
			g.Rules = append(g.Rules, core.Preserved[routeRule]{Value: rule})
		}
	})
}

// The first editor section shows the current members and a direct route to
// manage them; choosing a strategy never buries the group identity or Save.
func (r *routingPanel) memberSummary(c *ui.Context, g *routeGroup) {
	kit.Section(c, icons.Layers, nativeT("members"), nativeT("membersHint"), func() {
		if len(g.Members) == 0 {
			kit.Help(c, nativeT("noGroups"))
		}
		for _, id := range g.Members {
			ui.Column(c.Key(id)).FillWidth().Padding(8, 0).Children(func() { kit.Strong(c, r.memberLabel(id)).Selectable() })
		}
		if kit.Button(c, nativeT("addMember"), icons.Plus, false).Clicked() {
			r.editorTab = 1
		}
	})
}

// Setup remains available without pushing configured routes below the fold.
func (r *routingPanel) discoveryView(c *ui.Context, locked bool) {
	found := r.state.Found
	if kit.Toggle(c, &found, nativeT("findGroups"), nativeT("findGroupsHint")).Disabled(locked).Changed() {
		r.act("found", map[string]any{"on": found}, nil)
	}
	ui.Column(c).FillWidth().Gap(8).Children(func() {
		for _, p := range r.state.Skippable {
			on := !slices.Contains(r.state.Skip, p.ID)
			if ui.Checkbox(c.Key(p.ID), &on, p.Name).Disabled(locked).Changed() {
				r.act("skip", map[string]any{"skip": toggleString(r.state.Skip, p.ID, !on)}, nil)
			}
		}
	})
	if len(r.state.Templates) > 0 {
		kit.Group(c, icons.LayoutTemplate, nativeT("templates"), nativeT("templatesHint"), func() {
			for _, tp := range r.state.Templates {
				name := tp.Group.Value.Name
				if i18n.Has("aiNative.template." + tp.Kind) {
					name = nativeT("template." + tp.Kind)
				}
				ui.Row(c.Key(tp.Kind)).FillWidth().Gap(8).Padding(12, 16).Wrap().AlignItems(ui.Center).Children(func() {
					ui.Column(c).Grow(1).MinWidth(0).Children(func() {
						kit.Strong(c, name)
						why := tp.Why
						if i18n.Has("aiNative.templateNeed." + tp.Need) {
							why = i18n.T("aiNative.templateNeed."+tp.Need, "name", tp.Of)
						}
						if why != "" {
							kit.Help(c, why)
						}
					})
					if kit.GhostButton(c, i18n.T("aiRouting.editTemplate"), icons.Pencil, false).Disabled(locked || len(tp.Group.Value.Members) == 0).Clicked() {
						group := tp.Group
						group.Value.Name = name
						r.beginNew(group)
					}
					if kit.Button(c, nativeT("add"), icons.Plus, false).Disabled(locked || len(tp.Group.Value.Members) == 0).Clicked() {
						r.act("template", map[string]string{"kind": tp.Kind, "name": name}, nil)
					}
				})
			}
		})
	}
}
