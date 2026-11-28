package aiservices

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strconv"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

// Magpie ce2ce86 app.js contextPicks/outputField/compactField, in decimal
// K/M. These are explicit actions; none is installed as a model default.
var (
	contextPresets    = []float64{128000, 200000, 256000, 400000, 512000, 1000000}
	outputPresets     = []float64{8000, 16000, 32000, 64000, 128000}
	compactionPresets = []float64{128000, 200000, 272000, 500000, 1000000}
	// genericLevels are what a model with no reported levels may be
	// declared with (Magpie provider/modelprefs.go Levels); effortRank
	// adds ultra, which Magpie fits to max. A model's own levels are its
	// vendor's words, never this ladder.
	genericLevels = []string{"none", "minimal", "low", "medium", "high", "xhigh", "max"}
	effortRank    = append(append([]string{}, genericLevels...), "ultra")
)

func tokenLabel(v float64) string {
	switch {
	case math.Mod(v, 1000000) == 0:
		return strconv.FormatFloat(v/1000000, 'f', -1, 64) + "M"
	case math.Mod(v, 1000) == 0:
		return strconv.FormatFloat(v/1000, 'f', -1, 64) + "K"
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// settings edits one model's settings, staying in the page's draft: the
// model's routing ID never changes, and closing keeps the edits unsaved.
func (m *modelsPanel) settings(c *ui.Context, pr *provider, disabled bool) {
	p := m.p
	id, model := pr.ID, m.editing
	open := true
	meta := pr.ModelMetadata[model].Value
	report := pr.CatalogMetadata[model]
	// Only the report supplies choices and default labels. The controls keep
	// absent overrides absent, including after a model is fetched or selected.
	if len(report.Levels) > 0 {
		meta.Levels = &report.Levels
	}
	if len(report.Tiers) > 0 {
		meta.Tiers = &report.Tiers
	}
	if report.MaxContextWindow > 0 {
		meta.MaxContextWindow = ptr(float64(report.MaxContextWindow))
		meta.ListedContextWindow = ptr(float64(report.ListedContextWindow))
	}
	change := func(edit func(*modelMetadata)) {
		p.change(id, func(t *provider) {
			if t.ModelMetadata == nil {
				t.ModelMetadata = map[string]metadata{}
			}
			d := t.ModelMetadata[model]
			edit(&d.Value)
			t.ModelMetadata[model] = d
		})
	}
	defaultLabel := i18n.T("ai.modelDefault")
	name := pr.ModelNames[model]
	if name == "" {
		name = model
	}
	// These tabs are views of the same unsaved model. They never commit or
	// discard its draft; the page's Save/Undo transaction owns that decision.
	identity := pr.Name
	if name != model {
		identity += " · " + model
	}
	kit.Dialog(c, &open, kit.DialogOptions{Title: name, Message: identity,
		Cancel: i18n.T("common.close"), Width: 760, Note: i18n.T("aiComplete.modelDraft"),
		Header: func() {
			kit.SubTabs(c, &m.settingsTab, []string{i18n.T("aiComplete.modelBasics"), i18n.T("aiComplete.modelCapabilities"), i18n.T("aiComplete.modelRequest"), i18n.T("aiComplete.modelPricing")}, nil)
		},
		Actions: func() {
			if kit.GhostButton(c, i18n.T("ai.modelRestoreDefaults"), icons.RotateCcw, false).Disabled(disabled).Clicked() {
				p.change(id, func(t *provider) {
					delete(t.ModelMetadata, model)
					if name := t.CatalogNames[model]; name != "" {
						t.ModelNames[model] = name
					} else {
						delete(t.ModelNames, model)
					}
				})
				m.fields = fields{}
				m.modelHeaders = nil
				m.headersFor = ""
				// The dialog captured metadata before its action was handled.
				// Draw again with the cleared override, including typed numbers.
				c.Invalidate()
			}
		},
		Body: func() {
			ui.Column(c.Key(m.settingsTab)).FillWidth().Gap(20).Disabled(disabled).Children(func() {
				switch m.settingsTab {
				case 0:
					ui.Field(c, i18n.T("ai.modelDisplayName"), func() {
						field := m.fields.get("name")
						if name, edited := field.sync(pr.ModelNames[model]); edited {
							// Names show in lists; 80 characters, as React's field
							// allowed.
							if r := []rune(name); len(r) > 80 {
								name = string(r[:80])
								field.text, field.shown = name, name
							}
							p.change(id, func(t *provider) {
								names := map[string]string{}
								for k, v := range t.ModelNames {
									names[k] = v
								}
								if name == "" {
									delete(names, model)
								} else {
									names[model] = name
								}
								t.ModelNames = names
							})
						}
						kit.TextInput(c, &field.text).FillWidth().Placeholder(model).Label(i18n.T("ai.modelDisplayName"))
					})

					m.limitsOf(c, meta, change, defaultLabel, report.ContextWindow, report.MaxTokens)
				case 1:
					m.reasoning(c, meta, change, defaultLabel)
					m.efforts(c, meta, change)
					if pr.AuthMode == "api-key" && pr.Preset != "remote-magpie" && (pr.Protocol == "chat" || pr.Protocol == "responses" || pr.Endpoints != nil && (pr.Endpoints.Chat != "" || pr.Endpoints.Responses != "")) {
						kind := value(meta.Kind)
						if nativeSelect(c, i18n.T("ai.modelKind"), &kind, options("", defaultLabel, "image", i18n.T("ai.modelKindImage"), "video", i18n.T("ai.modelKindVideo")), disabled) {
							change(func(d *modelMetadata) { d.Kind = ptr(kind) })
						}
					}
				case 2:
					alias := m.fields.get("alias")
					if text, edited := alias.sync(value(meta.Alias)); edited {
						change(func(d *modelMetadata) { d.Alias = ptr(text) })
					}
					nativeText(c, completeT("modelAlias"), &alias.text, disabled)
					kit.Help(c, completeT("modelAliasHint"))
					ui.Row(c).FillWidth().GapX(16).GapY(12).Wrap().AlignItems(ui.Start).Children(func() {
						for _, key := range []string{"upstreamName", "same"} {
							key := key
							field := &meta.UpstreamName
							labelKey := "ai.modelUpstreamName"
							if key == "same" {
								field = &meta.Same
								labelKey = "ai.modelSame"
							}
							synced := m.fields.get(key)
							if text, edited := synced.sync(value(*field)); edited {
								change(func(d *modelMetadata) {
									if key == "same" {
										d.Same = ptr(text)
									} else {
										d.UpstreamName = ptr(text)
									}
								})
							}
							ui.Column(c.Key(key)).Basis(240).Grow(1).MinWidth(0).Gap(4).Children(func() {
								ui.Field(c, i18n.T(labelKey), func() {
									kit.TextInput(c, &synced.text).FillWidth().Placeholder(model).Label(i18n.T(labelKey))
								})
								kit.Help(c, i18n.T(labelKey+"Hint"))
							})
						}
					})
					ui.Field(c, i18n.T("ai.modelAPI"), func() {
						api := value(meta.API)
						changed, trigger := kit.Select(c, &api, []kit.SelectOption{{Value: "", Label: defaultLabel},
							{Value: "chat", Label: "OpenAI Chat Completions"}, {Value: "responses", Label: "OpenAI Responses"},
							{Value: "anthropic", Label: "Anthropic Messages"}, {Value: "gemini", Label: "Gemini generateContent"}, {Value: "decide", Label: "System One"}})
						trigger.Label(i18n.T("ai.modelAPI"))
						if changed {
							change(func(d *modelMetadata) { d.API = ptr(api) })
						}
					})
					system := m.fields.get("systemPrompt")
					if text, edited := system.sync(value(meta.SystemPrompt)); edited {
						change(func(d *modelMetadata) { d.SystemPrompt = ptr(text) })
					}
					ui.Column(c).FillWidth().Gap(8).Children(func() {
						kit.Label(c, i18n.T("ai.modelSystemPrompt"))
						kit.Help(c, i18n.T("ai.modelSystemPromptHint"))
						ui.TextArea(c, &system.text).FillWidth().Lines(3, 8).Label(i18n.T("ai.modelSystemPrompt"))
					})

					if pr.AuthMode == "api-key" {
						if m.headersFor != model {
							m.headersFor = model
							m.modelHeaders = nil
							for _, row := range headerRows(meta.Headers) {
								row := row
								m.modelHeaders = append(m.modelHeaders, &row)
							}
						}
						if requestHeaderRows(c, &m.modelHeaders, 32, disabled) {
							rows := make([]headerRow, len(m.modelHeaders))
							for i, row := range m.modelHeaders {
								rows[i] = *row
							}
							change(func(d *modelMetadata) { d.Headers = headersOf(rows) })
						}
						kit.Help(c, i18n.T("aiComplete.modelHeadersHint"))
					}
				case 3:
					var listed *core.Preserved[modelPrice]
					if report.Price != nil {
						raw, _ := json.Marshal(report.Price)
						_ = json.Unmarshal(raw, &listed)
					}
					m.pricing(c, meta, change, listed)
				}
			})
		}})
	if !open {
		m.editing = ""
		m.fields = fields{}
		m.modelHeaders = nil
		m.headersFor = ""
		m.settingsTab = 0
	}
}

// presetButtons are one-click values beside a field; the matching one
// shows pressed.
func presetButtons[T comparable](c *ui.Context, label string, current T, options []T, text func(T) string, choose func(T)) {
	ui.Row(c).FillWidth().Gap(8).Wrap().Role(ui.RoleGroup).Label(label).Children(func() {
		for _, option := range options {
			option := option
			b := kit.GhostButton(c.Key(text(option)), text(option), nil, false).Checked(current == option)
			if current == option {
				b.Background(c.Theme().SurfacePressed)
			}
			if b.Clicked() {
				choose(option)
			}
		}
	})
}

// efforts offers the levels the model reports. Older saved metadata had
// only the offered list: vendor words there are the model's levels;
// otherwise its levels are unknown and Magpie's declarable levels apply.
func (m *modelsPanel) efforts(c *ui.Context, meta modelMetadata, change func(func(*modelMetadata))) {
	levels, efforts := value(meta.Levels), value(meta.Efforts)
	legacyVendor := len(levels) == 0 && slices.ContainsFunc(efforts, func(e string) bool { return !slices.Contains(effortRank, e) })
	base := genericLevels
	switch {
	case len(levels) > 0:
		base = levels
	case legacyVendor:
		base = efforts
	}
	choices := unique(append(append([]string{}, base...), efforts...))
	if meta.Efforts == nil {
		efforts = append([]string{}, base...)
	}
	ui.Column(c).FillWidth().Gap(8).Role(ui.RoleGroup).Label(i18n.T("ai.modelOfferedEfforts")).Children(func() {
		kit.Label(c, i18n.T("ai.modelOfferedEfforts")).FontWeight(600)
		ui.Row(c).FillWidth().GapX(16).GapY(8).Wrap().Children(func() {
			for _, effort := range choices {
				effort := effort
				on := slices.Contains(efforts, effort)
				if ui.Checkbox(c.Key(effort), &on, effort).Changed() {
					change(func(d *modelMetadata) {
						if legacyVendor {
							kept := append([]string{}, choices...)
							d.Levels = &kept
						}
						var next []string
						for _, v := range choices {
							if v == effort && on || v != effort && slices.Contains(efforts, v) {
								next = append(next, v)
							}
						}
						if next == nil {
							next = []string{}
						}
						d.Efforts = &next
					})
				}
			}
		})
		kit.Help(c, i18n.T("ai.modelOfferedEffortsHint"))
	})
}

var priceParts5 = []string{"input", "output", "cache_read", "cache_write", "cache_write_1h"}

func (pp *priceParts) field(key string) **float64 {
	switch key {
	case "input":
		return &pp.Input
	case "output":
		return &pp.Output
	case "cache_read":
		return &pp.CacheRead
	case "cache_write":
		return &pp.CacheWrite
	}
	return &pp.CacheWrite1h
}

// pricing is Magpie's Names & levels prices in dollars per million
// tokens, with an optional long-context tier; an empty field keeps the
// list price.
func (m *modelsPanel) pricing(c *ui.Context, meta modelMetadata, change func(func(*modelMetadata)), listed *core.Preserved[modelPrice]) {
	var price modelPrice
	if meta.Price != nil {
		price = meta.Price.Value
	}
	setPrice := func(edit func(*core.Preserved[modelPrice])) {
		change(func(d *modelMetadata) {
			next := core.Preserved[modelPrice]{}
			if d.Price != nil {
				next = *d.Price
			} else if listed != nil {
				// Upstream edits a complete custom price starting from the
				// list price; untouched parts must not become zero/free.
				next = *listed
			}
			edit(&next)
			if equal(next, core.Preserved[modelPrice]{}) {
				d.Price = nil
			} else {
				d.Price = &next
			}
		})
	}
	listPrice := i18n.T("ai.modelListPrice")
	ui.Column(c).FillWidth().Gap(16).Children(func() {
		kit.Heading(c, i18n.T("ai.modelPrice"))
		ui.Row(c).FillWidth().GapX(16).GapY(12).Wrap().Children(func() {
			for _, key := range priceParts5 {
				key := key
				label := i18n.T("ai.modelPrice" + key)
				ui.Field(c.Key(key), label, func() {
					if v, changed := numberText(c, *price.field(key), 0, math.MaxFloat64, false, listPrice, label); changed {
						setPrice(func(d *core.Preserved[modelPrice]) { *d.Value.field(key) = v })
					}
				}).Basis(210).Grow(1).MinWidth(0)
			}
		})
		// Every upstream tier remains independently editable, including 1-hour cache
		// writes. The provider validates ascending boundaries on Save.
		remove := -1
		for index, doc := range price.Tiers {
			tier := doc.Value
			ui.Column(c.Key(index)).FillWidth().Gap(12).Padding(16, 0, 0, 0).BorderWidth(1, 0, 0, 0).BorderColor(kit.Colors(c).Hairline).Children(func() {
				ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
					kit.Label(c, i18n.T("aiComplete.priceTier", "number", index+1)).FontWeight(500).Grow(1)
					if kit.IconButton(c, icons.X, i18n.T("aiComplete.removePriceTier", "number", index+1)).Clicked() {
						remove = index
					}
				})
				ui.Field(c, i18n.T("ai.modelPriceAbove"), func() {
					if v, changed := numberText(c, ptr(tier.Above), 1, math.MaxInt, true, "", i18n.T("ai.modelPriceAbove")+" "+kit.Itoa(index+1)); changed {
						setPrice(func(d *core.Preserved[modelPrice]) { d.Value.Tiers[index].Value.Above = value(v) })
					}
				})
				ui.Row(c).FillWidth().GapX(16).GapY(12).Wrap().Children(func() {
					for _, key := range priceParts5 {
						label := i18n.T("aiComplete.priceTier", "number", index+1) + " · " + i18n.T("ai.modelPrice"+key)
						ui.Field(c.Key(key), i18n.T("ai.modelPrice"+key), func() {
							if v, changed := numberText(c, *tier.field(key), 0, math.MaxFloat64, false, listPrice, label); changed {
								setPrice(func(d *core.Preserved[modelPrice]) { *d.Value.Tiers[index].Value.field(key) = v })
							}
						}).Basis(210).Grow(1).MinWidth(0)
					}
				})
			})
		}
		if remove >= 0 {
			setPrice(func(d *core.Preserved[modelPrice]) {
				d.Value.Tiers = slices.Delete(slices.Clone(d.Value.Tiers), remove, remove+1)
			})
		}
		ui.Row(c).Children(func() {
			if kit.GhostButton(c, i18n.T("aiComplete.addPriceTier"), icons.Plus, false).Clicked() {
				above := 200000.0
				if len(price.Tiers) > 0 {
					above = price.Tiers[len(price.Tiers)-1].Value.Above + 200000
				}
				setPrice(func(d *core.Preserved[modelPrice]) {
					d.Value.Tiers = append(slices.Clone(d.Value.Tiers), core.Preserved[priceTier]{Value: priceTier{Above: above}})
				})
			}
		})

		kit.Help(c, i18n.T("ai.modelPriceHint"))
	})
}

func rest[T any](list []T) []T {
	if len(list) <= 1 {
		return nil
	}
	return list[1:]
}

// limitsOf are the model's window, output, compaction, reasoning level,
// service tier and capabilities. A zero or empty field keeps the model's
// default.
func (m *modelsPanel) limitsOf(c *ui.Context, meta modelMetadata, change func(func(*modelMetadata)), defaultLabel string, defaultContext, defaultOutput int) {
	window := value(meta.ContextWindow)
	contextLabel, outputLabel := defaultLabel, defaultLabel
	if defaultContext > 0 {
		contextLabel += " · " + tokenLabel(float64(defaultContext))
	}
	if defaultOutput > 0 {
		outputLabel += " · " + tokenLabel(float64(defaultOutput))
	}
	ui.Column(c).FillWidth().Gap(16).Children(func() {
		ui.Row(c).FillWidth().GapX(16).GapY(16).Wrap().AlignItems(ui.Start).Children(func() {
			ui.Column(c).Basis(260).Grow(1).MinWidth(0).Gap(8).Children(func() {
				ui.Field(c, i18n.T("ai.contextWindow"), func() {
					if v, changed := numberText(c, nonZero(meta.ContextWindow), 0, math.MaxInt, true, contextLabel, i18n.T("ai.contextWindow")); changed {
						change(func(d *modelMetadata) { d.ContextWindow = ptr(value(v)) })
					}
				})
				options := append([]float64{}, contextPresets...)
				if most := value(meta.MaxContextWindow); most > value(meta.ListedContextWindow) && most > 0 {
					options = append(options, most)
				}
				presetButtons(c, i18n.T("ai.contextPresets"), window, options, func(v float64) string {
					if v == value(meta.MaxContextWindow) && !slices.Contains(contextPresets, v) {
						return i18n.T("ai.modelMaximumContext", "size", tokenLabel(v))
					}
					return tokenLabel(v)
				}, func(v float64) { change(func(d *modelMetadata) { d.ContextWindow = ptr(v) }) })
			})
			ui.Column(c).Basis(260).Grow(1).MinWidth(0).Gap(8).Children(func() {
				ui.Field(c, i18n.T("ai.maxTokens"), func() {
					if v, changed := numberText(c, nonZero(meta.MaxTokens), 0, math.MaxInt, true, outputLabel, i18n.T("ai.maxTokens")); changed {
						change(func(d *modelMetadata) { d.MaxTokens = ptr(value(v)) })
					}
				})
				presetButtons(c, i18n.T("ai.outputPresets"), value(meta.MaxTokens), outputPresets, tokenLabel,
					func(v float64) { change(func(d *modelMetadata) { d.MaxTokens = ptr(v) }) })
			})
		})
		m.compaction(c, meta, change, defaultLabel, defaultContext)
	})
}

func nonZero(v *float64) *float64 {
	if v == nil || *v == 0 {
		return nil
	}
	return v
}

// Compaction uses Magpie's token threshold. A former percentage remains
// readable until edited; the next explicit edit stores the upstream unit.
func (m *modelsPanel) compaction(c *ui.Context, meta modelMetadata, change func(func(*modelMetadata)), defaultLabel string, defaultContext int) {
	threshold := nonZero(meta.CompactionThreshold)
	if percent := value(meta.CompactionPercent); percent > 0 {
		window := value(meta.ContextWindow)
		if window == 0 {
			window = float64(defaultContext)
		}
		if window > 0 {
			threshold = ptr(math.Floor(window * percent / 100))
		}
	}
	set := func(v *float64) {
		change(func(d *modelMetadata) { d.CompactionThreshold = v; d.CompactionPercent = nil })
	}
	ui.Column(c).FillWidth().Gap(8).Children(func() {
		ui.Field(c, i18n.T("ai.compactionThreshold"), func() {
			if v, changed := numberText(c, threshold, 0, math.MaxInt, true, defaultLabel, i18n.T("ai.compactionThreshold")); changed {
				set(v)
			}
		})
		presetButtons(c, i18n.T("ai.compactionPresets"), value(threshold), compactionPresets, tokenLabel, func(v float64) { set(ptr(v)) })
		kit.Help(c, i18n.T("ai.compactionDefaultHint"))
	})
}

// grouped writes a whole number with thousands separators, as
// toLocaleString did.
func grouped(v float64) string {
	s := strconv.FormatFloat(math.Trunc(v), 'f', 0, 64)
	var out []byte
	for i := range s {
		if i > 0 && (len(s)-i)%3 == 0 && s[i-1] != '-' {
			out = append(out, ',')
		}
		out = append(out, s[i])
	}
	return string(out)
}

// reasoning is the model's reasoning level, service tier and
// capabilities. A known vendor list decides the levels offered; a saved
// choice stays offered so a refresh never hides it.
func (m *modelsPanel) reasoning(c *ui.Context, meta modelMetadata, change func(func(*modelMetadata)), defaultLabel string) {
	efforts, levels := value(meta.Efforts), value(meta.Levels)
	offered := effortRank
	switch {
	case len(efforts) > 0:
		offered = efforts
	case len(levels) > 0:
		offered = levels
	}
	offered = append([]string{}, offered...)
	if e := value(meta.ReasoningEffort); e != "" {
		offered = append(offered, e)
	}
	offered = unique(offered)
	tiers := []string{"flex", "priority", "ultrafast"}
	if t := value(meta.Tiers); len(t) > 0 {
		tiers = t
	}
	offeredTiers := append([]string{"auto", "default"}, tiers...)
	if t := value(meta.Tier); t != "" {
		offeredTiers = append(offeredTiers, t)
	}
	offeredTiers = unique(offeredTiers)
	options := func(values []string) []kit.SelectOption {
		out := []kit.SelectOption{{Value: "", Label: defaultLabel}}
		for _, v := range values {
			out = append(out, kit.SelectOption{Value: v, Label: v})
		}
		return out
	}
	ui.Column(c).FillWidth().Gap(8).Children(func() {
		effort := value(meta.ReasoningEffort)
		ui.Field(c, i18n.T("ai.reasoningEffort"), func() {
			changed, trigger := kit.Select(c, &effort, options(offered))
			trigger.Label(i18n.T("ai.reasoningEffort"))
			if changed {
				change(func(d *modelMetadata) { d.ReasoningEffort = ptr(effort) })
			}
		})
		presetButtons(c, i18n.T("ai.reasoningPresets"), value(meta.ReasoningEffort), append([]string{""}, offered...), func(v string) string {
			if v == "" {
				return defaultLabel
			}
			return v
		}, func(v string) { change(func(d *modelMetadata) { d.ReasoningEffort = ptr(v) }) })
	})
	ui.Row(c).FillWidth().GapX(16).GapY(12).Wrap().AlignItems(ui.Start).Children(func() {
		tier := value(meta.Tier)
		ui.Field(c, i18n.T("ai.modelTier"), func() {
			changed, trigger := kit.Select(c, &tier, options(offeredTiers))
			trigger.Label(i18n.T("ai.modelTier"))
			if changed {
				change(func(d *modelMetadata) { d.Tier = ptr(tier) })
			}
		}).Basis(210).Grow(1).MinWidth(0)
	})
	for _, key := range []string{"reasoning", "vision"} {
		key := key
		field := map[string]*bool{"reasoning": meta.Reasoning, "vision": meta.Vision, "tools": meta.Tools}[key]
		current := ""
		if field != nil {
			current = fmt.Sprint(*field)
		}
		ui.Field(c.Key(key), i18n.T("ai.capability"+key), func() {
			if kit.Segmented(c, &current, []kit.SelectOption{{Value: "", Label: defaultLabel},
				{Value: "true", Label: i18n.T("ai.capabilityEnabled")}, {Value: "false", Label: i18n.T("ai.capabilityDisabled")}},
				i18n.T("ai.capability"+key)) {
				var v *bool
				if current != "" {
					v = ptr(current == "true")
				}
				change(func(d *modelMetadata) {
					switch key {
					case "reasoning":
						d.Reasoning = v
					case "vision":
						d.Vision = v
					default:
						d.Tools = v
					}
				})
			}
		})
	}
}
