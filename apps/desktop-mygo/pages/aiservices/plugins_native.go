package aiservices

import (
	"encoding/json"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/web"
)

type managedPlugin struct {
	BundleDirectory string         `json:"-"`
	Spec            string         `json:"spec"`
	Off             bool           `json:"off"`
	Options         map[string]any `json:"options"`
	OptionsExample  map[string]any `json:"optionsExample"`
	IsGateway       bool           `json:"isGateway"`
	IsMiddleware    bool           `json:"isMiddleware"`
	Error           string         `json:"error"`
	Providers       []string       `json:"providers"`
	Version         string         `json:"version"`
	Latest          string         `json:"latest"`
	OwnName         string         `json:"ownName"`
	Package         string         `json:"package"`
	Commit          string         `json:"commit"`
	Icon            string         `json:"icon"`
	ToolsServer     *struct {
		On     bool   `json:"on"`
		Plugin string `json:"plugin"`
	} `json:"toolsServer"`
	Head       string          `json:"head"`
	Moved      []string        `json:"moved"`
	Serves     []string        `json:"serves"`
	Page       json.RawMessage `json:"page"`
	Panels     json.RawMessage `json:"panels"`
	PanelReads []string        `json:"panelReads"`
	Actions    json.RawMessage `json:"actions"`
	Tools      []struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		MCP         string `json:"mcp"`
	} `json:"tools"`
	Clashes []struct {
		ID       string `json:"id"`
		Provider string `json:"provider"`
		Name     string `json:"name"`
		Spec     string `json:"spec"`
	} `json:"clashes"`
	Raw json.RawMessage `json:"-"`
}

func (p *managedPlugin) UnmarshalJSON(data []byte) error {
	type plain managedPlugin
	if err := json.Unmarshal(data, (*plain)(p)); err != nil {
		return err
	}
	p.Raw = append([]byte(nil), data...)
	return nil
}

type managedPlugins struct {
	Plugins    []managedPlugin `json:"plugins"`
	Bun        bool            `json:"bun"`
	BunVersion string          `json:"bunVersion"`
	Error      string          `json:"error"`
	Mirror     bool            `json:"mirror"`
	Registry   struct {
		URL  string `json:"url"`
		From string `json:"from"`
		Path string `json:"path"`
		Var  string `json:"var"`
	} `json:"registry"`
}
type marketPlugin struct {
	Kind        string            `json:"kind"`
	Icon        string            `json:"icon"`
	Community   bool              `json:"community"`
	Package     string            `json:"package"`
	Spec        string            `json:"spec"`
	Name        string            `json:"name"`
	Summary     map[string]string `json:"summary"`
	Description string            `json:"description"`
	Version     string            `json:"version"`
	Repo        string            `json:"repo"`
	Owner       string            `json:"owner"`
	License     string            `json:"license"`
	Stars       int               `json:"stars"`
	URL         string            `json:"url"`
	NPM         *pluginNPM        `json:"npm,omitempty"`
	Raw         json.RawMessage   `json:"-"`
}

func (p *marketPlugin) UnmarshalJSON(data []byte) error {
	type plain marketPlugin
	if err := json.Unmarshal(data, (*plain)(p)); err != nil {
		return err
	}
	p.Raw = append([]byte(nil), data...)
	return nil
}

// Only an author's own web page remains a WebView. The registry, discovery,
// install/update controls, permissions and options editor are native MyGo.
type pluginsPanel struct {
	githubField                                            syncedText
	p                                                      *page
	api, marketAPI, readmeAPI, npmAPI                      nativeAPI
	state                                                  *managedPlugins
	source                                                 string
	tab                                                    int
	query, spec                                            string
	market                                                 []marketPlugin
	marketLoaded                                           bool
	request                                                *pageRequest
	guest                                                  *web.Surface
	shown                                                  string
	optionsSpec, optionsText, optionsSaved, optionsExample string
	remove                                                 string
	switchOff                                              string
	readme, readmeTitle                                    string
	readmeSpec                                             string
	readmeMeta                                             *marketPlugin
	details                                                map[string]bool
	registryOpen, descending                               bool
	queryShown                                             string
	searchDue                                              time.Time
	clearOptions                                           bool
	searchAPI, githubAPI                                   nativeAPI
	hits, github                                           []marketPlugin
	githubLoaded                                           bool
	githubKind, githubHave, githubQuery                    string
	githubMissing                                          int
	checks                                                 map[string]pluginVersionCheck
}

func newPluginsPanel(p *page) *pluginsPanel {
	return &pluginsPanel{p: p, tab: 1, details: map[string]bool{}}
}
func (x *pluginsPanel) Close() { x.Leave() }
func (x *pluginsPanel) Leave() {
	x.api.close()
	x.marketAPI.close()
	x.readmeAPI.close()
	x.npmAPI.close()
	x.searchAPI.close()
	x.githubAPI.close()
	x.source = ""
	if x.guest != nil {
		x.guest.Close()
		x.guest = nil
	}
	x.shown = ""
	x.p.env.Activity.Report("ai-plugins", false, false, nil)
}
func (x *pluginsPanel) dirty() bool { return x.optionsSpec != "" && x.optionsText != x.optionsSaved }
func (x *pluginsPanel) discard()    { x.optionsSpec, x.optionsText, x.optionsSaved = "", "", "" }
func (x *pluginsPanel) load() {
	x.api.request(x.p, "GET", "/api/plugins", nil, func(raw json.RawMessage) {
		var state managedPlugins
		if decodeManagedPlugins(&x.api, raw, &state) {
			x.state = &state
		}
	})
}
func (x *pluginsPanel) changed() {
	x.p.extensions.Reload()
	x.p.reader.InvalidateCatalog()
	x.p.reader.Invalidate()
	x.p.catalog.loaded = false
	if x.p.env.Toast != nil {
		x.p.env.Toast(nativeT("pluginApplied"))
	}
}
func (x *pluginsPanel) act(action string, body any) {
	if x.dirty() && action != "options" && x.p.env.RequestChange != nil {
		x.p.env.Activity.Report("ai-plugins", true, x.api.busy, x.discard)
		x.p.env.RequestChange(func() { x.discard(); x.act(action, body) })
		return
	}
	if x.bundleAction(action, body) {
		return
	}
	x.api.request(x.p, "POST", "/api/plugins/"+action, body, func(raw json.RawMessage) {
		var state managedPlugins
		if decodeManagedPlugins(&x.api, raw, &state) {
			x.state = &state
			x.discard()
			x.remove = ""
			x.changed()
		}
	})
}
func (x *pluginsPanel) View(c *ui.Context) {
	p := x.p
	if p.advanced == nil {
		p.advanced = newAdvancedPanel(p)
	}
	if p.advanced.catalog == nil && !p.advanced.loading && p.advanced.err == "" {
		p.advanced.load()
	}
	p.env.Activity.Report("ai-plugins", x.dirty(), x.api.busy, x.discard)
	if p.extensions.state == nil {
		kit.Status(c, i18n.T("capture.loading"))
		return
	}
	if x.request != nil {
		if kit.Button(c, nativeT("backToPlugins"), icons.ArrowLeft, false).Clicked() {
			x.request = nil
			if x.guest != nil {
				x.guest.Close()
				x.guest = nil
			}
			return
		}
		if x.guest == nil || x.shown != x.request.nonce {
			if x.guest != nil {
				x.guest.Close()
			}
			x.shown = x.request.nonce
			x.guest = web.New(p.env, web.Options{URL: pageURL(p.extensions.state.URL, x.request), Label: i18n.T("aiPlugins.panels"), Theme: "winone-plugin-theme", Accept: []string{"winone-plugin-height", "winone-plugin-changed", "winone-plugin-native", "winone-plugin-status"}, HeightType: "winone-plugin-height", Height: 600, Receive: x.receiveGuest})
		}
		x.guest.View(c, p.locked())
		return
	}
	if x.source != p.extensions.state.URL {
		x.api.close()
		x.marketAPI.close()
		x.readmeAPI.close()
		x.npmAPI.close()
		x.source = p.extensions.state.URL
		x.load()
	}
	x.api.notice(c, x.load)
	if p.external != nil {
		p.env.Activity.Report("ai-external", p.external.dirty(), p.external.busy(), p.external.discard)
		if p.external.err != "" {
			kit.Notice(c, kit.Failure, "", p.external.err, nil)
		}
		if p.external.library != nil {
			externalDetails(c, p.external.library["problems"])
		}
	}
	x.headerView(c)
	if x.state == nil {
		if x.api.busy {
			kit.Status(c, i18n.T("aiPlugins.loading"))
		}
		return
	}
	locked := p.locked() || x.api.busy || p.external != nil && p.external.busy()
	if x.state.Error != "" {
		kit.Notice(c, kit.Failure, "", x.state.Error, nil)
	}
	switch x.tab {
	case 0:
		x.builtinView(c, locked)
	case 1:
		x.marketView(c, locked)
	case 2:
		x.installedView(c, locked)
	}
	if x.remove != "" {
		open := true
		if kit.Dialog(c, &open, kit.DialogOptions{Title: nativeT("removePlugin"), Message: x.remove + "\n" + nativeT("removePluginHint"), Confirm: nativeT("remove"), Cancel: i18n.T("common.cancel"), Danger: true, Busy: x.api.busy, Error: x.api.err}) {
			x.act("remove", map[string]string{"spec": x.remove})
		}
		if !open {
			x.remove = ""
		}
	}
	if kit.Dialog(c, &x.clearOptions, kit.DialogOptions{Title: upstreamT("Clear"), Message: upstreamT("Saved options will be removed. The plugin will run with no options."), Confirm: upstreamT("Clear"), Cancel: i18n.T("common.cancel"), Danger: true, Busy: x.api.busy}) {
		x.act("options", map[string]any{"spec": x.optionsSpec, "options": nil})
		x.clearOptions = false
	}
	if x.readmeTitle != "" {
		open := true
		kit.Dialog(c, &open, kit.DialogOptions{Title: x.readmeTitle, Cancel: i18n.T("common.close"), Width: 820, Body: func() {
			if x.readmeMeta != nil {
				x.marketInfoView(c, *x.readmeMeta)
			}
			x.readmeAPI.notice(c, nil)
			if x.readmeAPI.busy {
				kit.Status(c, i18n.T("capture.loading"))
			} else {
				markdown.View(c, x.readme, &markdown.Options{}).FillWidth()
			}
		}, Actions: func() {
			if x.readmeSpec != "" && kit.PrimaryButton(c, nativeT("install"), icons.Download, false).Disabled(x.api.busy || x.installed(x.readmeSpec)).Clicked() {
				x.act("add", map[string]string{"spec": x.readmeSpec})
			}
		}})
		if !open {
			x.readmeTitle = ""
			x.readmeAPI.close()
		}
	}
}
func (x *pluginsPanel) installedView(c *ui.Context, locked bool) {
	plugins := x.effectivePlugins()
	if len(plugins) == 0 {
		kit.Empty(c, icons.Blocks, upstreamT("No plugins yet"), upstreamT("Install a subscription or a middleware from Discover; a subscription signs in from here."), func() {
			if kit.Button(c, upstreamT("Discover plugins"), icons.Search, false).Clicked() {
				x.tab = 1
			}
		})
		return
	}
	rows := slices.Clone(plugins)
	slices.SortFunc(rows, func(a, b managedPlugin) int {
		n := strings.Compare(strings.ToLower(pluginDisplayName(a)), strings.ToLower(pluginDisplayName(b)))
		if x.descending {
			return -n
		}
		return n
	})
	updates := 0
	for _, p := range rows {
		if pluginHasUpdate(p) {
			updates++
		}
	}
	if updates > 0 {
		ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
			kit.Help(c, upstreamT("Newer versions of {n} plugins are out: {names}", "n", updates, "names", x.updateNames()))
			if kit.Button(c, upstreamT("Update all ({n})", "n", updates), icons.Download, false).Disabled(locked).Clicked() {
				x.act("update", nil)
			}
		})
	}
	ui.Row(c).FillWidth().Children(func() {
		ui.Spacer(c).Grow(1)
		if kit.GhostButton(c, upstreamT("Name")+" ↑↓", nil, false).Clicked() {
			x.descending = !x.descending
		}
	})
	for _, plugin := range rows {
		if x.query != "" && !strings.Contains(strings.ToLower(pluginDisplayName(plugin)+" "+plugin.Spec), strings.ToLower(x.query)) {
			continue
		}
		x.installedRow(c.Key(plugin.Spec), plugin, locked)
	}
	ui.Row(c).FillWidth().Gap(8).Wrap().AlignItems(ui.Center).Children(func() {
		kit.Help(c, upstreamT("Plugins run on Bun {v}", "v", x.state.BunVersion))
		ui.Spacer(c).Grow(1)
		if kit.GhostButton(c, nativeT("checkUpdates"), icons.RefreshCw, x.api.busy).Disabled(locked).Clicked() {
			x.checkUpdates()
		}
	})
	// Author pages without an npm owner still retain an entry, as before.
	if x.p.extensions.state != nil {
		for _, page := range x.p.extensions.state.Pages {
			if !slices.ContainsFunc(rows, func(p managedPlugin) bool { return p.Spec == page.Spec }) {
				if kit.Button(c, page.title(), icons.PanelRightOpen, false).Disabled(locked).Clicked() {
					x.request = &pageRequest{key: page.Key, nonce: nonce()}
				}
			}
		}
	}
}
func (x *pluginsPanel) optionsView(c *ui.Context, locked bool) {
	ui.Column(c).FillWidth().Gap(12).Padding(12, 0).Children(func() { x.optionsContent(c, locked) })
}

func (x *pluginsPanel) optionsContent(c *ui.Context, locked bool) {
	ui.TextArea(c, &x.optionsText).FillWidth().MinHeight(180).Font(kit.MonoFont).Label(nativeT("pluginOptions")).Disabled(locked)
	var options map[string]any
	valid := json.Unmarshal([]byte(x.optionsText), &options) == nil && options != nil
	x.p.env.Rail.Set(
		core.Action{Label: i18n.T("common.save"), Disabled: locked || !valid || !x.dirty(), Busy: x.api.busy, Run: func() { x.act("options", map[string]any{"spec": x.optionsSpec, "options": options}) }},
		core.Action{Disabled: locked || !x.dirty(), Run: x.discard},
	)
	if !valid {
		kit.InlineError(c, nativeT("optionsObject"))
	}
	ui.Row(c).Gap(8).Children(func() {
		if kit.GhostButton(c, upstreamT("Clear"), nil, false).Disabled(locked).Clicked() {
			x.clearOptions = true
		}
		if x.optionsExample != "" && x.optionsExample != "null" && kit.Button(c, nativeT("useOptionsExample"), icons.FileText, false).Disabled(locked).Clicked() {
			x.optionsText = x.optionsExample
		}
		if kit.PrimaryButton(c, i18n.T("common.save"), icons.Save, false).Disabled(locked || !valid || !x.dirty()).Clicked() {
			x.act("options", map[string]any{"spec": x.optionsSpec, "options": options})
		}
		if kit.Button(c, i18n.T("common.cancel"), nil, false).Disabled(locked).Clicked() {
			if x.dirty() && x.p.env.RequestChange != nil {
				x.p.env.RequestChange(x.discard)
			} else {
				x.discard()
			}
		}
	})
}
func (x *pluginsPanel) showReadme(spec string) {
	x.readmeTitle = spec
	x.readmeSpec = spec
	x.readmeMeta = nil
	for _, entry := range append(append(slices.Clone(x.market), x.github...), x.hits...) {
		if pluginSpec(entry) == spec {
			copy := entry
			x.readmeMeta = &copy
			break
		}
	}
	x.readme = ""
	x.readmeAPI.request(x.p, "GET", "/api/plugins/page?name="+url.QueryEscape(spec), nil, func(raw json.RawMessage) {
		var page struct {
			Readme string     `json:"readme"`
			NPM    *pluginNPM `json:"npm"`
		}
		if nativeDecode(&x.readmeAPI, raw, &page) {
			x.readme = page.Readme
			if page.NPM != nil {
				if x.readmeMeta == nil {
					x.readmeMeta = &marketPlugin{Spec: spec}
				}
				x.readmeMeta.NPM = page.NPM
			}
		}
	})
}
func (x *pluginsPanel) marketLoad(kind string) {
	endpoint := "/api/plugins/listings"
	if kind == "search" {
		endpoint = "/api/plugins/search?q=" + url.QueryEscape(x.query)
	}
	if kind == "github" {
		endpoint = "/api/plugins/github?again=1"
	}
	x.marketAPI.request(x.p, "GET", endpoint, nil, func(raw json.RawMessage) {
		var value struct {
			Listings, Hits, Repos []marketPlugin
			Error                 string `json:"error"`
		}
		if nativeDecode(&x.marketAPI, raw, &value) {
			x.market = append(append(value.Listings, value.Hits...), value.Repos...)
			x.marketLoaded = true
			if kind == "listings" {
				x.loadNPMInfo()
			}
			if value.Error != "" {
				x.marketAPI.err = value.Error
			}
		}
	})
}
func (x *pluginsPanel) marketView(c *ui.Context, locked bool) {
	if !x.marketLoaded && !x.marketAPI.busy && x.marketAPI.err == "" {
		x.marketLoad("listings")
	}
	x.marketAPI.notice(c, func() { x.marketLoad("listings") })
	x.npmAPI.notice(c, x.loadNPMInfo)
	if x.query == "" {
		kit.Panel(c, func() {
			ui.Column(c).FillWidth().Gap(6).Padding(16, 18).Children(func() {
				kit.Strong(c, upstreamT("Plugins extend magpie"))
				kit.Help(c, upstreamT("Plugins sign in to coding plans and make their requests; the models then work in every agent, like any provider's. They're OpenCode's provider plugins or pi's packages, run on Bun."))
				kit.Help(c, upstreamT("A plugin is someone else's code with your sign-in or your requests: install the ones you trust."))
				ui.Link(c, upstreamT("Write a plugin"), "https://usemagpie.ai/docs/plugins")
			})
		})
	}
	groups := []struct{ kind, title string }{{"provider", "Subscriptions"}, {"middleware", "Gateway middleware"}, {"agent", "Agents"}, {"tools", "Tools for agents"}, {"page", "Pages"}}
	for _, group := range groups {
		var rows []marketPlugin
		for _, entry := range x.market {
			kind := entry.Kind
			if kind == "" {
				kind = "provider"
			}
			if kind == "hooks" {
				kind = "middleware"
			}
			if kind == group.kind && pluginMatches(entry, x.query) {
				rows = append(rows, entry)
			}
		}
		x.marketSection(c, upstreamT(group.title), rows, locked)
	}
	if x.query != "" {
		x.searchAPI.notice(c, nil)
		x.marketSection(c, upstreamT("On npm"), x.hits, locked)
	}
	x.githubView(c, locked)
	if x.query == "" {
		x.manualView(c, locked)
		ui.Collapsible(c, upstreamT("npm registry"), &x.registryOpen, func() { x.registryView(c, locked) })
	}
}

// Kept as a callable view for existing layout probes; production shows these
// controls at the end of Discover, as upstream does.
func (x *pluginsPanel) sourcesView(c *ui.Context, locked bool) {
	x.manualView(c, locked)
	x.registryView(c, locked)
}
func (x *pluginsPanel) registryView(c *ui.Context, locked bool) {
	ui.Column(c).FillWidth().Gap(16).Padding(12, 0).Children(func() { x.registryContent(c, locked) })
}

func (x *pluginsPanel) registryContent(c *ui.Context, locked bool) {
	if x.p.draft == nil || x.state == nil {
		return
	}

	s := x.p.draft.Value.settings()
	kit.Toggle(c, &s.ChinaMirror, nativeT("mirror"), nativeT("mirrorHint")).Disabled(locked)
	nativeText(c, nativeT("npmRegistry"), &s.NPMRegistry, locked)
	kit.Stack(c, completeT("githubToken"), completeT("githubTokenHint"), func() {
		f := &x.githubField
		current := ""
		if s.GitHubToken != nil {
			current = *s.GitHubToken
		}
		if text, changed := f.sync(current); changed {
			s.GitHubToken = &text
		}
		placeholder := i18n.T("ai.keyPlaceholder")
		if s.HasGitHubToken {
			placeholder = i18n.T("ai.keySaved")
		}
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			ui.TextInput(c, &f.text).Grow(1).MinWidth(0).Password().Label(completeT("githubToken")).Placeholder(placeholder).Disabled(locked)
			if kit.IconButton(c, icons.Trash2, completeT("clearGitHubToken")).Disabled(locked).Clicked() {
				s.GitHubToken = new("")
				f.text, f.shown = "", ""
			}
		})
	})
	kit.Help(c, x.state.Registry.URL+" · "+x.state.Registry.From+" · "+x.state.Registry.Path)
	if kit.Button(c, nativeT("saveRegistry"), icons.Save, false).Disabled(locked || !x.p.dirty()).Clicked() {
		x.p.save()
	}
	kit.Help(c, "Bun "+x.state.BunVersion)
}
