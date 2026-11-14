// Package aiservices manages provider connections, result preferences and the local gateway.
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
	tabGateway
	tabHome
)

// The provider editor's tabs, for connections of the custom kind.
const (
	providerConnection = iota
	providerAdvanced
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
	connectionBusy  bool
	read            int
	err             string

	providerTab      int
	preferenceFields map[string]*preferenceField
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

	editor  *editor
	gateway *gatewayPanel
}

// curlNotice reports what a pasted cURL example filled in.
type curlNotice struct {
	provider string
	messages []string
}

func newPage(env *core.Env) *page {
	p := &page{env: env, tab: tabHome, selectedBy: map[int]string{}, keys: keyEdits{}, reader: readerFor(env)}
	p.gateway = newGatewayPanel(env)
	p.load()
	return p
}

// navigationLocked holds the tabs while work runs that leaving would lose
// or interrupt: a save, a sign-in, a model discovery or a picture upload.
func (p *page) navigationLocked() bool {
	return p.saving || p.accountBusy || p.modelBusy || p.avatarBusy || p.connectionBusy
}

// locked also holds the forms while the configuration is read.
func (p *page) locked() bool { return p.navigationLocked() || p.loading }

func (p *page) dirty() bool {
	return p.draft != nil && (!equal(p.draft, p.saved) || len(p.keys) > 0 || p.preferencesInvalid())
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
	p.preferenceFields = nil
	p.savedGen++
	p.gateway.releaseRuntime()
}

func (p *page) save() {
	if p.draft == nil || p.locked() || p.preferencesInvalid() {
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
	p.env.Activity.Report("ai-settings", false, false, nil)
}

func (p *page) View(c *ui.Context) {
	p.env.Activity.Report("ai-settings", p.dirty(), p.navigationLocked(), p.load)
	if (p.tab == tabCustom || p.tab == tabAccounts || p.anyPreset()) && !p.catalog.loaded && !p.catalog.busy && p.catalog.err == "" {
		p.loadCatalog()
	}
	p.rail()
	// Use the same column and shell scroller as other settings pages. An
	// independent workspace scroller would bypass their shared width and title.
	kit.Page(c, func() {
		p.upstreamNavigation(c)
		ui.Column(c).FillWidth().Gap(18).Disabled(p.saving).Children(func() {
			if p.tab == tabHome || p.tab == tabCustom || p.tab == tabAccounts {
				p.overview(c)
			}
			p.notices(c)
			switch p.tab {
			case tabHome:
				p.homeView(c)
			case tabCustom, tabAccounts:
				p.providersView(c)
			case tabGateway:
				var providers []widgets.AIProvider
				if p.saved != nil {
					providers = pickers(p.saved.Value.Providers)
				}
				p.gateway.View(c, providers, p)

			}
		})
	})
	if p.tab != tabGateway {
		p.gateway.Report()
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
	if p.draft == nil && p.err == "" && p.tab != tabGateway {
		ui.Column(c).FillWidth().Gap(16).Children(func() {
			kit.Status(c, i18n.T("capture.loading"))
			kit.Skeleton(c, 4)
		})
	}
}

// rail shows Save and Undo for the configuration. The gateway saves its
// own settings and has its own save action.
func (p *page) rail() {
	if p.draft == nil || p.tab == tabGateway {
		return
	}
	dirty := p.dirty()
	p.env.Rail.Set(
		core.Action{Label: i18n.T("ai.save"), Disabled: !dirty || p.locked() || p.preferencesInvalid(), Busy: p.saving, Run: p.save},
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
		if len(pr.Models) > 0 && !pr.Off {
			enabled = append(enabled, pr)
			models += len(pr.Models)
		}
	}
	t := c.Theme()
	// One shared surface and inline items keep this a compact locator. The
	// layout follows shadcn/ui Item's media/content/action composition; all
	// controls, marks and spacing use WinOne's existing native components.
	kit.Panel(c, func() {
		ui.Row(c).FillWidth().Padding(6, 12).GapX(12).GapY(4).Wrap().AlignItems(ui.Center).Children(func() {
			kit.Caption(c, i18n.T("ai.providerTotal", "count", len(enabled), "models", models)).SingleLine().Margin(4, 0)
			if len(enabled) == 0 {
				kit.Help(c, i18n.T("ai.providerOverviewEmpty"))
				return
			}
			for _, pr := range enabled {
				name := pr.Name
				if name == "" {
					name = i18n.T("ai.unnamed")
				}
				id := pr.ID
				b := ui.ButtonBase(c.Key(id)).MinHeight(28).Padding(4, 6).Gap(6).Radius(t.Radius).
					AlignItems(ui.Center).MaxWidth(280).
					Label(i18n.T("ai.providerCount", "name", name, "count", len(pr.Models))).Description(name).
					Disabled(p.locked() || removed[id])
				if b.Pressed() {
					b.Background(t.SurfacePressed)
				} else if b.Hovered() {
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
	}).Role(ui.RoleGroup).Label(i18n.T("ai.providerOverview"))
}

// Home and provider editors share the same configuration draft and save rail.
func (p *page) responseBehaviorView(c *ui.Context) {
	d := &p.draft.Value
	locked := p.locked()
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

func (p *page) defaultModelView(c *ui.Context) {
	if p.draft == nil {
		return
	}
	d := &p.draft.Value
	locked := p.locked()
	// The tab already names this group; its purpose leads it instead of a
	// heading that repeats the tab.
	ui.Column(c).FillWidth().Gap(kit.HeadingGap).Children(func() {
		kit.Help(c, i18n.T("ai.defaultServiceHint"))
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
}
