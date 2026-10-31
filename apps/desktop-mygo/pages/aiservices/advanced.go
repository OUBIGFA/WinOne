package aiservices

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/web"
	"winone/desktopmygo/widgets"
)

// modelChoice is a model a Magpie helper can use.
type modelChoice struct {
	ID           string `json:"id"`
	Name         string `json:"name,omitempty"`
	Provider     string `json:"provider,omitempty"`
	ProviderName string `json:"providerName,omitempty"`
	Icon         string `json:"icon,omitempty"`
}

// searchChoice is a web search: a provider's own search, with its models.
type searchChoice struct {
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	Small     string        `json:"small,omitempty"`
	SmallName string        `json:"smallName,omitempty"`
	Service   bool          `json:"service,omitempty"`
	Models    []modelChoice `json:"models,omitempty"`
}

type searchVendor struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	NeedURL     bool   `json:"needURL,omitempty"`
	KeyOptional bool   `json:"keyOptional,omitempty"`
	KeysURL     string `json:"keysURL,omitempty"`
}

// schemaProperty is one field of a built-in plugin's settings.
type schemaProperty struct {
	Type        string  `json:"type"`
	Title       string  `json:"title"`
	Description string  `json:"description,omitempty"`
	Enum        []any   `json:"enum,omitempty"`
	Minimum     *float64 `json:"minimum,omitempty"`
	Maximum     *float64 `json:"maximum,omitempty"`
	Default     any     `json:"default"`
}

type builtinPlugin struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Sub       string         `json:"sub"`
	Essential bool           `json:"essential"`
	On        bool           `json:"on"`
	Chosen    bool           `json:"chosen"`
	State     string         `json:"state"`
	InUse     int            `json:"inUse"`
	Why       string         `json:"why,omitempty"`
	Requires  []string       `json:"requires,omitempty"`
	Config    map[string]any `json:"config,omitempty"`
	Schema    *struct {
		Properties map[string]schemaProperty `json:"properties"`
		Order      []string                  `json:"x-order"`
	} `json:"schema,omitempty"`
}

type bundledPlugin struct {
	Directory string            `json:"directory"`
	Package   string            `json:"package"`
	Name      string            `json:"name"`
	Version   string            `json:"version"`
	Kind      string            `json:"kind,omitempty"`
	Summary   map[string]string `json:"summary,omitempty"`
	Providers []string          `json:"providers"`
}

// magpieCatalog is what Magpie offers for the advanced settings: helper
// models, search services, accounts, and the plugins.
type magpieCatalog struct {
	SearchVendors   []searchVendor  `json:"searchVendors,omitempty"`
	VisionModels    []modelChoice   `json:"visionModels,omitempty"`
	ImageGenModels  []modelChoice   `json:"imageGenModels,omitempty"`
	SearchChoices   []searchChoice  `json:"searchChoices,omitempty"`
	TitleModels     []modelChoice   `json:"titleModels,omitempty"`
	VisionAuto      string          `json:"visionAuto,omitempty"`
	VisionMissing   string          `json:"visionMissing,omitempty"`
	ImageGenAuto    string          `json:"imageGenAuto,omitempty"`
	ImageGenMissing string          `json:"imageGenMissing,omitempty"`
	SearchAuto      string          `json:"searchAuto,omitempty"`
	SearchUnused    string          `json:"searchUnused,omitempty"`
	SearchRelays    []string        `json:"searchRelays,omitempty"`
	SearchLeftOut   []string        `json:"searchLeftOut,omitempty"`
	PluginCheckins  []named         `json:"pluginCheckins,omitempty"`
	CodexUsers      []string        `json:"codexUsers,omitempty"`
	Quotas          *[]struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		User string `json:"user,omitempty"`
	} `json:"quotas,omitempty"`
	Gateway         string          `json:"gateway,omitempty"`
	NoAccountsEnv   bool            `json:"noAccountsEnv,omitempty"`
	BuiltinPlugins  []builtinPlugin `json:"builtinPlugins,omitempty"`
	BundledPlugins  []bundledPlugin `json:"bundledPlugins,omitempty"`
}

// advancedPanel is Magpie's advanced settings: quotas, helper models and
// search, privacy, built-in features and the plugins' settings panels.
// Its edits are part of the page's draft and save with it.
type advancedPanel struct {
	p        *page
	catalog  *magpieCatalog
	loading  bool
	err      string
	revision float64
	loadedAt bool
	read     int

	fields  fields
	times   map[string]*timeField
	proxy   proxyDraft
	proxyOf string // the saved proxy the draft was read from
	proxyErr string
	search  searchForm
	revealed map[string]string
	searchErr string
	rule    ruleForm
	model   *privacyModel
	records *web.Surface
	recordsFor string
	// focusPrivacy moves to the privacy form when the records ask.
	focusPrivacy bool
	options     map[string]*pluginOptions
	builtinTab  int
	opens       map[string]bool
	listFields  map[string]*listField
}

func newAdvancedPanel(p *page) *advancedPanel {
	return &advancedPanel{p: p, fields: fields{}, times: map[string]*timeField{}, revealed: map[string]string{},
		options: map[string]*pluginOptions{}, opens: map[string]bool{}}
}

func (a *advancedPanel) Close() {
	a.read++
	if a.model != nil {
		a.model.Close()
	}
	if a.records != nil {
		a.records.Close()
	}
	a.p.env.Activity.Report("ai-proxy", false, false, nil)
}

// load reads Magpie's catalog, again after a save.
func (a *advancedPanel) load() {
	a.read++
	owner := a.read
	a.loading, a.err = true, ""
	core.Go(a.p.env, func() (magpieCatalog, error) {
		var value magpieCatalog
		err := a.p.env.Host.CallCompletingInto("capture.ai.magpie.catalog", map[string]any{}, &value)
		return value, err
	}, func(value magpieCatalog, err error) {
		if owner != a.read {
			return
		}
		a.loading = false
		if err != nil {
			a.err = err.Error()
			return
		}
		a.catalog = &value
	})
}

// settings returns the draft's Magpie settings to edit.
func (a *advancedPanel) settings() *magpieSettings { return a.p.draft.Value.settings() }

// advancedView is the advanced tab: its own tabs over the settings.
func (p *page) advancedView(c *ui.Context) {
	if p.draft == nil {
		return
	}
	if p.advanced == nil {
		p.advanced = newAdvancedPanel(p)
	}
	a := p.advanced
	if p.saved != nil && (!a.loadedAt || a.revision != p.saved.Value.Revision) && !a.loading {
		a.loadedAt, a.revision = true, p.saved.Value.Revision
		a.load()
	}
	labels := []string{i18n.T("ai.quotaSettings"), i18n.T("ai.modelSettings"), i18n.T("ai.privacySettings"), i18n.T("aiPlugins.featureSettings")}
	settingsPanels := false
	if p.extensions.state != nil {
		for _, panel := range p.extensions.state.Panels {
			settingsPanels = settingsPanels || panel.Slot == "settings"
		}
	}
	if settingsPanels {
		labels = append(labels, i18n.T("aiPlugins.panels"))
	}
	if p.advancedTab >= len(labels) {
		p.advancedTab = advancedQuota
	}
	ui.Column(c).Disabled(p.navigationLocked()).Children(func() {
		kit.SubTabs(c, &p.advancedTab, labels, p.env.RequestChange)
	})
	if a.loading {
		kit.Status(c, i18n.T("capture.loading"))
	}
	if a.err != "" {
		kit.Notice(c, kit.Failure, "", a.err, func() {
			if kit.Button(c, i18n.T("common.retry"), icons.RefreshCw, false).Disabled(a.loading).Clicked() {
				a.load()
			}
		})
	}
	catalog := a.catalog
	if catalog == nil {
		catalog = &magpieCatalog{}
	}
	locked := p.locked()
	if p.advancedTab != advancedModels {
		a.reportProxy(false)
	}
	ui.Column(c).FillWidth().Gap(kit.GroupGap).Children(func() {
		switch p.advancedTab {
		case advancedQuota:
			a.quota(c, catalog, locked)
		case advancedModels:
			a.modelOptions(c, catalog, locked)
		case advancedPrivacy:
			a.privacy(c, catalog, locked)
		case advancedFeatures:
			a.features(c, catalog, locked)
		case advancedExtensions:
			p.extensions.Slot(c, "settings", nil, locked)
		}
	})
	if p.advancedTab != advancedPrivacy {
		a.leavePrivacy()
	}
}

// leavePrivacy closes the privacy model's lease and the records page when
// the privacy settings no longer show, as React unmounted them.
func (a *advancedPanel) leavePrivacy() {
	if a.model != nil {
		a.model.Close()
		a.model = nil
	}
	if a.records != nil {
		a.records.Close()
		a.records, a.recordsFor = nil, ""
	}
}

// choiceRow is a setting with a few choices shown at once.
func choiceRow(c *ui.Context, label, hint string, value *string, options []kit.SelectOption, disabled bool) bool {
	changed := false
	kit.Row(c, label, hint, func() {
		ui.Column(c).Disabled(disabled).Children(func() { changed = kit.Segmented(c, value, options, label) })
	})
	return changed
}

func options(pairs ...string) []kit.SelectOption {
	out := make([]kit.SelectOption, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, kit.SelectOption{Value: pairs[i], Label: pairs[i+1]})
	}
	return out
}

// numberRow is a setting that is a number, empty or zero meaning off or
// default as the setting says.
func numberRow(c *ui.Context, label string, current *float64, low, high float64, whole bool, placeholder string, disabled bool, set func(*float64)) {
	kit.Row(c, label, "", func() {
		ui.Column(c).Disabled(disabled).Children(func() {
			if v, changed := numberText(c, current, low, high, whole, placeholder, label); changed {
				set(v)
			}
		})
	})
}

func fmtNumber(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// quota is how quotas show, alerts, the tray's quotas, and account
// automation: warm-ups and daily check-ins.
func (a *advancedPanel) quota(c *ui.Context, catalog *magpieCatalog, locked bool) {
	s := a.settings()
	kit.Group(c, icons.Gauge, i18n.T("settingsDesign.quotaDisplay"), "", func() {
		left := strconv.FormatBool(s.QuotaLeft)
		if choiceRow(c, i18n.T("ai.quotaDisplay"), "", &left, options("false", i18n.T("ai.quotaUsed"), "true", i18n.T("ai.quotaLeft")), locked) {
			s.QuotaLeft = left == "true"
		}
		reads := s.QuotaReads
		if choiceRow(c, i18n.T("ai.quotaReads"), i18n.T("ai.quotaReadsHint"), &reads, options("", i18n.T("ai.automatic"), "asked", i18n.T("ai.quotaAsked")), locked) {
			s.QuotaReads = reads
		}
		currency := s.Currency
		if currency == "" {
			currency = "usd"
		}
		if choiceRow(c, i18n.T("ai.currency"), "", &currency, options("usd", "USD", "cny", "CNY"), locked) {
			s.Currency = currency
		}
		units := strconv.FormatBool(s.ChineseUnits)
		if choiceRow(c, i18n.T("ai.numberUnits"), "", &units, options("false", "K / M / B", "true", "万 / 亿"), locked) {
			s.ChineseUnits = units == "true"
		}
	})
	kit.Group(c, icons.BellRing, i18n.T("settingsDesign.quotaAlerts"), "", func() {
		usage := value(s.UsageAlert) != 0
		if kit.Toggle(c, &usage, i18n.T("ai.usageAlert"), i18n.T("ai.usageAlertHint")).Disabled(locked).Changed() {
			if usage {
				v := value(s.UsageAlert)
				if v == 0 {
					v = 80
				}
				s.UsageAlert = ptr(v)
			} else {
				s.UsageAlert = ptr(0.0)
			}
		}
		if usage {
			numberRow(c, i18n.T("ai.usageAlertPercent"), s.UsageAlert, 1, 100, true, "", locked, func(v *float64) {
				if v != nil {
					s.UsageAlert = v
				}
			})
		}
		balance := value(s.BalanceAlert) != 0
		if kit.Toggle(c, &balance, i18n.T("ai.balanceAlert"), i18n.T("ai.balanceAlertHint")).Disabled(locked).Changed() {
			if balance {
				v := value(s.BalanceAlert)
				if v == 0 {
					v = 5
				}
				s.BalanceAlert = ptr(v)
			} else {
				s.BalanceAlert = ptr(0.0)
			}
		}
		if balance {
			numberRow(c, i18n.T("ai.balanceAlertAmount"), s.BalanceAlert, 0, 1e15, false, "", locked, func(v *float64) {
				if v != nil {
					s.BalanceAlert = v
				}
			})
		}
		reminders := []float64{12, 24, 48}
		if r := value(s.ResetReminder); r != 0 && !slices.Contains(reminders, r) {
			reminders = append(reminders, r)
		}
		pairs := []string{"0", i18n.T("ai.off")}
		for _, r := range reminders {
			pairs = append(pairs, fmtNumber(r), i18n.T("ai.hoursBefore", "count", r))
		}
		reminder := fmtNumber(value(s.ResetReminder))
		if choiceRow(c, i18n.T("ai.resetReminder"), "", &reminder, options(pairs...), locked) {
			v, _ := strconv.ParseFloat(reminder, 64)
			s.ResetReminder = ptr(v)
		}
	})
	kit.Group(c, icons.PanelTop, i18n.T("ai.trayQuotas"), i18n.T("ai.trayQuotasHint"), func() {
		type quota struct{ id, name string }
		var quotas []quota
		if catalog.Quotas != nil {
			for _, q := range *catalog.Quotas {
				quotas = append(quotas, quota{q.ID, q.Name})
			}
		} else {
			for _, d := range a.p.draft.Value.Providers {
				if d.Value.AuthMode == "account" {
					quotas = append(quotas, quota{d.Value.ID, d.Value.Name})
				}
			}
		}
		for _, id := range s.TrayUsages {
			if !slices.ContainsFunc(quotas, func(q quota) bool { return q.id == id }) {
				quotas = append(quotas, quota{id, id})
			}
		}
		if len(quotas) == 0 {
			kit.Stack(c, "", i18n.T("ai.quotaNoAccounts"), func() {})
		}
		for _, q := range quotas {
			q := q
			on := slices.Contains(s.TrayUsages, q.id)
			ui.Row(c.Key(q.id)).FillWidth().MinHeight(kit.RowMinHeight).Padding(10, 16).AlignItems(ui.Center).Children(func() {
				if ui.Checkbox(c, &on, q.name).Disabled(locked).Changed() {
					if on {
						s.TrayUsages = append(append([]string{}, s.TrayUsages...), q.id)
					} else {
						s.TrayUsages = slices.DeleteFunc(append([]string{}, s.TrayUsages...), func(id string) bool { return id == q.id })
					}
				}
			})
		}
		every := value(s.TrayUsageEvery)
		if every == 0 {
			every = 3
		}
		var pairs []string
		for _, m := range []float64{1, 3, 5, 10, 30} {
			pairs = append(pairs, fmtNumber(m), i18n.T("ai.minutes", "count", m))
		}
		current := fmtNumber(every)
		if choiceRow(c, i18n.T("ai.trayQuotaEvery"), "", &current, options(pairs...), locked) {
			v, _ := strconv.ParseFloat(current, 64)
			s.TrayUsageEvery = ptr(v)
		}
	})
	automation := s.CodexWarmup != "" || s.ClaudeWarmup != "" || s.CodexWarmAt != "" || s.ClaudeWarmAt != "" ||
		len(value(s.CodexWarmAts)) > 0 || len(value(s.ClaudeWarmAts)) > 0 || len(s.CodexWarmAtOf) > 0 || len(s.CodexAutoReset) > 0 ||
		len(s.CodexNoCredits) > 0 || s.WorkbuddyCheckin || s.TraeCheckin || s.MinimaxCheckin || s.QoderCheckin
	for _, on := range s.PluginCheckins {
		automation = automation || on
	}
	if _, ok := a.opens["automation"]; !ok {
		a.opens["automation"] = automation
	}
	open := a.opens["automation"]
	ui.Collapsible(c, i18n.T("settingsDesign.accountAutomation"), &open, func() {
		ui.Column(c).FillWidth().Gap(kit.GroupGap).Padding(12, 0).Children(func() {
			a.warmup(c, catalog, "codex", locked)
			a.warmup(c, catalog, "claude", locked)
			kit.Group(c, icons.CalendarCheck, i18n.T("ai.dailyCheckins"), i18n.T("ai.dailyCheckinsHint"), func() {
				for _, item := range []struct {
					name  string
					value *bool
				}{{"WorkBuddy", &s.WorkbuddyCheckin}, {"Trae CN", &s.TraeCheckin}, {"MiniMax Code", &s.MinimaxCheckin}, {"Qoder", &s.QoderCheckin}} {
					kit.Toggle(c.Key(item.name), item.value, item.name, "").Disabled(locked)
				}
				plugins := append([]named{}, catalog.PluginCheckins...)
				for id := range s.PluginCheckins {
					if !slices.ContainsFunc(plugins, func(n named) bool { return n.ID == id }) {
						plugins = append(plugins, named{ID: id, Name: id})
					}
				}
				for _, plugin := range plugins {
					plugin := plugin
					on := s.PluginCheckins[plugin.ID]
					if kit.Toggle(c.Key("plugin:"+plugin.ID), &on, plugin.Name, "").Disabled(locked).Changed() {
						next := map[string]bool{}
						for k, v := range s.PluginCheckins {
							next[k] = v
						}
						next[plugin.ID] = on
						s.PluginCheckins = next
					}
				}
			})
		})
	}).FontSize(kit.HelpSize(c))
	a.opens["automation"] = open
}

// timeField is a time of day being edited, kept between frames.
type timeField struct {
	value time.Time
	shown string
}

// timeInput edits a "15:04" time, returning the new one after an edit.
func (a *advancedPanel) timeInput(c *ui.Context, key, current, label string, disabled bool) (string, bool) {
	f := a.times[key]
	if f == nil {
		f = &timeField{}
		a.times[key] = f
	}
	if format := f.value.Format("15:04"); f.shown != "" && format != f.shown {
		f.shown = format
		ui.TimeInput(c, &f.value).Label(label).Disabled(disabled)
		return format, true
	}
	if current != f.shown {
		parsed, err := time.Parse("15:04", current)
		if err != nil {
			parsed, _ = time.Parse("15:04", "06:00")
		}
		f.value, f.shown = parsed, parsed.Format("15:04")
	}
	ui.TimeInput(c, &f.value).Label(label).Disabled(disabled)
	return "", false
}

// warmup is an agent's warm-ups: after quota resets, and daily at chosen
// times; for Codex, each account's own time, reset and credits.
func (a *advancedPanel) warmup(c *ui.Context, catalog *magpieCatalog, agentID string, locked bool) {
	s := a.settings()
	name := "Codex"
	reset, times, legacy := &s.CodexWarmup, &s.CodexWarmAts, &s.CodexWarmAt
	if agentID == "claude" {
		name = "Claude Code"
		reset, times, legacy = &s.ClaudeWarmup, &s.ClaudeWarmAts, &s.ClaudeWarmAt
	}
	ats := value(*times)
	if *times == nil && *legacy != "" {
		ats = []string{*legacy}
	}
	change := func(values []string) {
		copied := append([]string{}, values...)
		*times = &copied
		*legacy = ""
		if len(values) > 0 {
			*legacy = values[0]
		}
	}
	kit.Group(c, nil, i18n.T("ai.warmupTitle", "name", name), "", func() {
		current := *reset
		if choiceRow(c, i18n.T("ai.warmupReset"), "", &current, options("", i18n.T("ai.off"), "week", i18n.T("ai.warmupWeek"), "all", i18n.T("ai.warmupAll")), locked) {
			*reset = current
		}
		daily := len(ats) > 0
		if kit.Toggle(c, &daily, i18n.T("ai.warmupDaily"), i18n.T("ai.warmupHint")).Disabled(locked).Changed() {
			if daily {
				change([]string{"06:00"})
			} else {
				change([]string{})
			}
		}
		if len(ats) > 0 {
			kit.Stack(c, "", "", func() {
				ui.Row(c).FillWidth().Gap(12).Wrap().AlignItems(ui.End).Children(func() {
					for i, at := range ats {
						i, at := i, at
						label := i18n.T("ai.warmupTime", "index", i+1)
						ui.Row(c.Key(i)).Gap(4).AlignItems(ui.End).Children(func() {
							ui.Field(c, label, func() {
								if next, edited := a.timeInput(c, agentID+":"+kit.Itoa(i), at, label, locked); edited {
									values := append([]string{}, ats...)
									values[i] = next
									change(values)
								}
							})
							if kit.IconButton(c, icons.Trash2, i18n.T("ai.removeTime", "time", at)).Disabled(locked).Clicked() {
								change(slices.Delete(append([]string{}, ats...), i, i+1))
							}
						})
					}
					if kit.Button(c, i18n.T("ai.addTime"), icons.Plus, false).Disabled(locked).Clicked() {
						last := ats[len(ats)-1]
						t, err := time.Parse("15:04", last)
						if err != nil {
							t, _ = time.Parse("15:04", "06:00")
						}
						next := (t.Hour()*60 + t.Minute() + 305) % 1440
						change(append(append([]string{}, ats...), fmt.Sprintf("%02d:%02d", next/60, next%60)))
					}
				})
			})
		}
		if agentID != "codex" {
			return
		}
		users := append([]string{}, catalog.CodexUsers...)
		for user := range s.CodexWarmAtOf {
			users = append(users, user)
		}
		users = unique(append(append(users, s.CodexAutoReset...), s.CodexNoCredits...))
		for _, user := range users {
			user := user
			key := strings.ToLower(user)
			at, own := s.CodexWarmAtOf[key]
			mode := "same"
			switch {
			case own && at == "off":
				mode = "off"
			case own:
				mode = "own"
			}
			kit.Stack(c.Key("user:"+user), "", "", func() {
				ui.Field(c, user, func() {
					ui.Column(c).Disabled(locked).Children(func() {
						if kit.Segmented(c, &mode, options("same", i18n.T("ai.warmupSame"), "own", i18n.T("ai.warmupOwn"), "off", i18n.T("ai.off")), user) {
							next := map[string]string{}
							for k, v := range s.CodexWarmAtOf {
								next[k] = v
							}
							switch mode {
							case "same":
								delete(next, key)
							case "off":
								next[key] = "off"
							default:
								first := "06:00"
								if len(ats) > 0 {
									first = ats[0]
								}
								next[key] = first
							}
							s.CodexWarmAtOf = next
						}
					})
				})
				if mode == "own" {
					label := i18n.T("ai.accountWarmupTime", "user", user)
					ui.Field(c, label, func() {
						if next, edited := a.timeInput(c, "user:"+key, at, label, locked); edited {
							copied := map[string]string{}
							for k, v := range s.CodexWarmAtOf {
								copied[k] = v
							}
							copied[key] = next
							s.CodexWarmAtOf = copied
						}
					})
				}
				autoReset := slices.Contains(s.CodexAutoReset, key)
				if ui.Checkbox(c, &autoReset, i18n.T("ai.codexAutoReset")).Disabled(locked).Changed() {
					if autoReset {
						s.CodexAutoReset = append(append([]string{}, s.CodexAutoReset...), key)
					} else {
						s.CodexAutoReset = slices.DeleteFunc(append([]string{}, s.CodexAutoReset...), func(v string) bool { return v == key })
					}
				}
				credits := !slices.Contains(s.CodexNoCredits, key)
				if ui.Checkbox(c, &credits, i18n.T("ai.codexUseCredits")).Disabled(locked).Changed() {
					if credits {
						s.CodexNoCredits = slices.DeleteFunc(append([]string{}, s.CodexNoCredits...), func(v string) bool { return v == key })
					} else {
						s.CodexNoCredits = append(append([]string{}, s.CodexNoCredits...), key)
					}
				}
			})
		}
	})
}

// helperModel chooses a helper model: automatic, off, or a model.
func (a *advancedPanel) helperModel(c *ui.Context, label string, current string, models []modelChoice, automatic, missing string, locked bool, set func(string)) {
	p := a.p
	auto := i18n.T("ai.automatic")
	if automatic != "" {
		name := automatic
		for _, m := range models {
			if m.ID == automatic && m.Name != "" {
				name = m.Name
			}
		}
		auto += " · " + name
	}
	items := []widgets.PictureOption{{Value: "", Label: auto}, {Value: "off", Label: i18n.T("ai.off")}}
	for _, m := range models {
		m := m
		name := m.Name
		if name == "" {
			name = m.ID
		}
		owner := m.ProviderName
		if owner == "" {
			owner = m.Provider
		}
		items = append(items, widgets.PictureOption{Value: m.ID, Label: name + " · " + owner,
			Picture: func(c *ui.Context) { widgets.ModelIcon(c, p.env, m.ID, m.Name, 16) }})
	}
	if current != "" && !slices.ContainsFunc(items, func(o widgets.PictureOption) bool { return o.Value == current }) {
		items = append(items, widgets.PictureOption{Value: current, Label: current})
	}
	kit.Stack(c, label, "", func() {
		value := current
		if widgets.PictureSelect(c, &value, items, "", label, locked) {
			set(value)
		}
		if missing != "" {
			kit.Notice(c, kit.Caution, "", i18n.T("ai.helperMissing", "model", missing), nil)
		}
	})
}

// mcpAddress is one of Magpie's Streamable HTTP servers, to copy.
func mcpAddress(c *ui.Context, title, copyLabel, hint, address string) {
	services := c.Services()
	kit.Stack(c, title, hint, func() {
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			kit.Mono(c, address).Selectable().Grow(1).Shrink(1).MinWidth(0)
			if kit.IconButton(c, icons.Copy, copyLabel).Clicked() {
				services.WriteClipboard(address)
			}
		})
	})
}

// modelOptions are the proxy, images, web search, replies and the models
// clients see.
func (a *advancedPanel) modelOptions(c *ui.Context, catalog *magpieCatalog, locked bool) {
	s := a.settings()
	a.proxyView(c, locked)
	kit.Group(c, icons.Image, i18n.T("ai.images"), "", func() {
		a.helperModel(c, i18n.T("ai.visionModel"), s.Vision, catalog.VisionModels, catalog.VisionAuto, catalog.VisionMissing, locked,
			func(v string) { s.Vision = v })
		kit.Stack(c, "", i18n.T("ai.visionModelHint"), func() {})
		a.helperModel(c, i18n.T("ai.imageGenModel"), s.ImageGen, catalog.ImageGenModels, catalog.ImageGenAuto, catalog.ImageGenMissing, locked,
			func(v string) { s.ImageGen = v })
		kit.Stack(c, "", i18n.T("ai.imageGenHint"), func() {})
		timeout := value(s.ImageTimeout)
		if timeout == 0 {
			timeout = 5
		}
		var pairs []string
		presets := []float64{5, 10, 15, 30, 60}
		if !slices.Contains(presets, timeout) {
			presets = append(presets, timeout)
		}
		for _, m := range presets {
			pairs = append(pairs, fmtNumber(m), i18n.T("ai.minutes", "count", m))
		}
		current := fmtNumber(timeout)
		if choiceRow(c, i18n.T("ai.imageTimeoutPresets"), "", &current, options(pairs...), locked) {
			v, _ := strconv.ParseFloat(current, 64)
			s.ImageTimeout = ptr(v)
		}
		numberRow(c, i18n.T("ai.imageTimeout"), &timeout, 1, 60, true, "", locked, func(v *float64) {
			if v != nil {
				s.ImageTimeout = v
			}
		})
		kit.Stack(c, "", i18n.T("ai.imageTimeoutHint"), func() {})
		if catalog.Gateway != "" {
			mcpAddress(c, i18n.T("ai.visionMCP"), i18n.T("ai.copyVisionMCP"), i18n.T("ai.visionMCPHint"), catalog.Gateway+"/mcp/magpie/vision")
			mcpAddress(c, i18n.T("ai.imageMCP"), i18n.T("ai.copyImageMCP"), i18n.T("ai.imageMCPHint"), catalog.Gateway+"/mcp/magpie/image")
		}
	})
	kit.Group(c, icons.Search, i18n.T("ai.webSearch"), "", func() {
		auto := i18n.T("ai.automatic")
		if catalog.SearchAuto != "" {
			auto += " · " + catalog.SearchAuto
		}
		items := []kit.SelectOption{{Value: "", Label: auto}, {Value: "off", Label: i18n.T("ai.off")}}
		for _, choice := range catalog.SearchChoices {
			kind := choice.SmallName
			switch {
			case choice.Service:
				kind = i18n.T("ai.nativeWebSearch")
			case kind == "":
				kind = choice.Small
			}
			if kind == "" {
				kind = i18n.T("ai.modelDefault")
			}
			items = append(items, kit.SelectOption{Value: choice.ID, Label: choice.Name + " · " + kind})
			for _, m := range choice.Models {
				if m.ID == choice.ID+"/"+choice.Small {
					continue
				}
				name := m.Name
				if name == "" {
					name = m.ID
				}
				items = append(items, kit.SelectOption{Value: m.ID, Label: name + " · " + choice.Name})
			}
		}
		if s.Searcher != "" && !slices.ContainsFunc(items, func(o kit.SelectOption) bool { return o.Value == s.Searcher }) {
			items = append(items, kit.SelectOption{Value: s.Searcher, Label: s.Searcher})
		}
		kit.Stack(c, i18n.T("ai.searcher"), i18n.T("ai.searcherHint"), func() {
			searcher := s.Searcher
			changed, trigger := kit.Select(c, &searcher, items)
			trigger.Label(i18n.T("ai.searcher")).Disabled(locked)
			if changed {
				s.Searcher = searcher
			}
		})
		if catalog.SearchUnused != "" {
			kit.Stack(c, "", "", func() { kit.Notice(c, kit.Caution, "", i18n.T("ai.searchUnavailable", "reason", catalog.SearchUnused), nil) })
		}
		if len(catalog.SearchRelays) > 0 {
			kit.Stack(c, "", i18n.T("ai.searchRelays", "names", strings.Join(catalog.SearchRelays, "、")), func() {})
		}
		if len(catalog.SearchLeftOut) > 0 {
			kit.Stack(c, "", i18n.T("ai.searchLeftOut", "names", strings.Join(catalog.SearchLeftOut, "、")), func() {})
		}
		a.searchAPIs(c, catalog, locked)
		first := s.SearchFirst
		if first == "" {
			first = "model"
		}
		if choiceRow(c, i18n.T("ai.searchFirst"), "", &first, options("model", i18n.T("ai.searchModelFirst"), "api", i18n.T("ai.searchAPIFirst")), locked) {
			s.SearchFirst = ""
			if first == "api" {
				s.SearchFirst = "api"
			}
		}
		if catalog.Gateway != "" {
			mcpAddress(c, i18n.T("ai.searchMCP"), i18n.T("ai.copySearchMCP"), "", strings.TrimRight(catalog.Gateway, "/")+"/mcp/magpie/web-search")
		}
	})
	kit.Group(c, icons.MessagesSquare, i18n.T("ai.replies"), "", func() {
		classify := value(s.ClassifyTimeout)
		numberRow(c, i18n.T("ai.classifyTimeout"), &classify, 0, 60, true, "", locked, func(v *float64) { s.ClassifyTimeout = ptr(value(v)) })
		kit.Stack(c, "", i18n.T("ai.classifyTimeoutHint"), func() {})
		runs := value(s.ClaudeRunsMost)
		numberRow(c, i18n.T("ai.claudeRunsMost"), &runs, 0, 256, true, "", locked, func(v *float64) { s.ClaudeRunsMost = ptr(value(v)) })
		kit.Stack(c, "", i18n.T("ai.claudeRunsMostHint"), func() {})
		kit.Toggle(c, &s.ClaudeLeanPrompt, i18n.T("ai.claudeLeanPrompt"), i18n.T("ai.claudeLeanPromptHint")).Disabled(locked)
		kit.Toggle(c, &s.MemberModel, i18n.T("ai.memberModel"), i18n.T("ai.memberModelHint")).Disabled(locked)
		guard := !s.NoLoopGuard
		if kit.Toggle(c, &guard, i18n.T("ai.loopGuard"), i18n.T("ai.loopGuardHint")).Disabled(locked).Changed() {
			s.NoLoopGuard = !guard
		}
	})
	kit.Group(c, icons.LayoutGrid, i18n.T("ai.clientModels"), "", func() {
		names := "on"
		switch {
		case s.PlainNames:
			names = "off"
		case s.PlainOwnNames:
			names = "own"
		}
		if choiceRow(c, i18n.T("ai.providerInModelNames"), "", &names, options("on", i18n.T("ai.always"), "own", i18n.T("ai.exceptOwnNames"), "off", i18n.T("ai.off")), locked) {
			s.PlainNames, s.PlainOwnNames = names == "off", names == "own"
		}
		kit.Toggle(c, &s.CodexAgentsV1, i18n.T("ai.codexAgentsV1"), "").Disabled(locked)
		kit.Toggle(c, &s.FullContext, i18n.T("ai.fullContext"), "").Disabled(locked)
		if !s.FullContext {
			compact := value(s.CompactAt)
			if s.CompactAt == nil {
				compact = 272000
			}
			numberRow(c, i18n.T("ai.compactionThreshold"), &compact, 0, 1e12, true, "", locked, func(v *float64) { s.CompactAt = ptr(value(v)) })
		}
		a.helperModel(c, i18n.T("ai.codexTitles"), s.CodexTitles, catalog.TitleModels, "", "", locked, func(v string) { s.CodexTitles = v })
		a.helperModel(c, i18n.T("ai.codexAutoReview"), s.CodexAutoReview, catalog.TitleModels, "", "", locked, func(v string) { s.CodexAutoReview = v })
	})
}

// searchForm is the form adding a search service's key or address.
type searchForm struct {
	vendor, key, url string
}

// searchAPIs are the search services, in the order they are tried: a
// key or address added, each one's switch, how several keys take turns,
// reading and copying a saved key, reordering and removal.
func (a *advancedPanel) searchAPIs(c *ui.Context, catalog *magpieCatalog, locked bool) {
	s := a.settings()
	vendors := catalog.SearchVendors
	f := &a.search
	if f.vendor == "" {
		f.vendor = "tavily"
	}
	var selected *searchVendor
	for i := range vendors {
		if vendors[i].ID == f.vendor {
			selected = &vendors[i]
		}
	}
	if selected == nil && len(vendors) > 0 {
		selected = &vendors[0]
	}
	apis := s.SearchAPIs
	services := c.Services()
	kit.Stack(c, i18n.T("ai.searchAPI"), "", func() {
		var items []kit.SelectOption
		for _, v := range vendors {
			items = append(items, kit.SelectOption{Value: v.ID, Label: v.Name})
		}
		vendor := ""
		if selected != nil {
			vendor = selected.ID
		}
		changed, trigger := kit.Select(c, &vendor, items)
		trigger.Label(i18n.T("ai.searchAPI")).Disabled(locked || len(vendors) == 0)
		if changed {
			f.vendor = vendor
		}
		ui.Field(c, i18n.T("ai.searchAPIKey"), func() {
			ui.TextInput(c, &f.key).FillWidth().Password().Placeholder(i18n.T("ai.searchAPIKeyHint")).Label(i18n.T("ai.searchAPIKey")).Disabled(locked)
		})
		kit.Help(c, i18n.T("ai.searchMultipleKeysHint"))
		placeholder := "https://api.example.com"
		if selected != nil && selected.NeedURL {
			placeholder = "https://searx.example.com"
		}
		ui.Field(c, i18n.T("ai.searchAPIURL"), func() {
			ui.TextInput(c, &f.url).FillWidth().Placeholder(placeholder).Label(i18n.T("ai.searchAPIURL")).Disabled(locked)
		})
		kit.Help(c, i18n.T("ai.searchAPIURLHint"))
		ui.Row(c).FillWidth().Gap(12).Wrap().AlignItems(ui.Center).Children(func() {
			hasKey := selected != nil && slices.ContainsFunc(apis, func(d core.Preserved[searchAPI]) bool {
				return d.Value.Vendor == selected.ID && (d.Value.HasKey || d.Value.Key != "")
			})
			blocked := selected == nil || (selected.NeedURL && strings.TrimSpace(f.url) == "") ||
				(!selected.KeyOptional && !selected.NeedURL && strings.TrimSpace(f.key) == "" && !hasKey)
			if kit.Button(c, i18n.T("ai.addSearchAPI"), icons.Plus, false).Disabled(locked || blocked).Clicked() {
				// Editing a credential keeps the service's switch, key turns
				// and place in the order.
				var entry core.Preserved[searchAPI]
				found := -1
				for i, d := range apis {
					if d.Value.Vendor == selected.ID {
						entry, found = d, i
					}
				}
				entry.Value.Vendor = selected.ID
				if key := strings.TrimSpace(f.key); key != "" {
					entry.Value.Key = key
				}
				entry.Value.URL = strings.TrimSpace(f.url)
				next := append([]core.Preserved[searchAPI]{}, apis...)
				if found >= 0 {
					next[found] = entry
				} else {
					next = append(next, entry)
				}
				s.SearchAPIs = next
				f.key, f.url = "", ""
			}
			if selected != nil && selected.KeysURL != "" {
				ui.Link(c, i18n.T("ai.getSearchKey"), selected.KeysURL).FontSize(kit.HelpSize(c))
			}
		})
		if a.searchErr != "" {
			kit.Notice(c, kit.Failure, "", a.searchErr, nil)
		}
	})
	for index, d := range apis {
		index, api := index, d.Value
		name := api.Vendor
		for _, v := range vendors {
			if v.ID == api.Vendor {
				name = v.Name
			}
		}
		kit.Stack(c.Key("api:"+api.Vendor), "", "", func() {
			ui.Row(c).FillWidth().Gap(8).Wrap().AlignItems(ui.Center).Justify(ui.SpaceBetween).Children(func() {
				kit.Label(c, kit.Itoa(index+1)+". "+name)
				ui.Row(c).Gap(4).Children(func() {
					move := func(by int) {
						next := append([]core.Preserved[searchAPI]{}, apis...)
						next[index], next[index+by] = next[index+by], next[index]
						s.SearchAPIs = next
					}
					if kit.IconButton(c, icons.ChevronUp, i18n.T("ai.searchMoveUp", "name", name)).Disabled(locked || index == 0).Clicked() {
						move(-1)
					}
					if kit.IconButton(c, icons.ChevronDown, i18n.T("ai.searchMoveDown", "name", name)).Disabled(locked || index == len(apis)-1).Clicked() {
						move(1)
					}
					if api.HasKey || api.Key != "" {
						shown := a.revealed[api.Vendor] != ""
						label, icon := "ai.showSearchKey", icons.Eye
						if shown {
							label, icon = "ai.hideSearchKey", icons.EyeOff
						}
						if kit.IconButton(c, icon, i18n.T(label, "name", api.Vendor)).Disabled(locked).Clicked() {
							if shown {
								delete(a.revealed, api.Vendor)
							} else {
								a.readSearchKey(api, func(key string) { a.revealed[api.Vendor] = key })
							}
						}
						if kit.IconButton(c, icons.Copy, i18n.T("ai.copySearchKey", "name", api.Vendor)).Disabled(locked).Clicked() {
							a.readSearchKey(api, func(key string) { services.WriteClipboard(key) })
						}
					}
					if kit.IconButton(c, icons.Trash2, i18n.T("ai.removeSearchAPI", "name", api.Vendor)).Disabled(locked).Clicked() {
						s.SearchAPIs = slices.DeleteFunc(append([]core.Preserved[searchAPI]{}, apis...), func(d core.Preserved[searchAPI]) bool {
							return d.Value.Vendor == api.Vendor
						})
					}
				})
			})
			on := !api.Off
			if ui.Checkbox(c, &on, i18n.T("ai.searchAPIEnabled", "name", name)).Disabled(locked).Changed() {
				a.editSearchAPI(api.Vendor, func(v *searchAPI) { v.Off = !on })
			}
			pick := api.Pick
			ui.Field(c, i18n.T("ai.searchKeyPick", "name", name), func() {
				ui.Column(c).Disabled(locked).Children(func() {
					if kit.Segmented(c, &pick, options("", i18n.T("ai.searchKeyTurns"), "order", i18n.T("ai.searchKeyOrder")), i18n.T("ai.searchKeyPick", "name", name)) {
						a.editSearchAPI(api.Vendor, func(v *searchAPI) { v.Pick = pick })
					}
				})
			})
			var parts []string
			if api.URL != "" {
				parts = append(parts, api.URL)
			}
			if key := a.revealed[api.Vendor]; key != "" {
				parts = append(parts, key)
			}
			if len(parts) > 0 {
				ui.Text(c, strings.Join(parts, " · ")).FontSize(kit.HelpSize(c)).Selectable().MinWidth(0)
			}
		})
	}
}

func (a *advancedPanel) editSearchAPI(vendor string, edit func(*searchAPI)) {
	s := a.settings()
	next := append([]core.Preserved[searchAPI]{}, s.SearchAPIs...)
	for i := range next {
		if next[i].Value.Vendor == vendor {
			edit(&next[i].Value)
		}
	}
	s.SearchAPIs = next
}

// readSearchKey reads a search service's key: a draft's own key, or the
// saved one from the Host.
func (a *advancedPanel) readSearchKey(api searchAPI, done func(string)) {
	a.searchErr = ""
	if api.Key != "" {
		done(api.Key)
		return
	}
	core.Go(a.p.env, func() (string, error) {
		var result struct {
			Key string `json:"key"`
		}
		err := a.p.env.Host.CallCompletingInto("capture.ai.magpie.search-key", map[string]string{"vendor": api.Vendor}, &result)
		return result.Key, err
	}, func(key string, err error) {
		if err != nil {
			a.searchErr = err.Error()
			return
		}
		done(key)
	})
}
