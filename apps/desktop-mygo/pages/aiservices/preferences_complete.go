package aiservices

import (
	"github.com/egoist/mygo/ui"
	"slices"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/internal/ai"
)

// Keep the primary address compatible with existing saved configurations.
// Additional endpoints become an explicit set only after the user's edit.
func (e *editor) endpointPreferences(c *ui.Context, pr *provider, locked bool) {
	// Padding keeps the disclosure clear of the panel's edge, as its rows are.
	ui.Column(c).FillWidth().Padding(8, 8).Children(func() {
		ui.Collapsible(c, i18n.T("aiComplete.endpoints"), &e.endpointsOpen, func() {
			kit.Help(c, i18n.T("aiComplete.endpointsHint"))
			current := ai.ProviderEndpoints{}
			if pr.Endpoints != nil {
				current = *pr.Endpoints
			} else if preset := e.p.catalog.preset(pr.Preset); preset != nil {
				v := preset.endpoints
				for _, region := range preset.Regions {
					if region.ID == pr.PresetRegion {
						v = region.endpoints
					}
				}
				current = ai.ProviderEndpoints{Chat: v.Chat, Responses: v.Responses, Anthropic: v.Anthropic, Gemini: v.Gemini, Decide: v.Decide}
				// An edited primary URL intentionally stops inheriting preset URLs.
				if v.of(pr.Protocol) != pr.BaseURL {
					current = ai.ProviderEndpoints{}
				}
			}
			for _, row := range []struct {
				key, label string
				value      *string
			}{
				{"chat", "OpenAI Chat Completions", &current.Chat}, {"responses", "OpenAI Responses", &current.Responses},
				{"anthropic", "Anthropic Messages", &current.Anthropic}, {"gemini", "Google Gemini", &current.Gemini}, {"decide", "System One Decide", &current.Decide},
			} {
				if row.key == pr.Protocol {
					*row.value = pr.BaseURL
					continue
				}
				f := e.fields.get("endpoint:" + row.key)
				if text, changed := f.sync(*row.value); changed {
					*row.value = text
					copy := current
					pr.Endpoints = &copy
				}
				kit.Stack(c, row.label, "", func() { kit.TextInput(c, &f.text).FillWidth().Label(row.label + " URL").Disabled(locked) })
			}
		})
	})
}

func (e *editor) modelDiscoveryPreferences(c *ui.Context, pr *provider, locked bool) {
	s := &pr.ProviderPreferences
	kit.Group(c, nil, i18n.T("aiComplete.discovery"), "", func() {
		if pr.ModelRefreshError != "" {
			kit.Notice(c, kit.Caution, "", completeT("modelRefreshFailure"), nil)
		}
		if !pr.ModelRefreshAt.IsZero() {
			kit.Help(c, i18n.T("aiComplete.modelRefreshTime", "time", pr.ModelRefreshAt.Local().Format("2006-01-02 15:04")))
		}
		fetch := !s.ListOff
		if kit.Toggle(c, &fetch, i18n.T("aiComplete.fetchList"), i18n.T("aiComplete.fetchListHint")).Disabled(locked).Changed() {
			s.ListOff = !fetch
		}
		if pr.AuthMode == "account" {
			kit.Help(c, completeT("accountRefreshHint"))
		} else {
			kit.Stack(c, "", "", func() { nativeText(c, i18n.T("aiComplete.modelsURL"), &s.ModelsURL, locked || s.ListOff) })
			kit.Stack(c, "", i18n.T("aiComplete.refreshEveryHint"), func() { nativeText(c, i18n.T("aiComplete.refreshEvery"), &s.RefreshEvery, locked || s.ListOff) })
		}
		kit.Stack(c, i18n.T("aiComplete.autoInclude"), i18n.T("aiComplete.autoIncludeHint"), func() {
			before := append([]string{}, s.AutoInclude...)
			e.preferenceList(c, "autoInclude", i18n.T("aiComplete.autoInclude"), &s.AutoInclude, locked || s.ListOff)
			if !slices.Equal(before, s.AutoInclude) {
				pr.Models = ai.IncludeDiscovered(pr.Models, nil, pr.available(), s.AutoInclude)
			}
		})
		kit.Toggle(c, &s.Unlisted, i18n.T("aiComplete.unlisted"), i18n.T("aiComplete.unlistedHint")).Disabled(locked)
	})
	kit.Group(c, nil, i18n.T("aiComplete.modelCatalog"), "", func() {
		kit.Stack(c, "", "", func() {
			nativeText(c, i18n.T("aiComplete.family"), &s.Family, locked)
			nativeText(c, i18n.T("aiComplete.catalog"), &s.Catalog, locked)
		})
		kit.Help(c, completeT("contextRulesHint"))
		for _, id := range sortedKeys(s.Contexts) {
			ui.Row(c.Key(id)).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
				v := s.Contexts[id]
				ui.Column(c).Grow(1).Children(func() {
					if nativeInt(c, id, &v, 0, 100000000, locked) {
						s.Contexts[id] = v
					}
				})
				if kit.IconButton(c, icons.Trash2, i18n.T("common.delete")+" "+id).Disabled(locked).Clicked() {
					delete(s.Contexts, id)
				}
			})
		}
		newRule := e.fields.get("newContextRule")
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			kit.TextInput(c, &newRule.text).Grow(1).MinWidth(0).Label(completeT("contextRule")).Placeholder("*").Disabled(locked)
			if kit.Button(c, completeT("addContextRule"), icons.Plus, false).Disabled(locked || newRule.text == "").Clicked() {
				if s.Contexts == nil {
					s.Contexts = map[string]int{}
				}
				s.Contexts[newRule.text] = 128000
				newRule.text = ""
			}
		})
		kit.Stack(c, i18n.T("aiComplete.hiddenDrawers"), "", func() {
			e.preferenceList(c, "hiddenDrawers", i18n.T("aiComplete.hiddenDrawers"), &s.HiddenDrawers, locked)
		})
		kit.Stack(c, i18n.T("aiComplete.hiddenVideomakers"), "", func() {
			e.preferenceList(c, "hiddenVideomakers", i18n.T("aiComplete.hiddenVideomakers"), &s.HiddenVideomakers, locked)
		})
	})
}

func (e *editor) servicePreferences(c *ui.Context, pr *provider, locked bool) {
	s := &pr.ProviderPreferences
	kit.Group(c, nil, completeT("serviceLinks"), "", func() {
		kit.Stack(c, "", "", func() {
			nativeText(c, completeT("website"), &s.Website, locked)
			nativeText(c, completeT("keysURL"), &s.KeysURL, locked)
		})
	})
	if pr.AuthMode == "api-key" {
		kit.Group(c, nil, i18n.T("aiComplete.serviceCredentials"), i18n.T("aiComplete.serviceCredentialsHint"), func() {
			for _, field := range []struct {
				key   string
				value **string
				has   bool
			}{
				{"balanceToken", &s.BalanceToken, s.HasBalanceToken}, {"accessKeyID", &s.AccessKeyID, s.HasAccessKeyID}, {"secretAccessKey", &s.SecretAccessKey, s.HasSecretAccessKey},
			} {
				label := i18n.T("aiComplete." + field.key)
				kit.Stack(c.Key(field.key), label, "", func() {
					f := e.fields.get("serviceCredential:" + field.key)
					current := ""
					if *field.value != nil {
						current = **field.value
					}
					if text, changed := f.sync(current); changed {
						*field.value = new(text)
					}
					placeholder := "ai.keyPlaceholder"
					if field.has {
						placeholder = "ai.keySaved"
					}
					ui.Row(c).FillWidth().Gap(8).Children(func() {
						kit.TextInput(c, &f.text).Grow(1).MinWidth(0).Label(label).Password().Placeholder(i18n.T(placeholder)).Disabled(locked)
						if kit.IconButton(c, icons.Trash2, i18n.T("aiComplete.clearCredential", "name", label)).Disabled(locked).Clicked() {
							*field.value = new("")
							f.text, f.shown = "", ""
						}
					})
				})
			}
		})
	}
	kit.Group(c, nil, i18n.T("aiComplete.balanceService"), i18n.T("aiComplete.balanceServiceHint"), func() {
		kit.Stack(c, "", "", func() {
			nativeText(c, i18n.T("aiComplete.balanceURL"), &s.BalanceURL, locked)
			nativeText(c, i18n.T("aiComplete.balancePath"), &s.BalancePath, locked)
		})
		choiceRow(c, i18n.T("aiComplete.balanceMethod"), "", &s.BalanceMethod, options("", "GET", "POST", "POST"), locked)
	})
	if pr.Preset == "google-vertex" {
		kit.Group(c, nil, "Google Vertex AI", "", func() {
			v := ai.Vertex{}
			if s.Vertex != nil {
				v = *s.Vertex
			}
			before := v
			kit.Stack(c, "", "", func() {
				nativeText(c, i18n.T("aiComplete.cloudProject"), &v.Project, locked)
				nativeText(c, i18n.T("aiComplete.location"), &v.Location, locked)
				nativeText(c, i18n.T("aiComplete.credentialsFile"), &v.Credentials, locked)
				nativeText(c, i18n.T("aiComplete.impersonate"), &v.Impersonate, locked)
			})
			if v != before {
				s.Vertex = &v
			}
		})
	}
	kit.Group(c, nil, i18n.T("aiComplete.zhipuTeam"), "", func() {
		v := ai.ZhipuTeam{}
		if s.ZhipuTeam != nil {
			v = *s.ZhipuTeam
		}
		before := v
		kit.Stack(c, "", "", func() {
			nativeText(c, i18n.T("aiComplete.organization"), &v.Org, locked)
			nativeText(c, i18n.T("aiComplete.cloudProject"), &v.Project, locked)
		})
		if v != before {
			s.ZhipuTeam = &v
		}
	})
}
