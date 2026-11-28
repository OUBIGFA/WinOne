package wintray

import (
	"context"
	"strings"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/modules/wintray/applications"
)

// Adapted from WinTray 89c246a's installed-app picker. Selection, filters,
// original icons and cancellation stay upstream; additions enter WinOne's
// existing save/undo draft instead of saving the standalone product's file.
type installedAppPicker struct {
	open, loaded, loading bool
	query, kind, failure  string
	apps                  []applications.Application
	icons                 map[string]*ui.Bitmap
	selected              map[string]bool
	list                  ui.ListState
	cancel                context.CancelFunc
	generation            int
}

var enumerateInstalledApps = applications.Enumerate

func (p *page) openInstalledApps() {
	if p.installed == nil {
		p.installed = &installedAppPicker{kind: "all", selected: map[string]bool{}}
	}
	p.installed.open = true
	if !p.installed.loaded && !p.installed.loading {
		p.loadInstalledApps()
	}
}

func (p *page) loadInstalledApps() {
	picker := p.installed
	if picker.cancel != nil {
		picker.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	picker.cancel = cancel
	picker.generation++
	generation := picker.generation
	picker.loading, picker.failure = true, ""
	picker.selected = map[string]bool{}
	core.Go(p.env, func() ([]applications.Application, error) {
		defer cancel()
		return enumerateInstalledApps(ctx)
	}, func(apps []applications.Application, err error) {
		if generation != picker.generation || !picker.open {
			return
		}
		picker.loading, picker.cancel = false, nil
		if err != nil {
			picker.failure = err.Error()
			return
		}
		picker.apps, picker.loaded = apps, true
		picker.icons = make(map[string]*ui.Bitmap, len(apps))
		for _, app := range apps {
			if app.Icon != nil {
				picker.icons[app.Key()] = ui.NewBitmap(app.Icon)
			}
		}
		picker.list = ui.ListState{}
	})
}

// Close cancels only this page's catalog read; it never changes applications.
func (p *page) Close() {
	if p.installed != nil {
		p.installed.generation++
		if p.installed.cancel != nil {
			p.installed.cancel()
			p.installed.cancel = nil
		}
	}
}

func (p *page) addInstalledApplications(apps []applications.Application) {
	if p.blocked() || p.draft == nil {
		return
	}
	for _, app := range apps {
		value := newProgram(app.Path, app.Name)
		value.Value.Args, value.Value.WorkingDirectory = app.Args, app.WorkingDirectory
		p.draft.Value.ManagedApps = append(p.draft.Value.ManagedApps, value)
		p.selected = value.Value.ID
		if app.Icon != nil {
			p.meta[app.Path] = meta{name: app.Name, icon: ui.NewBitmap(app.Icon)}
			p.reading[app.Path] = true
		}
	}
}

func (p *page) installedApps(c *ui.Context) {
	picker := p.installed
	if picker == nil || !picker.open {
		return
	}
	width, height := c.Size()
	ui.Modal(c, &picker.open, func() {
		ui.Column(c).Width(min(720, width-48)).Height(min(580, height-64)).MinHeight(0).Gap(16).Children(func() {
			ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
				kit.Heading(c, i18n.T("wintray.installedTitle")).Grow(1)
				if kit.Button(c, i18n.T("wintray.installedRefresh"), nil, false).Disabled(picker.loading).Clicked() {
					p.loadInstalledApps()
				}
			})
			if kit.TextInput(c, &picker.query).FillWidth().Label(i18n.T("wintray.installedSearch")).Placeholder(i18n.T("wintray.installedSearch")).Changed() {
				picker.list = ui.ListState{}
			}
			if kit.Segmented(c, &picker.kind, []kit.SelectOption{
				{Value: "all", Label: i18n.T("wintray.installedAll")},
				{Value: "desktop", Label: i18n.T("wintray.installedDesktop")},
				{Value: "store", Label: i18n.T("wintray.installedStore")},
			}, i18n.T("wintray.installedTitle")) {
				picker.list = ui.ListState{}
			}
			var filtered []applications.Application
			query := strings.ToLower(strings.TrimSpace(picker.query))
			for _, app := range picker.apps {
				packaged := applications.IsPath(app.Path)
				if picker.kind == "store" && !packaged || picker.kind == "desktop" && packaged {
					continue
				}
				if query == "" || strings.Contains(strings.ToLower(app.Name+" "+app.Path+" "+app.Args), query) {
					filtered = append(filtered, app)
				}
			}
			ui.Column(c).FillWidth().Grow(1).MinHeight(0).Children(func() {
				switch {
				case picker.loading:
					kit.Help(c, i18n.T("wintray.installedLoading"))
				case picker.failure != "":
					kit.Notice(c, kit.Failure, i18n.T("wintray.installedFailed"), picker.failure, nil)
				case len(filtered) == 0:
					kit.Help(c, i18n.T("wintray.installedEmpty"))
				default:
					ui.List(c, &picker.list, len(filtered), func(index int) {
						app := filtered[index]
						key := app.Key()
						ui.Row(c).Key(key).FillWidth().Padding(8, 8).Gap(12).AlignItems(ui.Center).Children(func() {
							selected := picker.selected[key]
							choice := ui.Checkbox(c, &selected, "").Label(app.Name).Grow(1).Shrink(1).MinWidth(0).Description(strings.TrimSpace(app.Path + " " + app.Args))
							choice.Children(func() {
								if icon := picker.icons[key]; icon != nil {
									ui.Image(c, icon).Size(32, 32).Shrink(0)
								} else {
									ui.Icon(c, icons.AppWindow).Size(32, 32).Shrink(0).TextColor(c.Theme().TextMuted)
								}
								ui.Text(c, app.Name).SingleLine().Ellipsis("…").Grow(1).Shrink(1).MinWidth(0)
							})
							if choice.Changed() {
								if selected {
									picker.selected[key] = true
								} else {
									delete(picker.selected, key)
								}
							}
							kind := "wintray.installedDesktop"
							if applications.IsPath(app.Path) {
								kind = "wintray.installedStore"
							}
							kit.Caption(c, i18n.T(kind)).Shrink(0)
						})
					}).Fill().MinHeight(0).Label(i18n.T("wintray.installedTitle"))
				}
			})
			ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
				kit.Caption(c, i18n.T("wintray.installedSelected", "count", len(picker.selected))).Grow(1)
				if kit.Button(c, i18n.T("common.cancel"), nil, false).Clicked() {
					picker.open = false
				}
				if kit.PrimaryButton(c, i18n.T("wintray.installedAdd"), icons.Plus, false).Disabled(picker.loading || picker.failure != "" || len(picker.selected) == 0 || p.blocked()).Clicked() {
					var selected []applications.Application
					for _, app := range picker.apps {
						if picker.selected[app.Key()] {
							selected = append(selected, app)
						}
					}
					p.addInstalledApplications(selected)
					picker.selected, picker.open = map[string]bool{}, false
				}
			})
		})
	})
	if !picker.open && picker.cancel != nil {
		picker.generation++
		picker.cancel()
		picker.cancel, picker.loading = nil, false
	}
}
