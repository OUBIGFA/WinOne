package aiservices

import (
	"github.com/egoist/mygo/ui"
	"slices"
	"strings"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/widgets"
)

func completeT(key string) string { return i18n.T("aiComplete." + key) }

// Helper overrides use the levels accepted by upstream Settings. Per-model
// editors continue to use each vendor's own advertised vocabulary.
func (a *advancedPanel) effortField(c *ui.Context, key string, current *string, locked bool) {
	choices := options("", i18n.T("ai.automatic"))
	for _, level := range []string{"none", "minimal", "low", "medium", "high", "xhigh", "max", "ultra"} {
		if (key == "desktopSubagentEffort" || key == "agentEffort") && level == "ultra" {
			continue
		}
		choices = append(choices, kit.SelectOption{Value: level, Label: level})
	}
	nativeSelect(c, completeT(key), current, choices, locked)
}

func (a *advancedPanel) autoResetWhen(c *ui.Context, key string, locked bool) {
	s := a.settings()
	when := s.CodexAutoResetOnly[key]
	if nativeSelect(c, completeT("autoResetWhen"), &when, options("", completeT("resetBoth"), "week", completeT("resetWeek"), "expiring", completeT("resetExpiring")), locked) {
		if s.CodexAutoResetOnly == nil {
			s.CodexAutoResetOnly = map[string]string{}
		}
		if when == "" {
			delete(s.CodexAutoResetOnly, key)
		} else {
			s.CodexAutoResetOnly[key] = when
		}
	}
}

func (a *advancedPanel) completeClientOptions(c *ui.Context, catalog *magpieCatalog, locked bool) {
	s := a.settings()
	kit.Group(c, icons.Layers, completeT("clientRequests"), completeT("clientRequestsHint"), func() {
		kit.Toggle(c, &s.BareModelIDs, completeT("bareModelIDs"), completeT("bareModelIDsHint")).Disabled(locked)
		a.helperModel(c, completeT("codexDescriptions"), s.CodexDescriptions, catalog.TitleModels, "", "", locked, func(v string) { s.CodexDescriptions = v })
		a.helperModel(c, completeT("codexPredictions"), s.CodexPredictions, catalog.TitleModels, "", "", locked, func(v string) { s.CodexPredictions = v })
		models := []routeModel{}
		for _, m := range catalog.TitleModels {
			models = append(models, routeModel{ID: m.ID, Name: m.Name, Provider: m.Provider, ProviderName: m.ProviderName})
		}
		nativeModelField(c, a.p, completeT("desktopSubagentModel"), &s.DesktopSubagentModel, models, []widgets.ModelChoice{{Value: "", Label: completeT("followRequest")}}, locked)
		a.effortField(c, "desktopSubagentEffort", &s.DesktopSubagentEffort, locked)
		kit.Toggle(c, &s.DesktopLongest, completeT("desktopLongest"), completeT("desktopLongestHint")).Disabled(locked)
		nativeList(c, a.fields, "disabledModels", completeT("disabledModels"), &s.DisabledModels, locked)
		kit.Help(c, completeT("disabledModelsHint"))
		kit.Toggle(c, &s.CodexAsyncCompact, completeT("codexAsyncCompact"), completeT("codexAsyncCompactHint")).Disabled(locked)
		if s.CodexAsyncCompact {
			nativeInt(c, completeT("codexAsyncCompactPercent"), &s.CodexAsyncCompactPercent, 0, 100, locked)
		}
		if kit.Button(c, completeT("copyAsyncConfig"), icons.Copy, false).Disabled(!s.CodexAsyncCompact).Clicked() {
			c.Services().WriteClipboard(s.CodexAsyncSnippet())
		}
	})
	kit.Group(c, icons.ListChecks, completeT("clientModels"), completeT("clientModelsHint"), func() {
		choices := []kit.SelectOption{}
		for _, client := range catalog.Clients {
			choices = append(choices, kit.SelectOption{Value: client.ID, Label: client.Name})
		}
		if a.clientID == "" {
			a.clientID = "codex"
		}
		if !slices.ContainsFunc(choices, func(v kit.SelectOption) bool { return v.Value == a.clientID }) {
			choices = append(choices, kit.SelectOption{Value: a.clientID, Label: a.clientID})
		}
		nativeSelect(c, completeT("client"), &a.clientID, choices, locked)
		nativeText(c, completeT("clientIdentifier"), &a.clientID, locked)
		id := a.clientID
		countAs := s.ClientAgents[id]
		nativeText(c, completeT("clientCountAs"), &countAs, locked)
		if countAs != s.ClientAgents[id] {
			if s.ClientAgents == nil {
				s.ClientAgents = map[string]string{}
			}
			if countAs == "" {
				delete(s.ClientAgents, id)
			} else {
				s.ClientAgents[id] = countAs
			}
		}
		effort := s.AgentEfforts[id]
		a.effortField(c, "agentEffort", &effort, locked)
		if effort != s.AgentEfforts[id] {
			if s.AgentEfforts == nil {
				s.AgentEfforts = map[string]string{}
			}
			if effort == "" {
				delete(s.AgentEfforts, id)
			} else {
				s.AgentEfforts[id] = effort
			}
		}
		hide := slices.Contains(s.OwnModelsHidden, id)
		if kit.Toggle(c, &hide, completeT("ownModelsHidden"), "").Disabled(locked).Changed() {
			s.OwnModelsHidden = toggleString(s.OwnModelsHidden, id, hide)
		}
		_, only := s.PickedModels[id]
		if kit.Toggle(c, &only, completeT("onlyPickedModels"), completeT("onlyPickedModelsHint")).Disabled(locked).Changed() {
			if s.PickedModels == nil {
				s.PickedModels = map[string][]string{}
			}
			if only {
				s.PickedModels[id] = []string{}
			} else {
				delete(s.PickedModels, id)
			}
		}
		for _, row := range []struct {
			key     string
			mapping *map[string][]string
		}{
			{"visible", &s.Visible}, {"pickedModels", &s.PickedModels}, {"hiddenModels", &s.HiddenModels}, {"orderedModels", &s.OrderedModels}, {"fastPicks", &s.FastPicks}, {"ultraPicks", &s.UltraPicks},
		} {
			if row.key == "pickedModels" && !only || row.key == "hiddenModels" && only {
				continue
			}
			open := a.opens["client:"+row.key]
			ui.Collapsible(c, completeT(row.key), &open, func() {
				list := slices.Clone((*row.mapping)[id])
				nativeList(c, a.fields, "client:"+id+":"+row.key, completeT(row.key), &list, locked)
				if !slices.Equal(list, (*row.mapping)[id]) {
					if *row.mapping == nil {
						*row.mapping = map[string][]string{}
					}
					(*row.mapping)[id] = list
					if row.key == "ultraPicks" {
						if s.FastPicks == nil {
							s.FastPicks = map[string][]string{}
						}
						s.FastPicks[id] = unique(append(s.FastPicks[id], list...))
					}
					if row.key == "fastPicks" && s.UltraPicks != nil {
						s.UltraPicks[id] = slices.DeleteFunc(slices.Clone(s.UltraPicks[id]), func(v string) bool { return !slices.Contains(list, v) })
					}
				}
			})
			a.opens["client:"+row.key] = open
		}
	})
	a.observability(c, locked)
}

func (a *advancedPanel) observability(c *ui.Context, locked bool) {
	if !a.p.builtinEnabled("otel") {
		return
	}
	s := &a.settings().OTel
	kit.Group(c, icons.Activity, completeT("observability"), completeT("observabilityHint"), func() {
		kit.Toggle(c, &s.Enabled, completeT("otelEnabled"), "").Disabled(locked)
		if a.otelKind == "" {
			a.otelKind = "otlp"
			if strings.HasSuffix(s.Endpoint, langfusePath) {
				a.otelKind = "langfuse"
			}
		}
		choiceRow(c, upstreamT("Collector"), "", &a.otelKind, options("otlp", "OTLP", "langfuse", "Langfuse"), locked)
		if a.otelKind == "langfuse" {
			a.langfuseView(c, locked)
		} else {
			nativeText(c, completeT("otelEndpoint"), &s.Endpoint, locked)
		}
		for _, row := range []struct {
			key   string
			value *bool
		}{{"otelMetrics", &s.Metrics}, {"otelSessions", &s.Sessions}, {"otelBodies", &s.Bodies}, {"otelBodiesWhole", &s.BodiesWhole}} {
			hint := ""
			if row.key == "otelSessions" {
				hint = completeT("otelSessionsUnavailable")
			}
			kit.Toggle(c, row.value, completeT(row.key), hint).Disabled(locked || row.key == "otelSessions")
		}
		for _, name := range sortedKeys(s.Headers) {
			ui.Row(c.Key(name)).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
				value := s.Headers[name]
				ui.Column(c).Grow(1).Children(func() {
					ui.Field(c, name, func() {
						kit.TextInput(c, &value).FillWidth().Password().Placeholder(completeT("savedHeader")).Disabled(locked)
					})
				})
				s.Headers[name] = value
				if kit.IconButton(c, icons.Trash2, i18n.T("common.delete")+" "+name).Disabled(locked).Clicked() {
					delete(s.Headers, name)
				}
			})
		}
		field := a.fields.get("otelHeader")
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			kit.TextInput(c, &field.text).Grow(1).MinWidth(0).Label(completeT("otelHeader")).Disabled(locked)
			if kit.Button(c, completeT("addHeader"), icons.Plus, false).Disabled(locked || field.text == "").Clicked() {
				if s.Headers == nil {
					s.Headers = map[string]string{}
				}
				s.Headers[field.text] = ""
				field.text = ""
			}
		})
	})
}
