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

// Rename only a member reference and its links. A reasoning choice must not
// orphan its weight, pause rule, manual selection or place in the group.
func replaceMemberReference(g *routeGroup, from, to string) {
	replace := func(values []string) {
		for i, id := range values {
			if id == from {
				values[i] = to
			}
		}
	}
	replace(g.Members)
	replace(g.Off)
	replace(g.Fast)
	replace(g.Placed)
	if g.Pick == from {
		g.Pick = to
	}
	if weight, ok := g.Weights[from]; ok {
		delete(g.Weights, from)
		g.Weights[to] = weight
	}
	if levels, ok := g.EffortMaps[from]; ok {
		delete(g.EffortMaps, from)
		// Fixed/subset members already specify what is sent; upstream drops
		// their map too, rather than presenting a map that cannot run.
		if !strings.Contains(to, ":") {
			g.EffortMaps[to] = levels
		}
	}
	for i := range g.Rules {
		rule := &g.Rules[i].Value
		if rule.Use == from {
			rule.Use = to
		}
		replace(rule.More)
	}
}

// removeRouteMember mirrors the upstream editor: references that no longer
// exist leave the group's choices and rules instead of causing an invalid save.
func removeRouteMember(g *routeGroup, id string) {
	g.Members = toggleString(g.Members, id, false)
	g.Off = toggleString(g.Off, id, false)
	g.Fast = toggleString(g.Fast, id, false)
	g.Placed = toggleString(g.Placed, id, false)
	delete(g.Weights, id)
	delete(g.EffortMaps, id)
	if g.Pick == id {
		g.Pick = ""
	}
	kept := g.Rules[:0]
	for _, doc := range g.Rules {
		rule := &doc.Value
		uses := append([]string{rule.Use}, rule.More...)
		uses = toggleString(uses, id, false)
		if len(uses) == 0 {
			continue
		}
		rule.Use, rule.More = uses[0], uses[1:]
		kept = append(kept, doc)
	}
	g.Rules = kept
}

func (r *routingPanel) memberEffort(c *ui.Context, g *routeGroup, id string, locked bool) {
	var base *routeModel
	models := append([]routeModel{}, r.state.Models...)
	for _, group := range r.state.Groups {
		if group.Value.ID == g.ID {
			continue
		}
		models = append(models, routeModel{ID: group.Value.Ref, Name: group.Value.Name, Efforts: group.Value.Offers})
	}
	for i := range models {
		model := &models[i]
		if id == model.ID || strings.HasPrefix(id, model.ID+":") {
			if base == nil || len(model.ID) > len(base.ID) {
				base = model
			}
		}
	}
	if base == nil || len(base.Efforts) == 0 {
		return
	}
	selected := strings.TrimPrefix(strings.TrimPrefix(id, base.ID), ":")
	choices := []kit.SelectOption{{Value: "", Label: nativeT("followCaller")}}
	for _, effort := range base.Efforts {
		choices = append(choices, kit.SelectOption{Value: effort, Label: effort})
	}
	if selected != "" && !slices.Contains(base.Efforts, selected) {
		choices = append(choices, kit.SelectOption{Value: selected, Label: selected})
	}
	if nativeSelect(c, nativeT("memberEffort"), &selected, choices, locked) {
		next := base.ID
		if selected != "" {
			next += ":" + selected
		}
		replaceMemberReference(g, id, next)
	}
	if selected == "" && !strings.HasPrefix(id, "group/") {
		mapping := ui.Local(ui.Column(c), "effort-map", func() bool { return len(g.EffortMaps[id]) > 0 })
		ui.Collapsible(c, nativeT("effortMapping"), mapping, func() {
			kit.Help(c, nativeT("effortMappingHint"))
			// provider.MemberEfforts is the map's contract; unrestricted model
			// effort words elsewhere continue to use the vendor's own options.
			levels := []string{"none", "minimal", "low", "medium", "high", "xhigh", "max"}
			for _, from := range levels {
				to := g.EffortMaps[id][from]
				choices := append([]kit.SelectOption{{Value: "", Label: nativeT("asRequested")}}, stringChoices(levels)...)
				if nativeSelect(c, from, &to, choices, locked) {
					if to == "" || to == from {
						delete(g.EffortMaps[id], from)
					} else {
						if g.EffortMaps == nil {
							g.EffortMaps = map[string]map[string]string{}
						}
						if g.EffortMaps[id] == nil {
							g.EffortMaps[id] = map[string]string{}
						}
						g.EffortMaps[id][from] = to
					}
					if len(g.EffortMaps[id]) == 0 {
						delete(g.EffortMaps, id)
					}
				}
			}
		})
	}
	open := ui.Local(ui.Column(c), "subset", func() bool { return strings.Contains(selected, "+") })
	ui.Collapsible(c, nativeT("memberEffortSubset"), open, func() {
		levels := strings.Split(selected, "+")
		ui.Row(c).Gap(8).Wrap().Children(func() {
			for _, effort := range base.Efforts {
				on := slices.Contains(levels, effort)
				if ui.Checkbox(c.Key(effort), &on, effort).Disabled(locked).Changed() {
					levels = toggleString(levels, effort, on)
					var ordered []string
					for _, level := range base.Efforts {
						if slices.Contains(levels, level) {
							ordered = append(ordered, level)
						}
					}
					next := base.ID
					if len(ordered) > 0 {
						next += ":" + strings.Join(ordered, "+")
					}
					replaceMemberReference(g, id, next)
				}
			}
		})
	})
}

func nativeFolderList(c *ui.Context, p *page, f fields, key, label string, values *[]string, disabled bool) {
	box := ui.Column(c).FillWidth().Gap(6).Children(func() {
		nativeList(c, f, key, label, values, disabled)
		if kit.Button(c, nativeT("chooseFolder"), icons.FolderOpen, false).Disabled(disabled).Clicked() {
			core.Go(p.env, func() ([]string, error) { return p.env.Dialogs.PickFolders(label) }, func(paths []string, err error) {
				if err != nil {
					p.err = err.Error()
					return
				}
				for _, path := range paths {
					*values = toggleString(*values, path, true)
				}
			})
		}
	})
	widgets.DropArea(c, p.env, box, widgets.DropFolders, true, disabled, func(paths []string, err error) {
		if err != nil {
			p.err = err.Error()
			return
		}
		for _, path := range paths {
			*values = toggleString(*values, path, true)
		}
	})
}
func nativeWeekdays(c *ui.Context, days *[]string, locked bool) {
	ui.Field(c, nativeT("weekDays"), func() {
		ui.Row(c).Gap(10).Wrap().Children(func() {
			for _, day := range []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"} {
				on := slices.Contains(*days, day)
				if ui.Checkbox(c.Key(day), &on, i18n.T("aiNative.day."+day)).Disabled(locked).Changed() {
					*days = toggleString(*days, day, on)
				}
			}
		})
	})
}

// Ordered selections retain their upstream references, while the user works
// with the same model names as in the group. A removed model stays removable.
func (r *routingPanel) memberOrderView(c *ui.Context, label string, values *[]string, available []string, locked bool) {
	ui.Field(c, label, func() {
		for i, id := range slices.Clone(*values) {
			ui.Row(c.Key(id)).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
				kit.Label(c, r.memberLabel(id)).Grow(1).MinWidth(0)
				if kit.IconButton(c, icons.ArrowUp, nativeT("moveUp")+" "+r.memberLabel(id)).Disabled(locked || i == 0).Clicked() {
					(*values)[i], (*values)[i-1] = (*values)[i-1], (*values)[i]
				}
				if kit.IconButton(c, icons.ArrowDown, nativeT("moveDown")+" "+r.memberLabel(id)).Disabled(locked || i == len(*values)-1).Clicked() {
					(*values)[i], (*values)[i+1] = (*values)[i+1], (*values)[i]
				}
				if kit.IconButton(c, icons.X, nativeT("remove")+" "+r.memberLabel(id)).Disabled(locked).Clicked() {
					*values = toggleString(*values, id, false)
				}
			})
		}
		choices := []kit.SelectOption{{Value: "", Label: nativeT("addMember")}}
		for _, id := range available {
			if !slices.Contains(*values, id) {
				choices = append(choices, kit.SelectOption{Value: id, Label: r.memberLabel(id)})
			}
		}
		selected := ""
		if nativeSelect(c, nativeT("addMember"), &selected, choices, locked || len(choices) == 1) && selected != "" {
			*values = append(*values, selected)
		}
	})
}
