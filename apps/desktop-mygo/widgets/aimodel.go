package widgets

import (
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

// AIProvider is what the model pickers show of a configured AI service:
// the public part of capture.ai.configuration.get, never credentials.
// Pages read it for display only; they save configurations they received
// as they were (core.Preserved), so fields not declared here are kept.
type AIProvider struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Icon   string `json:"icon,omitempty"`
	Avatar string `json:"avatar,omitempty"`
	// Models are the enabled models, the only ones a task can use.
	Models []string `json:"models"`
	// AvailableModels and ModelNames also name models that are not
	// enabled; labels tell apart models that share a name among them all.
	AvailableModels []string                   `json:"availableModels,omitempty"`
	ModelNames      map[string]string          `json:"modelNames,omitempty"`
	ModelFree       map[string]bool            `json:"modelFree,omitempty"`
	ModelMetadata   map[string]AIModelMetadata `json:"modelMetadata,omitempty"`

	// aiPickLabels caches the labels of every model, computed once per
	// decoded provider as React kept them per provider object.
	aiPickLabels map[string]string
}

// AIModelMetadata is what a model's badges show: its credit rate, the rate
// before a discount, and the context window the vendor lists.
type AIModelMetadata struct {
	Rate                float64  `json:"rate,omitempty"`
	RateWas             float64  `json:"rateWas,omitempty"`
	ContextWindow       *float64 `json:"contextWindow,omitempty"`
	ListedContextWindow *float64 `json:"listedContextWindow,omitempty"`
}

// AISelection refers to a provider and one of its models.
type AISelection struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

// AIModelSelectOptions configure AIModelSelect.
type AIModelSelectOptions struct {
	// Env reads users' own provider pictures through the Host.
	Env       *core.Env
	Providers []AIProvider
	// Value is the task's selection; nil follows the default service.
	Value *AISelection
	// Label names the task; the pickers are named "<Label> · 供应商" and
	// "<Label> · 模型名称".
	Label string
	// InheritLabel offers following the default service under this name;
	// empty offers "选择供应商" instead.
	InheritLabel string
	Disabled     bool
}

// aiPickInherit is the provider picker's value for following the default.
const aiPickInherit = "__inherit"

// AIModelSelect chooses the provider and model of one task, as React's
// AIModelSelect did (features/ai-model-select.tsx): settings store a
// reference only, and a choice that is no longer enabled stays shown with
// a warning instead of silently switching to another provider.
//
// The provider list offers following the default service (InheritLabel),
// "未配置" while the value names no provider, every provider with its
// mark, and a removed provider the value still names. Choosing a provider
// selects its first enabled model; choosing to follow the default returns
// nil; choosing "未配置" returns an empty provider. The model list offers
// the provider's enabled models with their marks, labels told apart and
// badges.
//
// It returns the new selection and whether the user changed it.
func AIModelSelect(c *ui.Context, o AIModelSelectOptions) (*AISelection, bool) {
	value := o.Value
	var provider *AIProvider
	if value != nil {
		provider = aiPickFind(o.Providers, value.Provider)
	}
	invalid := value != nil && value.Provider != "" && (provider == nil || !slices.Contains(provider.Models, value.Model))

	var providerItems []aiPickItem
	if o.InheritLabel != "" {
		providerItems = append(providerItems, aiPickItem{value: aiPickInherit, label: o.InheritLabel})
		if value != nil && value.Provider == "" {
			providerItems = append(providerItems, aiPickItem{value: "", label: i18n.T("ai.unconfigured")})
		}
	} else {
		providerItems = append(providerItems, aiPickItem{value: "", label: i18n.T("ai.chooseProvider")})
	}
	for i := range o.Providers {
		p := &o.Providers[i]
		mark := p.Avatar
		if mark == "" {
			mark = p.Icon
		}
		providerItems = append(providerItems, aiPickItem{value: p.ID, label: p.Name, icon: func(c *ui.Context) {
			ProviderIcon(c, o.Env, mark, aiPickIconSize)
		}})
	}
	if value != nil && value.Provider != "" && provider == nil {
		providerItems = append(providerItems, aiPickItem{value: value.Provider, label: value.Provider})
	}
	var modelItems []aiPickItem
	if provider != nil {
		for _, model := range provider.Models {
			name := provider.ModelNames[model]
			modelItems = append(modelItems, aiPickItem{value: model, label: ModelLabel(provider, model),
				icon:  func(c *ui.Context) { ModelIcon(c, o.Env, model, name, aiPickIconSize) },
				badge: aiPickBadges(provider, model)})
		}
	}

	providerValue, modelValue := "", ""
	switch {
	case value == nil && o.InheritLabel != "":
		providerValue = aiPickInherit
	case value != nil:
		providerValue, modelValue = value.Provider, value.Model
	}
	placeholder := "ai.chooseProviderFirst"
	switch {
	case value == nil && o.InheritLabel != "":
		placeholder = "ai.inherit"
	case provider != nil:
		placeholder = "ai.noEnabledModels"
	}

	next, changed := value, false
	ui.Column(c).FillWidth().Gap(8).Children(func() {
		ui.Row(c).FillWidth().GapX(16).GapY(12).Wrap().AlignItems(ui.Start).Children(func() {
			aiPickColumn(c, i18n.T("ai.supplier"), func() {
				if aiPickSelect(c, &providerValue, providerItems, "", o.Label+" · "+i18n.T("ai.supplier"), o.Disabled) {
					changed = true
					switch chosen := aiPickFind(o.Providers, providerValue); {
					case chosen != nil:
						model := ""
						if len(chosen.Models) > 0 {
							model = chosen.Models[0]
						}
						next = &AISelection{Provider: chosen.ID, Model: model}
					case o.InheritLabel != "" && providerValue == "":
						next = &AISelection{}
					default:
						next = nil
					}
				}
			})
			aiPickColumn(c, i18n.T("ai.model"), func() {
				disabled := o.Disabled || provider == nil || len(modelItems) == 0
				if aiPickSelect(c, &modelValue, modelItems, i18n.T(placeholder), o.Label+" · "+i18n.T("ai.model"), disabled) && provider != nil {
					changed = true
					next = &AISelection{Provider: provider.ID, Model: modelValue}
				}
			})
		})
		if invalid {
			model := value.Model
			if model == "" {
				model = i18n.T("ai.unconfigured")
			}
			kit.Status(c, i18n.T("ai.selectionUnavailable", "model", model))
		}
	})
	return next, changed
}

const aiPickIconSize = 16

// aiPickFind returns the first provider with id, as Array.find did.
func aiPickFind(providers []AIProvider, id string) *AIProvider {
	for i := range providers {
		if providers[i].ID == id {
			return &providers[i]
		}
	}
	return nil
}

// aiPickColumn is one picker under its name, which a click on the name
// focuses; the two pickers share a row and wrap below each other when
// narrow.
func aiPickColumn(c *ui.Context, label string, fn func()) {
	ui.Field(c, label, fn).Basis(200).Grow(1).Shrink(1).MinWidth(0)
}

// aiPickItem is one option of aiPickSelect: its value and label, and what
// shows beside the label.
type aiPickItem struct {
	value, label string
	icon         func(c *ui.Context)
	badge        func(c *ui.Context)
}

// PictureOption is one option of PictureSelect: its value and label, a
// picture before the label (a provider's or model's mark) and badges
// after it.
type PictureOption struct {
	Value, Label string
	Picture      func(c *ui.Context)
	Badge        func(c *ui.Context)
}

// PictureSelect is kit.Select for options marked by pictures rather than
// icons, as the provider pickers are. name labels it for assistive
// technology; placeholder shows while nothing is chosen. It returns
// whether the user chose another option.
func PictureSelect(c *ui.Context, selected *string, options []PictureOption, placeholder, name string, disabled bool) bool {
	items := make([]aiPickItem, len(options))
	for i, o := range options {
		items[i] = aiPickItem{value: o.Value, label: o.Label, icon: o.Picture, badge: o.Badge}
	}
	return aiPickSelect(c, selected, items, placeholder, name, disabled)
}

// ModelBadges returns the badges of a model of p (free, credit rate,
// context window), nil when it has none.
func ModelBadges(p *AIProvider, id string) func(c *ui.Context) { return aiPickBadges(p, id) }

// aiPickSelect is kit.Select for options whose marks are pictures, not
// icons: a provider's logo, a model family's, with badges after the label.
// The trigger shows the chosen option, the value itself when no option
// has it (a model since removed), or placeholder when empty.
func aiPickSelect(c *ui.Context, selected *string, items []aiPickItem, placeholder, name string, disabled bool) bool {
	t, p := c.Theme(), kit.Colors(c)
	before := *selected
	sel := ui.SelectBase(c, selected)
	var current *aiPickItem
	for i := range items {
		if items[i].value == *selected {
			current = &items[i]
		}
	}
	b := sel.Trigger.FillWidth().MinHeight(kit.ControlHeight).Padding(5, 10).Gap(8).Radius(t.Radius).Background(t.Surface).
		Border(1, t.Border).Justify(ui.SpaceBetween).AlignItems(ui.Center).MinWidth(0).Label(name).Disabled(disabled)
	if b.Hovered() {
		b.Background(t.SurfaceHover)
	}
	if current != nil {
		b.Tooltip(current.label)
	} else if *selected != "" {
		b.Tooltip(*selected)
	}
	b.Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Grow(1).Shrink(1).MinWidth(0).Children(func() {
			switch {
			case current != nil:
				if current.icon != nil {
					current.icon(c)
				}
				ui.Text(c, current.label).SingleLine().Shrink(1).MinWidth(0)
				if current.badge != nil {
					current.badge(c)
				}
			case *selected != "":
				ui.Text(c, *selected).SingleLine().Shrink(1).MinWidth(0)
			default:
				ui.Text(c, placeholder).SingleLine().Shrink(1).MinWidth(0).TextColor(t.TextMuted)
			}
		})
		ui.Icon(c, icons.ChevronDown).FontSize(14).TextColor(t.TextMuted).Shrink(0)
	})
	alpha := float32(0.16)
	if t.Dark {
		alpha = 0.7
	}
	sel.Popup(func(panel ui.Element) {
		panel.Margin(4, 0, 0, 0).Padding(4).Radius(kit.PanelRadius).Background(p.Panel).Border(1, t.Border).
			Shadow(0, 8, 24, 0, ui.RGBA(0, 0, 0, alpha)).MinWidth(220).MaxWidth(520)
		for _, item := range items {
			row := sel.Item(item.value).Padding(6, 10).Gap(8).Radius(t.Radius).AlignItems(ui.Center)
			if row.Highlighted() {
				row.Background(t.SurfaceHover)
			}
			chosen := item.value == *selected
			row.Children(func() {
				if item.icon != nil {
					item.icon(c)
				}
				label := ui.Text(c, item.label).Grow(1).Shrink(1).MinWidth(0)
				if item.badge != nil {
					item.badge(c)
				}
				if chosen {
					label.FontWeight(600)
					ui.Icon(c, icons.Check).FontSize(14).Shrink(0)
				}
			})
		}
	})
	return *selected != before
}

// aiPickBadges returns the badges of a model, nil when it has none: free,
// its credit rate (with the rate before a discount struck through) and a
// context window of a million tokens or more that its name does not
// already state (Magpie's freeBadge, rateTag and contextTag).
func aiPickBadges(p *AIProvider, id string) func(c *ui.Context) {
	free := IsFreeModel(p, id)
	metadata := p.ModelMetadata[id]
	rate, was := metadata.Rate, metadata.RateWas
	context := 0.0
	switch {
	case metadata.ListedContextWindow != nil:
		context = *metadata.ListedContextWindow
	case metadata.ContextWindow != nil:
		context = *metadata.ContextWindow
	}
	name := id
	if text, ok := p.ModelNames[id]; ok {
		name = text
	}
	showRate := rate > 0 || was > rate
	showContext := context >= 1e6 && !aiPickMillions.MatchString(name)
	if !free && !showRate && !showContext {
		return nil
	}
	return func(c *ui.Context) {
		if free {
			aiPickBadge(c, "", func() { ui.Text(c, i18n.T("ai.free")).SingleLine() })
		}
		if showRate {
			key := "ai.modelRate"
			switch {
			case rate <= 0 && was > rate:
				key = "ai.modelFreeAfterDiscount"
			case was > rate:
				key = "ai.modelDiscountRate"
			}
			aiPickBadge(c, i18n.T(key, "rate", aiPickTimes(rate), "was", aiPickTimes(was)), func() {
				if was > rate {
					ui.Text(c, aiPickTimes(was)).SingleLine().Strikethrough()
				}
				if rate > 0 {
					ui.Text(c, aiPickTimes(rate)).SingleLine()
				}
			})
		}
		if showContext {
			millions := context / 1e6
			text := strconv.FormatFloat(millions, 'f', 1, 64)
			if millions == math.Trunc(millions) {
				text = strconv.FormatFloat(millions, 'f', 0, 64)
			}
			aiPickBadge(c, i18n.T("ai.modelContextTokens", "count", strconv.FormatFloat(context, 'f', -1, 64)), func() {
				ui.Text(c, text+"M").SingleLine()
			})
		}
	}
}

var aiPickMillions = regexp.MustCompile(`\b\d+M\b`)

// aiPickBadge is kit.Badge holding several texts, with a tooltip.
func aiPickBadge(c *ui.Context, tip string, fn func()) {
	t := c.Theme()
	b := ui.Row(c).Gap(4).Padding(2, 6).Radius(4).Background(t.Surface).TextColor(t.TextMuted).
		FontSize(kit.CaptionSize(c)).Shrink(0).AlignItems(ui.Center)
	if tip != "" {
		b.Tooltip(tip)
	}
	b.Children(fn)
}

// aiPickTimes writes a rate as Magpie does: two decimals at most, "<0.01"
// below them, then "×".
func aiPickTimes(value float64) string {
	if value < 0.01 {
		return "<0.01×"
	}
	rounded, _ := strconv.ParseFloat(strconv.FormatFloat(value, 'f', 2, 64), 64)
	return strconv.FormatFloat(rounded, 'f', -1, 64) + "×"
}

var (
	aiPickFreeSuffix = regexp.MustCompile(`(?i)\s*[(（](?:free|免费)[)）]\s*$`)
	aiPickFreeWord   = regexp.MustCompile(`(?i)(^|[^a-z])free($|[^a-z])|[(（]免费[)）]`)
)

// aiPickBaseLabel is a model's display name without a "(free)" suffix,
// which the free badge shows instead.
func aiPickBaseLabel(p *AIProvider, id string) string {
	name := p.ModelNames[id]
	if name == "" {
		name = id
	}
	if trimmed := aiPickFreeSuffix.ReplaceAllString(name, ""); trimmed != "" {
		return trimmed
	}
	return name
}

// IsFreeModel reports Magpie's two positive signals that a model is free:
// the vendor's metadata, or "free" as a whole word in its ID or name.
// Unknown pricing and names such as Freedom stay unmarked.
func IsFreeModel(p *AIProvider, id string) bool {
	if p.ModelFree[id] {
		return true
	}
	return aiPickFreeWord.MatchString(id) || aiPickFreeWord.MatchString(p.ModelNames[id])
}

// ModelLabel returns how pickers name a model of p: its display name, and
// when another model of p has the same name, the part of its ID that
// tells them apart, as "GPT-5 · openai" (Magpie's modelTells).
func ModelLabel(p *AIProvider, id string) string {
	if p.aiPickLabels == nil {
		seen := map[string]bool{}
		var ids []string
		add := func(id string) {
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
		for _, id := range p.AvailableModels {
			add(id)
		}
		for _, id := range p.Models {
			add(id)
		}
		for id := range p.ModelNames {
			add(id)
		}
		tells := aiPickTells(ids, func(id string) string { return aiPickBaseLabel(p, id) })
		p.aiPickLabels = make(map[string]string, len(ids))
		for _, id := range ids {
			label := aiPickBaseLabel(p, id)
			if tell, ok := tells[id]; ok {
				label += " · " + tell
			}
			p.aiPickLabels[id] = label
		}
	}
	if label, ok := p.aiPickLabels[id]; ok {
		return label
	}
	return aiPickBaseLabel(p, id)
}

// aiPickTells is Magpie's modelTells (MIT, see
// apps/desktop/frontend/src/features/magpie-model-tells.js): for IDs that
// share a label, the "/" segments of each ID left after the segments all
// of them share at the start and at the end; the whole IDs when those do
// not tell them apart.
func aiPickTells(ids []string, label func(string) string) map[string]string {
	groups := map[string][]string{}
	var order []string
	for _, id := range ids {
		name := label(id)
		if id == "" || name == "" {
			continue
		}
		if _, ok := groups[name]; !ok {
			order = append(order, name)
		}
		if !slices.Contains(groups[name], id) {
			groups[name] = append(groups[name], id)
		}
	}
	out := map[string]string{}
	for _, name := range order {
		set := groups[name]
		if len(set) < 2 {
			continue
		}
		segments := make([][]string, len(set))
		for i, id := range set {
			segments[i] = strings.Split(id, "/")
		}
		first := segments[0]
		every := func(ok func(s []string) bool) bool {
			for _, s := range segments {
				if !ok(s) {
					return false
				}
			}
			return true
		}
		prefix := 0
		for every(func(s []string) bool { return len(s) > prefix && s[prefix] == first[prefix] }) {
			prefix++
		}
		suffix := 0
		for every(func(s []string) bool {
			return len(s)-suffix > prefix && s[len(s)-1-suffix] == first[len(first)-1-suffix]
		}) {
			suffix++
		}
		tells := make([]string, len(set))
		unique := map[string]bool{}
		for i, s := range segments {
			tells[i] = strings.Join(s[prefix:len(s)-suffix], "/")
			if tells[i] == "" {
				tells[i] = set[i]
			}
			unique[tells[i]] = true
		}
		if len(unique) < len(tells) {
			tells = set
		}
		for i, id := range set {
			out[id] = tells[i]
		}
	}
	return out
}
