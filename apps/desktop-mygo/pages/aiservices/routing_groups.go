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

func (r *routingPanel) groupsView(c *ui.Context) {
	locked := r.api.busy || r.p.locked()
	r.routingSummary(c, locked)
	ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
		ui.TextInput(c, &r.search).Grow(1).MinWidth(180).Placeholder(nativeT("searchGroups")).Label(nativeT("searchGroups"))
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
					kit.Help(c, i18n.T("aiRouting.importedMissing", "models", strings.Join(result.Missing, " · ")))
				}
				if len(result.Instead) > 0 {
					kit.Help(c, i18n.T("aiRouting.importedInstead", "models", strings.Join(result.Instead, " · ")))
				}
				if kit.GhostButton(c, i18n.T("common.close"), nil, false).Clicked() {
					r.imported = nil
				}
			})
		})
	}
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
	ui.Row(c).Gap(8).Wrap().Children(func() {
		if kit.GhostButton(c, nativeT("selectAll"), nil, false).Clicked() {
			r.selectVisible()
		}
		if kit.GhostButton(c, nativeT("selectNone"), nil, false).Clicked() {
			clear(r.selected)
		}
		if len(r.selected) > 0 && kit.GhostButton(c, nativeT("deleteSelected"), icons.Trash2, false).Disabled(locked).Clicked() {
			r.dialog = "delete-selected"
		}
	})
	kit.Toggle(c, &r.showHidden, i18n.T("aiRouting.showHidden"), "")
	for index, doc := range r.state.Groups {
		g := doc.Value
		if !r.groupVisible(g) {
			continue
		}
		ui.Column(c.Key(g.ID)).FillWidth().Gap(8).Padding(12, 0).BorderWidth(0, 0, 1, 0).BorderColor(kit.Colors(c).Hairline).Children(func() {
			ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
				selected := r.selected[g.ID]
				if ui.Checkbox(c, &selected, "").Label(nativeT("select") + " " + g.Name).Changed() {
					if selected {
						r.selected[g.ID] = true
					} else {
						delete(r.selected, g.ID)
					}
				}
				ui.Column(c).Grow(1).MinWidth(0).Children(func() { kit.Strong(c, g.Name).Selectable(); kit.Mono(c, g.Ref).Selectable() })
				on := !g.Disabled && !g.Hidden
				if ui.Switch(c, &on).Label(nativeT("enabled") + " " + g.Name).Disabled(locked).Changed() {
					if g.Hidden && on {
						r.act("show", map[string]string{"id": g.ID}, nil)
					} else {
						r.act("switch", map[string]any{"id": g.ID, "on": on}, nil)
					}
				}
			})
			kit.Help(c, strings.Join(g.Members, " · ")).Selectable()
			if len(g.Match) > 0 {
				kit.Help(c, nativeT("patterns")+": "+strings.Join(g.Match, " · ")).Selectable()
			}
			for _, pattern := range g.Patterns {
				kit.Help(c, i18n.T("aiNative.patternMatches", "pattern", pattern.Pattern, "count", pattern.Models)).Selectable()
			}
			ui.Row(c).Gap(6).Wrap().Children(func() {
				if kit.Button(c, nativeT("edit"), icons.Pencil, false).Disabled(locked).Clicked() {
					r.begin(doc)
				}
				if kit.GhostButton(c, nativeT("copy"), icons.Copy, false).Disabled(locked).Clicked() {
					r.dialog = "copy"
					r.text = g.ID
					r.name = g.Name + " copy"
				}
				if kit.GhostButton(c, nativeT("share"), icons.Share2, false).Disabled(locked).Clicked() {
					r.act("share", map[string]string{"id": g.ID}, func() { r.dialog = "share"; r.text = r.state.Shared })
				}
				if kit.IconButton(c, icons.ArrowUp, nativeT("moveUp")+" "+g.Name).Disabled(locked || index == 0).Clicked() {
					r.moveGroup(index, -1)
				}
				if kit.IconButton(c, icons.ArrowDown, nativeT("moveDown")+" "+g.Name).Disabled(locked || index == len(r.state.Groups)-1).Clicked() {
					r.moveGroup(index, 1)
				}
				if kit.IconButton(c, icons.Trash2, nativeT("delete")+" "+g.Name).Disabled(locked).Clicked() {
					r.dialog = "delete"
					r.text = g.ID
				}
			})
			if g.Routing == "manual" {
				pick := g.Pick
				if nativeSelect(c, nativeT("manualPick"), &pick, stringChoices(g.Members), locked) {
					r.act("pick", map[string]string{"id": g.ID, "pick": pick}, nil)
				}
			}
		})
	}
	if len(r.state.Groups) == 0 {
		kit.Empty(c, icons.Waypoints, nativeT("noGroups"), nativeT("templatesHint"), nil)
	}
}

func (r *routingPanel) groupVisible(g routeGroup) bool {
	return (r.showHidden || !g.Hidden) && strings.Contains(strings.ToLower(g.Name+" "+g.ID+" "+strings.Join(g.Members, " ")), strings.ToLower(r.search))
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
		ui.Row(c).FillWidth().Gap(12).Wrap().AlignItems(ui.Center).Children(func() {
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
	order[index], order[index+delta] = order[index+delta], order[index]
	r.act("arrange", map[string]any{"order": order}, nil)
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
	g := &r.edit.Value
	mask := r.accountMasker()
	locked := r.api.busy || r.p.locked()
	ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
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

	// These tabs are views of the same group form, not navigation away from it.
	// A draft spans all sections and is saved or discarded only as a whole.
	kit.SubTabs(c, &r.editorTab, []string{i18n.T("aiRouting.basics"), nativeT("members"), nativeT("rules"), i18n.T("aiRouting.advanced"), nativeT("accounts")}, nil)
	ui.Column(c).FillWidth().Gap(kit.GroupGap).Children(func() {
		switch r.editorTab {
		case 0:
			kit.Section(c, nil, i18n.T("aiRouting.basics"), "", func() {
				ui.Column(c).FillWidth().Gap(16).Children(func() {
					nativeText(c, nativeT("groupName"), &g.Name, locked)
					nativeText(c, nativeT("groupID"), &g.ID, locked || g.Auto)
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
				kit.Toggle(c, &g.Sink, nativeT("sink"), nativeT("sinkHint")).Disabled(locked)
				kit.Stack(c, "", "", func() { nativeInt(c, nativeT("firstToken"), &g.FirstToken, 0, 3600, locked) })
				kit.Stack(c, "", nativeT("patienceHint"), func() { nativeInt(c, nativeT("patience"), &g.Patience, -1, 240, locked) })
				kit.Stack(c, "", nativeT("contextHint"), func() { nativeInt(c, nativeT("context"), &g.Context, -1, 10000000, locked) })
				kit.Stack(c, "", "", func() { nativeText(c, nativeT("family"), &g.Family, locked) })
				kit.Toggle(c, &g.Unlists, nativeT("unlistMembers"), "").Disabled(locked)
				kit.Stack(c, "", "", func() { nativeList(c, r.fields, "levels", nativeT("offeredLevels"), &g.Levels, locked) })
			})
		case 4:
			kit.Section(c, icons.Users, nativeT("accounts"), nativeT("accountsHint"), func() {
				for _, provider := range sortedKeys(r.state.Accounts) {
					kit.Section(c, nil, provider, "", func() {
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
		for i, id := range g.Members {
			ui.Column(c.Key(id)).FillWidth().Gap(6).Padding(12, 16).Children(func() {
				ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
					on := !slices.Contains(g.Off, id)
					if ui.Checkbox(c, &on, id).Grow(1).MinWidth(0).Disabled(locked).Changed() {
						g.Off = toggleString(g.Off, id, !on)
					}
					if kit.IconButton(c, icons.ArrowUp, nativeT("moveUp")+" "+id).Disabled(locked || i == 0).Clicked() {
						g.Members[i], g.Members[i-1] = g.Members[i-1], g.Members[i]
					}
					if kit.IconButton(c, icons.ArrowDown, nativeT("moveDown")+" "+id).Disabled(locked || i == len(g.Members)-1).Clicked() {
						g.Members[i], g.Members[i+1] = g.Members[i+1], g.Members[i]
					}
					if kit.IconButton(c, icons.X, nativeT("remove")+" "+id).Disabled(locked).Clicked() {
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
			kit.Stack(c, "", "", func() { nativeSelect(c, nativeT("manualPick"), &g.Pick, stringChoices(g.Members), locked) })
		}
		kit.Stack(c, "", "", func() {
			ui.TextInput(c, &r.memberSearch).FillWidth().Placeholder(nativeT("searchModels")).Label(nativeT("searchModels"))
		})
		ui.Scroll(c).FillWidth().Height(240).Padding(8, 16).Children(func() {
			all := append(append([]routeModel{}, r.state.Models...), r.state.Drawers...)
			for _, other := range r.state.Groups {
				if other.Value.ID != g.ID {
					all = append(all, routeModel{ID: other.Value.Ref, Name: other.Value.Name})
				}
			}
			for _, model := range all {
				if r.memberSearch != "" && !strings.Contains(strings.ToLower(model.ID+" "+model.Name+" "+model.ProviderName), strings.ToLower(r.memberSearch)) {
					continue
				}
				on := slices.Contains(g.Members, model.ID)
				if ui.Checkbox(c.Key(model.ID), &on, model.Name+" · "+model.ID).Disabled(locked).Changed() {
					g.Members = toggleString(g.Members, model.ID, on)
				}
			}
		})
		// A literal member field keeps plugin-defined references and effort
		// suffixes usable, including subsets such as :low+high, without a host
		// whitelist. Upstream validates references and cyclic groups at save.
		nativeText(c, nativeT("memberReference"), &r.newMember, locked)
		if kit.Button(c, nativeT("addMember"), icons.Plus, false).Disabled(locked || strings.TrimSpace(r.newMember) == "").Clicked() {
			g.Members = toggleString(g.Members, strings.TrimSpace(r.newMember), true)
			r.newMember = ""
		}
		nativeList(c, r.fields, "placed", nativeT("memberOrder"), &g.Placed, locked)
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
		for i := range g.Rules {
			rule := &g.Rules[i].Value
			ui.Column(c.Key(i)).FillWidth().Gap(8).Padding(12, 0).BorderWidth(1, 0, 0, 0).BorderColor(kit.Colors(c).Hairline).Children(func() {
				open := ui.Local(ui.Column(c), "editing", func() bool { return len(g.Rules) == 1 })
				section := ui.Local(ui.Column(c), "section", func() int { return 0 })
				ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Column(c).Grow(1).MinWidth(0).Children(func() {
						kit.Strong(c, nativeT("rule")+" "+kit.Itoa(i+1))
						kit.Help(c, rule.Use+" · "+rule.Intent)
					})
					if kit.IconButton(c, icons.Pencil, i18n.T("aiRouting.editRule")).Disabled(locked).Clicked() {
						*open = !*open
					}
					if kit.IconButton(c, icons.ArrowUp, nativeT("moveUp")).Disabled(locked || i == 0).Clicked() {
						g.Rules[i], g.Rules[i-1] = g.Rules[i-1], g.Rules[i]
						r.fields = fields{}
					}
					if kit.IconButton(c, icons.ArrowDown, nativeT("moveDown")).Disabled(locked || i == len(g.Rules)-1).Clicked() {
						g.Rules[i], g.Rules[i+1] = g.Rules[i+1], g.Rules[i]
						r.fields = fields{}
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
							nativeSelect(c, nativeT("useMember"), &rule.Use, stringChoices(g.Members), locked)
						})
						kit.Toggle(c, &rule.Pause, nativeT("pauseMember"), "").Disabled(locked)
						if !rule.Pause {
							kit.Stack(c, "", "", func() {
								nativeList(c, r.fields, prefix+"more", nativeT("thenMembers"), &rule.More, locked)
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
		}
		if kit.Button(c, nativeT("addRule"), icons.Plus, false).Disabled(locked).Clicked() {
			g.Rules = append(g.Rules, core.Preserved[routeRule]{})
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
			ui.Column(c.Key(id)).FillWidth().Padding(8, 0).Children(func() { kit.Strong(c, id).Selectable() })
		}
		if kit.Button(c, nativeT("addMember"), icons.Plus, false).Clicked() {
			r.editorTab = 1
		}
	})
}
