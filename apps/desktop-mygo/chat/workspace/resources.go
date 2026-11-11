package workspace

import (
	"fmt"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

// Resources mirrors the actual Pi discovery response used by ChatResourcesPanel.
// It never discovers resources by scanning arbitrary UI-side directories.
type Resources struct {
	Agents []struct {
		Path string `json:"path"`
	} `json:"agents"`
	Skills []struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Path        string `json:"path"`
	} `json:"skills"`
	Plugins []struct {
		Name    string `json:"name"`
		Path    string `json:"path"`
		Enabled bool   `json:"enabled"`
	} `json:"plugins"`
	MCPServers []struct {
		Name    string `json:"name"`
		Status  string `json:"status"`
		Error   string `json:"error"`
		Enabled bool   `json:"enabled"`
	} `json:"mcpServers"`
	Diagnostics []string `json:"diagnostics"`
}
type ResourceOptions struct {
	Prefix, SessionID string
	OnOpenFile        func(string)
	OnReference       func(string)
}
type ResourcePane struct {
	env                     *core.Env
	options                 ResourceOptions
	value                   Resources
	loading, loaded, closed bool
	err                     string
	revision                uint64
}

func NewResourcePane(env *core.Env, options ResourceOptions) *ResourcePane {
	p := &ResourcePane{env: env, options: options}
	p.Refresh()
	return p
}
func (p *ResourcePane) Close() { p.closed = true; p.revision++ }
func (p *ResourcePane) Refresh() {
	if p.closed || p.loading || p.options.SessionID == "" {
		return
	}
	p.loading = true
	p.revision++
	revision := p.revision
	core.Go(p.env, func() (Resources, error) {
		var result Resources
		err := p.env.Host.CallInto(p.options.Prefix+"resources", map[string]string{"id": p.options.SessionID}, &result)
		return result, err
	}, func(result Resources, err error) {
		if p.closed || revision != p.revision {
			return
		}
		p.loading = false
		if err != nil {
			p.err = err.Error()
			return
		}
		p.value = result
		p.loaded = true
		p.err = ""
	})
}
func (p *ResourcePane) View(c *ui.Context) {
	if p.closed {
		return
	}
	ui.Column(c).Fill().MinWidth(0).Children(func() {
		if p.err != "" {
			ui.Column(c).FillWidth().Padding(0, 12, 8, 12).Children(func() { kit.InlineError(c, p.err) })
		}
		if p.options.SessionID == "" {
			emptyHint(c, icons.BookOpen, i18n.T("chat.selectConversation"))
			return
		}
		if p.loading && !p.loaded {
			ui.Row(c).Padding(8, 12).Children(func() { kit.Loader(c) })
		}
		if !p.loaded {
			return
		}
		ui.Scroll(c).Grow(1).MinHeight(0).FillWidth().Padding(4, 12, 16, 12).Gap(20).Children(func() {
			first := true
			// group is a kind of resource: its name and count over one panel
			// of rows, as the settings pages group theirs. The first group's
			// line also holds the action that reads them all again.
			group := func(name string, count int, body func()) {
				ui.Column(c.Key("group:" + name)).FillWidth().Gap(6).Children(func() {
					ui.Row(c).FillWidth().MinHeight(30).Padding(0, 0, 0, 4).Gap(6).AlignItems(ui.Center).Children(func() {
						kit.Caption(c, fmt.Sprintf("%s · %d", name, count)).Grow(1).Role(ui.RoleHeading).Level(2)
						if first {
							if toolButton(c, icons.RefreshCw, i18n.T("chat.refresh")).Disabled(p.loading).Clicked() {
								p.Refresh()
							}
						}
						first = false
					})
					if count == 0 {
						kit.Help(c, i18n.T("chat.noResources")).Padding(0, 4)
						return
					}
					kit.Panel(c, body)
				})
			}
			group("AGENTS.md", len(p.value.Agents), func() {
				for _, entry := range p.value.Agents {
					p.file(c.Key(entry.Path), "AGENTS.md", entry.Path, entry.Path, icons.ScrollText)
				}
			})
			group(i18n.T("chat.skills"), len(p.value.Skills), func() {
				for _, entry := range p.value.Skills {
					p.file(c.Key(entry.Path), entry.Name, entry.Description, entry.Path, icons.BookOpen)
				}
			})
			group(i18n.T("chat.plugins"), len(p.value.Plugins), func() {
				for _, entry := range p.value.Plugins {
					state := i18n.T("chat.disabled")
					if entry.Enabled {
						state = i18n.T("chat.enabled")
					}
					p.readonly(c.Key(entry.Path+entry.Name), entry.Name, entry.Path, state, icons.Puzzle)
				}
			})
			group("MCP", len(p.value.MCPServers), func() {
				for _, entry := range p.value.MCPServers {
					state := entry.Status
					if state == "" {
						state = i18n.T("chat.disabled")
						if entry.Enabled {
							state = i18n.T("chat.enabled")
						}
					}
					p.readonly(c.Key(entry.Name), entry.Name, entry.Error, state, icons.Plug)
				}
			})
			for _, diagnostic := range p.value.Diagnostics {
				kit.Help(c, diagnostic).Role(ui.RoleStatus)
			}
		})
	})
}

// resourceRow lays out one resource: its icon, its name over what describes
// it, and its state at the right.
func resourceRow(c *ui.Context, name, detail, state string, icon *ui.SVG) {
	t := c.Theme()
	ui.Row(c).Size(16, 21).Shrink(0).AlignItems(ui.Center).Children(func() { ui.Icon(c, icon).FontSize(15).TextColor(t.TextMuted) })
	ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
		ui.Text(c, name).LineHeight(1.5).MinWidth(0)
		if detail != "" {
			kit.Caption(c, detail).LineHeight(1.45).FillWidth().MinWidth(0)
		}
	})
	if state != "" {
		kit.Badge(c, state, kit.Info).Shrink(0)
	}
}

// file is a resource kept in a file, which opens beside the conversation.
func (p *ResourcePane) file(c *ui.Context, name, detail, path string, icon *ui.SVG) {
	row := ui.ButtonBase(c).FillWidth().MinWidth(0).Padding(10, 12).Gap(10).AlignItems(ui.Start).Label(name).Tooltip(path)
	if row.Hovered() {
		row.Background(c.Theme().SurfaceHover)
	}
	row.Children(func() { resourceRow(c, name, detail, "", icon) })
	if row.Clicked() && p.options.OnOpenFile != nil {
		p.options.OnOpenFile(path)
	}
	row.ContextMenu(func(m *ui.Menu) {
		if m.Item(i18n.T("chat.copyPath")).Chosen() {
			c.Services().WriteClipboard(path)
		}
		if p.options.OnReference != nil && m.Item(i18n.T("chat.fileReference")).Chosen() {
			p.options.OnReference(path)
		}
	})
}

// readonly is a resource this panel only lists: a plugin, a server.
func (p *ResourcePane) readonly(c *ui.Context, name, detail, state string, icon *ui.SVG) {
	ui.Row(c).FillWidth().MinWidth(0).Padding(10, 12).Gap(10).AlignItems(ui.Start).Children(func() {
		resourceRow(c, name, detail, state, icon)
	})
}
