package aiservices

import (
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/widgets"
)

// gatewayStatus is the local gateway's settings and state. models nil
// offers every enabled model; a list offers those, and none when empty.
type gatewayStatus struct {
	Revision float64                `json:"revision"`
	Enabled  bool                   `json:"enabled"`
	Port     float64                `json:"port"`
	Running  bool                   `json:"running"`
	HasKey   bool                   `json:"hasKey"`
	BaseURL  string                 `json:"baseUrl"`
	Error    string                 `json:"error,omitempty"`
	Models   *[]widgets.AISelection `json:"models"`
}

// gatewayPanel is the local gateway: its own settings, saved apart from
// the AI configuration. It stays while the page shows other tabs, so its
// draft does too.
type gatewayPanel struct {
	env     *core.Env
	saved   *gatewayStatus
	enabled bool
	port    string
	models  *[]widgets.AISelection
	working bool
	loading bool
	err     string
	key     string
	query   string
	list    ui.ListState
	height  float32
	read    int
	// interfaces shows the supported interfaces.
	interfaces bool
}

func newGatewayPanel(env *core.Env) *gatewayPanel {
	g := &gatewayPanel{env: env, height: 400}
	if h := readUIState(env.Directory).GatewayHeight; h >= 160 && h <= 1200 {
		g.height = h
	}
	return g
}

func (g *gatewayPanel) dirty() bool {
	return g.saved != nil && (g.saved.Enabled != g.enabled || strconv.FormatFloat(g.saved.Port, 'f', -1, 64) != g.port ||
		!equal(g.models, g.saved.Models))
}

func (g *gatewayPanel) Report() {
	g.env.Activity.Report("ai-gateway", g.dirty(), g.working, func() {
		if g.saved != nil {
			g.accept(*g.saved)
		}
	})
}

func (g *gatewayPanel) Close() {
	g.read++
	g.env.Activity.Report("ai-gateway", false, false, nil)
}

func (g *gatewayPanel) accept(status gatewayStatus) {
	g.saved = &status
	g.enabled, g.port, g.key = status.Enabled, strconv.FormatFloat(status.Port, 'f', -1, 64), ""
	g.models = nil
	if status.Models != nil {
		copied := append([]widgets.AISelection{}, *status.Models...)
		g.models = &copied
	}
}

// load reads the gateway's status; it holds the form, never navigation.
func (g *gatewayPanel) load() {
	g.read++
	owner := g.read
	g.loading, g.err = true, ""
	core.Go(g.env, func() (gatewayStatus, error) {
		var status gatewayStatus
		err := g.env.Host.CallCompletingInto("capture.ai.gateway.get", nil, &status)
		return status, err
	}, func(status gatewayStatus, err error) {
		if owner != g.read {
			return
		}
		g.loading = false
		if err != nil {
			g.err = err.Error()
			return
		}
		g.accept(status)
	})
}

var gatewayPort = regexp.MustCompile(`^[0-9]+$`)

func (g *gatewayPanel) save(resetKey bool) {
	if g.saved == nil {
		return
	}
	port, err := strconv.Atoi(g.port)
	if !gatewayPort.MatchString(g.port) || err != nil || port < 1024 || port > 65535 {
		g.err = i18n.T("gateway.portError")
		return
	}
	g.working, g.err = true, ""
	params := map[string]any{"settings": map[string]any{"revision": g.saved.Revision, "enabled": g.enabled, "port": port, "models": g.models},
		"resetKey": resetKey}
	core.Go(g.env, func() (gatewayStatus, error) {
		var status gatewayStatus
		err := g.env.Host.CallCompletingInto("capture.ai.gateway.save", params, &status)
		return status, err
	}, func(status gatewayStatus, err error) {
		g.working = false
		if err != nil {
			// The form stays, so a port that could not be bound can be
			// changed and saved again.
			g.err = err.Error()
			return
		}
		g.accept(status)
		g.env.Toast(i18n.T("common.saved"))
	})
}

// accessKey shows the access key, or copies it.
func (g *gatewayPanel) accessKey(services ui.Services, copy bool) {
	g.working, g.err = true, ""
	core.Go(g.env, func() (string, error) {
		var result struct {
			Key string `json:"key"`
		}
		err := g.env.Host.CallCompletingInto("capture.ai.gateway.key", nil, &result)
		if err == nil && result.Key == "" {
			err = errors.New(i18n.T("gateway.copyFailed"))
		}
		return result.Key, err
	}, func(key string, err error) {
		g.working = false
		if err != nil {
			g.err = err.Error()
			return
		}
		if copy {
			services.WriteClipboard(key)
			g.env.Toast(i18n.T("gateway.copied"))
			return
		}
		g.key = key
	})
}

// View is the gateway's settings, connection and models. providers are
// the saved ones: the gateway offers only saved models.
func (g *gatewayPanel) View(c *ui.Context, providers []widgets.AIProvider) {
	g.Report()
	busy := g.working || g.loading
	if g.saved == nil && !busy && g.err == "" {
		g.load()
	}
	services := c.Services()
	kit.Help(c, i18n.T("gateway.hint"))
	if g.err != "" {
		kit.Notice(c, kit.Failure, "", g.err, func() {
			if g.saved == nil && kit.Button(c, i18n.T("common.retry"), icons.RefreshCw, false).Disabled(busy).Clicked() {
				g.load()
			}
		})
	}
	if g.saved == nil {
		if g.err == "" {
			kit.Status(c, i18n.T("gateway.loading"))
		}
		return
	}
	s := g.saved
	dirty := g.dirty()
	g.env.Rail.Set(
		core.Action{Label: i18n.T("gateway.save"), Disabled: busy || (!dirty && s.Running == s.Enabled && s.Error == ""), Busy: g.working,
			Run: func() { g.save(false) }},
		core.Action{Disabled: busy || !dirty, Run: func() { g.accept(*s) }},
	)
	if s.Error != "" {
		kit.Notice(c, kit.Failure, "", s.Error, nil)
	}
	ui.Column(c).FillWidth().Gap(kit.GroupGap).Disabled(busy).Children(func() {
		kit.Group(c, icons.Server, i18n.T("settingsDesign.gatewayRun"), "", func() {
			kit.Toggle(c, &g.enabled, i18n.T("gateway.enable"), "")
			kit.Row(c, i18n.T("gateway.port"), i18n.T("gateway.localHint"), func() {
				ui.TextInput(c, &g.port).Width(160).Label(i18n.T("gateway.port"))
			})
			state := "gateway.stopped"
			if s.Running {
				state = "gateway.running"
			}
			kit.Row(c, i18n.T("gateway.status")+": "+i18n.T(state), "", func() {
				if kit.Button(c, i18n.T("common.refresh"), icons.RefreshCw, g.loading).Disabled(dirty).Clicked() {
					g.load()
				}
			})
		})
		kit.Group(c, icons.Plug, i18n.T("settingsDesign.gatewayConnect"), "", func() {
			kit.Stack(c, i18n.T("gateway.address"), "", func() {
				address := s.BaseURL
				ui.TextInput(c, &address).FillWidth().ReadOnly(true).Label(i18n.T("gateway.address"))
			})
			kit.Stack(c, i18n.T("gateway.key"), i18n.T("gateway.keyHint"), func() {
				placeholder := ""
				if s.HasKey {
					placeholder = "••••••••"
				}
				key := g.key
				ui.TextInput(c, &key).FillWidth().ReadOnly(true).Placeholder(placeholder).Label(i18n.T("gateway.key"))
				ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
					label, icon := "gateway.showKey", icons.Eye
					if g.key != "" {
						label, icon = "gateway.hideKey", icons.EyeOff
					}
					if kit.Button(c, i18n.T(label), icon, false).Disabled(!s.HasKey).Clicked() {
						if g.key != "" {
							g.key = ""
						} else {
							g.accessKey(services, false)
						}
					}
					if kit.Button(c, i18n.T("gateway.copyKey"), icons.Copy, false).Disabled(!s.HasKey).Clicked() {
						g.accessKey(services, true)
					}
					if kit.Button(c, i18n.T("gateway.resetKey"), icons.RotateCcw, false).Disabled(dirty).Clicked() {
						g.save(true)
					}
				})
			})
		})
		g.modelsView(c, providers, busy)
		ui.Collapsible(c, i18n.T("settingsDesign.gatewayInterfaces"), &g.interfaces, func() {
			kit.Help(c, i18n.T("gateway.protocols")).Padding(8, 0)
		})
	})
}

// gatewayEntry is a row of the gateway's model list: a provider's heading
// with its models below, indented.
type gatewayEntry struct {
	provider *widgets.AIProvider
	model    string // "" for the heading
	models   []string
}

func route(s widgets.AISelection) string { return s.Provider + "/" + s.Model }

// modelsView chooses the models the gateway offers, apart from the models
// features use: grouped by provider, with names and marks, never route
// IDs. Saving none offers none.
func (g *gatewayPanel) modelsView(c *ui.Context, providers []widgets.AIProvider, disabled bool) {
	t, palette := c.Theme(), kit.Colors(c)
	var all []widgets.AISelection
	for _, pr := range providers {
		for _, model := range pr.Models {
			all = append(all, widgets.AISelection{Provider: pr.ID, Model: model})
		}
	}
	enabled := map[string]bool{}
	source := all
	if g.models != nil {
		source = *g.models
	}
	for _, s := range source {
		enabled[route(s)] = true
	}
	count := 0
	for _, s := range all {
		if enabled[route(s)] {
			count++
		}
	}
	set := func(next map[string]bool) {
		chosen := []widgets.AISelection{}
		for _, s := range all {
			if next[route(s)] {
				chosen = append(chosen, s)
			}
		}
		g.models = &chosen
	}
	toggle := func(items []widgets.AISelection, on bool) {
		next := map[string]bool{}
		for k, v := range enabled {
			next[k] = v
		}
		for _, s := range items {
			next[route(s)] = on
		}
		set(next)
	}
	terms := strings.Fields(strings.ToLower(g.query))
	var entries []gatewayEntry
	for i := range providers {
		pr := &providers[i]
		var matched []string
		for _, model := range pr.Models {
			text := pr.Name + " " + widgets.ModelLabel(pr, model)
			if widgets.IsFreeModel(pr, model) {
				text += " " + i18n.T("ai.free")
			}
			text = strings.ToLower(text)
			match := true
			for _, term := range terms {
				match = match && strings.Contains(text, term)
			}
			if match {
				matched = append(matched, model)
			}
		}
		if len(matched) == 0 {
			continue
		}
		entries = append(entries, gatewayEntry{provider: pr, models: matched})
		for _, model := range matched {
			entries = append(entries, gatewayEntry{provider: pr, model: model})
		}
	}
	kit.Section(c, icons.ListChecks, i18n.T("gateway.models"), i18n.T("gateway.modelsHint"), func() {
		kit.Caption(c, i18n.T("gateway.enabledModels", "count", count, "total", len(all)))
		if len(all) == 0 {
			kit.Help(c, i18n.T("gateway.empty"))
			return
		}
		ui.Row(c).FillWidth().Gap(8).Wrap().AlignItems(ui.Center).Children(func() {
			ui.TextInput(c, &g.query).Grow(1).Basis(192).MinWidth(0).Placeholder(i18n.T("gateway.searchModels")).Label(i18n.T("gateway.searchModels"))
			ui.Row(c).Gap(4).Role(ui.RoleGroup).Label(i18n.T("ai.modelBulkActions")).Children(func() {
				if kit.GhostButton(c, i18n.T("ai.selectAll"), nil, false).Tooltip(i18n.T("ai.modelBulkScope")).
					Disabled(disabled || count == len(all)).Clicked() {
					everything := append([]widgets.AISelection{}, all...)
					g.models = &everything
				}
				if kit.GhostButton(c, i18n.T("ai.selectNone"), nil, false).Tooltip(i18n.T("ai.modelBulkScope")).
					Disabled(disabled || count == 0).Clicked() {
					g.models = &[]widgets.AISelection{}
				}
			})
		})
		g.list.Key = func(i int) any { return entries[i].provider.ID + "\x00" + entries[i].model }
		list := ui.List(c, &g.list, len(entries), func(i int) {
			entry := entries[i]
			pr := entry.provider
			if entry.model == "" {
				chosen := 0
				var refs []widgets.AISelection
				for _, model := range pr.Models {
					ref := widgets.AISelection{Provider: pr.ID, Model: model}
					refs = append(refs, ref)
					if enabled[route(ref)] {
						chosen++
					}
				}
				on := chosen == len(pr.Models)
				box := ui.CheckboxBase(c, &on).FillWidth().Padding(8, 8).Gap(12).AlignItems(ui.Center).Radius(t.Radius).
					Background(palette.Hairline).Label(i18n.T("gateway.enableProvider", "provider", pr.Name)).Disabled(disabled)
				if box.Changed() {
					toggle(refs, on)
				}
				box.Children(func() {
					if chosen > 0 && chosen < len(pr.Models) {
						kit.PartialMark(c)
					} else {
						kit.CheckMark(c, on, box.Hovered())
					}
					widgets.ProviderIcon(c, g.env, providerMark(pr), 18)
					ui.Text(c, pr.Name).FontWeight(600).Grow(1).Shrink(1).MinWidth(0)
					kit.Caption(c, kit.Itoa(chosen)+" / "+kit.Itoa(len(pr.Models))).Shrink(0)
				})
				return
			}
			ref := widgets.AISelection{Provider: pr.ID, Model: entry.model}
			on := enabled[route(ref)]
			name := widgets.ModelLabel(pr, entry.model)
			box := ui.CheckboxBase(c, &on).FillWidth().Padding(8, 8, 8, 32).Gap(6).AlignItems(ui.Center).Radius(t.Radius).
				Label(pr.Name + " · " + name).Disabled(disabled)
			if box.Hovered() {
				box.Background(t.SurfaceHover)
			}
			if box.Changed() {
				toggle([]widgets.AISelection{ref}, on)
			}
			box.Children(func() {
				kit.CheckMark(c, on, box.Hovered())
				widgets.ModelIcon(c, g.env, entry.model, pr.ModelNames[entry.model], 16)
				ui.Text(c, name).Shrink(1).MinWidth(0)
				if badges := widgets.ModelBadges(pr, entry.model); badges != nil {
					badges(c)
				}
			})
		}).FillWidth().Height(g.height).Padding(8).Gap(2).Border(1, palette.Hairline).Radius(kit.PanelRadius).
			Label(i18n.T("gateway.models")).Tooltip(i18n.T("ai.resizeModelList"))
		list.Children(func() {
			if len(entries) == 0 {
				kit.Help(c, i18n.T("gateway.noMatchingModels")).Padding(12)
			}
		})
		g.height = resizeGrip(c, g.height, 160, 1200, func(height float32) {
			writeUIState(g.env.Directory, func(s *uiState) { s.GatewayHeight = height })
		})
	})
}

func providerMark(p *widgets.AIProvider) string {
	if p.Avatar != "" {
		return p.Avatar
	}
	return p.Icon
}
