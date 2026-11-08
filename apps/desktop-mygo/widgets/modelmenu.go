package widgets

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"winone/modules/nativeui/modelmenu"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

// ModelMenu is the one model list every model picker of WinOne opens,
// the Chat composer's, drawn by winone/modules/nativeui/modelmenu which
// the result windows share too. Change it there and every picker follows;
// this file only turns the window's providers into its data.

// ModelChoice is an entry of a model menu that is not a model.
type ModelChoice struct {
	Value, Label string
	Icon         *ui.SVG
}

// ModelGroup is a provider's models in a model menu.
type ModelGroup struct {
	ID, Name, Mark string
	Models         []ModelEntry
}

// ModelEntry is one model of a ModelGroup. Value is what the menu returns
// when it is chosen; ID and Name pick its mark; Label is what it shows.
type ModelEntry struct {
	Value, ID, Name, Label string
	Badge                  func(c *ui.Context)
}

// ModelMenuState is what a model menu keeps between frames.
type ModelMenuState = modelmenu.State

// ModelMenuOptions configure ModelMenu.
type ModelMenuOptions struct {
	Env      *core.Env
	Choices  []ModelChoice
	Groups   []ModelGroup
	Selected string
	// Label names the list for assistive technology.
	Label string
	// Empty is shown when there are no models at all.
	Empty string
}

// SelectionValue is how ProviderGroups names a provider's model.
func SelectionValue(s AISelection) string { return s.Provider + "\x00" + s.Model }

// ParseSelectionValue reverses SelectionValue; ok is false for values that
// name no model, as those of choices.
func ParseSelectionValue(value string) (AISelection, bool) {
	provider, model, ok := strings.Cut(value, "\x00")
	return AISelection{Provider: provider, Model: model}, ok
}

// ProviderGroups lists the enabled models of providers for a model menu.
func ProviderGroups(providers []AIProvider) []ModelGroup {
	groups := make([]ModelGroup, 0, len(providers))
	for i := range providers {
		p := &providers[i]
		if p.Off {
			continue
		}
		mark := p.Avatar
		if mark == "" {
			mark = p.Icon
		}
		group := ModelGroup{ID: p.ID, Name: p.Name, Mark: mark}
		for _, model := range p.Models {
			group.Models = append(group.Models, ModelEntry{Value: SelectionValue(AISelection{Provider: p.ID, Model: model}),
				ID: model, Name: p.ModelNames[model], Label: ModelLabel(p, model), Badge: aiPickBadges(p, model)})
		}
		groups = append(groups, group)
	}
	return groups
}

// ModelMenuToggle opens or closes the menu of a trigger that was clicked.
// Opening clears the search and unfolds the group of the chosen model.
func ModelMenuToggle(s *ModelMenuState, selectedGroup string) { modelmenu.Toggle(s, selectedGroup) }

// ModelMenu shows the menu of state on trigger while it is open. It returns
// the value chosen and whether one was chosen; choosing closes the menu.
func ModelMenu(c *ui.Context, trigger ui.Element, state *ModelMenuState, o ModelMenuOptions) (string, bool) {
	choices := make([]modelmenu.Choice, len(o.Choices))
	for i, choice := range o.Choices {
		choices[i] = modelmenu.Choice{Value: choice.Value, Label: choice.Label, Icon: choice.Icon}
	}
	groups := make([]modelmenu.Group, 0, len(o.Groups))
	for _, g := range o.Groups {
		mark := g.Mark
		group := modelmenu.Group{ID: g.ID, Name: g.Name,
			Icon: func(c *ui.Context, size float32) ui.Element { return ProviderIcon(c, o.Env, mark, size) }}
		for _, m := range g.Models {
			id, name := m.ID, m.Name
			group.Models = append(group.Models, modelmenu.Entry{Value: m.Value, Label: m.Label, Keywords: m.ID + " " + m.Name,
				Icon:  func(c *ui.Context, size float32) ui.Element { return ModelIcon(c, o.Env, id, name, size) },
				Badge: m.Badge})
		}
		groups = append(groups, group)
	}
	return modelmenu.Menu(c, trigger, state, modelmenu.Options{Choices: choices, Groups: groups, Selected: o.Selected,
		Strings: modelmenu.Strings{Search: i18n.T("chat.searchModels"), List: o.Label, Empty: o.Empty, NoMatch: i18n.T("chat.modelUnavailable")}})
}

// ModelFieldOptions configure ModelField.
type ModelFieldOptions struct {
	ModelMenuOptions
	// Placeholder shows while the value names nothing the menu offers and
	// is empty.
	Placeholder string
	Disabled    bool
}

// ModelField is a form's model picker: a field showing the chosen model
// (its provider's mark, provider and model) or choice, opening ModelMenu.
// A value the menu no longer offers stays shown as it is.
func ModelField(c *ui.Context, o ModelFieldOptions) (string, bool) {
	t := c.Theme()
	var group *ModelGroup
	var entry *ModelEntry
	var choice *ModelChoice
	for i := range o.Choices {
		if o.Choices[i].Value == o.Selected {
			choice = &o.Choices[i]
		}
	}
	for i := range o.Groups {
		for j := range o.Groups[i].Models {
			if o.Groups[i].Models[j].Value == o.Selected {
				group, entry = &o.Groups[i], &o.Groups[i].Models[j]
			}
		}
	}
	b := ui.ButtonBase(c).FillWidth().MinHeight(kit.ControlHeight).Padding(5, 10).Gap(8).Radius(t.Radius).Background(t.Surface).
		Border(1, t.Border).Justify(ui.SpaceBetween).AlignItems(ui.Center).MinWidth(0).Label(o.Label).Disabled(o.Disabled)
	state := ui.Local(b, "model-menu", func() ModelMenuState { return ModelMenuState{} })
	if b.Hovered() || state.Open {
		b.Background(t.SurfaceHover)
	}
	shown := o.Placeholder
	switch {
	case entry != nil:
		shown = group.Name + " · " + entry.Label
	case choice != nil:
		shown = choice.Label
	case o.Selected != "":
		if s, ok := ParseSelectionValue(o.Selected); ok {
			shown = s.Provider
			for i := range o.Groups {
				if o.Groups[i].ID == s.Provider {
					shown = o.Groups[i].Name
				}
			}
			if s.Model != "" {
				shown += " · " + s.Model
			}
		} else {
			shown = o.Selected
		}
	}
	b.Tooltip(shown)
	b.Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Grow(1).Shrink(1).MinWidth(0).Children(func() {
			switch {
			case entry != nil:
				ProviderIcon(c, o.Env, group.Mark, aiPickIconSize).Shrink(0)
				ui.Text(c, group.Name).TextColor(t.TextMuted).SingleLine().Ellipsis("…").MinWidth(0).Shrink(2)
				ui.Text(c, "·").TextColor(t.TextMuted)
				ui.Text(c, entry.Label).SingleLine().Ellipsis("…").MinWidth(0).Shrink(1)
				if entry.Badge != nil {
					entry.Badge(c)
				}
			case choice != nil:
				icon := choice.Icon
				if icon == nil {
					icon = icons.Bot
				}
				ui.Icon(c, icon).FontSize(15).TextColor(t.TextMuted).Shrink(0)
				ui.Text(c, shown).SingleLine().Ellipsis("…").MinWidth(0).Shrink(1)
			case o.Selected != "":
				ui.Text(c, shown).SingleLine().Ellipsis("…").MinWidth(0).Shrink(1)
			default:
				ui.Text(c, shown).SingleLine().Ellipsis("…").MinWidth(0).Shrink(1).TextColor(t.TextMuted)
			}
		})
		ui.Icon(c, icons.ChevronDown).FontSize(14).TextColor(t.TextMuted).Shrink(0)
	})
	if b.Clicked() {
		selectedGroup := ""
		if group != nil {
			selectedGroup = group.ID
		}
		ModelMenuToggle(state, selectedGroup)
	}
	return ModelMenu(c, b, state, o.ModelMenuOptions)
}
