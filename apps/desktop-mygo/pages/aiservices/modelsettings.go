package aiservices

import (
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

// Magpie a8ca790 app.js contextPicks/outputField, in decimal K/M. The
// percentage shortcuts are WinOne's.
var (
	contextPresets    = []float64{128000, 200000, 256000, 1000000}
	outputPresets     = []float64{8000, 16000, 32000, 64000, 128000}
	compactionPresets = []float64{50, 60, 70, 80, 90, 95}
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
	kit.Dialog(c, &open, kit.DialogOptions{Title: i18n.T("ai.modelSettings"), Message: model, Cancel: i18n.T("common.close"), Width: 640,
		Body: func() {
			ui.Column(c).FillWidth().Gap(16).Disabled(disabled).Children(func() {
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
					ui.TextInput(c, &field.text).FillWidth().Placeholder(model).Label(i18n.T("ai.modelDisplayName"))
				})
				ui.Row(c).FillWidth().GapX(16).GapY(12).Wrap().AlignItems(ui.Start).Children(func() {
					for _, key := range []string{"upstreamName", "same"} {
						key := key
						field := &meta.UpstreamName
						if key == "same" {
							field = &meta.Same
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
							ui.Field(c, i18n.T("ai.model"+key), func() {
								ui.TextInput(c, &synced.text).FillWidth().Placeholder(model).Label(i18n.T("ai.model" + key))
							})
							kit.Help(c, i18n.T("ai.model"+key+"Hint"))
						})
					}
				})
				ui.Field(c, i18n.T("ai.modelAPI"), func() {
					api := value(meta.API)
					changed, trigger := kit.Select(c, &api, []kit.SelectOption{{Value: "", Label: defaultLabel},
						{Value: "chat", Label: "OpenAI Chat Completions"}, {Value: "responses", Label: "OpenAI Responses"},
						{Value: "anthropic", Label: "Anthropic Messages"}})
					trigger.Label(i18n.T("ai.modelAPI"))
					if changed {
						change(func(d *modelMetadata) { d.API = ptr(api) })
					}
				})
				m.efforts(c, meta, change)
				m.pricing(c, meta, change)
				ui.Row(c).Children(func() {
					if kit.Button(c, i18n.T("ai.modelRestoreDefaults"), icons.RotateCcw, false).Clicked() {
						p.change(id, func(t *provider) {
							d := t.ModelMetadata[model]
							d.Value.Same, d.Value.UpstreamName, d.Value.API, d.Value.Efforts, d.Value.Price, d.Value.Vision = nil, nil, nil, nil, nil, nil
							if t.ModelMetadata == nil {
								t.ModelMetadata = map[string]metadata{}
							}
							t.ModelMetadata[model] = d
							names := map[string]string{}
							for k, v := range t.ModelNames {
								if k != model {
									names[k] = v
								}
							}
							t.ModelNames = names
						})
					}
				})
				m.limitsOf(c, meta, change, defaultLabel)
				kit.Help(c, i18n.T("ai.modelMetadataHint"))
			})
		}})
	if !open {
		m.editing = ""
		m.fields = fields{}
	}
}

// presetButtons are one-click values beside a field; the matching one
// shows pressed.
func presetButtons[T comparable](c *ui.Context, label string, current T, options []T, text func(T) string, choose func(T)) {
	ui.Row(c).FillWidth().Gap(6).Wrap().Role(ui.RoleGroup).Label(label).Children(func() {
		for _, option := range options {
			option := option
			b := kit.Button(c.Key(text(option)), text(option), nil, false).Checked(current == option)
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
func (m *modelsPanel) pricing(c *ui.Context, meta modelMetadata, change func(func(*modelMetadata))) {
	var price modelPrice
	if meta.Price != nil {
		price = meta.Price.Value
	}
	setPrice := func(edit func(*core.Preserved[modelPrice])) {
		change(func(d *modelMetadata) {
			next := core.Preserved[modelPrice]{}
			if d.Price != nil {
				next = *d.Price
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
	ui.Column(c).FillWidth().Gap(12).Padding(16, 0, 0, 0).BorderWidth(1, 0, 0, 0).BorderColor(kit.Colors(c).Hairline).Children(func() {
		kit.Label(c, i18n.T("ai.modelPrice")).FontWeight(600)
		ui.Row(c).FillWidth().GapX(16).GapY(12).Wrap().Children(func() {
			for _, key := range priceParts5 {
				key := key
				label := i18n.T("ai.modelPrice" + key)
				ui.Field(c.Key(key), label, func() {
					if v, changed := numberText(c, *price.field(key), 0, math.MaxFloat64, false, listPrice, label); changed {
						setPrice(func(d *core.Preserved[modelPrice]) { *d.Value.field(key) = v })
					}
				}).Basis(240).Grow(1)
			}
		})
		var tier priceTier
		if len(price.Tiers) > 0 {
			tier = price.Tiers[0].Value
		}
		if m.tierFor != m.editing {
			// Open at first when the model has a long-context price.
			m.tierFor, m.tierOpen = m.editing, len(price.Tiers) > 0
		}
		ui.Collapsible(c, i18n.T("ai.modelLongContextPrice"), &m.tierOpen, func() {
			ui.Row(c).FillWidth().GapX(16).GapY(12).Wrap().Padding(8, 0).Children(func() {
				var above *float64
				if tier.Above != 0 {
					above = &tier.Above
				}
				ui.Field(c, i18n.T("ai.modelPriceAbove"), func() {
					if v, changed := numberText(c, above, 1, math.MaxFloat64, true, "", i18n.T("ai.modelPriceAbove")); changed {
						setPrice(func(d *core.Preserved[modelPrice]) {
							if v == nil {
								d.Value.Tiers = nil
								return
							}
							first := core.Preserved[priceTier]{}
							if len(d.Value.Tiers) > 0 {
								first = d.Value.Tiers[0]
							}
							first.Value.Above = *v
							d.Value.Tiers = append([]core.Preserved[priceTier]{first}, rest(d.Value.Tiers)...)
						})
					}
				}).Basis(240).Grow(1)
				for _, key := range priceParts5[:4] {
					key := key
					label := i18n.T("ai.modelLongContextPrice") + " · " + i18n.T("ai.modelPrice"+key)
					ui.Field(c.Key("tier-"+key), label, func() {
						if v, changed := numberText(c, *tier.field(key), 0, math.MaxFloat64, false, listPrice, label); changed {
							setPrice(func(d *core.Preserved[modelPrice]) {
								first := core.Preserved[priceTier]{}
								if len(d.Value.Tiers) > 0 {
									first = d.Value.Tiers[0]
								}
								*first.Value.field(key) = v
								d.Value.Tiers = append([]core.Preserved[priceTier]{first}, rest(d.Value.Tiers)...)
							})
						}
					}).Basis(240).Grow(1)
				}
			})
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
func (m *modelsPanel) limitsOf(c *ui.Context, meta modelMetadata, change func(func(*modelMetadata)), defaultLabel string) {
	window := value(meta.ContextWindow)
	ui.Column(c).FillWidth().Gap(16).Children(func() {
		ui.Row(c).FillWidth().GapX(16).GapY(16).Wrap().AlignItems(ui.Start).Children(func() {
			ui.Column(c).Basis(260).Grow(1).MinWidth(0).Gap(6).Children(func() {
				ui.Field(c, i18n.T("ai.contextWindow"), func() {
					if v, changed := numberText(c, nonZero(meta.ContextWindow), 0, 100000000, true, defaultLabel, i18n.T("ai.contextWindow")); changed {
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
			ui.Column(c).Basis(260).Grow(1).MinWidth(0).Gap(6).Children(func() {
				ui.Field(c, i18n.T("ai.maxTokens"), func() {
					if v, changed := numberText(c, nonZero(meta.MaxTokens), 0, 100000000, true, defaultLabel, i18n.T("ai.maxTokens")); changed {
						change(func(d *modelMetadata) { d.MaxTokens = ptr(value(v)) })
					}
				})
				presetButtons(c, i18n.T("ai.outputPresets"), value(meta.MaxTokens), outputPresets, tokenLabel,
					func(v float64) { change(func(d *modelMetadata) { d.MaxTokens = ptr(v) }) })
			})
		})
		m.compaction(c, meta, change, defaultLabel)
		m.reasoning(c, meta, change, defaultLabel)
	})
}

func nonZero(v *float64) *float64 {
	if v == nil || *v == 0 {
		return nil
	}
	return v
}

// compaction is the share of the window at which Chat compacts a long
// conversation. Older metadata gave a token count, which keeps working
// until this field is edited; reading it migrates nothing.
func (m *modelsPanel) compaction(c *ui.Context, meta modelMetadata, change func(func(*modelMetadata)), defaultLabel string) {
	configured := value(meta.CompactionPercent)
	legacy := 0.0
	if configured <= 0 {
		legacy = value(meta.CompactionThreshold)
	}
	window := value(meta.ContextWindow)
	var percent *float64
	switch {
	case configured > 0:
		percent = &configured
	case legacy > 0 && window > 0 && legacy/window*100 <= 100:
		v, _ := strconv.ParseFloat(strconv.FormatFloat(legacy/window*100, 'f', 4, 64), 64)
		percent = &v
	}
	set := func(v float64) {
		change(func(d *modelMetadata) { d.CompactionPercent, d.CompactionThreshold = ptr(v), ptr(0.0) })
	}
	ui.Column(c).FillWidth().Gap(6).Children(func() {
		ui.Field(c, i18n.T("ai.compactionPercent"), func() {
			if v, changed := numberText(c, percent, 0, 100, false, defaultLabel, i18n.T("ai.compactionPercent")); changed {
				set(value(v))
			}
		})
		current := -1.0
		if legacy == 0 && percent != nil {
			current = *percent
		}
		presetButtons(c, i18n.T("ai.compactionPresets"), current, compactionPresets, func(v float64) string {
			return strconv.FormatFloat(v, 'f', -1, 64) + "%"
		}, set)
		hint := i18n.T("ai.compactionPercentHint")
		switch {
		case legacy > 0:
			hint = i18n.T("ai.legacyCompactionHint", "tokens", grouped(legacy))
		case window > 0 && percent != nil && *percent > 0:
			hint = i18n.T("ai.compactionTokenPreview", "tokens", grouped(math.Floor(window**percent/100)), "percent",
				strconv.FormatFloat(*percent, 'f', -1, 64))
		}
		kit.Help(c, hint)
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
	ui.Column(c).FillWidth().Gap(6).Children(func() {
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
		}).Basis(240).Grow(1)
	})
	for _, key := range []string{"reasoning", "vision", "tools"} {
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
