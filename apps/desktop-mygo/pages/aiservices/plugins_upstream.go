package aiservices

import (
	"encoding/json"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/widgets"
)

// The upstream words, layout and actions are ported from plugins.js/plugins.css
// at the reviewed Magpie revision. Only the rendering and Host boundary differ.
func upstreamT(key string, args ...any) string {
	if i18n.Has("aiUpstream." + key) {
		return i18n.T("aiUpstream."+key, args...)
	}
	return strings.ReplaceAll(key, "magpie", "WinOne")
}

func pluginDisplayName(p managedPlugin) string {
	if p.OwnName != "" {
		return p.OwnName
	}
	if p.Package != "" {
		return p.Package
	}
	name := pluginPackage(p.Spec)
	if pluginPathPattern.MatchString(name) {
		parts := strings.Split(strings.TrimRight(strings.ReplaceAll(name, "\\", "/"), "/"), "/")
		return parts[len(parts)-1]
	}
	return name
}
func pluginHasUpdate(p managedPlugin) bool {
	if p.Commit != "" && p.Head != "" {
		return !p.Off && !strings.HasPrefix(p.Head, p.Commit) && !strings.HasPrefix(p.Commit, p.Head)
	}
	return pluginVersionNewer(p.Latest, p.Version)
}

var pluginVersionPattern = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)(?:-([^+]*))?(?:\+.*)?$`)

// Same ordering as plugins.js newer: prereleases sort before a final release;
// numeric prerelease identifiers sort numerically and before text identifiers.
func pluginVersionNewer(a, b string) bool {
	x, y := pluginVersionPattern.FindStringSubmatch(strings.TrimSpace(a)), pluginVersionPattern.FindStringSubmatch(strings.TrimSpace(b))
	if x == nil || y == nil {
		return false
	}
	for i := 1; i <= 3; i++ {
		left, _ := strconv.ParseUint(x[i], 10, 64)
		right, _ := strconv.ParseUint(y[i], 10, 64)
		if left != right {
			return left > right
		}
	}
	if x[4] == y[4] {
		return false
	}
	if x[4] == "" || y[4] == "" {
		return x[4] == ""
	}
	s, u := strings.Split(x[4], "."), strings.Split(y[4], ".")
	for i := 0; i < len(s) && i < len(u); i++ {
		left, le := strconv.ParseUint(s[i], 10, 64)
		right, re := strconv.ParseUint(u[i], 10, 64)
		if le == nil && re == nil {
			if left != right {
				return left > right
			}
		} else if (le == nil) != (re == nil) {
			return re == nil
		} else if s[i] != u[i] {
			return s[i] > u[i]
		}
	}
	return len(s) > len(u)
}
func (x *pluginsPanel) updateNames() string {
	var names []string
	for _, p := range x.state.Plugins {
		if pluginHasUpdate(p) {
			names = append(names, pluginDisplayName(p))
		}
	}
	return strings.Join(names, ", ")
}

func (x *pluginsPanel) headerView(c *ui.Context) {
	ui.Row(c).FillWidth().Gap(10).Wrap().AlignItems(ui.Center).Children(func() {
		labels := []string{upstreamT("Built-in"), nativeT("discover"), nativeT("installed")}
		kit.Help(c, upstreamT("Plugins")+" / "+labels[x.tab])
		if x.tab == 0 {
			return
		}
		ui.Spacer(c).Grow(1)
		if x.p.draft != nil {
			on := x.p.draft.Value.settings().ChinaMirror
			kit.Help(c, upstreamT("Mirrors in China"))
			if ui.Switch(c, &on).Label(upstreamT("Mirrors in China")).Disabled(x.p.locked() || x.p.dirty()).Changed() {
				x.p.draft.Value.settings().ChinaMirror = on
				x.p.save()
			}
		}
		kit.TextInput(c, &x.query).Width(260).MaxWidthPercent(100).Label(nativeT("searchPlugins")).Placeholder(upstreamT("Search plugins and npm…"))
	})
	if x.queryShown != x.query {
		x.queryShown = x.query
		x.searchAPI.close()
		x.hits = nil
		x.searchDue = c.Now().Add(350 * time.Millisecond)
	}
	if !x.searchDue.IsZero() {
		if c.Now().Before(x.searchDue) {
			c.After(x.searchDue.Sub(c.Now()))
			return
		}
		x.searchDue = time.Time{}
		if len([]rune(strings.TrimSpace(x.query))) >= 2 {
			x.searchAPI.request(x.p, "GET", "/api/plugins/search?q="+url.QueryEscape(x.query), nil, func(raw json.RawMessage) {
				var result struct{ Hits []marketPlugin }
				if nativeDecode(&x.searchAPI, raw, &result) {
					x.hits = result.Hits
				}
			})
		}
	}
}

func (x *pluginsPanel) builtinView(c *ui.Context, locked bool) {
	if x.p.advanced == nil {
		x.p.advanced = newAdvancedPanel(x.p)
	}
	a := x.p.advanced
	if a.catalog == nil && !a.loading && a.err == "" {
		a.load()
	}
	kit.Help(c, upstreamT("Parts of magpie not everyone needs. Switched off, a part leaves the sidebar, its pages and Settings; magpie works the same either way."))
	if a.err != "" {
		kit.InlineError(c, a.err)
	}
	if a.catalog == nil {
		kit.Status(c, i18n.T("capture.loading"))
		return
	}
	s := a.settings()
	for _, plugin := range a.catalog.BuiltinPlugins {
		if plugin.ID == "sessions" || plugin.ID == "library" {
			continue
		}
		choice := s.BuiltinPlugins[plugin.ID].Value
		on := plugin.On
		if choice.Enabled != nil {
			on = *choice.Enabled
		}
		if plugin.InUse > 0 {
			on = true
		}
		ui.Column(c.Key(plugin.ID)).FillWidth().Gap(6).Padding(10, 0).BorderWidth(0, 0, 1, 0).BorderColor(c.Theme().Border).Children(func() {
			kit.Row(c, upstreamT(plugin.Name), upstreamT(plugin.Sub), func() {
				if ui.Switch(c, &on).Label(upstreamT(plugin.Name)).Disabled(locked || plugin.InUse > 0 || x.p.dirty()).Changed() {
					if s.BuiltinPlugins == nil {
						s.BuiltinPlugins = map[string]core.Preserved[builtinChoice]{}
					}
					doc := s.BuiltinPlugins[plugin.ID]
					doc.Value.Enabled = ptr(on)
					s.BuiltinPlugins[plugin.ID] = doc
					x.p.save()
				}
			})
			if plugin.InUse > 0 {
				kit.Help(c, upstreamT("In use, so it stays on"))
			}
			if on && plugin.Schema != nil {
				for _, key := range plugin.Schema.Order {
					property, ok := plugin.Schema.Properties[key]
					if !ok {
						continue
					}
					value, set := choice.Config[key]
					if !set {
						value, set = plugin.Config[key]
					}
					if !set {
						value = property.Default
					}
					a.schemaField(c, plugin.ID+":"+key, property, value, locked, func(value any) {
						doc := s.BuiltinPlugins[plugin.ID]
						if doc.Value.Config == nil {
							doc.Value.Config = map[string]any{}
						}
						doc.Value.Config[key] = value
						if s.BuiltinPlugins == nil {
							s.BuiltinPlugins = map[string]core.Preserved[builtinChoice]{}
						}
						s.BuiltinPlugins[plugin.ID] = doc
						x.p.save()
					})
				}
			}
		})
	}
}

func pluginMatches(p marketPlugin, q string) bool {
	return q == "" || strings.Contains(strings.ToLower(p.Name+" "+p.Package+" "+p.Description+" "+p.Summary["zh"]+" "+p.Summary["en"]), strings.ToLower(strings.TrimSpace(q)))
}
func pluginSpec(p marketPlugin) string {
	if p.Spec != "" {
		return p.Spec
	}
	if p.Package != "" {
		return p.Package
	}
	if p.Repo != "" {
		return "github:" + p.Repo
	}
	return p.URL
}

func (x *pluginsPanel) installed(spec string) bool {
	return x.installedEntry(marketPlugin{Spec: spec}) != nil
}

var pluginPathPattern = regexp.MustCompile(`^(?:file://|\.|[/\\]|[A-Za-z]:[/\\])`)
var pluginGitRepoPattern = regexp.MustCompile(`(?i)^(?:github:|(?:git\+)?(?:https?|ssh|git)://(?:[^@/\s]+@)?(?:www\.|ssh\.)?github\.com(?::\d+)?[/:])([^/#\s]+/[^/#\s]+?)(?:\.git)?(?:[/#].*)?$`)
var pluginShortRepoPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*/[A-Za-z0-9_.-]+(?:#\S+)?$`)

func pluginRepository(spec string) string {
	if m := pluginGitRepoPattern.FindStringSubmatch(spec); m != nil {
		return strings.ToLower(m[1])
	}
	if pluginShortRepoPattern.MatchString(spec) {
		repo, _, _ := strings.Cut(spec, "#")
		return strings.ToLower(repo)
	}
	return ""
}

func pluginPackage(spec string) string {
	if pluginPathPattern.MatchString(spec) || pluginRepository(spec) != "" || strings.Contains(spec, "://") {
		return spec
	}
	if i := strings.LastIndex(spec, "@"); i > 0 && !strings.Contains(spec[i:], "/") {
		return spec[:i]
	}
	return spec
}

// A Git listing is identified by its repository, never by a package name a
// different repository can reuse. npm version pins do not change identity.
func (x *pluginsPanel) installedEntry(entry marketPlugin) *managedPlugin {
	spec := pluginSpec(entry)
	repo := pluginRepository(spec)
	if repo == "" && entry.Repo != "" {
		repo = strings.ToLower(entry.Repo)
	}
	for _, p := range x.effectivePlugins() {
		if p.Spec == spec || (repo != "" && pluginRepository(p.Spec) == repo) ||
			(repo == "" && pluginPackage(p.Spec) == pluginPackage(spec)) {
			copy := p
			return &copy
		}
	}
	return nil
}
func (x *pluginsPanel) marketSection(c *ui.Context, title string, entries []marketPlugin, locked bool) {
	if len(entries) == 0 {
		return
	}
	ui.Column(c.Key(title)).FillWidth().Gap(9).Padding(10, 0).Children(func() {
		if title != "" {
			kit.Strong(c, title)
		}
		row := ui.Row(c).FillWidth().Wrap().Gap(10).AlignItems(ui.Stretch)
		width := row.Bounds().W
		if width <= 0 {
			width, _ = c.Size()
			width -= 84
		}
		columns := max(1, int(math.Floor(float64((width+10)/272))))
		cardWidth := (width - float32(columns-1)*10) / float32(columns)
		row.Children(func() {
			for _, entry := range entries {
				ui.Column(c.Key(pluginSpec(entry))).Width(cardWidth).MaxWidthPercent(100).Shrink(0).Children(func() { x.marketCard(c, entry, locked) })
			}
		})
	})
}
func (x *pluginsPanel) marketCard(c *ui.Context, entry marketPlugin, locked bool) {
	spec := pluginSpec(entry)
	name := entry.Name
	if name == "" {
		name = spec
	}
	ui.Column(c).FillWidth().MinWidth(0).Gap(8).Padding(12).Border(1, c.Theme().Border).Radius(12).Background(kit.Colors(c).Panel).Children(func() {
		ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
			widgets.ProviderIcon(c, x.p.env, entry.Icon, 32)
			ui.Column(c).Grow(1).MinWidth(0).Children(func() {
				button := ui.ButtonBase(c).FillWidth().MinWidth(0).Justify(ui.Start).Label(name).Children(func() { kit.Strong(c, name).MinWidth(0) })
				if button.Clicked() {
					x.showReadme(spec)
					x.readmeTitle = name
				}
				if entry.NPM != nil {
					kit.Help(c, entry.NPM.Publisher)
				}
			})
			x.marketAction(c, entry, locked)
		})
		description := entry.Summary["en"]
		if i18n.Locale() == i18n.Chinese && entry.Summary["zh"] != "" {
			description = entry.Summary["zh"]
		}
		if description == "" {
			description = entry.Description
		}
		kit.Help(c, description).MinHeight(36)
		ui.Row(c).FillWidth().Gap(8).Wrap().AlignItems(ui.Center).Children(func() {
			version := entry.Version
			if entry.NPM != nil && entry.NPM.Version != "" {
				version = entry.NPM.Version
			}
			kit.Help(c, version)
			if entry.NPM != nil && entry.NPM.Weekly != nil {
				kit.Help(c, upstreamT("{n}/week", "n", *entry.NPM.Weekly))
			}
			kind := map[string]string{"provider": "Provider", "middleware": "Middleware", "agent": "Agent", "tools": "Tools", "page": "Page"}[pluginKind(entry)]
			if entry.Kind == "hooks" {
				kind = "Request hook"
			}
			kit.Help(c, upstreamT(kind))
			if entry.Repo != "" {
				kit.Help(c, "★ "+kit.Itoa(entry.Stars))
			}
		})
	})
}

func (x *pluginsPanel) marketAction(c *ui.Context, entry marketPlugin, locked bool) {
	spec := pluginSpec(entry)
	installed := x.installedEntry(entry)
	if installed == nil {
		if kit.PrimaryButton(c, nativeT("install"), icons.Download, false).Disabled(locked || spec == "").Clicked() {
			x.act("add", map[string]string{"spec": spec})
		}
	} else if installed.Off {
		if kit.Button(c, upstreamT("Switch on"), nil, false).Disabled(locked).Clicked() {
			x.act("off", map[string]any{"spec": installed.Spec, "off": false})
		}
	} else if pluginHasUpdate(*installed) {
		if kit.Button(c, nativeT("update"), icons.Download, false).Disabled(locked).Clicked() {
			x.act("upgrade", map[string]string{"spec": installed.Spec})
		}
	} else {
		kit.Help(c, nativeT("installed"))
	}
}

func (x *pluginsPanel) githubView(c *ui.Context, locked bool) {
	if !x.githubLoaded && !x.githubAPI.busy && x.githubAPI.err == "" {
		x.loadGithub()
	}
	x.githubAPI.notice(c, x.loadGithub)
	if len(x.github) == 0 {
		return
	}
	kit.Strong(c, upstreamT("Unofficial, on GitHub"))
	kit.Help(c, upstreamT("repositories their authors tagged {topic} — nobody has reviewed them; read the code before you install one", "topic", "magpie-plugin"))
	if x.githubMissing > 0 {
		kit.Help(c, upstreamT("{n} more on GitHub couldn't be read: {error}", "n", x.githubMissing, "error", x.githubAPI.err))
	}
	if len(x.github) > 6 {
		kit.TextInput(c, &x.githubQuery).FillWidth().Label(upstreamT("Search GitHub plugins"))
		kinds := []kit.SelectOption{{Value: "", Label: upstreamT("All")}}
		for _, kind := range []struct{ id, title string }{{"provider", "Provider"}, {"middleware", "Middleware"}, {"agent", "Agent"}, {"tools", "Tools"}, {"page", "Page"}} {
			count := 0
			for _, entry := range x.github {
				if pluginKind(entry) == kind.id && pluginMatches(entry, x.query) && pluginMatches(entry, x.githubQuery) {
					count++
				}
			}
			if count > 0 {
				kinds = append(kinds, kit.SelectOption{Value: kind.id, Label: upstreamT(kind.title) + " · " + kit.Itoa(count)})
			}
		}
		if len(kinds) > 2 {
			kit.Segmented(c, &x.githubKind, kinds, upstreamT("Kind of plugin"))
		}
		kit.Segmented(c, &x.githubHave, options("", upstreamT("All"), "have", upstreamT("Installed"), "none", upstreamT("Not installed")), upstreamT("Installed"))
	}
	var entries []marketPlugin
	for _, p := range x.github {
		here := x.installedEntry(p) != nil
		if pluginMatches(p, x.query) && pluginMatches(p, x.githubQuery) && (x.githubKind == "" || pluginKind(p) == x.githubKind) && (x.githubHave == "" || here == (x.githubHave == "have")) {
			entries = append(entries, p)
		}
	}
	x.marketSection(c, "", entries, locked)
	if len(entries) == 0 {
		kit.Help(c, upstreamT("No plugins found"))
	}
}
func pluginKind(entry marketPlugin) string {
	switch entry.Kind {
	case "":
		return "provider"
	case "hooks":
		return "middleware"
	default:
		return entry.Kind
	}
}
func (x *pluginsPanel) loadGithub() {
	path := "/api/plugins/github"
	if x.githubLoaded || x.githubAPI.err != "" {
		path += "?again=1"
	}
	x.githubAPI.request(x.p, "GET", path, nil, func(raw json.RawMessage) {
		var result struct {
			Repos   []marketPlugin
			Error   string
			Missing int
		}
		if nativeDecode(&x.githubAPI, raw, &result) {
			for i := range result.Repos {
				entry := &result.Repos[i]
				entry.NPM = &pluginNPM{Version: entry.Version, Publisher: entry.Owner, Repository: entry.URL, License: entry.License}
			}
			if len(result.Repos) > 0 || result.Error == "" {
				x.github = result.Repos
			}
			x.githubLoaded = true
			x.githubAPI.err = result.Error
			x.githubMissing = result.Missing
		}
	})
}
func (x *pluginsPanel) manualView(c *ui.Context, locked bool) {
	ui.Row(c).FillWidth().Wrap().Gap(8).AlignItems(ui.Center).Children(func() {
		kit.Help(c, upstreamT("Have one in mind?"))
		field := kit.TextInput(c, &x.spec).Grow(1).MinWidth(180).Label(nativeT("pluginSource")).Placeholder(upstreamT("npm package, GitHub repo (github:owner/repo) or a folder")).Disabled(locked)
		widgets.DropArea(c, x.p.env, field, widgets.DropFolders, false, locked, func(paths []string, err error) {
			if err != nil {
				x.api.err = err.Error()
			} else if len(paths) > 0 {
				x.spec = paths[0]
			}
		})
		if kit.IconButton(c, icons.FolderOpen, nativeT("chooseFolder")).Disabled(locked).Clicked() {
			core.Go(x.p.env, func() (string, error) { return x.p.env.Dialogs.PickFolder(nativeT("pluginSource")) }, func(path string, err error) {
				if err != nil {
					x.api.err = err.Error()
				} else if path != "" {
					x.spec = path
				}
			})
		}
		if kit.Button(c, nativeT("install"), icons.Download, false).Disabled(locked || strings.TrimSpace(x.spec) == "").Clicked() {
			x.act("add", map[string]string{"spec": strings.TrimSpace(x.spec)})
		}
	})
}

func (x *pluginsPanel) installedRow(c *ui.Context, p managedPlugin, locked bool) {
	ui.Column(c).FillWidth().Gap(8).Padding(12, 0).BorderWidth(0, 0, 1, 0).BorderColor(c.Theme().Border).Children(func() {
		ui.Row(c).FillWidth().Wrap().Gap(10).AlignItems(ui.Center).Children(func() {
			widgets.ProviderIcon(c, x.p.env, p.Icon, 32)
			ui.Column(c).Grow(1).Basis(230).MinWidth(0).Gap(3).Children(func() {
				if kit.GhostButton(c, pluginDisplayName(p), nil, false).Clicked() {
					x.showReadme(p.Spec)
					x.readmeTitle = pluginDisplayName(p)
				}
				kit.Help(c, p.Version+" · "+p.Spec).Selectable()
				if p.Commit != "" {
					kit.Help(c, p.Commit)
				}
				if p.Off {
					kit.Help(c, upstreamT("Off"))
				}
				if check, ok := x.checks[p.Spec]; ok {
					if check.Status == "unknown" {
						kit.Help(c, upstreamT("Couldn't check")).Tooltip(pluginCheckReason(check))
						kit.InlineError(c, pluginCheckReason(check))
					} else if check.Status == "current" {
						kit.Help(c, upstreamT("Up to date"))
					}
				}
				if len(p.Moved)+len(p.Serves) > 0 && !p.Off {
					kit.Help(c, upstreamT("Serves {names}", "names", strings.Join(append(append([]string{}, p.Moved...), p.Serves...), ", ")))
				}
				if p.Error != "" {
					kit.InlineError(c, p.Error)
				}
			})
			ui.Row(c).Wrap().Gap(6).Children(func() {
				if pluginHasUpdate(p) && kit.Button(c, nativeT("update"), icons.Download, false).Disabled(locked).Clicked() {
					x.act("upgrade", map[string]string{"spec": p.Spec})
				}
				for _, a := range x.p.catalog.agents {
					if a.Plugin == p.Spec && !p.Off {
						if kit.Button(c, nativeT("connect")+" "+a.Name, icons.LogIn, false).Disabled(locked).Clicked() {
							if err := x.p.openPluginProvider(a, true); err != nil {
								x.api.err = err.Error()
							}
						}
					}
				}
				if p.ToolsServer != nil && !p.Off {
					word := "Give to agents"
					if p.ToolsServer.On {
						word = "Hide from agents"
					}
					if kit.GhostButton(c, upstreamT(word), nil, false).Disabled(locked).Clicked() {
						if x.p.external == nil {
							x.p.external = newExternalPanel(x.p)
						}
						x.p.external.connect(func() {
							x.p.external.request("POST", "/api/library/servers/plugin-on", map[string]any{"name": p.ToolsServer.Plugin, "on": !p.ToolsServer.On}, func(raw json.RawMessage) {
								x.p.external.acceptLibrary(raw)
								if x.p.external.err == "" {
									x.load()
								}
							})
						})
					}
				}
				if !p.Off && (p.Options != nil || p.OptionsExample != nil || p.IsMiddleware || p.IsGateway) && kit.GhostButton(c, nativeT("pluginOptions"), nil, false).Disabled(locked).Clicked() {
					x.editOptions(p)
				}
				word := "Switch off"
				if p.Off {
					word = "Switch on"
				}
				if kit.GhostButton(c, upstreamT(word), nil, false).Disabled(locked).Clicked() {
					if !p.Off && len(p.Moved) > 0 {
						x.switchOff = p.Spec
					} else {
						x.act("off", map[string]any{"spec": p.Spec, "off": !p.Off})
					}
				}
				if kit.GhostButton(c, nativeT("remove"), nil, false).Disabled(locked).Clicked() {
					x.remove = p.Spec
				}
			})
		})
		if x.switchOff == p.Spec {
			kit.Help(c, upstreamT("Serves {names}", "names", strings.Join(p.Moved, ", ")))
			ui.Row(c).Gap(8).Children(func() {
				if kit.GhostButton(c, i18n.T("common.cancel"), nil, false).Clicked() {
					x.switchOff = ""
				}
				if kit.Button(c, upstreamT("Switch off"), nil, false).Disabled(locked).Clicked() {
					x.act("off", map[string]any{"spec": p.Spec, "off": true})
					x.switchOff = ""
				}
			})
		}
		for _, clash := range p.Clashes {
			id := clash.Provider
			if id == "" {
				id = clash.ID
			}
			if kit.GhostButton(c, nativeT("preferPlugin")+" "+clash.Name, icons.Check, false).Disabled(locked).Clicked() {
				x.act("prefer", map[string]string{"spec": p.Spec, "provider": id})
			}
		}
		x.pluginDetailsView(c, p)
		if x.p.extensions.state != nil {
			for _, page := range x.p.extensions.state.Pages {
				if page.Spec == p.Spec && kit.Button(c, page.title(), icons.PanelRightOpen, false).Disabled(locked).Clicked() {
					x.request = &pageRequest{key: page.Key, nonce: nonce()}
				}
			}
		}
		if x.optionsSpec == p.Spec {
			x.optionsView(c, locked)
		}
	})
}
func (x *pluginsPanel) editOptions(p managedPlugin) {
	change := func() {
		x.optionsSpec = p.Spec
		v := p.Options
		if v == nil {
			v = p.OptionsExample
		}
		if v == nil {
			v = map[string]any{}
		}
		b, _ := json.MarshalIndent(v, "", "  ")
		x.optionsText = string(b)
		x.optionsSaved = x.optionsText
		e, _ := json.MarshalIndent(p.OptionsExample, "", "  ")
		x.optionsExample = string(e)
	}
	if x.dirty() && x.p.env.RequestChange != nil {
		x.p.env.RequestChange(change)
	} else {
		change()
	}
}
