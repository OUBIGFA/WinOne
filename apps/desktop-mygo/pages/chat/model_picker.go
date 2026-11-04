package chat

import (
	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/widgets"
)

// modelPicker is the composer's model: a compact trigger showing the
// provider's mark, its name and the model, opening a searchable list of
// the enabled models grouped by provider: widgets.ModelMenu, which every
// model picker of the window shares.
// A model since removed is not a usable choice: the trigger asks to
// choose one rather than keep it.
func (p *Chat) modelPicker(c *ui.Context, provider *widgets.AIProvider, selection widgets.AISelection, disabled bool) {
	if p.ai == nil {
		return
	}
	t := c.Theme()
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
	if trigger.Hovered() || p.ui.models.Open {
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
		widgets.ModelMenuToggle(&p.ui.models, selection.Provider)
	}
	chosen, picked := widgets.ModelMenu(c, trigger, &p.ui.models, widgets.ModelMenuOptions{Env: p.env, Groups: widgets.ProviderGroups(p.ai.Providers),
		Selected: widgets.SelectionValue(selection), Label: i18n.T("chat.model"), Empty: i18n.T("chat.modelUnavailable")})
	if next, ok := widgets.ParseSelectionValue(chosen); picked && ok {
		p.run(func(done func(error)) { p.configure(&next, "", done) })
	}
}

// badges shows a model's badges, if it has any.
func badges(c *ui.Context, provider *widgets.AIProvider, id string) {
	if draw := widgets.ModelBadges(provider, id); draw != nil {
		draw(c)
	}
}
