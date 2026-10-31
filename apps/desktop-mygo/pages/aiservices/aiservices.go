// Package aiservices is the AI services page: the connections and
// accounts that provide models, the default service, the local gateway,
// routing, usage statistics, plugins and Magpie's advanced settings.
//
// React source: features/ai-settings.tsx and the ai-* parts it mounts;
// main.tsx mounts the page with a shared account reader. Feature parity:
// docs/mygo-parity/ai-services.md. Pages the Host serves (the routing
// editor, the plugin manager, privacy records and plugins' panels) show in
// web views (package web), as React's iframes did.
package aiservices

import (
	"crypto/rand"
	"encoding/hex"
	"strings"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/shell"
	"winone/desktopmygo/widgets"
)

func init() {
	shell.Register("ai-services", core.Workspace, func(env *core.Env) core.Page { return newPage(env) })
}

// The page's tabs, in their order.
const (
	tabCustom = iota
	tabAccounts
	tabDefaults
	tabGateway
	tabRouting
	tabUsage
	tabPlugins
	tabAdvanced
)

// The provider editor's tabs, for connections of the custom kind.
const (
	providerConnection = iota
	providerAdvanced
)

// The advanced tab's own tabs.
const (
	advancedQuota = iota
	advancedModels
	advancedPrivacy
	advancedFeatures
	advancedExtensions
)

// maxProviders is the Host's limit of configured services.
const maxProviders = 32

// keyEdits are new keys by provider and slot, sent with the next save
// and never kept after it.
type keyEdits map[string]map[string]string

type page struct {
	env *core.Env
	tab int

	// saved is the configuration as last read or saved; draft is the
	// page's edit of it. savedGen counts the times saved was replaced, as
	// React's resetToken: editors drop revealed keys and typed rows then.
	saved, draft *document
	savedGen     int
	keys         keyEdits
	// selected is the provider shown on the custom and accounts tabs,
	// remembered per tab.
	selected   string
	selectedBy map[int]string

	loading, saving bool
	accountBusy     bool
	accountLoading  bool
	modelBusy       bool
	avatarBusy      bool
	read            int
	err             string

	providerTab int
	advancedTab int
	// pluginInvalid holds Save while a plugin's options are not JSON.
	pluginInvalid bool
	// maskRecord opens the privacy records at one record.
	maskRecord string
	// focusModels moves to the model list after the overview opens a
	// provider.
	focusModels bool
	// focusAccount moves to the sign-in of an account just added.
	focusAccount bool

	reader  *accountReader
	catalog catalogState
	// choosingAgent opens the account type picker; choosingPreset the
	// presets.
	choosingAgent  bool
	choosingPreset bool
	presetSearch   string
	agentSearch    string
	curl           *curlNotice

	editor     *editor
	gateway    *gatewayPanel
	advanced   *advancedPanel
	extensions *extensions
	routing    *routingPanel
	plugins    *pluginsPanel
	usage      *usageTab
}

// curlNotice reports what a pasted cURL example filled in.
type curlNotice struct {
	provider string
	messages []string
}

func newPage(env *core.Env) *page {
	p := &page{env: env, selectedBy: map[int]string{}, keys: keyEdits{}, reader: readerFor(env)}
	p.gateway = newGatewayPanel(env)
	p.extensions = newExtensions(env)
	p.plugins = newPluginsPanel(p)
	p.routing = newRoutingPanel(p)
	p.extensions.open = func(r *pageRequest) {
		p.plugins.request = r
		p.switchTab(tabPlugins)
		p.plugins.request = r
	}
	p.load()
	return p
}

// navigationLocked holds the tabs while work runs that leaving would lose
// or interrupt: a save, a sign-in, a model discovery or a picture upload.
func (p *page) navigationLocked() bool {
	return p.saving || p.accountBusy || p.modelBusy || p.avatarBusy
}

// locked also holds the forms while the configuration is read.
func (p *page) locked() bool { return p.navigationLocked() || p.loading }

func (p *page) dirty() bool {
	return p.draft != nil && (!equal(p.draft, p.saved) || len(p.keys) > 0)
}

// load reads the configuration, replacing the draft; Undo and discarding
// changes read it again too.
func (p *page) load() {
	p.read++
	owner := p.read
	p.loading, p.err = true, ""
	core.Go(p.env, func() (document, error) {
		var value document
		err := p.env.Host.CallCompletingInto("capture.ai.configuration.get", nil, &value)
		return value, err
	}, func(value document, err error) {
		if owner != p.read {
			return
		}
		p.loading = false
		if err != nil {
			p.err = err.Error()
			return
		}
		widgets.RefreshProviderAvatars()
		p.accept(value)
		if p.draft.Value.find(p.selected) == nil {
			p.selected = ""
			if len(p.draft.Value.Providers) > 0 {
				p.selected = p.draft.Value.Providers[0].Value.ID
			}
		}
	})
}

// accept makes value the saved configuration and the draft.
func (p *page) accept(value document) {
	normalize(&value)
	saved, draft := clone(value), clone(value)
	p.saved, p.draft = &saved, &draft
	p.keys = keyEdits{}
	p.savedGen++
}

func (p *page) save() {
	if p.draft == nil || p.locked() || p.pluginInvalid {
		return
	}
	p.saving, p.err = true, ""
	params := map[string]any{"configuration": clone(*p.draft)}
	if len(p.keys) > 0 {
		params["keyUpdates"] = p.keys
	}
	core.Go(p.env, func() (document, error) {
		var value document
		err := p.env.Host.CallCompletingInto("capture.ai.configuration.save", params, &value)
		return value, err
	}, func(value document, err error) {
		p.saving = false
		if err != nil {
			// A failed save keeps the draft and the new keys, so the
			// user can correct them and save again.
			p.err = err.Error()
			return
		}
		widgets.RefreshProviderAvatars()
		p.accept(value)
		p.env.Toast(i18n.T("common.saved"))
	})
}

// enableRoutingGroups saves the routing page's one-click choice alone:
// provider, key and advanced drafts are not part of it, and stay drafts.
func (p *page) enableRoutingGroups(done func(error)) {
	p.saving, p.err = true, ""
	core.Go(p.env, func() (document, error) {
		var current document
		if err := p.env.Host.CallCompletingInto("capture.ai.configuration.get", nil, &current); err != nil {
			return current, err
		}
		normalize(&current)
		s := current.Value.settings()
		if s.BuiltinPlugins == nil {
			s.BuiltinPlugins = map[string]core.Preserved[builtinChoice]{}
		}
		groups := s.BuiltinPlugins["groups"]
		groups.Value.Enabled = ptr(true)
		s.BuiltinPlugins["groups"] = groups
		var value document
		err := p.env.Host.CallCompletingInto("capture.ai.configuration.save", map[string]any{"configuration": current}, &value)
		return value, err
	}, func(value document, err error) {
		p.saving = false
		if err != nil {
			p.err = err.Error()
			done(err)
			return
		}
		normalize(&value)
		saved := clone(value)
		p.saved = &saved
		if p.draft == nil {
			draft := clone(value)
			p.draft = &draft
		} else {
			// The draft takes the new revision and the choice, and keeps
			// its other edits.
			p.draft.Value.Revision = value.Value.Revision
			s := p.draft.Value.settings()
			if s.BuiltinPlugins == nil {
				s.BuiltinPlugins = map[string]core.Preserved[builtinChoice]{}
			}
			groups := s.BuiltinPlugins["groups"]
			groups.Value.Enabled = ptr(true)
			s.BuiltinPlugins["groups"] = groups
		}
		done(nil)
	})
}

// change edits a provider of the draft. A new model catalog clears task
// references to models it no longer offers: paused plugin picks keep
// theirs, but a call needs an enabled, listed model.
func (p *page) change(id string, edit func(*provider)) {
	if p.draft == nil {
		return
	}
	target := p.draft.Value.find(id)
	if target == nil {
		return
	}
	before := target.AvailableModels
	edit(target)
	if target.AvailableModels == before {
		return
	}
	models := map[string]bool{}
	for _, list := range [][]string{target.available(), target.Models, value(target.Delisted)} {
		for _, m := range list {
			models[m] = true
		}
	}
	for task, selection := range p.draft.Value.Tasks {
		if selection.Provider == id && !models[selection.Model] {
			p.draft.Value.Tasks[task] = widgets.AISelection{}
		}
	}
}

// changeKey stages a new key for a slot, or forgets one with nil.
func (p *page) changeKey(provider, slot string, key *string) {
	edits := p.keys[provider]
	if key == nil {
		delete(edits, slot)
		if len(edits) == 0 {
			delete(p.keys, provider)
		}
		return
	}
	if edits == nil {
		edits = map[string]string{}
		p.keys[provider] = edits
	}
	edits[slot] = *key
}

// importCurl fills the draft from a pasted cURL example, without saving:
// vendor examples are data, never commands. It reports whether the text
// was one; slot names the key field it was pasted into.
func (p *page) importCurl(text string, pr *provider, slot string) bool {
	parsed := curlOf(text)
	if parsed == nil {
		return false
	}
	var messages []string
	id := pr.ID
	savedOne := p.saved != nil && p.saved.Value.find(id) != nil
	p.change(id, func(target *provider) {
		if target.Preset == "" {
			if parsed.API != "" {
				target.Protocol = parsed.API
				if parsed.API == "openai" {
					target.Protocol = "chat"
				}
				target.BaseURL = parsed.Base
			} else {
				messages = append(messages, i18n.T("ai.curlUnknownEndpoint"))
			}
			if parsed.URLHole != "" {
				messages = append(messages, i18n.T("ai.curlReplaceAddress"))
			}
			if parsed.Model != "" && !savedOne {
				models := unique(append(append([]string{}, target.Models...), parsed.Model))
				target.Models = models
				available := unique(append(append([]string{}, value(target.AvailableModels)...), models...))
				target.AvailableModels = &available
			}
		}
		if parsed.Key != "" {
			slots := target.slots()
			if slot == "" {
				slot = "primary"
				if len(slots) > 0 {
					slot = slots[0].ID
				}
			}
			found := false
			for _, s := range slots {
				found = found || s.ID == slot
			}
			if !found {
				slots = append(slots, keySlot{ID: slot, Enabled: true})
			}
			copied := append([]keySlot{}, slots...)
			target.Keys = &copied
		} else if parsed.Placeholder != "" {
			messages = append(messages, i18n.T("ai.curlPlaceholderKey"))
		}
	})
	if parsed.Key != "" {
		key := parsed.Key
		p.changeKey(id, slot, &key)
	}
	p.curl = &curlNotice{provider: id, messages: append([]string{i18n.T("ai.curlImported")}, messages...)}
	return true
}

// uniqueName names a connection after its type, numbering duplicates.
// Naming one connection never renames another.
func (p *page) uniqueName(base, except string) string {
	names := map[string]bool{}
	for _, d := range p.draft.Value.Providers {
		if d.Value.ID != except {
			names[strings.ToLower(strings.TrimSpace(d.Value.Name))] = true
		}
	}
	name := base
	for i := 2; names[strings.ToLower(name)]; i++ {
		name = base + " " + kit.Itoa(i)
	}
	return name
}

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	h := hex.EncodeToString(b)
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

// add adds a connection: an account of agent, or a custom connection.
func (p *page) add(a *agent) string {
	pr := provider{ID: newID(), AuthMode: "api-key", Protocol: "chat", Models: []string{}, AvailableModels: &[]string{}}
	if a != nil {
		pr.AuthMode, pr.Name, pr.Agent, pr.Icon = "account", p.uniqueName(a.Name, ""), a.ID, a.Icon
		pr.AgentBackend = a.Backend
		if pr.AgentBackend == "" {
			pr.AgentBackend = "builtin"
		}
	}
	p.draft.Value.Providers = append(p.draft.Value.Providers, providerDoc{Value: pr})
	p.selected = pr.ID
	return pr.ID
}

// addPreset adds a preset's connection. A new custom connection the user
// has not filled in yet is replaced instead, so no empty one is left.
func (p *page) addPreset(preset servicePreset) {
	current := p.draft.Value.find(p.selected)
	var empty *provider
	if current != nil && current.AuthMode == "api-key" && (p.saved == nil || p.saved.Value.find(current.ID) == nil) &&
		strings.TrimSpace(current.Name) == "" && strings.TrimSpace(current.BaseURL) == "" && len(current.Models) == 0 &&
		len(value(current.AvailableModels)) == 0 && p.keys[current.ID] == nil && !hasSavedKey(current) {
		empty = current
	}
	region := ""
	if len(preset.Regions) > 0 {
		region = preset.Regions[0].ID
	}
	id := newID()
	if empty != nil {
		id = empty.ID
	}
	protocol, base := presetConnection(preset, region, "chat")
	models := append([]string{}, preset.Models...)
	pr := provider{ID: id, Name: p.uniqueName(preset.Name, id), AuthMode: "api-key", Preset: preset.ID, PresetRegion: region,
		Protocol: protocol, BaseURL: base, Models: []string{}, AvailableModels: &models, Icon: preset.Icon}
	if empty != nil {
		for i := range p.draft.Value.Providers {
			if p.draft.Value.Providers[i].Value.ID == id {
				p.draft.Value.Providers[i] = providerDoc{Value: pr}
			}
		}
	} else {
		p.draft.Value.Providers = append(p.draft.Value.Providers, providerDoc{Value: pr})
	}
	p.selected = id
}

func hasSavedKey(pr *provider) bool {
	for _, s := range value(pr.Keys) {
		if s.HasKey {
			return true
		}
	}
	return false
}

// changeProtocol changes the API; a preset's address follows the API
// while the user has not edited it.
func (p *page) changeProtocol(pr *provider, protocol string) {
	preset := p.catalog.preset(pr.Preset)
	p.change(pr.ID, func(target *provider) {
		if preset == nil {
			target.Protocol = protocol
			return
		}
		_, previous := presetConnection(*preset, target.PresetRegion, target.Protocol)
		nextProtocol, next := presetConnection(*preset, target.PresetRegion, protocol)
		target.Protocol = protocol
		if target.BaseURL == previous && nextProtocol == protocol {
			target.BaseURL = next
		}
	})
}

// remove takes a provider out of the draft, with the tasks that used it
// and its new keys.
func (p *page) remove(id string) {
	c := &p.draft.Value
	for task, selection := range c.Tasks {
		if selection.Provider == id {
			c.Tasks[task] = widgets.AISelection{}
		}
	}
	for i := range c.Providers {
		if c.Providers[i].Value.ID == id {
			c.Providers = append(c.Providers[:i], c.Providers[i+1:]...)
			break
		}
	}
	delete(p.keys, id)
	p.selected = ""
}

// openProvider shows a provider's models from the overview, in the same
// draft: reading the configuration again would lose unsaved edits.
func (p *page) openProvider(id string) {
	target := p.draft.Value.find(id)
	if target == nil || p.locked() {
		return
	}
	p.selectedBy[p.tab] = p.selected
	next := tabCustom
	if target.AuthMode == "account" {
		next = tabAccounts
	}
	p.selectedBy[next] = id
	p.tab, p.selected, p.providerTab = next, id, providerConnection
	p.focusModels = true
}

// sectionProviders are the providers of the custom or accounts tab.
func (p *page) sectionProviders() []*provider {
	if p.draft == nil || (p.tab != tabCustom && p.tab != tabAccounts) {
		return nil
	}
	mode := "api-key"
	if p.tab == tabAccounts {
		mode = "account"
	}
	var out []*provider
	for i := range p.draft.Value.Providers {
		if pr := &p.draft.Value.Providers[i].Value; pr.AuthMode == mode {
			out = append(out, pr)
		}
	}
	return out
}

// current is the provider the editor shows: the selected one, or the
// tab's first.
func (p *page) current() *provider {
	providers := p.sectionProviders()
	for _, pr := range providers {
		if pr.ID == p.selected {
			return pr
		}
	}
	if len(providers) > 0 {
		return providers[0]
	}
	return nil
}

// switchTab changes the page's tab, remembering each tab's provider.
func (p *page) switchTab(next int) {
	if pr := p.current(); pr != nil {
		p.selectedBy[p.tab] = pr.ID
	}
	if next == tabPlugins {
		p.plugins.request = nil
	}
	p.tab = next
	p.selected = p.selectedBy[next]
}

// Close implements core.Closer: leaving the page ends a sign-in, closes
// the Host's pages and drops answers on their way.
func (p *page) Close() {
	p.read++
	if p.editor != nil {
		p.editor.Close()
	}
	p.gateway.Close()
	p.extensions.Close()
	if p.routing != nil {
		p.routing.Close()
	}
	if p.plugins != nil {
		p.plugins.Close()
	}
	if p.advanced != nil {
		p.advanced.Close()
	}
	if p.usage != nil {
		p.usage.Close()
	}
	p.env.Activity.Report("ai-settings", false, false, nil)
}

func (p *page) View(c *ui.Context) {
	p.env.Activity.Report("ai-settings", p.dirty() || p.pluginInvalid, p.navigationLocked(), p.load)
	if p.saved != nil {
		p.extensions.Follow(p.saved.Value.Revision)
	}
	if (p.tab == tabCustom || p.tab == tabAccounts || p.anyPreset()) && !p.catalog.loaded && !p.catalog.busy && p.catalog.err == "" {
		p.loadCatalog()
	}
	p.rail()
	kit.WidePage(c, func() {
		kit.Help(c, i18n.T("ai.shared"))
		p.overview(c)
		ui.Column(c).FillWidth().Gap(16).Disabled(p.saving).Children(func() {
			ui.Column(c).FillWidth().Disabled(p.navigationLocked()).Children(func() {
				next := p.tab
				if kit.Tabs(c, &next, []kit.Tab{
					{Label: i18n.T("ai.customProviders"), Icon: icons.LayersPlus},
					{Label: i18n.T("ai.addProvidersTab"), Icon: icons.UserPlus},
					{Label: i18n.T("ai.defaultService"), Icon: icons.LayoutGrid},
					{Label: i18n.T("gateway.title"), Icon: icons.Share2},
					{Label: i18n.T("routing.title"), Icon: icons.Waypoints},
					{Label: i18n.T("usage.title"), Icon: icons.ChartNoAxesCombined},
					{Label: i18n.T("aiPlugins.title"), Icon: icons.Blocks},
					{Label: i18n.T("ai.advanced"), Icon: icons.SlidersHorizontal},
				}, p.env.RequestChange) {
					// kit.Tabs set next once the guard allowed it.
					p.switchTab(next)
				}
			})
			p.notices(c)
			switch p.tab {
			case tabCustom, tabAccounts:
				p.providersView(c)
			case tabDefaults:
				p.defaultsView(c)
			case tabGateway:
				var providers []widgets.AIProvider
				if p.saved != nil {
					providers = pickers(p.saved.Value.Providers)
				}
				p.gateway.View(c, providers)
			case tabRouting:
				p.routing.View(c)
			case tabUsage:
				p.usageView(c)
			case tabPlugins:
				p.plugins.View(c)
			case tabAdvanced:
				p.advancedView(c)
			}
		})
	})
	if p.tab != tabGateway {
		p.gateway.Report()
	}
	// The Host's pages close with their tabs, as React's iframes did.
	if p.tab != tabRouting {
		p.routing.Leave()
	}
	if p.tab != tabPlugins {
		p.plugins.Leave()
	}
	if p.tab != tabAdvanced && p.advanced != nil {
		p.advanced.leavePrivacy()
	}
	p.agentPicker(c)
	p.presetPicker(c)
}

// notices are the page's errors above the tab's content.
func (p *page) notices(c *ui.Context) {
	if p.err != "" {
		kit.Notice(c, kit.Failure, "", p.err, func() {
			if p.draft == nil && kit.Button(c, i18n.T("common.retry"), icons.RefreshCw, p.loading).Disabled(p.saving).Clicked() {
				p.load()
			}
		})
	}
	if err := p.extensions.err; err != "" {
		kit.Notice(c, kit.Failure, "", err, func() {
			if kit.Button(c, i18n.T("common.retry"), icons.RefreshCw, false).Clicked() {
				p.extensions.Reload()
			}
		})
	}
	if p.tab == tabAccounts && p.catalog.err != "" {
		kit.Notice(c, kit.Failure, "", p.catalog.err, func() {
			if kit.Button(c, i18n.T("common.retry"), icons.RefreshCw, false).Clicked() {
				p.catalog.err = ""
				p.loadCatalog()
			}
		})
	}
	if p.tab == tabAccounts && p.catalog.busy {
		kit.Status(c, i18n.T("capture.loading"))
	}
	if p.draft == nil && p.err == "" && p.tab != tabGateway && p.tab != tabUsage {
		ui.Column(c).FillWidth().Gap(16).Children(func() {
			kit.Status(c, i18n.T("capture.loading"))
			kit.Skeleton(c, 4)
		})
	}
}

// rail shows Save and Undo for the configuration. The gateway saves its
// own settings; usage statistics and plugins show them only once
// something changed there.
func (p *page) rail() {
	if p.draft == nil || p.tab == tabGateway || ((p.tab == tabUsage || p.tab == tabPlugins) && !p.dirty()) {
		return
	}
	dirty := p.dirty()
	p.env.Rail.Set(
		core.Action{Label: i18n.T("ai.save"), Disabled: !dirty || p.locked() || p.pluginInvalid, Busy: p.saving, Run: p.save},
		core.Action{Disabled: !dirty || p.locked(), Run: p.load},
	)
}

// anyPreset reports a draft provider made from a preset, whose regions
// and endpoints come from the catalog.
func (p *page) anyPreset() bool {
	if p.draft == nil {
		return false
	}
	for _, d := range p.draft.Value.Providers {
		if d.Value.Preset != "" {
			return true
		}
	}
	return false
}

// pickers converts providers for the model pickers.
func pickers(docs []providerDoc) []widgets.AIProvider {
	out := make([]widgets.AIProvider, len(docs))
	for i := range docs {
		out[i] = docs[i].Value.picker()
	}
	return out
}

// overview lists the providers with enabled models, the draft's and the
// saved ones it removed; a provider opens its models.
func (p *page) overview(c *ui.Context) {
	var all []*provider
	removed := map[string]bool{}
	switch {
	case p.draft != nil:
		for i := range p.draft.Value.Providers {
			all = append(all, &p.draft.Value.Providers[i].Value)
		}
		if p.saved != nil {
			for i := range p.saved.Value.Providers {
				pr := &p.saved.Value.Providers[i].Value
				if p.draft.Value.find(pr.ID) == nil {
					all = append(all, pr)
					removed[pr.ID] = true
				}
			}
		}
	case p.saved != nil:
		for i := range p.saved.Value.Providers {
			all = append(all, &p.saved.Value.Providers[i].Value)
		}
	}
	var enabled []*provider
	models := 0
	for _, pr := range all {
		if len(pr.Models) > 0 {
			enabled = append(enabled, pr)
			models += len(pr.Models)
		}
	}
	t, palette := c.Theme(), kit.Colors(c)
	ui.Column(c).FillWidth().Gap(10).Role(ui.RoleGroup).Label(i18n.T("ai.providerOverview")).Children(func() {
		ui.Row(c).FillWidth().Gap(8).Wrap().AlignItems(ui.Baseline).Children(func() {
			kit.Heading(c, i18n.T("ai.providerOverview"))
			kit.Caption(c, i18n.T("ai.providerTotal", "count", len(enabled), "models", models))
		})
		if len(enabled) == 0 {
			kit.Help(c, i18n.T("ai.providerOverviewEmpty"))
			return
		}
		ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
			for _, pr := range enabled {
				name := pr.Name
				if name == "" {
					name = i18n.T("ai.unnamed")
				}
				id := pr.ID
				b := ui.ButtonBase(c.Key(id)).Padding(6, 10).Gap(8).Radius(t.Radius).Border(1, palette.Hairline).
					Background(palette.Panel).AlignItems(ui.Center).MaxWidth(280).
					Label(i18n.T("ai.providerCount", "name", name, "count", len(pr.Models))).Tooltip(name).
					Disabled(p.locked() || removed[id])
				if b.Hovered() {
					b.Background(t.SurfaceHover)
				}
				if b.Clicked() {
					p.openProvider(id)
				}
				b.Children(func() {
					widgets.ProviderIcon(c, p.env, pr.mark(), 18)
					ui.Text(c, name).SingleLine().Shrink(1).MinWidth(0)
					kit.Caption(c, i18n.T("ai.overviewModelCount", "count", len(pr.Models))).Shrink(0)
					ui.Icon(c, icons.ChevronRight).FontSize(14).TextColor(t.TextMuted).Shrink(0)
				})
			}
		})
	})
}

// defaultsView is the default service and how results behave.
func (p *page) defaultsView(c *ui.Context) {
	if p.draft == nil {
		return
	}
	d := &p.draft.Value
	locked := p.locked()
	kit.Section(c, icons.LayoutGrid, i18n.T("ai.defaultService"), i18n.T("ai.defaultServiceHint"), func() {
		current, has := d.Tasks["default"]
		var offered []widgets.AIProvider
		for i := range d.Providers {
			pr := &d.Providers[i].Value
			if len(pr.Models) > 0 || (has && pr.ID == current.Provider) {
				offered = append(offered, pr.picker())
			}
		}
		var selection *widgets.AISelection
		if has {
			selection = &current
		}
		kit.Panel(c, func() {
			kit.Stack(c, "", "", func() {
				next, changed := widgets.AIModelSelect(c, widgets.AIModelSelectOptions{Env: p.env, Providers: offered,
					Value: selection, Label: i18n.T("ai.defaultService"), Disabled: locked})
				if changed {
					if next == nil {
						delete(d.Tasks, "default")
					} else {
						d.Tasks["default"] = *next
					}
				}
			})
		})
	})
	kit.Group(c, icons.MessagesSquare, i18n.T("settingsDesign.responseBehavior"), "", func() {
		retranslate := d.AutoRetranslate == nil || *d.AutoRetranslate
		if kit.Toggle(c, &retranslate, i18n.T("ai.autoRetranslate"), "").Disabled(locked).Changed() {
			d.AutoRetranslate = ptr(retranslate)
		}
		reasoning := d.ShowReasoning == nil || *d.ShowReasoning
		if kit.Toggle(c, &reasoning, i18n.T("ai.showReasoning"), "").Disabled(locked).Changed() {
			d.ShowReasoning = ptr(reasoning)
		}
	})
}
