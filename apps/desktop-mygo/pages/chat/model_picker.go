package chat

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/widgets"
)

// modelPicker is the composer's model: a compact trigger showing the
// provider's mark, its name and the model, opening a searchable list of
// the enabled models grouped by provider (the React page's model menu).
// A model since removed is not a usable choice: the trigger asks to
// choose one rather than keep it.
func (p *Chat) modelPicker(c *ui.Context, provider *widgets.AIProvider, selection widgets.AISelection, disabled bool) {
	if p.ai == nil {
		return
	}
	t, pal := c.Theme(), kit.Colors(c)
	name := i18n.T("chat.chooseModel")
	if provider != nil {
		name = widgets.ModelLabel(provider, selection.Model)
	}
	title := name
	if provider != nil {
		title = provider.Name + " · " + name
	}
	trigger := ui.ButtonBase(c.Key("model")).Height(32).Padding(0, 8).Gap(6).Radius(t.Radius).AlignItems(ui.Center).MaxWidth(320).MinWidth(0).
		Label(i18n.T("chat.model")).Tooltip(title).Disabled(disabled)
	if trigger.Hovered() || p.ui.modelsOpen {
		trigger.Background(t.SurfaceHover)
	}
	trigger.Children(func() {
		if provider != nil {
			widgets.ProviderIcon(c, p.env, first(provider.Avatar, provider.Icon), 16).Shrink(0)
			ui.Text(c, provider.Name).FontSize(kit.HelpSize(c)).TextColor(t.TextMuted).SingleLine().Ellipsis("…").MinWidth(0).Shrink(2)
			ui.Text(c, "·").FontSize(kit.HelpSize(c)).TextColor(t.TextMuted)
		} else {
			ui.Icon(c, icons.Bot).FontSize(15).Shrink(0)
		}
		ui.Text(c, name).FontSize(kit.HelpSize(c)).SingleLine().Ellipsis("…").MinWidth(0).Shrink(1)
		if provider != nil && selection.Model != "" {
			badges(c, provider, selection.Model)
		}
		ui.Icon(c, icons.ChevronDown).FontSize(14).TextColor(t.TextMuted).Shrink(0)
	})
	if trigger.Clicked() {
		p.ui.modelsOpen = !p.ui.modelsOpen
		if p.ui.modelsOpen {
			p.ui.modelSearch = ""
			if p.ui.collapsedModels != nil {
				delete(p.ui.collapsedModels, selection.Provider)
			}
		}
	}
	ui.Popover(c, trigger, &p.ui.modelsOpen, func() {
		query := strings.ToLower(p.ui.modelSearch)
		ui.Column(c).Width(340).MaxWidthPercent(90).Gap(6).Children(func() {
			ui.TextInput(c, &p.ui.modelSearch).FillWidth().Placeholder(i18n.T("chat.searchModels")).Label(i18n.T("chat.searchModels")).AutoFocus()
			any := false
			ui.Scroll(c).FillWidth().MaxHeight(360).Children(func() {
				ui.Column(c).FillWidth().Gap(2).Role(ui.RoleList).Label(i18n.T("chat.model")).Children(func() {
					for i := range p.ai.Providers {
						pr := &p.ai.Providers[i]
						var matches []string
						for _, id := range pr.Models {
							if strings.Contains(strings.ToLower(pr.Name+" "+id+" "+pr.ModelNames[id]), query) {
								matches = append(matches, id)
							}
						}
						if len(matches) == 0 {
							continue
						}
						any = true
						expanded := query != "" || !p.ui.collapsedModels[pr.ID]
						ui.Column(c.Key("models-" + pr.ID)).FillWidth().Gap(2).Children(func() {
							group := ui.ButtonBase(c).FillWidth().Padding(6, 8).Gap(8).Radius(t.Radius).AlignItems(ui.Center).Label(pr.Name).Expanded(expanded)
							if group.Hovered() {
								group.Background(t.SurfaceHover)
							}
							group.Children(func() {
								widgets.ProviderIcon(c, p.env, first(pr.Avatar, pr.Icon), 16).Shrink(0)
								ui.Text(c, pr.Name).FontSize(kit.HelpSize(c)).FontWeight(500).Grow(1).SingleLine().Ellipsis("…").MinWidth(0)
								ui.Text(c, kit.Itoa(len(matches))).FontSize(kit.CaptionSize(c)).TextColor(t.TextMuted)
								chevron := icons.ChevronRight
								if expanded {
									chevron = icons.ChevronDown
								}
								ui.Icon(c, chevron).FontSize(13).TextColor(t.TextMuted)
							})
							if group.Clicked() {
								if p.ui.collapsedModels == nil {
									p.ui.collapsedModels = map[string]bool{}
								}
								p.ui.collapsedModels[pr.ID] = expanded
							}
							if !expanded {
								return
							}
							for _, id := range matches {
								id := id
								chosen := selection.Provider == pr.ID && selection.Model == id
								row := ui.ButtonBase(c.Key("model-"+id)).FillWidth().Padding(6, 8, 6, 24).Gap(8).Radius(t.Radius).AlignItems(ui.Center).
									Role(ui.RoleListItem).Selected(chosen).Label(widgets.ModelLabel(pr, id))
								switch {
								case chosen:
									row.Background(pal.Selected).TextColor(t.Text)
								case row.Hovered():
									row.Background(t.SurfaceHover)
								}
								row.Children(func() {
									widgets.ModelIcon(c, p.env, id, pr.ModelNames[id], 16).Shrink(0)
									ui.Text(c, widgets.ModelLabel(pr, id)).Grow(1).SingleLine().Ellipsis("…").MinWidth(0)
									badges(c, pr, id)
									if chosen {
										ui.Icon(c, icons.Check).FontSize(14).Shrink(0)
									}
								})
								if chosen && p.ui.modelsOpen && !p.ui.modelShown {
									row.ScrollIntoView()
								}
								if row.Clicked() {
									next := widgets.AISelection{Provider: pr.ID, Model: id}
									p.ui.modelsOpen = false
									p.run(func(done func(error)) { p.configure(&next, "", done) })
								}
							}
						})
					}
				})
			})
			p.ui.modelShown = true
			if !any {
				kit.Help(c, i18n.T("chat.modelUnavailable"))
			}
		})
	})
	if !p.ui.modelsOpen {
		p.ui.modelShown = false
	}
}

// badges shows a model's badges, if it has any.
func badges(c *ui.Context, provider *widgets.AIProvider, id string) {
	if draw := widgets.ModelBadges(provider, id); draw != nil {
		draw(c)
	}
}
