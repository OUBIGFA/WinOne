package aiservices

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/widgets"
)

// modelTest is the result of testing one model with the editor's
// connection.
type modelTest struct {
	Account  string `json:"account,omitempty"`
	Model    string `json:"model,omitempty"`
	OK       *bool  `json:"ok"`
	MS       int    `json:"ms"`
	Protocol string `json:"protocol,omitempty"`
	Status   int    `json:"status,omitempty"`
	Error    string `json:"error,omitempty"`
}

// discovered is a model a catalog lists.
type discovered struct {
	ID       string          `json:"id"`
	Name     string          `json:"name,omitempty"`
	Icon     string          `json:"icon,omitempty"`
	Free     bool            `json:"free,omitempty"`
	Metadata json.RawMessage `json:"metadata,omitempty"`
}

// modelsPanel is a provider's models: discovery, manual entry, which ones
// features may use, connectivity tests and each model's settings.
// Discovery replaces the catalog; checking a model enables it.
type modelsPanel struct {
	p        *page
	id       string
	query    string
	fetching bool
	testing  bool
	err      string
	tested   map[string]modelTest
	notice   string
	// clients are the agent clients a model can be tested as, with why
	// one cannot; nil until a test reported them.
	clients map[string]string
	editing string
	rows    ui.ListState
	height  float32
	// limitsOpen shows the request limits, open at first when one is set.
	limitsOpen bool
	// fields are the model settings' typed texts; tierOpen shows the
	// long-context price of the model tierFor.
	fields   fields
	tierOpen bool
	tierFor  string
	closed   bool
}

func newModelsPanel(p *page, pr *provider) *modelsPanel {
	return &modelsPanel{p: p, id: pr.ID, tested: map[string]modelTest{}, fields: fields{}, height: modelListHeight(p.env.Directory, pr.ID),
		limitsOpen: pr.MaxConcurrency != 0 || pr.MaxRPM != 0 || pr.QueueLimit != 0 || pr.QueueWait != 0}
}

func (m *modelsPanel) Close() { m.closed = true }

func (m *modelsPanel) working() bool { return m.fetching || m.testing }

// connectionSaved reports whether the provider's connection is the saved
// one: then the Host reads it, else the editor sends its unsaved form.
func (p *page) connectionSaved(pr *provider) bool {
	if p.saved == nil {
		return false
	}
	old := p.saved.Value.find(pr.ID)
	return old != nil && pr.AuthMode == old.AuthMode && pr.Agent == old.Agent && pr.AgentBackend == old.AgentBackend &&
		pr.Preset == old.Preset && pr.PresetRegion == old.PresetRegion && pr.BaseURL == old.BaseURL && pr.Protocol == old.Protocol &&
		p.keys[pr.ID] == nil && equal(pr.Keys, old.Keys) && equal(pr.KeyPolicy, old.KeyPolicy) && equal(pr.Headers, old.Headers)
}

// connectionPayload names the provider for discovery and tests: a saved
// connection by its ID, an unsaved one with its form and new keys, which
// the Host uses without saving.
func (p *page) connectionPayload(pr *provider) map[string]any {
	payload := map[string]any{"provider": pr.ID}
	if p.connectionSaved(pr) {
		return payload
	}
	if pr.AuthMode == "account" {
		payload["draft"] = draftAgent{Agent: pr.Agent, AgentBackend: pr.AgentBackend}
		return payload
	}
	protocol := pr.Protocol
	if protocol == "" {
		protocol = "chat"
	}
	connection := map[string]any{"protocol": protocol, "baseUrl": pr.BaseURL, "keys": pr.slots()}
	if pr.Headers != nil {
		connection["headers"] = pr.Headers
	}
	if pr.LimitRestOff {
		connection["limitRestOff"] = true
	}
	if pr.AdditionalTools {
		connection["additionalTools"] = true
	}
	if pr.Preset != "" {
		connection["preset"] = pr.Preset
	}
	if pr.PresetRegion != "" {
		connection["presetRegion"] = pr.PresetRegion
	}
	if pr.KeyPolicy != nil {
		connection["keyPolicy"] = pr.KeyPolicy
	}
	payload["connection"] = connection
	if edits := p.keys[pr.ID]; edits != nil {
		payload["keyUpdates"] = edits
	}
	return payload
}

// canFetch reports whether the provider can list models: an account, or a
// connection with an address.
func canFetch(pr *provider) bool {
	return pr.AuthMode == "account" || strings.TrimSpace(pr.BaseURL) != ""
}

// test tests models with the editor's connection, as Magpie's
// provider/test does: nothing is saved, and every model must have a
// result for the test to count.
func (m *modelsPanel) test(pr *provider, models []string, as string) {
	p := m.p
	if m.working() || len(models) == 0 {
		return
	}
	payload := p.connectionPayload(pr)
	payload["models"] = models
	if as != "" {
		payload["as"] = as
	}
	if pr.ModelMetadata != nil {
		payload["metadata"] = pr.ModelMetadata
	}
	m.testing, m.err, m.notice = true, "", ""
	p.modelBusy = true
	type reply struct {
		Results []modelTest       `json:"results"`
		Clients map[string]string `json:"clients"`
	}
	core.Go(p.env, func() (reply, error) {
		var r reply
		if err := p.env.Host.CallCompletingInto("capture.ai.models.test", payload, &r); err != nil {
			return r, err
		}
		if len(r.Results) != len(models) {
			return r, errors.New(i18n.T("ai.modelTestIncomplete"))
		}
		for _, result := range r.Results {
			if result.OK == nil {
				return r, errors.New(i18n.T("ai.modelTestIncomplete"))
			}
		}
		return r, nil
	}, func(r reply, err error) {
		m.testing, p.modelBusy = false, false
		if m.closed {
			return
		}
		if err != nil {
			m.err = err.Error()
			return
		}
		m.clients = r.Clients
		if m.clients == nil {
			m.clients = map[string]string{}
		}
		failed := 0
		for i, model := range models {
			m.tested[model] = r.Results[i]
			if !*r.Results[i].OK {
				failed++
			}
		}
		key := "ai.modelTestsPassed"
		if failed > 0 {
			key = "ai.modelTestsFailed"
		}
		m.notice = i18n.T(key, "count", len(models), "failed", failed)
	})
}

// fetch replaces the catalog with the vendor's current list. A list is a
// snapshot: names, prices and metadata of retired models go with them.
// An account's plugin may list nothing for a while, so its enabled models
// that left the list are paused, not forgotten; Select none forgets them.
func (m *modelsPanel) fetch(pr *provider) {
	p := m.p
	payload := p.connectionPayload(pr)
	m.fetching, m.err = true, ""
	p.modelBusy = true
	core.Go(p.env, func() ([]discovered, error) {
		var r struct {
			Models []discovered `json:"models"`
		}
		err := p.env.Host.CallCompletingInto("capture.ai.models.fetch", payload, &r)
		return r.Models, err
	}, func(found []discovered, err error) {
		m.fetching, p.modelBusy = false, false
		if m.closed {
			return
		}
		if err != nil {
			m.err = err.Error()
			return
		}
		names, free, meta := map[string]string{}, map[string]bool{}, map[string]metadata{}
		var ids []string
		for _, model := range found {
			if strings.TrimSpace(model.Name) != "" {
				names[model.ID] = model.Name
			}
			free[model.ID] = model.Free
			if len(model.Metadata) > 0 && string(model.Metadata) != "null" {
				var d metadata
				if json.Unmarshal(model.Metadata, &d) == nil {
					meta[model.ID] = d
				}
			}
			ids = append(ids, model.ID)
		}
		available := unique(ids)
		listed := map[string]bool{}
		for _, id := range available {
			listed[id] = true
		}
		p.change(m.id, func(t *provider) {
			restored := t.Models
			if t.AuthMode == "account" {
				restored = append(append([]string{}, value(t.Delisted)...), t.Models...)
			}
			restored = unique(restored)
			models, delisted := []string{}, []string{}
			for _, id := range restored {
				if listed[id] {
					models = append(models, id)
				} else if t.AuthMode == "account" {
					delisted = append(delisted, id)
				}
			}
			t.AvailableModels, t.Models, t.Delisted = &available, models, &delisted
			t.ModelNames, t.ModelFree, t.ModelMetadata = names, free, meta
		})
		// Tests describe the previous list: test again after discovery.
		m.tested, m.notice, m.clients, m.editing = map[string]modelTest{}, "", nil, ""
	})
}

// View shows the models of pr.
func (m *modelsPanel) View(c *ui.Context, pr *provider) {
	p := m.p
	disabled := p.saving || p.loading || p.accountBusy || (pr.AuthMode == "account" && p.accountLoading)
	working := m.working()
	fetchable := canFetch(pr)
	all := pr.available()
	id := pr.ID
	kit.Section(c, icons.ListChecks, i18n.T("ai.models"), i18n.T("ai.modelSelectionHint"), func() {
		ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
			if kit.Button(c, i18n.T("ai.testModels"), icons.PlugZap, m.testing).Disabled(disabled || working || !fetchable || len(pr.Models) == 0).Clicked() {
				m.test(pr, append([]string{}, pr.Models...), "")
			}
			if kit.Button(c, i18n.T("ai.fetchModels"), icons.RefreshCw, m.fetching).Disabled(disabled || working || !fetchable).Clicked() {
				m.fetch(pr)
			}
		})
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			ui.TextInput(c, &m.query).Grow(1).MinWidth(144).Placeholder(i18n.T("ai.findOrAddModel")).Label(i18n.T("ai.findOrAddModel")).
				Disabled(disabled || working)
			if kit.Button(c, i18n.T("ai.addModel"), icons.Plus, false).Disabled(disabled || working || strings.TrimSpace(m.query) == "").Clicked() {
				m.add(all)
			}
		})
		if !fetchable && len(all) > 0 {
			kit.Help(c, i18n.T("ai.connectionForModels"))
		}
		if m.err != "" {
			kit.Notice(c, kit.Failure, "", m.err, nil)
		}
		if m.notice != "" {
			kit.Help(c, m.notice).Role(ui.RoleStatus)
		}
		for _, client := range sortedKeys(m.clients) {
			reason := m.clients[client]
			if reason == "" {
				reason = i18n.T("ai.clientTestAvailable")
			}
			label := "ai.testAsClaude"
			if client == "codex" {
				label = "ai.testAsCodex"
			}
			kit.Help(c, i18n.T(label)+" · "+reason)
		}
		ui.Row(c).FillWidth().Gap(8).Wrap().AlignItems(ui.Center).Justify(ui.SpaceBetween).Children(func() {
			kit.Caption(c, i18n.T("ai.enabledModels", "count", len(pr.Models)))
			ui.Row(c).Gap(4).Role(ui.RoleGroup).Label(i18n.T("ai.modelBulkActions")).Children(func() {
				if kit.GhostButton(c, i18n.T("ai.selectAll"), nil, false).Tooltip(i18n.T("ai.modelBulkScope")).
					Disabled(disabled || working || len(all) == 0 || len(pr.Models) == len(all)).Clicked() {
					p.change(id, func(t *provider) {
						everything := append([]string{}, all...)
						t.AvailableModels, t.Models = &everything, append([]string{}, all...)
					})
				}
				paused := len(value(pr.Delisted)) > 0
				if kit.GhostButton(c, i18n.T("ai.selectNone"), nil, false).Tooltip(i18n.T("ai.modelBulkScope")).
					Disabled(disabled || working || (len(pr.Models) == 0 && !paused)).Clicked() {
					p.change(id, func(t *provider) {
						everything := append([]string{}, all...)
						t.AvailableModels, t.Models, t.Delisted = &everything, []string{}, &[]string{}
					})
				}
			})
		})
		if n := len(value(pr.Delisted)); n > 0 {
			kit.Help(c, i18n.T("ai.delistedPicks", "count", n)).Role(ui.RoleStatus)
		}
		m.list(c, pr, all, disabled, working, fetchable)
		m.limits(c, pr, disabled || working)
	})
	if m.editing != "" {
		m.settings(c, pr, disabled || working)
	}
}

// add enables a model typed into the search field.
func (m *modelsPanel) add(all []string) {
	model := strings.TrimSpace(m.query)
	if model == "" {
		return
	}
	if len(model) > 256 {
		m.err = i18n.T("ai.modelNameTooLong")
		return
	}
	m.p.change(m.id, func(t *provider) {
		available := unique(append(append([]string{}, all...), model))
		t.AvailableModels, t.Models = &available, unique(append(append([]string{}, t.Models...), model))
	})
	m.query, m.err = "", ""
}

// list is the models matching the search, in a list the user can make
// taller or shorter; its height is kept per provider.
func (m *modelsPanel) list(c *ui.Context, pr *provider, all []string, disabled, working, fetchable bool) {
	p := m.p
	t, palette := c.Theme(), kit.Colors(c)
	query := strings.ToLower(strings.TrimSpace(m.query))
	var visible []string
	for _, model := range all {
		if strings.Contains(strings.ToLower(model+" "+pr.ModelNames[model]), query) {
			visible = append(visible, model)
		}
	}
	enabled := make(map[string]bool, len(pr.Models))
	for _, model := range pr.Models {
		enabled[model] = true
	}
	picker := pr.picker()
	id := pr.ID
	m.rows.Key = func(i int) any { return visible[i] }
	ui.Column(c).FillWidth().Children(func() {
		list := ui.List(c, &m.rows, len(visible), func(i int) {
			model := visible[i]
			on := enabled[model]
			name := widgets.ModelLabel(&picker, model)
			result, tested := m.tested[model]
			row := ui.Column(c).FillWidth().Padding(4, 8).Radius(t.Radius).Tooltip(model)
			if row.Hovered() {
				row.Background(t.SurfaceHover)
			}
			services := c.Services()
			menu := func(menu *ui.Menu) { m.menu(menu, services, pr, model, name, fetchable) }
			if !disabled && !working {
				row.ContextMenu(menu)
			}
			row.Children(func() {
				ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
					check := on
					box := ui.CheckboxBase(c, &check).Grow(1).Shrink(1).MinWidth(0).Gap(6).AlignItems(ui.Center).Padding(4, 0).
						Label(name).Disabled(disabled || working)
					if box.Changed() {
						p.change(id, func(t *provider) {
							everything := append([]string{}, all...)
							t.AvailableModels = &everything
							if check {
								t.Models = append(append([]string{}, t.Models...), model)
							} else {
								t.Models = slices.DeleteFunc(append([]string{}, t.Models...), func(v string) bool { return v == model })
							}
						})
					}
					box.Children(func() {
						kit.CheckMark(c, check, box.Hovered())
						icon := widgets.ModelIcon(c, p.env, model, pr.ModelNames[model], 16)
						if !on {
							icon.Opacity(0.55)
						}
						label := ui.Text(c, name).Shrink(1).MinWidth(0)
						if !on {
							label.TextColor(t.TextMuted)
						}
						if badges := widgets.ModelBadges(&picker, model); badges != nil {
							badges(c)
						}
					})
					if kit.IconButton(c, icons.SlidersHorizontal, i18n.T("ai.editModelSettings", "name", name)).Tooltip(i18n.T("ai.modelSettings")).
						Disabled(disabled || working).Clicked() {
						m.editing = model
					}
					kit.IconButton(c, icons.Ellipsis, i18n.T("ai.modelActions", "name", name)).Disabled(disabled || working).Menu(menu)
				})
				if tested {
					ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Start).Padding(0, 0, 0, 24).Children(func() {
						color, mark := t.TextMuted, icons.Check
						if !*result.OK {
							color, mark = t.Danger, icons.X
						}
						ui.Icon(c, mark).FontSize(14).TextColor(color).Shrink(0).Margin(2, 0, 0, 0)
						ui.Text(c, testLine(result, model)).FontSize(kit.HelpSize(c)).TextColor(color).Shrink(1).MinWidth(0)
					})
				}
			})
		}).FillWidth().Height(m.height).Padding(8).Border(1, palette.Hairline).Radius(kit.PanelRadius).
			Label(i18n.T("ai.models")).Tooltip(i18n.T("ai.resizeModelList"))
		list.Children(func() {
			if len(visible) == 0 {
				key := "ai.connectionForModels"
				switch {
				case len(all) > 0:
					key = "ai.noMatchingModels"
				case fetchable:
					key = "ai.fetchOrAddModels"
				}
				kit.Help(c, i18n.T(key)).Padding(12)
			}
		})
		m.height = resizeGrip(c, m.height, 240, 1200, func(height float32) {
			saveModelListHeight(p.env.Directory, id, height)
		})
	})
}

// testLine describes a test: the account, the model actually used, the
// API and the time, or the failure.
func testLine(r modelTest, model string) string {
	var parts []string
	if r.Account != "" {
		parts = append(parts, r.Account)
	}
	if r.Model != "" && r.Model != model {
		parts = append(parts, r.Model)
	}
	if r.Protocol != "" {
		parts = append(parts, r.Protocol)
	}
	if *r.OK {
		parts = append(parts, i18n.T("ai.modelTestTime", "ms", r.MS))
		return strings.Join(parts, " · ")
	}
	failure := r.Error
	if failure == "" {
		failure = i18n.T("ai.modelTestFailed")
	}
	if r.Status != 0 {
		failure = strconv.Itoa(r.Status) + " " + failure
	}
	return strings.Join(append(parts, failure), " · ")
}

// menu is a model's actions: tests, copying its ID, its settings.
func (m *modelsPanel) menu(menu *ui.Menu, services ui.Services, pr *provider, model, name string, fetchable bool) {
	if menu.Item(i18n.T("ai.testThisModel")).Disabled(!fetchable).Chosen() {
		m.test(pr, []string{model}, "")
	}
	for _, client := range []struct{ id, label string }{{"codex", "ai.testAsCodex"}, {"claude-code", "ai.testAsClaude"}} {
		reason, known := m.clients[client.id]
		unavailable := m.clients != nil && (!known || reason != "")
		if menu.Item(i18n.T(client.label)).Disabled(!fetchable || unavailable).Chosen() {
			m.test(pr, []string{model}, client.id)
		}
	}
	if menu.Item(i18n.T("ai.copyModelID")).Chosen() {
		services.WriteClipboard(model)
	}
	if menu.Item(i18n.T("ai.modelSettings")).Chosen() {
		m.editing = model
	}
}

// limits are the provider's request limits, open when one is set.
func (m *modelsPanel) limits(c *ui.Context, pr *provider, disabled bool) {
	p := m.p
	id := pr.ID
	ui.Collapsible(c, i18n.T("ai.requestLimits"), &m.limitsOpen, func() {
		ui.Column(c).FillWidth().Gap(12).Padding(8, 0).Disabled(disabled).Children(func() {
			ui.Row(c).FillWidth().GapX(16).GapY(12).Wrap().Children(func() {
				for _, field := range []struct {
					key  string
					max  float64
					read func(*provider) *float64
				}{
					{"maxConcurrency", 1000, func(t *provider) *float64 { return &t.MaxConcurrency }},
					{"maxRPM", 10000, func(t *provider) *float64 { return &t.MaxRPM }},
					{"queueLimit", 10000, func(t *provider) *float64 { return &t.QueueLimit }},
					{"queueWait", 3600, func(t *provider) *float64 { return &t.QueueWait }},
				} {
					field := field
					current := field.read(pr)
					var shown *float64
					if *current != 0 {
						shown = current
					}
					ui.Field(c.Key(field.key), i18n.T("ai."+field.key), func() {
						if v, changed := numberText(c, shown, 0, field.max, true, i18n.T("ai.noRequestLimit"), i18n.T("ai."+field.key)); changed {
							p.change(id, func(t *provider) { *field.read(t) = value(v) })
						}
					}).Basis(180).Grow(1)
				}
			})
			kit.Help(c, i18n.T("ai.requestLimitsHint"))
		})
	})
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// resizeGrip is a list's bottom edge, which drags its height between low
// and high; done keeps the height the user chose.
func resizeGrip(c *ui.Context, height, low, high float32, done func(float32)) float32 {
	t := c.Theme()
	grip := ui.Box(c).FillWidth().Height(8).Cursor(ui.CursorResizeNS).Focusable().Label(i18n.T("ai.resizeModelList")).
		Role(ui.RoleSplitter)
	line := kit.Colors(c).Hairline
	if grip.Hovered() || grip.Dragging() {
		line = t.Border
	}
	grip.Draw(func(painter *ui.Painter, r ui.Rect) {
		painter.Fill(ui.Rect{X: r.X + r.W/2 - 16, Y: r.Y + 3, W: 32, H: 2}, line, 1)
	})
	if _, dy, ok := grip.Dragged(); ok {
		height = max(low, min(high, height+dy))
	}
	if grip.Shortcut(0, ui.KeyUp) {
		height = max(low, height-24)
		done(height)
	}
	if grip.Shortcut(0, ui.KeyDown) {
		height = min(high, height+24)
		done(height)
	}
	if _, _, ended := grip.DragEnded(); ended {
		done(height)
	}
	return height
}

// The model lists' heights are kept in the profile, as React kept them in
// its storage.
const uiStateFile = "ai-services-ui.json"

type uiState struct {
	ModelHeights  map[string]float32 `json:"modelHeights,omitempty"`
	GatewayHeight float32            `json:"gatewayHeight,omitempty"`
}

var uiStateMu sync.Mutex

func readUIState(directory string) uiState {
	var s uiState
	data, err := os.ReadFile(filepath.Join(directory, uiStateFile))
	if err == nil {
		_ = json.Unmarshal(data, &s)
	}
	return s
}

func writeUIState(directory string, edit func(*uiState)) {
	uiStateMu.Lock()
	defer uiStateMu.Unlock()
	s := readUIState(directory)
	edit(&s)
	data, _ := json.Marshal(s)
	_ = os.WriteFile(filepath.Join(directory, uiStateFile), data, 0o600)
}

func modelListHeight(directory, id string) float32 {
	if h := readUIState(directory).ModelHeights[id]; h >= 240 && h <= 1200 {
		return h
	}
	return 400
}

func saveModelListHeight(directory, id string, height float32) {
	writeUIState(directory, func(s *uiState) {
		if s.ModelHeights == nil {
			s.ModelHeights = map[string]float32{}
		}
		s.ModelHeights[id] = height
	})
}
