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
	ui.Column(c).Fill().MinWidth(0).Gap(8).Children(func() {
		ui.Row(c).FillWidth().Padding(8).Gap(6).Children(func() {
			kit.Label(c, i18n.T("chat.resources")).Grow(1)
			if kit.IconButton(c, icons.RefreshCw, i18n.T("chat.refresh")).Disabled(p.loading || p.options.SessionID == "").Clicked() {
				p.Refresh()
			}
		})
		kit.InlineError(c, p.err)
		if p.options.SessionID == "" {
			kit.Help(c, i18n.T("chat.selectConversation"))
			return
		}
		if p.loading && !p.loaded {
			kit.Loader(c)
		}
		if !p.loaded {
			return
		}
		ui.Scroll(c).Grow(1).MinHeight(0).FillWidth().Padding(12).Gap(18).Children(func() {
			p.group(c, "AGENTS.md", len(p.value.Agents), func() {
				for _, entry := range p.value.Agents {
					p.file(c.Key(entry.Path), "AGENTS.md", entry.Path, entry.Path, icons.ScrollText)
				}
			})
			p.group(c, i18n.T("chat.skills"), len(p.value.Skills), func() {
				for _, entry := range p.value.Skills {
					p.file(c.Key(entry.Path), entry.Name, entry.Description, entry.Path, icons.BookOpen)
				}
			})
			p.group(c, i18n.T("chat.plugins"), len(p.value.Plugins), func() {
				for _, entry := range p.value.Plugins {
					state := i18n.T("chat.disabled")
					if entry.Enabled {
						state = i18n.T("chat.enabled")
					}
					p.readonly(c, entry.Name, entry.Path+" · "+state, icons.Puzzle)
				}
			})
			p.group(c, "MCP", len(p.value.MCPServers), func() {
				for _, entry := range p.value.MCPServers {
					state := entry.Error
					if state == "" {
						state = entry.Status
					}
					if state == "" {
						state = i18n.T("chat.disabled")
						if entry.Enabled {
							state = i18n.T("chat.enabled")
						}
					}
					p.readonly(c, entry.Name, state, icons.Plug)
				}
			})
			for _, diagnostic := range p.value.Diagnostics {
				kit.Help(c, diagnostic).Role(ui.RoleStatus)
			}
		})
	})
}
func (p *ResourcePane) group(c *ui.Context, name string, count int, body func()) {
	kit.Section(c, nil, fmt.Sprintf("%s · %d", name, count), "", func() {
		if count == 0 {
			kit.Help(c, i18n.T("chat.noResources"))
		} else {
			body()
		}
	})
}
func (p *ResourcePane) file(c *ui.Context, name, detail, path string, icon *ui.SVG) {
	row := kit.GhostButton(c, name, icon, false).FillWidth().MinWidth(0).Justify(ui.Start).Tooltip(path)
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
	if detail != "" {
		kit.Help(c, detail).FillWidth()
	}
}
func (p *ResourcePane) readonly(c *ui.Context, name, detail string, icon *ui.SVG) {
	ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Start).Children(func() {
		ui.Icon(c, icon).FontSize(16).Shrink(0)
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() { kit.Label(c, name); kit.Help(c, detail).FillWidth() })
	})
}
