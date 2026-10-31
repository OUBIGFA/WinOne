// Package overview is the Home page: every tool with its switch, module
// packages to install, update and remove, and the shared runtimes they use.
//
// React source: features/modules.tsx, module-installation.tsx,
// module-removal.tsx and package-types.ts. Changes from that layout: the
// tools are rows of one list instead of a grid of cards, success messages
// are toasts, and loading shows placeholder rows.
package overview

import (
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

// homeModules names, for each feature of the sidebar, the module whose
// switch Home shows for it. Home lists the sidebar's features in its
// order (shell.Features); its test fails for a feature it does not know,
// so adding or removing one in the sidebar changes Home too.
var homeModules = map[string]string{"capture": "capture", "selection": "selection", "quick-assistant": "quick-assistant",
	"wintray": "wintray", "convenience": "always-on-top"}

// builtIn are the features without a module: always available, no switch.
var builtIn = map[string]bool{"interconnect": true}

// homeRow is one feature of Home: its sidebar page, and its module unless
// it is built in.
type homeRow struct {
	page  shell.PageInfo
	entry *widgets.ModuleEntry
}

// rows lists the sidebar's features in its order, each with its module.
// A module the snapshot lacks (an optional one not installed) leaves its
// feature out, as the sidebar's page then has nothing to show.
func (p *page) rows() []homeRow {
	var rows []homeRow
	for _, feature := range shell.Features() {
		row := homeRow{page: feature}
		if id, ok := homeModules[feature.ID]; ok {
			for i := range p.Snapshot.Modules {
				if p.Snapshot.Modules[i].Manifest.ID == id {
					row.entry = &p.Snapshot.Modules[i]
				}
			}
			if row.entry == nil {
				continue
			}
		} else if !builtIn[feature.ID] {
			continue
		}
		rows = append(rows, row)
	}
	return rows
}

// others are the installed modules no feature of the sidebar holds:
// optional packages, kept here to switch and remove. Mouse gestures and
// timed power keep their lifecycle inside Convenience.
func (p *page) others() []widgets.ModuleEntry {
	held := map[string]bool{"stroke": true, "timed-power": true}
	for _, id := range homeModules {
		held[id] = true
	}
	var entries []widgets.ModuleEntry
	for _, entry := range p.Snapshot.Modules {
		if !held[entry.Manifest.ID] {
			entries = append(entries, entry)
		}
	}
	return entries
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

func (p *page) tools(c *ui.Context) {
	s := p.Snapshot
	rows := p.rows()
	active, attention := 0, 0
	for _, row := range rows {
		if row.entry == nil || row.entry.Enabled && !row.entry.PendingStop {
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
				kit.Caption(c, i18n.T("dashboard.enabledCount", "count", active, "total", len(rows))).SingleLine()
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
			for _, row := range rows {
				if row.entry == nil {
					p.builtInRow(c, row.page)
				} else {
					p.toolRow(c, *row.entry, row.page.Icon, i18n.T(row.page.Title))
				}
			}
		}).Role(ui.RoleList)
		if others := p.others(); len(others) > 0 {
			kit.Section(c, icons.Package, i18n.T("overview.otherModules"), "", func() {
				kit.Panel(c, func() {
					for _, entry := range others {
						p.toolRow(c, entry, icons.Package, entry.Manifest.Name.String())
					}
				}).Role(ui.RoleList)
			})
		}
	})
}

// builtInRow is a feature without a module: always available.
func (p *page) builtInRow(c *ui.Context, page shell.PageInfo) {
	t := c.Theme()
	title := i18n.T(page.Title)
	ui.Row(c.Key(page.ID)).FillWidth().Padding(14, 16).GapX(14).GapY(10).AlignItems(ui.Start).Wrap().Role(ui.RoleListItem).Children(func() {
		ui.Icon(c, page.Icon).FontSize(20).TextColor(t.TextMuted).Shrink(0).Margin(1, 0, 0, 0)
		ui.Column(c).Basis(260).Grow(1).Shrink(1).MinWidth(0).Gap(3).Children(func() {
			kit.Strong(c, title).Role(ui.RoleHeading).Level(3)
			if key := "moduleDescription." + page.ID; i18n.Has(key) {
				kit.Help(c, i18n.T(key))
			}
		})
		ui.Row(c).Gap(12).AlignItems(ui.Center).Shrink(0).MinHeight(28).Children(func() {
			kit.Help(c, i18n.T("overview.alwaysAvailable")).SingleLine()
		})
	})
}

// toolRow is a module's row: its feature's icon and name, its state and
// switch, and what needs the user's attention.
func (p *page) toolRow(c *ui.Context, entry widgets.ModuleEntry, icon *ui.SVG, title string) {
	t := c.Theme()
	s := p.Snapshot
	id := entry.Manifest.ID
	// Pinning is listed as Convenience, the page that holds it.
	grouped := id == "always-on-top"
	ui.Row(c.Key(id)).FillWidth().Padding(14, 16).GapX(14).GapY(10).AlignItems(ui.Start).Wrap().Role(ui.RoleListItem).Children(func() {
		ui.Icon(c, icon).FontSize(20).TextColor(t.TextMuted).Shrink(0).Margin(1, 0, 0, 0)
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
