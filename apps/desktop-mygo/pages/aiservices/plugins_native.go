package aiservices

import (
	"encoding/json"
	"net/url"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/web"
	"winone/desktopmygo/widgets"
)

type managedPlugin struct {
	Spec           string          `json:"spec"`
	Off            bool            `json:"off"`
	Options        map[string]any  `json:"options"`
	OptionsExample map[string]any  `json:"optionsExample"`
	Error          string          `json:"error"`
	Providers      []string        `json:"providers"`
	Version        string          `json:"version"`
	Latest         string          `json:"latest"`
	OwnName        string          `json:"ownName"`
	Package        string          `json:"package"`
	Commit         string          `json:"commit"`
	Head           string          `json:"head"`
	Moved          []string        `json:"moved"`
	Serves         []string        `json:"serves"`
	Page           json.RawMessage `json:"page"`
	Panels         json.RawMessage `json:"panels"`
	PanelReads     []string        `json:"panelReads"`
	Actions        json.RawMessage `json:"actions"`
	Tools          []struct {
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
	Package     string            `json:"package"`
	Spec        string            `json:"spec"`
	Name        string            `json:"name"`
	Summary     map[string]string `json:"summary"`
	Description string            `json:"description"`
	Version     string            `json:"version"`
	Repo        string            `json:"repo"`
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
	readme, readmeTitle                                    string
	details                                                map[string]bool
}

func newPluginsPanel(p *page) *pluginsPanel { return &pluginsPanel{p: p, details: map[string]bool{}} }
func (x *pluginsPanel) Close()              { x.Leave() }
func (x *pluginsPanel) Leave() {
	x.api.close()
	x.marketAPI.close()
	x.readmeAPI.close()
	x.npmAPI.close()
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
	kit.SubTabs(c, &x.tab, []string{nativeT("installed"), nativeT("discover"), nativeT("pluginSources")}, p.env.RequestChange)
	if x.state == nil {
		if x.api.busy {
			kit.Status(c, i18n.T("aiPlugins.loading"))
		}
		return
	}
	locked := p.locked() || x.api.busy
	if x.state.Error != "" {
		kit.Notice(c, kit.Failure, "", x.state.Error, nil)
	}
	switch x.tab {
	case 0:
		x.installedView(c, locked)
	case 1:
		x.marketView(c, locked)
	case 2:
		x.sourcesView(c, locked)
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
	if x.readmeTitle != "" {
		open := true
		kit.Dialog(c, &open, kit.DialogOptions{Title: x.readmeTitle, Cancel: i18n.T("common.close"), Width: 820, Body: func() {
			x.readmeAPI.notice(c, nil)
			if x.readmeAPI.busy {
				kit.Status(c, i18n.T("capture.loading"))
			} else {
				markdown.View(c, x.readme, &markdown.Options{}).FillWidth()
			}
		}})
		if !open {
			x.readmeTitle = ""
			x.readmeAPI.close()
		}
	}
}
func (x *pluginsPanel) installedView(c *ui.Context, locked bool) {
	ui.Row(c).Gap(8).Wrap().Children(func() {
		if kit.Button(c, nativeT("checkUpdates"), icons.RefreshCw, false).Disabled(locked).Clicked() {
			x.checkUpdates()
		}
		if kit.Button(c, nativeT("updateAll"), icons.Download, false).Disabled(locked).Clicked() {
			x.act("update", nil)
		}
		if kit.GhostButton(c, i18n.T("common.refresh"), icons.RefreshCw, false).Disabled(locked).Clicked() {
			x.load()
		}
	})
	if len(x.state.Plugins) == 0 {
		kit.Empty(c, icons.Blocks, nativeT("noInstalledPlugins"), nativeT("pluginInstallHint"), nil)
	}
	for _, plugin := range x.state.Plugins {
		name := plugin.OwnName
		if name == "" {
			name = plugin.Spec
		}
		ui.Column(c.Key(plugin.Spec)).FillWidth().Gap(8).Padding(12, 0).BorderWidth(0, 0, 1, 0).BorderColor(kit.Colors(c).Hairline).Children(func() {
			ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Children(func() { kit.Strong(c, name); kit.Help(c, plugin.Spec+" · "+plugin.Version).Selectable() })
				on := !plugin.Off
				if ui.Switch(c, &on).Label(nativeT("enabled") + " " + name).Disabled(locked).Changed() {
					x.act("off", map[string]any{"spec": plugin.Spec, "off": !on})
				}
			})
			if plugin.Error != "" {
				kit.Notice(c, kit.Failure, "", plugin.Error, nil)
			}
			if plugin.Latest != "" && plugin.Latest != plugin.Version {
				kit.Help(c, nativeT("latestVersion")+": "+plugin.Latest)
			}
			if plugin.Commit != "" {
				kit.Mono(c, plugin.Commit+" → "+plugin.Head).Selectable()
			}
			ui.Row(c).Gap(8).Wrap().Children(func() {
				if kit.Button(c, nativeT("update"), icons.Download, false).Disabled(locked).Clicked() {
					x.act("upgrade", map[string]string{"spec": plugin.Spec})
				}
				if kit.Button(c, nativeT("pluginOptions"), icons.Settings2, false).Disabled(locked).Clicked() {
					change := func() {
						x.optionsSpec = plugin.Spec
						value := plugin.Options
						if value == nil {
							value = map[string]any{}
						}
						b, _ := json.MarshalIndent(value, "", "  ")
						x.optionsText = string(b)
						x.optionsSaved = x.optionsText
						example, _ := json.MarshalIndent(plugin.OptionsExample, "", "  ")
						x.optionsExample = string(example)
					}
					if x.dirty() && x.p.env.RequestChange != nil {
						x.p.env.RequestChange(change)
					} else {
						change()
					}
				}
				if kit.Button(c, "README", icons.FileText, false).Disabled(x.readmeAPI.busy).Clicked() {
					x.showReadme(plugin.Spec)
				}
				if kit.GhostButton(c, nativeT("remove"), icons.Trash2, false).Disabled(locked).Clicked() {
					x.remove = plugin.Spec
				}
			})
			// Account actions retain WinOne's private provider and sign-in flow.
			for _, a := range x.p.catalog.agents {
				if a.Plugin != plugin.Spec {
					continue
				}
				if kit.Button(c, nativeT("connect")+" "+a.Name, icons.LogIn, false).Disabled(locked).Clicked() {
					if err := x.p.openPluginProvider(a, true); err != nil {
						x.api.err = err.Error()
					}
				}
			}
			for _, clash := range plugin.Clashes {
				id := clash.Provider
				if id == "" {
					id = clash.ID
				}
				if kit.Button(c, nativeT("preferPlugin")+" "+clash.Name, icons.Check, false).Disabled(locked).Clicked() {
					x.act("prefer", map[string]string{"spec": plugin.Spec, "provider": id})
				}
			}
			if x.p.extensions.state != nil {
				for _, page := range x.p.extensions.state.Pages {
					if page.Spec == plugin.Spec {
						page := page
						if kit.Button(c, page.title(), icons.PanelRightOpen, false).Disabled(locked).Clicked() {
							x.request = &pageRequest{key: page.Key, nonce: nonce()}
						}
					}
				}
			}
			open := x.details[plugin.Spec]
			ui.Collapsible(c, nativeT("capabilitiesPermissions"), &open, func() {
				for _, tool := range plugin.Tools {
					kit.Strong(c, tool.Name+" · "+tool.MCP)
					kit.Help(c, tool.Description)
				}
				// Keep every declared permission, middleware/gateway/agent state,
				// option example and automatic-update receipt from the real runtime.
				x.pluginDetailsView(c, plugin)
				nativeJSON(c, plugin.Raw)
			})
			x.details[plugin.Spec] = open
			if x.optionsSpec == plugin.Spec {
				x.optionsView(c, locked)
			}
		})
	}
	// A page key need not derive from its npm spec; list all declared pages
	// explicitly as well rather than guessing ownership and losing an entry.
	if x.p.extensions.state != nil {
		for _, page := range x.p.extensions.state.Pages {
			if slices.ContainsFunc(x.state.Plugins, func(plugin managedPlugin) bool { return plugin.Spec == page.Spec }) {
				continue
			}
			page := page
			if kit.Button(c, page.title(), icons.PanelRightOpen, false).Disabled(locked).Clicked() {
				x.request = &pageRequest{key: page.Key, nonce: nonce()}
			}
		}
	}
}
func (x *pluginsPanel) optionsView(c *ui.Context, locked bool) {
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
	x.readme = ""
	x.readmeAPI.request(x.p, "GET", "/api/plugins/page?name="+url.QueryEscape(spec), nil, func(raw json.RawMessage) {
		var page struct {
			Readme string `json:"readme"`
		}
		if nativeDecode(&x.readmeAPI, raw, &page) {
			x.readme = page.Readme
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
	ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
		ui.TextInput(c, &x.query).Grow(1).MinWidth(160).Label(nativeT("searchPlugins")).Placeholder(nativeT("searchPlugins"))
		if kit.Button(c, nativeT("search"), icons.Search, false).Disabled(x.marketAPI.busy || x.query == "").Clicked() {
			x.marketLoad("search")
		}
		if kit.Button(c, nativeT("recommended"), icons.Sparkles, false).Disabled(x.marketAPI.busy).Clicked() {
			x.marketLoad("listings")
		}
		if kit.Button(c, "GitHub", icons.GitBranch, false).Disabled(x.marketAPI.busy).Clicked() {
			x.marketLoad("github")
		}
	})
	kit.Notice(c, kit.Info, "", nativeT("pluginTrustHint"), nil)
	x.marketAPI.notice(c, func() { x.marketLoad("listings") })
	x.npmAPI.notice(c, x.loadNPMInfo)
	for _, entry := range x.market {
		spec := entry.Spec
		if spec == "" {
			spec = entry.Package
		}
		if spec == "" && entry.Repo != "" {
			spec = "github:" + entry.Repo
		}
		if spec == "" {
			spec = entry.URL
		}
		name := entry.Name
		if name == "" {
			name = spec
		}
		ui.Column(c.Key(spec)).FillWidth().Gap(6).Padding(12, 0).Children(func() {
			kit.Strong(c, name)
			lang := "en"
			if strings.HasPrefix(i18n.Locale(), "zh") {
				lang = "zh"
			}
			description := entry.Summary[lang]
			if description == "" {
				description = entry.Description
			}
			kit.Help(c, description)
			x.marketInfoView(c, entry)
			ui.Row(c).Gap(8).Children(func() {
				if kit.Button(c, nativeT("install"), icons.Download, false).Disabled(locked || spec == "").Clicked() {
					x.spec = spec
					x.tab = 2
				}
				if kit.Button(c, "README", icons.FileText, false).Disabled(x.readmeAPI.busy || spec == "").Clicked() {
					x.showReadme(spec)
				}
			})
		})
	}
}
func (x *pluginsPanel) sourcesView(c *ui.Context, locked bool) {
	if x.p.draft == nil {
		return
	}
	kit.Help(c, nativeT("pluginInstallHint"))
	field := ui.TextInput(c, &x.spec).FillWidth().Label(nativeT("pluginSource")).Placeholder("@scope/package · https://… · C:\\plugins\\example").Disabled(locked)
	widgets.DropArea(c, x.p.env, field, widgets.DropFolders, false, locked, func(paths []string, err error) {
		if err != nil {
			x.api.err = err.Error()
		} else if len(paths) > 0 {
			x.spec = paths[0]
		}
	})
	ui.Row(c).Gap(8).Wrap().Children(func() {
		if kit.PrimaryButton(c, nativeT("install"), icons.Download, x.api.busy).Disabled(locked || strings.TrimSpace(x.spec) == "").Clicked() {
			x.act("add", map[string]string{"spec": strings.TrimSpace(x.spec)})
		}
		if kit.Button(c, nativeT("chooseFolder"), icons.FolderOpen, false).Disabled(locked).Clicked() {
			core.Go(x.p.env, func() (string, error) { return x.p.env.Dialogs.PickFolder(nativeT("pluginSource")) }, func(path string, err error) {
				if err != nil {
					x.api.err = err.Error()
				} else if path != "" {
					x.spec = path
				}
			})
		}
	})
	s := x.p.draft.Value.settings()
	kit.Toggle(c, &s.ChinaMirror, nativeT("mirror"), nativeT("mirrorHint")).Disabled(locked)
	nativeText(c, nativeT("npmRegistry"), &s.NPMRegistry, locked)
	kit.Help(c, x.state.Registry.URL+" · "+x.state.Registry.From+" · "+x.state.Registry.Path)
	if kit.Button(c, nativeT("saveRegistry"), icons.Save, false).Disabled(locked || !x.p.dirty()).Clicked() {
		x.p.save()
	}
	kit.Help(c, "Bun "+x.state.BunVersion)
}
