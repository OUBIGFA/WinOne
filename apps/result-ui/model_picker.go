package resultui

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"winone/internal/ai"
	"winone/internal/nativeui/modelmenu"
)

func (p *Panel) modelLabel(provider ai.Provider, id string) string {
	name := provider.ModelName(id)
	for _, suffix := range []string{" (free)", " (Free)", " (免费)", "（免费）"} {
		name = strings.TrimSuffix(name, suffix)
	}
	return name
}

// Match Magpie's separate green Free badge rather than changing model names.
// Its green is a data color, kept in the neutral interface.
func (p *Panel) freeModelBadge(c *ui.Context, t *ui.Theme, provider ai.Provider, id string) {
	if !provider.IsFreeModel(id) {
		return
	}
	tn := toneFor(t, c.Preferences().HighContrast)
	ui.Text(c, p.T("免费", "Free")).Label(p.T("免费模型", "Free model")).FontSize(em(t, sizeBadge)).Padding(1, 4).Radius(4).Shrink(0).TextColor(tn.free).Background(tn.freeBackground)
}

var contextInModelName = regexp.MustCompile(`\b\d+M\b`)

// Preserve Magpie ede1e32's rateTag/contextTag semantics in native pickers too.
// Routing IDs remain unchanged; labels are separate, non-shrinking controls.
func (p *Panel) modelBadges(c *ui.Context, t *ui.Theme, provider ai.Provider, id string) {
	p.freeModelBadge(c, t, provider, id)
	tn := toneFor(t, c.Preferences().HighContrast)
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
		ui.Row(c).Label(p.T("额度倍率", "Model rate")).Tooltip(title).Gap(3).Padding(1, 4).Radius(4).Shrink(0).Background(tn.hover).Children(func() {
			if was > rate {
				ui.Text(c, times(was)).FontSize(em(t, sizeBadge)).TextColor(t.TextMuted).Strikethrough().Shrink(0)
			}
			if rate > 0 {
				ui.Text(c, times(rate)).FontSize(em(t, sizeBadge)).TextColor(t.TextMuted).Shrink(0)
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
		ui.Box(c).Label(p.T("百万级上下文", "Large context")).Tooltip(fmt.Sprintf(p.T("可容纳 %d Token", "Holds %d tokens"), context)).Padding(1, 4).Radius(4).Shrink(0).Background(tn.hover).Children(func() {
			ui.Text(c, value+"M").FontSize(em(t, sizeBadge)).TextColor(t.TextMuted)
		})
	}
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
	tn := toneFor(t, c.Preferences().HighContrast)
	open := &p.ModelPickerOpen
	choose := p.chooseModel
	triggerLabel := p.T("切换供应商和模型", "Change provider and model")
	key := "model-picker"
	if translation {
		open = &p.TranslationPickerOpen
		choose = p.chooseTranslationModel
		triggerLabel = p.T("切换翻译模型", "Change translation model")
		key = "translation-picker"
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
	button := ui.ButtonBase(c.Key(key)).Label(triggerLabel).Tooltip(provider.Name+" · "+label).
		MinWidth(0).MaxWidthPercent(78).Padding(3, 6).Gap(5).Radius(6).AlignItems(ui.Center).Cursor(ui.CursorPointer).Disabled(p.Busy).
		Children(func() {
			p.providerImage(c, t, provider.DisplayIcon(), 16)
			// Show the account as well as the model: identical model IDs can be
			// served by different providers. The full route remains in the tooltip.
			if provider.Name != "" {
				name := p.T("当前供应商：", "Current provider: ") + provider.Name
				if translation {
					name = provider.Name
				}
				ui.Text(c, provider.Name).Label(name).SingleLine().Ellipsis("…").MinWidth(0).MaxWidth(88).FontSize(em(t, sizeMeta)).TextColor(t.TextMuted)
				ui.Text(c, "·").FontSize(em(t, sizeMeta)).TextColor(t.TextMuted).Shrink(0)
			}
			name := p.T("当前模型：", "Current model: ") + label
			if translation {
				name = label
			}
			ui.Text(c, label).Label(name).SingleLine().Ellipsis("…").MinWidth(0).FontSize(em(t, sizeMeta)).TextColor(t.Text)
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
	} else {
		width, _ := c.Size()
		// Reserve the bottom row's other controls: restart, clear, send or
		// stop, and the optional reasoning control.
		reserved := 3*iconButtonSize + 4*4
		if p.expandedReasoning() {
			reserved += iconButtonSize + 4
		}
		button.MaxWidth(max(24, min(width, readingMaxWidth+2*gutter)-2*composerInset-2*bleed-reserved))
	}
	if !p.Busy {
		if button.Pressed() {
			button.Background(tn.pressed)
		} else if button.Hovered() || *open {
			button.Background(tn.hover)
		}
	}
	menu := &p.modelMenu
	if translation {
		menu = &p.translationMenu
	}
	if button.Clicked() {
		menu.Open = *open
		modelmenu.Toggle(menu, selected.Provider)
	}
	if p.Busy {
		menu.Open = false
	}
	*open = menu.Open
	// The window shares the settings window's model list: the Chat
	// composer's, drawn by modelmenu. "使用任务默认" comes first, as the
	// list's other choices do.
	chosen, picked := modelmenu.Menu(c, button, menu, modelmenu.Options{
		Choices:  []modelmenu.Choice{{Value: "", Label: p.T("使用任务默认", "Use task defaults"), Icon: svg("retry")}},
		Groups:   p.modelMenuGroups(),
		Selected: modelValue(selected),
		Strings: modelmenu.Strings{Search: p.T("搜索供应商或模型", "Search providers or models"), List: p.T("可用模型", "Available models"),
			Empty: p.T("请先在 AI 服务中启用模型", "Enable a model in AI services first"), NoMatch: p.T("没有匹配的模型", "No matching models"),
			Group: p.T("供应商：", "Provider: ")},
	})
	*open = menu.Open
	if !picked {
		return
	}
	if provider, model, ok := strings.Cut(chosen, "\x00"); ok {
		choose(&ai.Selection{Provider: provider, Model: model})
	} else {
		choose(nil)
	}
}

// modelValue names a route in the model list.
func modelValue(s ai.Selection) string {
	if s.Provider == "" && s.Model == "" {
		return "\x01"
	}
	return s.Provider + "\x00" + s.Model
}

// modelMenuGroups are the enabled models by provider. IDs, rather than
// display names, identify a route, so two accounts with the same name
// remain independent; the free badge's word is searched too.
func (p *Panel) modelMenuGroups() []modelmenu.Group {
	groups := make([]modelmenu.Group, 0, len(p.Configuration.Providers))
	for _, provider := range p.Configuration.Providers {
		provider := provider
		group := modelmenu.Group{ID: provider.ID, Name: provider.Name, Icon: func(c *ui.Context, size float32) ui.Element {
			return p.providerImage(c, c.Theme(), provider.DisplayIcon(), size)
		}}
		for _, model := range provider.Models {
			model := model
			keywords := model
			if provider.IsFreeModel(model) {
				keywords += " " + p.T("免费", "Free")
			}
			group.Models = append(group.Models, modelmenu.Entry{Value: provider.ID + "\x00" + model, Label: p.modelLabel(provider, model), Keywords: keywords,
				Icon: func(c *ui.Context, size float32) ui.Element {
					return providerImage(c, c.Theme(), provider.ModelIcon(model), size)
				},
				Badge: func(c *ui.Context) { p.modelBadges(c, c.Theme(), provider, model) }})
		}
		groups = append(groups, group)
	}
	return groups
}
