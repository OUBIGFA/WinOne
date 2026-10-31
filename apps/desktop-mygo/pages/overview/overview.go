// Package overview is the Home page: every tool with its switch, module
// packages to install, update and remove, and the shared runtimes they use.
//
// React source: features/modules.tsx, module-installation.tsx,
// module-removal.tsx and package-types.ts. Changes from that layout: the
// tools are rows of one list instead of a grid of cards, success messages
// are toasts, and loading shows placeholder rows.
package overview

import (
	"slices"
	"strconv"
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
	shell.Register("overview", core.Settings, func(env *core.Env) core.Page { return newPage(env) })
}

type page struct {
	*widgets.Modules
	env *core.Env

	install installer
	removal removal
	details map[string]*bool
}

func newPage(env *core.Env) *page {
	p := &page{Modules: widgets.NewModules(env), env: env, details: map[string]*bool{}}
	p.install.page, p.removal.page = p, p
	return p
}

// moduleOrder is the shared order of Home and the sidebar (module-order.ts).
var moduleOrder = []string{"capture", "selection", "quick-assistant", "interconnect", "wintray", "convenience"}

func moduleRank(id string) int {
	if id == "always-on-top" {
		id = "convenience"
	}
	if rank := slices.Index(moduleOrder, id); rank >= 0 {
		return rank
	}
	return len(moduleOrder)
}

func (p *page) View(c *ui.Context) {
	busy := p.Busy != "" || p.install.busy || p.install.choosing || p.removal.busy
	p.env.Activity.Report("module-availability", false, busy, nil)
	kit.Page(c, func() {
		p.Notices(c)
		if p.Snapshot == nil {
			if p.Loading {
				kit.Skeleton(c, 5)
			} else {
				kit.Empty(c, icons.House, i18n.T("module.loading"), "", func() {
					if kit.Button(c, i18n.T("common.retry"), icons.RefreshCw, false).Clicked() {
						p.Err = ""
						p.Reload(nil)
					}
				})
			}
			return
		}
		p.tools(c)
		p.runtimes(c)
	})
	p.install.dialog(c)
	p.removal.dialog(c)
}

func (p *page) visible() []widgets.ModuleEntry {
	// Mouse gestures and timed power keep their own lifecycle inside
	// Convenience; Home lists the navigation modules.
	var entries []widgets.ModuleEntry
	for _, entry := range p.Snapshot.Modules {
		if entry.Manifest.ID != "stroke" && entry.Manifest.ID != "timed-power" {
			entries = append(entries, entry)
		}
	}
	slices.SortStableFunc(entries, func(a, b widgets.ModuleEntry) int { return moduleRank(a.Manifest.ID) - moduleRank(b.Manifest.ID) })
	return entries
}

func (p *page) tools(c *ui.Context) {
	s := p.Snapshot
	entries := p.visible()
	active, attention := 0, 0
	for _, entry := range entries {
		if entry.Enabled && !entry.PendingStop {
			active++
		}
	}
	for _, entry := range s.Modules {
		if entry.PendingStop || entry.Runtime.State == "faulted" || entry.Runtime.State == "unavailable" {
			attention++
		}
	}
	ui.Column(c).FillWidth().Gap(16).Children(func() {
		ui.Row(c).FillWidth().GapX(24).GapY(12).Wrap().AlignItems(ui.Center).Children(func() {
			kit.Help(c, i18n.T("module.intro")).Basis(240).Grow(1)
			ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
				kit.Caption(c, i18n.T("dashboard.enabledCount", "count", active, "total", len(entries))).SingleLine()
				p.install.button(c, p.Busy != "" || s.ConfigurationError != "")
			})
		})
		if attention > 0 {
			kit.Notice(c, kit.Info, "", i18n.T("dashboard.attention", "count", attention), nil)
		}
		if s.Cleanup != nil && s.Cleanup.Pending {
			message := s.Cleanup.Error
			if message == "" {
				message = i18n.T("module.cleanupPending")
			}
			kit.Notice(c, kit.Info, "", message, func() {
				if kit.Button(c, i18n.T("module.retryCleanup"), icons.RefreshCw, p.Busy == "cleanup").Disabled(p.Busy != "").Clicked() {
					p.Run(c, "cleanup", "cleanupPackages", map[string]any{}, i18n.T("module.cleanupFailed"), i18n.T("module.cleanupDone"))
				}
			})
		}
		kit.Panel(c, func() {
			for _, entry := range entries {
				p.toolRow(c, entry)
			}
		}).Role(ui.RoleList)
	})
}

var moduleIcons = map[string]*ui.SVG{}

func iconFor(id string) *ui.SVG {
	if len(moduleIcons) == 0 {
		moduleIcons = map[string]*ui.SVG{"capture": icons.ScanLine, "selection": icons.MousePointer2,
			"quick-assistant": icons.Astroid, "always-on-top": icons.Rocket, "wintray": icons.LaptopMinimal}
	}
	if icon := moduleIcons[id]; icon != nil {
		return icon
	}
	return icons.LaptopMinimal
}

func (p *page) toolRow(c *ui.Context, entry widgets.ModuleEntry) {
	t := c.Theme()
	s := p.Snapshot
	id := entry.Manifest.ID
	// Pinning is listed as Convenience, the page that holds it.
	grouped := id == "always-on-top"
	title := entry.Manifest.Name.String()
	if grouped {
		title = i18n.T("nav.convenience")
	}
	ui.Row(c.Key(id)).FillWidth().Padding(14, 16).GapX(14).GapY(10).AlignItems(ui.Start).Wrap().Role(ui.RoleListItem).Children(func() {
		ui.Icon(c, iconFor(id)).FontSize(20).TextColor(t.TextMuted).Shrink(0).Margin(1, 0, 0, 0)
		ui.Column(c).Basis(260).Grow(1).Shrink(1).MinWidth(0).Gap(3).Children(func() {
			kit.Strong(c, title).Role(ui.RoleHeading).Level(3)
			if grouped {
				kit.Label(c, i18n.T("nav.always-on-top"))
			}
			if key := "moduleDescription." + id; i18n.Has(key) {
				kit.Help(c, i18n.T(key))
			}
			p.toolExtras(c, entry)
		})
		ui.Row(c).Gap(12).AlignItems(ui.Center).Shrink(0).MinHeight(28).Children(func() {
			text := i18n.T(widgets.ModuleStatus(entry))
			if p.Busy == id {
				text = i18n.T("module.updated")
			}
			kit.Help(c, text).SingleLine()
			on := entry.Enabled && !entry.PendingStop
			sw := ui.Switch(c, &on).Label(i18n.T("module.enableTool", "name", title)).
				Disabled(p.Busy != "" || entry.PendingStop || entry.Blocked || s.ConfigurationError != "")
			if sw.Changed() {
				p.SetEnabled(c, entry, on)
			}
		})
	})
}

func (p *page) toolExtras(c *ui.Context, entry widgets.ModuleEntry) {
	s := p.Snapshot
	id := entry.Manifest.ID
	if entry.Blocked {
		kit.Help(c, i18n.T("module.packageBlocked"))
		open := p.detail("failure:" + id)
		ui.Collapsible(c, i18n.T("module.failureDetails"), open, func() {
			kit.Help(c, entry.Runtime.Detail).Selectable()
		}).FontSize(kit.HelpSize(c))
		label := "module.retryPackage"
		if entry.RecoveryID != "" {
			label = "module.restorePackage"
		}
		ui.Row(c).Margin(4, 0, 0, 0).Children(func() {
			if kit.Button(c, i18n.T(label), icons.RefreshCw, p.Busy == id).Disabled(p.Busy != "").Clicked() {
				p.Run(c, id, "recoverPackage", map[string]any{"id": id, "revision": s.Revision, "recoveryId": entry.RecoveryID},
					i18n.T("module.recoveryFailed"), i18n.T("module.packageRecovered", "name", entry.Manifest.Name.String()))
			}
		})
	}
	if entry.PendingStop && !entry.Blocked && s.ConfigurationError == "" {
		ui.Row(c).Margin(4, 0, 0, 0).Children(func() {
			if kit.Button(c, i18n.T("module.retryClose"), nil, false).Disabled(p.Busy != "").Clicked() {
				p.SetEnabled(c, entry, false)
			}
		})
	}
	if s.Packages != nil {
		if dependencies := s.Packages.Dependencies[id]; len(dependencies) > 0 {
			open := p.detail("dependencies:" + id)
			ui.Collapsible(c, i18n.T("sharedPackages.dependencies")+" · "+itoa(len(dependencies)), open, func() {
				for _, dependency := range dependencies {
					kit.Help(c, dependency.ID+" · "+dependency.Version).Selectable()
				}
			}).FontSize(kit.HelpSize(c))
		}
	}
	if entry.Manifest.Distribution == "optional" && !entry.Blocked {
		disabled := p.Busy != "" || entry.Enabled || entry.PendingStop || s.ConfigurationError != ""
		ui.Row(c).Margin(2, 0, 0, -10).Children(func() {
			b := kit.GhostButton(c, i18n.T("module.removePackage"), icons.Trash2, false).Disabled(disabled || p.removal.busy)
			if disabled {
				b.Tooltip(i18n.T("module.closeBeforeRemove"))
			}
			if b.Clicked() {
				p.removal.show(id, "", "")
			}
		})
	}
}

func (p *page) detail(key string) *bool {
	if open, ok := p.details[key]; ok {
		return open
	}
	open := new(bool)
	p.details[key] = open
	return open
}

// runtimes lists the shared runtime packages modules depend on.
func (p *page) runtimes(c *ui.Context) {
	s := p.Snapshot
	if s.Packages == nil || len(s.Packages.Runtimes) == 0 && s.Packages.Error == "" {
		return
	}
	kit.Section(c, icons.Package, i18n.T("sharedPackages.title"), i18n.T("sharedPackages.description"), func() {
		if s.Packages.Error != "" {
			kit.Notice(c, kit.Failure, "", s.Packages.Error, nil)
		}
		if len(s.Packages.Runtimes) == 0 {
			return
		}
		kit.Panel(c, func() {
			for _, runtime := range s.Packages.Runtimes {
				ui.Row(c.Key(runtime.ManifestSHA256)).FillWidth().Padding(12, 16).GapX(24).GapY(8).Wrap().AlignItems(ui.Center).Children(func() {
					ui.Column(c).Basis(256).Grow(1).Shrink(1).MinWidth(0).Gap(3).Children(func() {
						ui.Row(c).Gap(10).Wrap().AlignItems(ui.Baseline).Children(func() {
							kit.Strong(c, runtime.Name.String())
							kit.Caption(c, runtime.Version)
						})
						if runtime.Error != "" {
							kit.InlineError(c, runtime.Error)
						} else if len(runtime.RequiredBy) > 0 {
							names := make([]string, 0, len(runtime.RequiredBy))
							for _, reference := range runtime.RequiredBy {
								names = append(names, p.referenceName(reference))
							}
							kit.Help(c, i18n.T("sharedPackages.requiredBy", "names", strings.Join(names, "、")))
						} else {
							kit.Help(c, i18n.T("sharedPackages.unused"))
						}
					})
					disabled := p.Busy != "" || !runtime.CanRemove || s.ConfigurationError != ""
					b := kit.GhostButton(c, i18n.T("sharedPackages.remove"), icons.Trash2, false).Disabled(disabled || p.removal.busy)
					if !runtime.CanRemove {
						b.Tooltip(i18n.T("sharedPackages.inUse"))
					}
					if b.Clicked() {
						p.removal.show(runtime.ID, "runtime", runtime.ManifestSHA256)
					}
				})
			}
		})
	})
}

func (p *page) referenceName(reference widgets.PackageReference) string {
	for _, entry := range p.Snapshot.Modules {
		if entry.Manifest.ID == reference.ID {
			return entry.Manifest.Name.String()
		}
	}
	for _, runtime := range p.Snapshot.Packages.Runtimes {
		if runtime.ID == reference.ID {
			return runtime.Name.String()
		}
	}
	return reference.ID
}

func itoa(n int) string { return strconv.Itoa(n) }
