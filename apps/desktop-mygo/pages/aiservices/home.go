package aiservices

import (
	"github.com/egoist/mygo/ui"
	"slices"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

func homeT(key string) string { return i18n.T("aiHome." + key) }

func (p *page) homeView(c *ui.Context) {
	if p.draft == nil {
		return
	}
	// Home holds the everyday model and result behavior. Connections remain
	// directly reachable through the provider tabs and the compact preview.
	ui.Column(c).FillWidth().Gap(kit.GroupGap).Children(func() {
		p.quickStart(c)
		p.defaultModelView(c)
		p.responseBehaviorView(c)
		kit.Group(c, nil, i18n.T("aiUpdate.runtime"), "", func() {
			s := p.draft.Value.settings()
			kit.Toggle(c, &s.HideRequestText, i18n.T("aiUpdate.hideText"), i18n.T("aiUpdate.hideTextHint")).Disabled(p.locked())
			kit.Stack(c, "", i18n.T("aiUpdate.timeZoneHint"), func() { nativeText(c, i18n.T("aiUpdate.timeZone"), &s.TimeZone, p.locked()) })
		})
	})
}

// quickStart guides a first setup through the three things every AI
// feature needs: a provider, its enabled models and the everyday model.
// It reads the draft, so a step turns done as the user makes it, and it
// leaves the page once all three are done. Each step's action opens the
// existing tab or picker; the guide adds no settings of its own.
func (p *page) quickStart(c *ui.Context) {
	d := &p.draft.Value
	var first, withModels *provider
	for i := range d.Providers {
		pr := &d.Providers[i].Value
		if first == nil {
			first = pr
		}
		if withModels == nil && len(pr.Models) > 0 && !pr.Off {
			withModels = pr
		}
	}
	// A saved identifier is not a usable default when its model was
	// removed or the provider was disabled. Keep the recovery step visible.
	selection := d.Tasks["default"]
	selectedProvider := d.find(selection.Provider)
	chosen := selectedProvider != nil && !selectedProvider.Off && slices.Contains(selectedProvider.Models, selection.Model)
	steps := []bool{first != nil, withModels != nil, chosen}
	current := len(steps)
	for i, done := range steps {
		if !done {
			current = i
			break
		}
	}
	if current == len(steps) {
		return
	}
	locked := p.locked()
	full := len(d.Providers) >= maxProviders
	// The current step's first action is the page's primary action.
	action := func(step int, label string, icon *ui.SVG, disabled bool) bool {
		if step == current {
			return kit.PrimaryButton(c, label, icon, false).Disabled(locked || disabled).Clicked()
		}
		return kit.Button(c, label, icon, false).Disabled(locked || disabled).Clicked()
	}
	kit.Group(c, nil, homeT("startTitle"), "", func() {
		setupStep(c, 1, steps[0], homeT("stepProvider"), homeT("stepProviderHint"), func() {
			if action(0, i18n.T("ai.addPreset"), icons.PlugZap, full) {
				p.switchTab(tabCustom)
				p.presetSearch, p.choosingPreset = "", true
				p.loadCatalog()
			}
			if kit.Button(c, homeT("signIn"), icons.UserRound, false).Disabled(locked || full).Clicked() {
				p.switchTab(tabAccounts)
				p.agentSearch, p.choosingAgent = "", true
				p.loadCatalog()
			}
		})
		setupStep(c, 2, steps[1], homeT("stepModels"), homeT("stepModelsHint"), func() {
			if first == nil {
				return
			}
			if action(1, homeT("openModels"), icons.ListChecks, false) {
				p.openProvider(first.ID)
			}
		})
		setupStep(c, 3, steps[2], homeT("stepDefault"), homeT("stepDefaultHint"), nil)
	})
}

// setupStep is a row of the quick start: its number, or a check once
// done, its task and the action that does it.
func setupStep(c *ui.Context, n int, done bool, title, help string, actions func()) {
	t, palette := c.Theme(), kit.Colors(c)
	ui.Row(c.Key(n)).FillWidth().MinHeight(kit.RowMinHeight).Padding(12, kit.RowPadX).GapX(24).GapY(8).
		AlignItems(ui.Center).AlignContent(ui.Center).Wrap().Children(func() {
		ui.Row(c).Basis(260).Grow(1).Shrink(1).Gap(12).AlignItems(ui.Start).Children(func() {
			mark := ui.Row(c).Width(24).Height(24).Radius(kit.PillRadius).Shrink(0).Justify(ui.Center).AlignItems(ui.Center)
			if done {
				mark.Background(palette.SuccessSoft).Children(func() {
					ui.Icon(c, icons.Check).FontSize(14).TextColor(t.Success)
				})
			} else {
				mark.Background(t.Surface).Border(1, palette.Hairline).Children(func() {
					ui.Text(c, kit.Itoa(n)).FontSize(kit.CaptionSize(c)).TextColor(t.TextMuted)
				})
			}
			ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(4).Children(func() {
				label := kit.Label(c, title)
				if done {
					label.TextColor(t.TextMuted)
				}
				kit.Help(c, help)
			})
		})
		if actions != nil && !done {
			ui.Row(c).Shrink(0).Gap(8).Wrap().AlignItems(ui.Center).Children(actions)
		}
	})
}
