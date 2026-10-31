package aiservices

import (
	"net/url"
	"strings"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/widgets"
)

// language is the page's language for presets' notes: "zh" or "en".
func language() string {
	if strings.HasPrefix(i18n.Locale(), "zh") {
		return "zh"
	}
	return "en"
}

// offeredPresets are the inference services of the catalog in the user's
// language that match search. Magpie also catalogs routers that only
// choose another provider; they generate nothing, so they are left out.
func offeredPresets(presets []servicePreset, search string) []servicePreset {
	lang := language()
	query := strings.ToLower(strings.TrimSpace(search))
	var out []servicePreset
	for _, preset := range presets {
		inLanguage := len(preset.Langs) == 0
		for _, l := range preset.Langs {
			inLanguage = inLanguage || l == lang || l == i18n.Locale()
		}
		inference := preset.Chat != "" || preset.Responses != "" || preset.Anthropic != "" || preset.Endpoint != ""
		if inLanguage && inference && strings.Contains(strings.ToLower(preset.Name+" "+preset.ID), query) {
			out = append(out, preset)
		}
	}
	return out
}

// presetPicker chooses a known service to add.
func (p *page) presetPicker(c *ui.Context) {
	if !p.choosingPreset {
		return
	}
	t := c.Theme()
	kit.Dialog(c, &p.choosingPreset, kit.DialogOptions{Title: i18n.T("ai.choosePreset"), Cancel: i18n.T("common.cancel"), Width: 520,
		Body: func() {
			ui.TextInput(c, &p.presetSearch).FillWidth().Placeholder(i18n.T("ai.searchProviders")).Label(i18n.T("ai.searchProviders")).AutoFocus()
			if p.catalog.err != "" {
				kit.Notice(c, kit.Failure, "", p.catalog.err, func() {
					if kit.Button(c, i18n.T("common.retry"), icons.RefreshCw, false).Clicked() {
						p.catalog.err = ""
						p.loadCatalog()
					}
				})
			}
			if p.catalog.busy {
				kit.Status(c, i18n.T("capture.loading"))
				return
			}
			entries := offeredPresets(p.catalog.presets, p.presetSearch)
			lang := language()
			ui.Column(c).FillWidth().Gap(2).Children(func() {
				for _, preset := range entries {
					preset := preset
					ui.Column(c.Key(preset.ID)).FillWidth().Gap(2).Children(func() {
						b := ui.ButtonBase(c).FillWidth().MinHeight(44).Padding(8, 12).Gap(12).Radius(t.Radius).AlignItems(ui.Center).
							Justify(ui.Start).Label(preset.Name)
						if b.Hovered() {
							b.Background(t.SurfaceHover)
						}
						if b.Clicked() {
							p.addPreset(preset)
							p.choosingPreset = false
						}
						b.Children(func() {
							widgets.ProviderIcon(c, p.env, preset.Icon, 20)
							ui.Text(c, preset.Name).Shrink(1).MinWidth(0)
							if preset.Sponsored {
								kit.Caption(c, i18n.T("ai.magpiePartner"))
							}
						})
						if preset.Sponsored && (preset.Website != "" || preset.KeysURL != "") {
							ui.Row(c).Gap(12).Wrap().Padding(0, 12).Children(func() {
								if preset.Website != "" {
									host := preset.SiteHost
									if host == "" {
										if u, err := url.Parse(preset.Website); err == nil {
											host = u.Hostname()
										}
									}
									ui.Link(c, host, preset.Website).FontSize(kit.HelpSize(c))
								}
								if preset.KeysURL != "" {
									ui.Link(c, i18n.T("ai.openKeys"), preset.KeysURL).FontSize(kit.HelpSize(c))
								}
							})
						}
						note := preset.Note
						if preset.Sponsored {
							note = preset.Notes[lang]
							if note == "" {
								note = preset.Notes["en"]
							}
						}
						if note != "" {
							kit.Help(c, note).Padding(0, 12)
						}
					})
				}
				if len(entries) == 0 && p.catalog.err == "" {
					kit.Help(c, i18n.T("ai.noMatchingServices"))
				}
			})
		}})
}

// agentPicker chooses the account type before an account connection is
// added. Closing it adds nothing, so no empty connection is left and no
// sign-in starts on its own.
func (p *page) agentPicker(c *ui.Context) {
	if !p.choosingAgent {
		return
	}
	t := c.Theme()
	kit.Dialog(c, &p.choosingAgent, kit.DialogOptions{Title: i18n.T("ai.chooseAgent"), Message: i18n.T("ai.chooseAgentHint"),
		Cancel: i18n.T("common.cancel"), Width: 480, Body: func() {
			ui.TextInput(c, &p.agentSearch).FillWidth().Placeholder(i18n.T("ai.searchAgents")).Label(i18n.T("ai.searchAgents")).AutoFocus()
			query := strings.ToLower(strings.TrimSpace(p.agentSearch))
			shown := 0
			ui.Column(c).FillWidth().Gap(2).Children(func() {
				for _, a := range p.catalog.agents {
					if !strings.Contains(strings.ToLower(a.Name+" "+a.ID), query) {
						continue
					}
					shown++
					a := a
					b := ui.ButtonBase(c.Key(a.ID)).FillWidth().MinHeight(44).Padding(8, 12).Gap(12).Radius(t.Radius).
						AlignItems(ui.Center).Label(a.Name)
					if b.Hovered() {
						b.Background(t.SurfaceHover)
					}
					if b.Clicked() {
						p.add(&a)
						p.choosingAgent = false
						// The next step is signing in: bring it into view.
						p.focusAccount = true
					}
					b.Children(func() {
						widgets.ProviderIcon(c, p.env, a.Icon, 20)
						ui.Text(c, a.Name).Grow(1).Shrink(1).MinWidth(0)
						ui.Icon(c, icons.ChevronRight).FontSize(16).TextColor(t.TextMuted).Shrink(0)
					})
				}
			})
			if shown == 0 {
				kit.Status(c, i18n.T("ai.noMatchingAgents"))
			}
		}})
}
