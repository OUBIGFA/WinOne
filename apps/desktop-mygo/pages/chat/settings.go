package chat

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/widgets"
)

// chatSettings are Chat's preferences as the Host's chat.settings.get
// returns them (internal/chat.Preferences). Automations are not edited
// here and go back as they came.
type chatSettings struct {
	WorkspaceDirectory    string               `json:"workspaceDirectory"`
	TitleModel            *widgets.AISelection `json:"titleModel,omitempty"`
	SkillDirectories      []string             `json:"skillDirectories"`
	PluginDirectories     []string             `json:"pluginDirectories"`
	MCPServers            []mcpServer          `json:"mcpServers"`
	Automations           json.RawMessage      `json:"automations,omitempty"`
	DefaultThinkingLevel  string               `json:"defaultThinkingLevel,omitempty"`
	AutoCompaction        *bool                `json:"autoCompaction,omitempty"`
	AutoRetry             *bool                `json:"autoRetry,omitempty"`
	BackgroundCompaction  bool                 `json:"backgroundCompaction,omitempty"`
	CodeMode              *bool                `json:"codeMode,omitempty"`
	Planning              *bool                `json:"planning,omitempty"`
	ToolPreset            string               `json:"toolPreset,omitempty"`
	Subagents             *bool                `json:"subagents,omitempty"`
	MaxSubagents          int                  `json:"maxSubagents,omitempty"`
	SubagentModel         *widgets.AISelection `json:"subagentModel,omitempty"`
	SubagentThinkingLevel string               `json:"subagentThinkingLevel,omitempty"`
}

// mcpServer is one MCP server: a local command or a remote URL.
type mcpServer struct {
	Exposure     string            `json:"exposure,omitempty"`
	Description  string            `json:"description,omitempty"`
	ToolExposure map[string]string `json:"toolExposure,omitempty"`
	Name         string            `json:"name"`
	Enabled      bool              `json:"enabled"`
	Command      string            `json:"command,omitempty"`
	Args         []string          `json:"args,omitempty"`
	Cwd          string            `json:"cwd,omitempty"`
	URL          *string           `json:"url,omitempty"`
	Env          map[string]string `json:"env,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	Timeout      int               `json:"timeout,omitempty"`
}

// mcpMapFields are a server's JSON fields edited as text.
var mcpMapFields = []string{"env", "headers", "toolExposure"}

// settingsDialog is Chat settings' draft. It is edited as one: a failed
// save keeps every field, and closing keeps the draft until it is saved or
// reset, so reopening shows it again.
type settingsDialog struct {
	open, loading, saving bool
	value, saved          *chatSettings
	err                   string
	tab                   int
	// maps hold a server's JSON fields as typed, malformed text included,
	// by server and field; moving between tabs or a failed save keeps them.
	maps map[int]map[string]string
	// advanced are the servers whose advanced options are open.
	advanced map[int]bool
	// The connectors of automations: their own JSON, saved apart.
	connections, connectionsSaved string
	connectionServers             []string
	connectionsLoading            bool
	connectionsErr                string
}

// openSettings shows Chat's settings.
func (p *Chat) openSettings() { p.tools.settingsOpen = true }

func copySettings(s *chatSettings) *chatSettings {
	raw, _ := json.Marshal(s)
	var copy chatSettings
	_ = json.Unmarshal(raw, &copy)
	normalizeSettings(&copy)
	return &copy
}

// normalizeSettings reads an empty model as following the default, as the
// Host returns zero selections.
func normalizeSettings(s *chatSettings) {
	if s.TitleModel != nil && s.TitleModel.Provider == "" {
		s.TitleModel = nil
	}
	if s.SubagentModel != nil && s.SubagentModel.Provider == "" {
		s.SubagentModel = nil
	}
	if s.SkillDirectories == nil {
		s.SkillDirectories = []string{}
	}
	if s.PluginDirectories == nil {
		s.PluginDirectories = []string{}
	}
	if s.MCPServers == nil {
		s.MCPServers = []mcpServer{}
	}
}

func (d *settingsDialog) dirty() bool {
	if d.value == nil || d.saved == nil {
		return false
	}
	a, _ := json.Marshal(d.value)
	b, _ := json.Marshal(d.saved)
	return string(a) != string(b) || len(d.maps) > 0
}

// parseMCPMap reads a JSON object of strings; empty text clears the field.
func parseMCPMap(text, key string) (map[string]string, bool) {
	if strings.TrimSpace(text) == "" {
		return nil, true
	}
	var value map[string]string
	if json.Unmarshal([]byte(text), &value) != nil || value == nil {
		return nil, false
	}
	if key == "toolExposure" {
		for _, mode := range value {
			if !slices.Contains([]string{"direct", "deferred", "codemode", "hidden"}, mode) {
				return nil, false
			}
		}
	}
	return value, true
}

func (d *settingsDialog) invalidMaps() bool {
	for _, fields := range d.maps {
		for key, text := range fields {
			if _, ok := parseMCPMap(text, key); !ok {
				return true
			}
		}
	}
	return false
}

// loadSettings reads the settings and the connectors when the dialog
// opens without a draft.
func (p *Chat) loadSettings() {
	d := &p.settings
	if d.value == nil && !d.loading {
		d.loading, d.err = true, ""
		chatCall[chatSettings](p, "settings.get", map[string]any{}, func(s chatSettings, err error) {
			d.loading = false
			if err != nil {
				d.err = err.Error()
				return
			}
			normalizeSettings(&s)
			d.value, d.saved = copySettings(&s), copySettings(&s)
		})
	}
	if d.connectionsSaved == "" && !d.connectionsLoading {
		p.loadConnections()
	}
}

func (p *Chat) loadConnections() {
	d := &p.settings
	d.connectionsLoading, d.connectionsErr = true, ""
	type connections struct {
		Settings json.RawMessage `json:"settings"`
		Servers  []struct {
			Name    string `json:"name"`
			Enabled bool   `json:"enabled"`
		} `json:"servers"`
	}
	chatCall[connections](p, "automations.connections.get", map[string]any{}, func(v connections, err error) {
		d.connectionsLoading = false
		if err != nil {
			d.connectionsErr = err.Error()
			return
		}
		var pretty any
		_ = json.Unmarshal(v.Settings, &pretty)
		text, _ := json.MarshalIndent(pretty, "", "  ")
		d.connections, d.connectionsSaved = string(text), string(text)
		d.connectionServers = nil
		for _, s := range v.Servers {
			if s.Enabled {
				d.connectionServers = append(d.connectionServers, s.Name)
			}
		}
	})
}

func (p *Chat) saveSettings() {
	d := &p.settings
	d.saving, d.err = true, ""
	chatCall[chatSettings](p, "settings.save", map[string]any{"settings": d.value}, func(s chatSettings, err error) {
		d.saving = false
		if err != nil {
			d.err = err.Error()
			return
		}
		normalizeSettings(&s)
		if p.tools.automations != nil {
			p.tools.automations.RefreshSupport()
		}
		d.value, d.saved, d.maps = copySettings(&s), copySettings(&s), nil
		d.advanced = nil
		p.tools.settingsOpen = false
		if p.env.Toast != nil {
			p.env.Toast(i18n.T("chat.settingsSaved"))
		}
		p.refresh(func(err error) {
			if err != nil {
				p.err = err.Error()
			}
		})
	})
}

// settingsView shows Chat settings over the page while open.
func (p *Chat) settingsView(c *ui.Context) {
	if !p.tools.settingsOpen {
		return
	}
	d := &p.settings
	p.loadSettings()
	busy := d.loading || d.saving
	note := ""
	if d.dirty() {
		note = i18n.T("chat.unsavedSettings")
	}
	open := true
	confirmed := kit.Dialog(c, &open, kit.DialogOptions{
		Title: i18n.T("chat.settings"), Width: 640, Busy: d.saving, Error: d.err, Note: note,
		Cancel: i18n.T("chat.close"), Confirm: i18n.T("chat.saveSettings"),
		ConfirmDisabled: d.value == nil || !d.dirty() || d.invalidMaps() || busy,
		Header: func() {
			if d.value != nil {
				kit.SubTabs(c, &d.tab, []string{i18n.T("chat.general"), i18n.T("chat.resources"), "MCP"}, nil)
			}
		},
		Body: func() {
			switch {
			case d.value != nil:
				ui.Column(c).FillWidth().Gap(24).Padding(4, 0, 0, 0).Children(func() {
					switch d.tab {
					case 0:
						p.settingsGeneral(c, busy)
					case 1:
						p.settingsResources(c, busy)
					default:
						p.settingsMCP(c, busy)
					}
				})
			case d.loading:
				kit.Status(c, i18n.T("chat.settingsLoading"))
			default:
				if kit.Button(c, i18n.T("chat.retry"), icons.RefreshCw, false).Clicked() {
					p.loadSettings()
				}
			}
		},
		Actions: func() {
			if kit.Button(c, i18n.T("chat.reset"), nil, false).Disabled(busy || !d.dirty()).Clicked() {
				d.value, d.maps, d.err = copySettings(d.saved), nil, ""
			}
		},
	})
	if confirmed {
		p.saveSettings()
	}
	if !open && !d.saving {
		p.tools.settingsOpen = false
	}
}

func boolOn(v *bool) bool { return v == nil || *v }

// toggle is a kit.Toggle of a setting that is on unless turned off.
func toggle(c *ui.Context, v **bool, label string, disabled bool) {
	on := boolOn(*v)
	if kit.Toggle(c, &on, label, "").Disabled(disabled).Changed() {
		*v = &on
	}
}

func thinkingOptions(auto bool) []kit.SelectOption {
	var options []kit.SelectOption
	if auto {
		options = append(options, kit.SelectOption{Value: "", Label: i18n.T("chat.thinkingAuto")})
	}
	for _, level := range []string{"off", "minimal", "low", "medium", "high", "xhigh", "max"} {
		options = append(options, kit.SelectOption{Value: level, Label: i18n.T("chat.thinking_" + level)})
	}
	return options
}

// settingsSelect is a Row with a drop-down.
func settingsSelect(c *ui.Context, label string, value *string, options []kit.SelectOption, disabled bool) {
	kit.Row(c, label, "", func() {
		_, trigger := kit.Select(c, value, options)
		trigger.Disabled(disabled).Label(label)
	})
}

func (p *Chat) settingsModels() ([]widgets.AIProvider, bool) {
	if p.ai == nil {
		return nil, true
	}
	return p.ai.Providers, !p.catalogLoaded && p.err == "" || p.view.ModelError != ""
}

func (p *Chat) settingsGeneral(c *ui.Context, busy bool) {
	v := p.settings.value
	providers, modelsDisabled := p.settingsModels()
	if p.view.ModelError != "" {
		kit.InlineError(c, p.view.ModelError)
	}
	kit.Group(c, nil, i18n.T("chat.workspaceDirectory"), i18n.T("chat.workspaceHint"), func() {
		zone := ui.Row(c).FillWidth().Padding(12, 16).Gap(8).AlignItems(ui.Center)
		zone.Children(func() {
			kit.TextInput(c, &v.WorkspaceDirectory).Grow(1).MinWidth(0).Font(kit.MonoFont).FontSize(kit.HelpSize(c)).
				Label(i18n.T("chat.workspaceDirectory")).Disabled(busy)
			if kit.Button(c, i18n.T("chat.chooseLocation"), icons.FolderOpen, false).Disabled(busy).Clicked() {
				p.pickFolder(func(path string) { v.WorkspaceDirectory = path })
			}
		})
		p.dropFolder(c, zone, busy, func(path string) { v.WorkspaceDirectory = path })
	})
	kit.Group(c, nil, i18n.T("chat.titleModel"), i18n.T("chat.titleModelHint"), func() {
		kit.Stack(c, "", "", func() {
			if next, changed := widgets.AIModelSelect(c, widgets.AIModelSelectOptions{Env: p.env, Providers: providers, Value: v.TitleModel,
				Label: i18n.T("chat.titleModel"), InheritLabel: i18n.T("chat.titleModelInherit"), Disabled: busy || modelsDisabled}); changed {
				v.TitleModel = next
			}
		})
	})
	kit.Group(c, nil, i18n.T("chat.sessionDefaults"), i18n.T("chat.runtimeHint"), func() {
		settingsSelect(c, i18n.T("chat.defaultThinkingLevel"), &v.DefaultThinkingLevel, thinkingOptions(true), busy)
		toggle(c, &v.AutoCompaction, i18n.T("chat.autoCompaction"), busy)
		toggle(c, &v.AutoRetry, i18n.T("chat.autoRetry"), busy)
		background := v.BackgroundCompaction
		if kit.Toggle(c, &background, i18n.T("chat.backgroundCompaction"), "").Disabled(busy || !boolOn(v.AutoCompaction)).Changed() {
			v.BackgroundCompaction = background
		}
		toggle(c, &v.CodeMode, i18n.T("chat.codeMode"), busy)
		toggle(c, &v.Planning, i18n.T("chat.planning"), busy)
		preset := v.ToolPreset
		if preset == "" {
			preset = "all"
		}
		settingsSelect(c, i18n.T("chat.toolPreset"), &preset, []kit.SelectOption{
			{Value: "all", Label: i18n.T("chat.toolPresetAll")},
			{Value: "read-only", Label: i18n.T("chat.toolPresetReadOnly")},
			{Value: "chat-only", Label: i18n.T("chat.toolPresetChatOnly")}}, busy)
		if preset != first(v.ToolPreset, "all") {
			v.ToolPreset = preset
		}
	})
	kit.Group(c, nil, i18n.T("chat.subagentGroup"), "", func() {
		toggle(c, &v.Subagents, i18n.T("chat.subagentsEnabled"), busy)
		off := !boolOn(v.Subagents)
		shown := v.MaxSubagents
		if shown == 0 {
			shown = 4
		}
		limit := float64(shown)
		kit.Row(c, i18n.T("chat.maxSubagents"), "", func() {
			kit.NumberField(c, &limit, 1, 64, 1, "", i18n.T("chat.maxSubagents")).Disabled(busy || off)
		})
		if int(limit) != shown {
			v.MaxSubagents = int(limit)
		}
		kit.Stack(c, "", "", func() {
			if next, changed := widgets.AIModelSelect(c, widgets.AIModelSelectOptions{Env: p.env, Providers: providers, Value: v.SubagentModel,
				Label: i18n.T("chat.subagentModel"), FieldLabel: i18n.T("chat.subagentModel"), InheritLabel: i18n.T("chat.subagentModelInherit"),
				Disabled: busy || modelsDisabled || off}); changed {
				v.SubagentModel = next
			}
		})
		thinking := first(v.SubagentThinkingLevel, "medium")
		settingsSelect(c, i18n.T("chat.subagentThinkingLevel"), &thinking, thinkingOptions(false), busy || off)
		if thinking != first(v.SubagentThinkingLevel, "medium") {
			v.SubagentThinkingLevel = thinking
		}
	})
}

func (p *Chat) settingsResources(c *ui.Context, busy bool) {
	v := p.settings.value
	kit.Help(c, i18n.T("chat.resourceHint"))
	p.directoryList(c, i18n.T("chat.skillDirectories"), &v.SkillDirectories, busy)
	p.directoryList(c, i18n.T("chat.pluginDirectories"), &v.PluginDirectories, busy)
}

// directoryList is a group of folders: each with a way to remove it, a
// way to add one, and folders dragged onto it from File Explorer.
func (p *Chat) directoryList(c *ui.Context, title string, list *[]string, busy bool) {
	add := func(paths ...string) {
		for _, path := range paths {
			if !slices.Contains(*list, path) {
				*list = append(*list, path)
			}
		}
	}
	zone := kit.Section(c, nil, title, "", func() {
		kit.Panel(c, func() {
			for i, path := range *list {
				i, path := i, path
				ui.Row(c.Key("dir-"+path)).FillWidth().MinHeight(kit.RowMinHeight).Padding(6, 8, 6, 16).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, path).Font(kit.MonoFont).FontSize(kit.HelpSize(c)).Grow(1).MinWidth(0).SingleLine().Ellipsis("…").Tooltip(path).Selectable()
					if kit.IconButton(c, icons.Trash2, i18n.T("chat.removeDirectory")+" · "+path).Disabled(busy).Clicked() {
						*list = slices.Delete(slices.Clone(*list), i, i+1)
					}
				})
			}
			ui.Row(c).FillWidth().Padding(10, 16).Children(func() {
				if kit.Button(c, i18n.T("chat.addDirectory"), icons.FolderOpen, false).Disabled(busy).Clicked() {
					p.pickFolder(func(path string) { add(path) })
				}
			})
		})
	})
	widgets.DropArea(c, p.env, zone, widgets.DropFolders, true, busy, func(paths []string, err error) {
		if err != nil {
			p.settings.err = err.Error()
			return
		}
		add(paths...)
	})
}

func (p *Chat) settingsMCP(c *ui.Context, busy bool) {
	d := &p.settings
	v := d.value
	kit.Section(c, nil, i18n.T("chat.mcpServers"), "", func() {
		for i := range v.MCPServers {
			p.mcpServerView(c, i, busy)
		}
		ui.Row(c).Children(func() {
			if kit.Button(c, i18n.T("chat.addServer"), icons.Plus, false).Disabled(busy).Clicked() {
				v.MCPServers = append(v.MCPServers, mcpServer{Enabled: true})
			}
		})
	})
	p.connectionsView(c)
}

func (p *Chat) mcpServerView(c *ui.Context, index int, busy bool) {
	d := &p.settings
	server := &d.value.MCPServers[index]
	ui.Column(c.Key("mcp-" + strconv.Itoa(index))).FillWidth().Children(func() {
		kit.Panel(c, func() {
			ui.Row(c).FillWidth().Padding(12, 8, 12, 16).Gap(12).AlignItems(ui.End).Children(func() {
				ui.Field(c, i18n.T("chat.serverName"), func() {
					kit.TextInput(c, &server.Name).FillWidth().Disabled(busy)
				}).Grow(1).MinWidth(0)
				ui.Checkbox(c, &server.Enabled, i18n.T("chat.enabled")).Disabled(busy).Margin(0, 0, 8, 0)
				if kit.IconButton(c, icons.Trash2, i18n.T("chat.remove")+" · "+server.Name).Disabled(busy).Clicked() {
					d.removeServer(index)
				}
			})
			remote := server.URL != nil
			transport := "command"
			if remote {
				transport = "url"
			}
			kit.Row(c, i18n.T("chat.transport"), "", func() {
				changed, trigger := kit.Select(c, &transport, []kit.SelectOption{
					{Value: "command", Label: i18n.T("chat.commandTransport")}, {Value: "url", Label: i18n.T("chat.urlTransport")}})
				trigger.Disabled(busy).Label(i18n.T("chat.transport"))
				if changed {
					if transport == "url" {
						empty := ""
						server.URL, server.Command, server.Args = &empty, "", nil
					} else {
						server.URL = nil
					}
				}
			})
			exposure := first(server.Exposure, "direct")
			var modes []kit.SelectOption
			for _, mode := range []string{"direct", "deferred", "codemode", "hidden"} {
				modes = append(modes, kit.SelectOption{Value: mode, Label: i18n.T("chat.mcpExposure_" + mode)})
			}
			settingsSelect(c, i18n.T("chat.mcpExposure"), &exposure, modes, busy)
			if exposure != first(server.Exposure, "direct") {
				server.Exposure = exposure
			}
			kit.Stack(c, i18n.T("chat.mcpDescription"), "", func() {
				kit.TextInput(c, &server.Description).FillWidth().Label(i18n.T("chat.mcpDescription")).Disabled(busy)
				if runes := []rune(server.Description); len(runes) > 4096 {
					server.Description = string(runes[:4096])
				}
			})
			if server.URL != nil {
				kit.Stack(c, i18n.T("chat.serverUrl"), "", func() {
					kit.TextInput(c, server.URL).FillWidth().Font(kit.MonoFont).Label(i18n.T("chat.serverUrl")).Disabled(busy)
				})
			} else {
				kit.Stack(c, i18n.T("chat.command"), "", func() {
					input := kit.TextInput(c, &server.Command).FillWidth().Font(kit.MonoFont).Label(i18n.T("chat.command")).Disabled(busy)
					p.dropPath(c, input, widgets.DropFiles, busy, func(path string) { server.Command = path })
				})
				kit.Stack(c, i18n.T("chat.arguments"), "", func() {
					args := strings.Join(server.Args, "\n")
					if ui.TextArea(c, &args).FillWidth().Lines(3, 8).Font(kit.MonoFont).Label(i18n.T("chat.arguments")).Disabled(busy).Changed() {
						server.Args = nil
						if args != "" {
							server.Args = strings.Split(args, "\n")
						}
					}
				})
			}
			if d.advanced == nil {
				d.advanced = map[int]bool{}
			}
			open := d.advanced[index]
			ui.Column(c).FillWidth().Padding(4, 16, 12, 16).Children(func() {
				ui.Collapsible(c, i18n.T("chat.advanced"), &open, func() {
					ui.Column(c).FillWidth().Gap(12).Padding(8, 0, 0, 0).Children(func() {
						p.mcpMapField(c, index, "env", i18n.T("chat.environment"), busy)
						p.mcpMapField(c, index, "headers", i18n.T("chat.headers"), busy)
						p.mcpMapField(c, index, "toolExposure", i18n.T("chat.mcpToolExposure"), busy)
						ui.Field(c, i18n.T("chat.sourcePath"), func() {
							input := kit.TextInput(c, &server.Cwd).FillWidth().Font(kit.MonoFont).Disabled(busy)
							p.dropPath(c, input, widgets.DropFolders, busy, func(path string) { server.Cwd = path })
						}).FillWidth()
						shown := server.Timeout
						if shown == 0 {
							shown = 30
						}
						timeout := float64(shown)
						ui.Field(c, i18n.T("chat.timeout"), func() {
							kit.NumberField(c, &timeout, 1, 86400, 1, "", i18n.T("chat.timeout")).Disabled(busy)
						})
						if int(timeout) != shown {
							server.Timeout = int(timeout)
						}
					})
				})
			})
			d.advanced[index] = open
		})
	})
}

// mcpMapField edits a server's JSON object field as text: malformed text
// stays in the draft, is reported, and keeps the settings from saving.
func (p *Chat) mcpMapField(c *ui.Context, index int, key, label string, busy bool) {
	d := &p.settings
	server := &d.value.MCPServers[index]
	current := map[string]map[string]string{"env": server.Env, "headers": server.Headers, "toolExposure": server.ToolExposure}[key]
	text, typed := d.maps[index][key]
	if !typed && len(current) > 0 {
		raw, _ := json.MarshalIndent(current, "", "  ")
		text = string(raw)
	}
	_, valid := parseMCPMap(text, key)
	ui.Field(c, label, func() {
		if ui.TextArea(c, &text).FillWidth().Lines(3, 8).Font(kit.MonoFont).FontSize(kit.HelpSize(c)).Disabled(busy).Changed() {
			if d.maps == nil {
				d.maps = map[int]map[string]string{}
			}
			if d.maps[index] == nil {
				d.maps[index] = map[string]string{}
			}
			d.maps[index][key] = text
			if value, ok := parseMCPMap(text, key); ok {
				switch key {
				case "env":
					server.Env = value
				case "headers":
					server.Headers = value
				default:
					server.ToolExposure = value
				}
			}
		}
		if !valid {
			message := "chat.mcpMapInvalid"
			if key == "toolExposure" {
				message = "chat.mcpToolMapInvalid"
			}
			kit.InlineError(c, i18n.T(message))
		}
	}).FillWidth()
}

// removeServer removes a server; the text typed for the servers after it
// moves with them.
func (d *settingsDialog) removeServer(index int) {
	d.value.MCPServers = slices.Delete(slices.Clone(d.value.MCPServers), index, index+1)
	maps, advanced := map[int]map[string]string{}, map[int]bool{}
	for i, fields := range d.maps {
		switch {
		case i < index:
			maps[i] = fields
		case i > index:
			maps[i-1] = fields
		}
	}
	for i, open := range d.advanced {
		switch {
		case i < index:
			advanced[i] = open
		case i > index:
			advanced[i-1] = open
		}
	}
	d.maps, d.advanced = maps, advanced
	if len(d.maps) == 0 {
		d.maps = nil
	}
}

// dropPath lets a path field take one file or folder dragged onto it.
func (p *Chat) dropPath(c *ui.Context, target ui.Element, kind widgets.DropKind, disabled bool, use func(string)) {
	widgets.DropArea(c, p.env, target, kind, false, disabled, func(paths []string, err error) {
		if err != nil {
			p.settings.err = err.Error()
			return
		}
		use(paths[0])
	})
}

// connectionsView is the automations' connectors: method mappings onto
// the MCP servers above, saved on their own.
func (p *Chat) connectionsView(c *ui.Context) {
	d := &p.settings
	servers := strings.Join(d.connectionServers, " · ")
	if servers == "" {
		servers = i18n.T("chat.connectionsNone")
	}
	kit.Group(c, nil, i18n.T("chat.connections"), i18n.T("chat.connectionsHint"), func() {
		kit.Stack(c, i18n.T("chat.connectionsJSON"), i18n.T("chat.connectionsServers")+" · "+servers, func() {
			ui.TextArea(c, &d.connections).FillWidth().Lines(6, 14).Font(kit.MonoFont).FontSize(kit.HelpSize(c)).
				Label(i18n.T("chat.connectionsJSON")).Disabled(d.connectionsLoading)
			kit.Help(c, "jira.get · jira.search_jql · jira.add_comment · feishu.recent_chats · feishu.recent_messages · notify.feishu · notify.wecom")
			kit.InlineError(c, d.connectionsErr)
			ui.Row(c).Children(func() {
				save := kit.Button(c, i18n.T("chat.connectionsSave"), nil, d.connectionsLoading).
					Disabled(d.connectionsLoading || d.connections == d.connectionsSaved)
				if save.Clicked() {
					p.saveConnections()
				}
			})
		})
	})
}

func (p *Chat) saveConnections() {
	d := &p.settings
	var settings any
	if err := json.Unmarshal([]byte(d.connections), &settings); err != nil {
		d.connectionsErr = core.Errorf(i18n.T("chat.connectionsInvalid")).Error()
		return
	}
	d.connectionsLoading, d.connectionsErr = true, ""
	text := d.connections
	chatCall[json.RawMessage](p, "automations.connections.save", map[string]any{"settings": settings}, func(_ json.RawMessage, err error) {
		d.connectionsLoading = false
		if err != nil {
			d.connectionsErr = err.Error()
			return
		}
		d.connectionsSaved = text
		if p.tools.automations != nil {
			p.tools.automations.RefreshSupport()
		}
		if p.env.Toast != nil {
			p.env.Toast(i18n.T("chat.connectionsSaved"))
		}
	})
}
