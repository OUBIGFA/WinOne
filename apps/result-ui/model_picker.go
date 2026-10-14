package resultui

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"regexp"
	"strconv"
	"strings"
	"winone/internal/ai"
)

type modelPickerRow struct {
	provider ai.Provider
	model    string // An empty model denotes a provider heading, never a choice.
	count    int
}

func (p *Panel) modelLabel(provider ai.Provider, id string) string {
	name := provider.ModelName(id)
	for _, suffix := range []string{" (free)", " (Free)", " (免费)", "（免费）"} {
		name = strings.TrimSuffix(name, suffix)
	}
	return name
}

// Match Magpie's separate green Free badge rather than changing model names.
func (p *Panel) freeModelBadge(c *ui.Context, t *ui.Theme, provider ai.Provider, id string) {
	if !provider.IsFreeModel(id) {
		return
	}
	color, background := ui.Hex("#166534"), ui.Hex("#dcfce7")
	if t.Dark {
		color, background = ui.Hex("#86efac"), ui.Hex("#143322")
	}
	if c.Preferences().HighContrast {
		color, background = t.Text, t.SurfaceHover
	}
	ui.Text(c, p.T("免费", "Free")).Label(p.T("免费模型", "Free model")).FontSize(12).Padding(1, 4).Radius(4).Shrink(0).TextColor(color).Background(background)
}

var contextInModelName = regexp.MustCompile(`\b\d+M\b`)

// Preserve Magpie ede1e32's rateTag/contextTag semantics in native pickers too.
// Routing IDs remain unchanged; labels are separate, non-shrinking controls.
func (p *Panel) modelBadges(c *ui.Context, t *ui.Theme, provider ai.Provider, id string) {
	p.freeModelBadge(c, t, provider, id)
	metadata := provider.ModelMetadataFor(id)
	rate, was := metadata.Rate, metadata.RateWas
	times := func(n float64) string {
		if n < 0.01 {
			return "<0.01×"
		}
		return strings.TrimRight(strings.TrimRight(strconv.FormatFloat(n, 'f', 2, 64), "0"), ".") + "×"
	}
	if rate > 0 || was > rate {
		title := fmt.Sprintf(p.T("每次请求消耗 %s 倍额度", "Costs %s credits per request"), times(rate))
		if was > rate {
			title = fmt.Sprintf(p.T("当前 %s，原为 %s", "Currently %s, previously %s"), times(rate), times(was))
			if rate <= 0 {
				title = fmt.Sprintf(p.T("当前免费，原为 %s", "Free now, previously %s"), times(was))
			}
		}
		ui.Row(c).Label(p.T("额度倍率", "Model rate")).Tooltip(title).Gap(3).Padding(1, 4).Radius(4).Shrink(0).Background(t.SurfaceHover).Children(func() {
			if was > rate {
				ui.Text(c, times(was)).FontSize(12).TextColor(t.TextMuted).Strikethrough().Shrink(0)
			}
			if rate > 0 {
				ui.Text(c, times(rate)).FontSize(12).TextColor(t.TextMuted).Shrink(0)
			}
		})
	}
	context := metadata.ListedContextWindow
	if context == 0 {
		context = metadata.ContextWindow
	}
	if context >= 1000000 && !contextInModelName.MatchString(provider.ModelName(id)) {
		value := strconv.FormatFloat(float64(context)/1000000, 'f', 1, 64)
		if context%1000000 == 0 {
			value = strconv.Itoa(context / 1000000)
		}
		ui.Box(c).Label(p.T("百万级上下文", "Large context")).Tooltip(fmt.Sprintf(p.T("可容纳 %d Token", "Holds %d tokens"), context)).Padding(1, 4).Radius(4).Shrink(0).Background(t.SurfaceHover).Children(func() {
			ui.Text(c, value+"M").FontSize(12).TextColor(t.TextMuted)
		})
	}
}

// Search a provider and its model together. IDs, rather than display names,
// identify a route, so two accounts with the same name remain independent.
func (p *Panel) modelPickerRows(query string, collapsed map[string]bool) []modelPickerRow {
	terms := strings.Fields(strings.ToLower(query))
	var rows []modelPickerRow
	for _, provider := range p.Configuration.Providers {
		var models []modelPickerRow
		for _, model := range provider.Models {
			text := strings.ToLower(provider.Name + " " + model + " " + p.modelLabel(provider, model))
			if provider.IsFreeModel(model) {
				text += " " + strings.ToLower(p.T("免费", "Free"))
			}
			matches := true
			for _, term := range terms {
				if !strings.Contains(text, term) {
					matches = false
					break
				}
			}
			if matches {
				models = append(models, modelPickerRow{provider: provider, model: model})
			}
		}
		if len(models) > 0 {
			rows = append(rows, modelPickerRow{provider: provider, count: len(models)})
			if !collapsed[provider.ID] {
				rows = append(rows, models...)
			}
		}
	}
	return rows
}

func (p *Panel) chooseModel(selection *ai.Selection) {
	p.Selection = selection
	p.ModelPickerOpen = false
	if p.Actions.Selection != nil {
		p.Actions.Selection(selection)
	}
}

func (p *Panel) modelPicker(c *ui.Context, t *ui.Theme) { p.modelPickerFor(c, t, false) }
func (p *Panel) modelPickerFor(c *ui.Context, t *ui.Theme, translation bool) {
	open := &p.ModelPickerOpen
	choose := p.chooseModel
	triggerLabel := p.T("切换供应商和模型", "Change provider and model")
	if translation {
		open = &p.TranslationPickerOpen
		choose = p.chooseTranslationModel
		triggerLabel = p.T("切换翻译模型", "Change translation model")
	}
	selected := p.defaultSelection()
	if p.Selection != nil {
		selected = *p.Selection
	}
	if translation {
		selected = p.translationModel()
	}
	label := selected.Model
	if label == "" {
		label = p.T("选择模型", "Choose model")
		if translation && p.Source != "selection" && p.Configuration.Engine == "google" {
			label = "Google Translate"
		}
	}
	provider, _ := p.Configuration.Provider(selected.Provider)
	if selected.Model != "" {
		label = p.modelLabel(provider, selected.Model)
	}
	// The displayed identity comes from the actual request until the user
	// chooses another model, including streamed provider identities.
	if translation && !p.TranslationModelChanged {
		details := &p.ResultDetails
		if p.ResultPending != nil {
			details = p.ResultPending
		}
		if details.Model != "" {
			label = details.Model
		}
		if details.ProviderName != "" {
			provider.Name = details.ProviderName
			provider.Icon = p.messageProviderIcon(details)
			provider.Avatar = nil // Icon already resolves the actual response provider.
		}
	}
	// The trigger hugs its label. A separate flex spacer in the composer keeps
	// send at the right edge without stretching either this button or its popup.
	button := ui.Button(c, "").Label(triggerLabel).Tooltip(provider.Name+" · "+label).
		MinWidth(0).MaxWidthPercent(78).Padding(3, 4).Gap(5).Radius(6).Border(0, ui.Transparent).Background(ui.Transparent).Cursor(ui.CursorPointer).Disabled(p.Busy).
		Children(func() {
			p.providerImage(c, t, provider.DisplayIcon(), 16)
			// Show the account as well as the model: identical model IDs can be
			// served by different providers. The full route remains in the tooltip.
			if provider.Name != "" {
				ui.Text(c, provider.Name).Label(func() string {
					if translation {
						return provider.Name
					}
					return p.T("当前供应商：", "Current provider: ") + provider.Name
				}()).SingleLine().Ellipsis("…").MinWidth(0).MaxWidth(88).FontSize(13).TextColor(t.TextMuted)
				ui.Text(c, "·").TextColor(t.TextMuted).Shrink(0)
			}
			ui.Text(c, label).Label(func() string {
				if translation {
					return label
				}
				return p.T("当前模型：", "Current model: ") + label
			}()).SingleLine().Ellipsis("…").MinWidth(0).FontSize(14).TextColor(t.Text)
			p.modelBadges(c, t, provider, selected.Model)
			ui.Icon(c, svg("chevron")).Size(14, 14).Shrink(0).TextColor(t.TextMuted)
		})
	metadata := provider.ModelMetadataFor(selected.Model)
	context := metadata.ListedContextWindow
	if context == 0 {
		context = metadata.ContextWindow
	}
	if metadata.Rate > 0 || metadata.RateWas > metadata.Rate || context >= 1000000 && !contextInModelName.MatchString(provider.ModelName(selected.Model)) {
		button.Wrap()
	}
	if translation {
		button.AlignSelf(ui.Start)
	}
	if !translation {
		width, _ := c.Size()
		// Reset actions live above the composer; reserve only send/stop and
		// the optional reasoning control in this bottom row.
		reserved := float32(28 + 2*8)
		if p.expandedReasoning() {
			reserved += 28 + 8
		}
		button.MaxWidth(max(24, min(width, 872)-48-reserved))
	}
	if button.Hovered() {
		button.Background(t.SurfaceHover)
	}
	if button.Clicked() {
		*open = !*open
	}
	if p.Busy {
		*open = false
	}
	ui.PopoverBase(c, button, open, func(panel *ui.Element) {
		width, height := c.Size()
		// A single popup fits even the 320-DIP result window. Its list scrolls
		// independently; there is no nested combobox extending past the window.
		panel.Width(min(340, width-24)).Padding(6).Gap(4).Margin(6, 0, 0, 0).Radius(10).Background(t.Background).Border(1, t.Border)
		query := ui.Local(panel, "query", func() string { return "" })
		highlight := ui.Local(panel, "highlight", func() int { return -1 })
		state := ui.Local(panel, "list", func() ui.ListState { return ui.ListState{} })
		searchCollapsed := ui.Local(panel, "searchCollapsed", func() map[string]bool { return map[string]bool{} })
		var search *ui.Element
		ui.Row(c).AlignItems(ui.Center).Gap(8).Padding(5, 8).Children(func() {
			ui.Icon(c, svg("search")).Size(16, 16).Shrink(0).TextColor(t.TextMuted)
			search = ui.TextInput(c, query).Label(p.T("搜索供应商或模型", "Search providers or models")).Placeholder(p.T("搜索供应商或模型", "Search providers or models")).AutoFocus().Grow(1).MinWidth(0).Padding(3, 0).Border(0, ui.Transparent).Background(ui.Transparent)
		})
		ui.Box(c).Height(1).Background(t.Border)
		if search.Changed() {
			*highlight = -1
			state.ScrollTo(0, ui.Start)
			clear(*searchCollapsed)
		}
		if p.modelPickerCollapsed == nil {
			p.modelPickerCollapsed = map[string]bool{}
		}
		collapsed := p.modelPickerCollapsed
		// Search reveals matches without discarding the user's normal group
		// state. Collapsing a search result stays local to that query.
		if strings.TrimSpace(*query) != "" {
			collapsed = *searchCollapsed
		}
		rows := p.modelPickerRows(*query, collapsed)
		if *highlight < 0 || *highlight >= len(rows) {
			*highlight = -1
			for i, row := range rows {
				if row.model != "" && *highlight < 0 {
					*highlight = i
				}
				if *query == "" && row.provider.ID == selected.Provider && row.model == selected.Model {
					*highlight = i
					break
				}
			}
			if *highlight >= 0 {
				state.ScrollTo(*highlight, ui.Center)
			}
		}
		chooseRow := func(index int) {
			if index >= 0 && index < len(rows) && rows[index].model != "" {
				choose(&ai.Selection{Provider: rows[index].provider.ID, Model: rows[index].model})
			}
		}
		if len(rows) == 0 {
			message := p.T("没有匹配的模型", "No matching models")
			if len(p.modelPickerRows("", nil)) == 0 {
				message = p.T("请先在 AI 服务中启用模型", "Enable a model in AI services first")
			}
			ui.Text(c, message).Padding(18, 10).TextColor(t.TextMuted)
		} else {
			listHeight := float32(0)
			for i, row := range rows {
				if row.model == "" {
					listHeight += 36 // title plus separation from its first model
					if i > 0 {
						listHeight += 6
					}
				} else {
					listHeight += 34
				}
			}
			state.Selected = highlight
			state.Key = func(i int) any { return [2]string{rows[i].provider.ID, rows[i].model} }
			state.Header = func(i int) bool { return rows[i].model == "" }
			state.Label = func(i int) string { return rows[i].provider.Name + " " + p.modelLabel(rows[i].provider, rows[i].model) }
			list := ui.List(c, state, len(rows), func(i int) {
				row := rows[i]
				if row.model == "" {
					open := !collapsed[row.provider.ID]
					group := ui.Column(c).Background(t.Background).Padding(0, 0, 4, 0)
					if i > 0 {
						group.Padding(6, 0, 4, 0)
					}
					group.Children(func() {
						// Use MyGo's disclosure for mouse, keyboard and accessibility;
						// models remain virtual rows so large groups stay inexpensive.
						disclosure := ui.CollapsibleBase(c, &open)
						disclosure.Trigger.Label(p.T("供应商：", "Provider: ")+row.provider.Name).Tooltip(row.provider.Name).Height(32).Padding(6, 8).Gap(6).AlignItems(ui.Center).Radius(99).Border(1, t.Border).Background(ui.Transparent).Cursor(ui.CursorPointer).Children(func() {
							arrow := ui.Icon(c, svg("chevron")).Size(14, 14).Shrink(0).TextColor(t.TextMuted)
							if !open {
								arrow.Rotate(-90)
							}
							p.providerImage(c, t, row.provider.DisplayIcon(), 14)
							ui.Text(c, row.provider.Name).SingleLine().Ellipsis("…").MinWidth(0).Grow(1).FontSize(13).TextColor(t.TextMuted)
							ui.Textf(c, "%d", row.count).FontSize(13).Shrink(0).TextColor(t.TextMuted)
						})
						if disclosure.Trigger.Changed() {
							collapsed[row.provider.ID] = !open
							*highlight = -1
						}
					})
					return
				}
				// One compact level of indentation keeps each model beneath its
				// provider without pushing the icon away from its name.
				item := ui.Button(c, "").Label(p.modelLabel(row.provider, row.model)).Tooltip(row.provider.Name+" · "+row.model).Height(34).MinWidth(0).Padding(6, 8, 6, 28).Gap(6).Border(0, ui.Transparent).Background(t.Background).Radius(6).Cursor(ui.CursorPointer)
				if item.Hovered() || i == *highlight {
					item.Background(t.SurfaceHover)
				}
				item.Children(func() {
					providerImage(c, t, row.provider.ModelIcon(row.model), 18)
					ui.Row(c).MinWidth(0).Grow(1).Gap(4).AlignItems(ui.Center).Children(func() {
						ui.Text(c, p.modelLabel(row.provider, row.model)).SingleLine().Ellipsis("…").MinWidth(0)
						p.modelBadges(c, t, row.provider, row.model)
					})
					if row.provider.ID == selected.Provider && row.model == selected.Model {
						ui.Icon(c, svg("check")).Size(16, 16).Shrink(0).TextColor(t.TextMuted)
					}
				})
				if item.Clicked() {
					chooseRow(i)
				}
			}).Height(min(listHeight, min(320, max(80, height-180)))).Padding(0, 8, 0, 0).Label(p.T("可用模型", "Available models"))
			if search.Shortcut(0, ui.KeyDown) {
				list.Focus()
			}
			if list.Submitted() {
				chooseRow(*highlight)
			}
		}
		if search.Submitted() && !search.Composing() {
			chooseRow(*highlight)
		}
		ui.Box(c).Height(1).Background(t.Border)
		reset := ui.Button(c, "").Label(p.T("使用任务默认", "Use task defaults")).Height(30).Padding(5, 8).Gap(8).Border(0, ui.Transparent).Background(ui.Transparent).Cursor(ui.CursorPointer).Children(func() {
			ui.Icon(c, svg("retry")).Size(14, 14).TextColor(t.TextMuted)
			ui.Text(c, p.T("使用任务默认", "Use task defaults")).TextColor(t.TextMuted)
		})
		if reset.Hovered() {
			reset.Background(t.SurfaceHover)
		}
		if reset.Clicked() {
			choose(nil)
		}
	})
}
