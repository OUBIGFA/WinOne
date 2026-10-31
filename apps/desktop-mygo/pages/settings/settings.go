// Package settings is the Settings page: appearance and the start page,
// startup and WinOne's own shortcut, the administrator preference,
// configuration export and import, and updates with diagnostics.
//
// React source: components/settings-panel.tsx, appearance-settings.tsx,
// default-page-setting.tsx, startup-settings.tsx, permission-settings.tsx,
// sidebar-preference.ts, features/shortcuts.tsx, profiles.tsx,
// profile-import.tsx, profile-settings-review.tsx, updates.tsx,
// diagnostics.tsx and the wiring in main.tsx. Changes from that layout:
// theme and language share one "Appearance" group of rows, the permission
// status names the row of the restart action, and success is a toast.
package settings

import (
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/desktop"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/shell"
	"winone/desktopmygo/widgets"
	internalsettings "winone/internal/settings"
)

func init() {
	shell.Register("settings", core.Settings, func(env *core.Env) core.Page { return newPage(env) })
}

const (
	tabGeneral = iota
	tabStartup
	tabPermissions
	tabProfiles
	tabAbout
)

// runPermission and exitWindow reach the administrator helper and end the
// window; tests replace them.
var (
	runPermission = func(env *core.Env, action desktop.PermissionAction, guard func() bool) (desktop.PermissionResult, error) {
		return desktop.RunPermission(env.Host, env.Directory, action, guard)
	}
	exitWindow = desktop.ExitWindow
)

type page struct {
	env *core.Env
	tab int
	// aboutOpened mounts updates and diagnostics the first time their tab
	// shows, as the React panel did; they stay afterwards.
	aboutOpened bool

	// settings is what the page shows: the shell's snapshot, replaced by
	// each save's result until the shell reads a newer one.
	settings      internalsettings.Settings
	loaded        bool
	administrator bool

	// working is a preference change or a permission operation in flight;
	// reconnecting is a read of the snapshot after a failure.
	working, reconnecting bool
	// err is the last preference failure, shown with Reconnect; while it
	// shows, the connection reads as lost (main.tsx's error state).
	err        string
	sidebarErr string

	restarting bool
	// closing is set once the Host committed a restart as administrator:
	// the window ends, so nothing may start any more.
	closing       bool
	permissionErr string

	shortcuts *widgets.Shortcuts
	export    exporter
	importer  importer
	updates   updater
	logs      logs
}

func newPage(env *core.Env) *page {
	p := &page{env: env, shortcuts: widgets.NewShortcuts(env, "application")}
	p.export.page, p.importer.page, p.logs.page, p.updates.env = p, p, p, env
	p.sync()
	return p
}

// sync takes the shell's snapshot when it is newer than what the page
// shows, as after a save elsewhere or the title bar's refresh.
func (p *page) sync() {
	snapshot := p.env.Snapshot()
	if snapshot.Settings.SchemaVersion == 0 {
		return
	}
	if !p.loaded || snapshot.Settings.Revision > p.settings.Revision {
		p.settings = snapshot.Settings
	}
	p.loaded, p.administrator = true, snapshot.Administrator
}

// Close implements core.Closer: leaving the page ends a recording and an
// update check in progress.
func (p *page) Close() {
	p.shortcuts.Close()
	p.updates.cancel()
}

func (p *page) View(c *ui.Context) {
	p.sync()
	// A restart that is closing the window holds no work the shell must
	// wait for: the guard would only keep the window open.
	p.env.Activity.Report("global-settings", false, p.working, nil)
	p.export.report()
	p.importer.report()
	if p.tab != tabStartup {
		p.shortcuts.Report()
	}
	busy := p.working || p.reconnecting || p.closing
	tabs := []kit.Tab{
		{Label: i18n.T("shell.general"), Icon: icons.Palette},
		{Label: i18n.T("settingsDesign.startupAndShortcuts"), Icon: icons.Power},
		{Label: i18n.T("permission.title"), Icon: icons.ShieldCheck},
		{Label: i18n.T("settingsDesign.profileTransfer"), Icon: icons.FolderSync},
		{Label: i18n.T("settingsDesign.aboutAndDiagnostics"), Icon: icons.Info},
	}
	kit.Page(c, func() {
		kit.Tabs(c, &p.tab, tabs, p.env.RequestChange)
		if p.tab == tabAbout && !p.aboutOpened {
			p.aboutOpened = true
			p.updates.load()
			p.logs.read()
		}
		// A failed preference stays visible on every tab, below the tabs.
		if p.err != "" {
			kit.Notice(c, kit.Failure, i18n.T("common.failed"), p.err, func() {
				if kit.Button(c, i18n.T("common.reconnect"), icons.RefreshCw, p.reconnecting).Disabled(busy).Clicked() {
					p.reconnect()
				}
			})
		}
		// The forms of the first tabs are fieldsets: a running change
		// disables all of them, as the React panel did.
		content := ui.Column(c).FillWidth().Gap(kit.GroupGap)
		switch p.tab {
		case tabGeneral:
			content.Disabled(busy).Children(func() { p.general(c, busy || !p.loaded) })
		case tabStartup:
			content.Disabled(busy).Children(func() { p.startup(c, busy || !p.loaded) })
		case tabPermissions:
			content.Children(func() { p.permissions(c, busy) })
		case tabProfiles:
			content.Disabled(busy).Children(func() { p.profiles(c) })
		case tabAbout:
			content.Children(func() { p.about(c) })
		}
		if p.working && !p.restarting {
			kit.Status(c, i18n.T("settingsDesign.saving"))
		}
	})
}

// change saves the preference the user just chose. It reads the Host's
// settings at that moment and changes only that field, so a save another
// page made in between is kept and a conflicting write fails rather than
// overwriting it (bridge.ts savePreferenceChange).
func (p *page) change(edit func(*internalsettings.Settings)) {
	if !p.loaded || p.working || p.reconnecting || p.closing {
		return
	}
	p.working, p.err = true, ""
	core.Go(p.env, func() (internalsettings.Settings, error) {
		var snapshot struct {
			Settings core.Preserved[internalsettings.Settings] `json:"settings"`
		}
		if err := p.env.Host.CallInto("snapshot", nil, &snapshot); err != nil {
			return internalsettings.Settings{}, err
		}
		edit(&snapshot.Settings.Value)
		var saved internalsettings.Settings
		err := p.env.Host.CallCompletingInto("settings.save", snapshot.Settings, &saved)
		return saved, err
	}, func(saved internalsettings.Settings, err error) {
		p.working = false
		if err != nil {
			p.err = err.Error()
			return
		}
		p.settings = saved
		// Said in the language just saved, which the window takes next.
		p.env.Toast(i18n.Translate(i18n.Resolve(saved.Language, p.env.Snapshot().Locale), "common.saved"))
		// The shell applies the theme, the language and whether closing
		// keeps the window, from the snapshot it reads again.
		p.env.Refresh()
	})
}

// snapshotReply is the part of the Host's snapshot the page reads itself.
type snapshotReply struct {
	Settings      internalsettings.Settings `json:"settings"`
	Administrator bool                      `json:"administrator"`
}

// reconnect reads the snapshot again after a failure, then lets the shell
// read it too.
func (p *page) reconnect() {
	if p.reconnecting {
		return
	}
	p.reconnecting = true
	core.Go(p.env, func() (snapshotReply, error) {
		var snapshot snapshotReply
		err := p.env.Host.CallInto("snapshot", nil, &snapshot)
		return snapshot, err
	}, func(snapshot snapshotReply, err error) {
		p.reconnecting = false
		if err != nil {
			p.err = err.Error()
			return
		}
		p.err, p.settings, p.administrator, p.loaded = "", snapshot.Settings, snapshot.Administrator, true
		p.env.Refresh()
	})
}

func (p *page) general(c *ui.Context, disabled bool) {
	kit.Group(c, icons.Palette, i18n.T("shell.appearance"), "", func() {
		theme := p.settings.Theme
		if theme == "" {
			theme = "system"
		}
		kit.Row(c, i18n.T("shell.theme"), "", func() {
			ui.Row(c).Disabled(disabled).Children(func() {
				if kit.Segmented(c, &theme, []kit.SelectOption{
					{Value: "system", Label: i18n.T("shell.system")},
					{Value: "light", Label: i18n.T("shell.light")},
					{Value: "dark", Label: i18n.T("shell.dark")},
				}, i18n.T("shell.theme")) {
					p.change(func(s *internalsettings.Settings) { s.Theme = theme })
				}
			})
		})
		language := p.settings.Language
		if language == "" {
			language = i18n.Chinese
		}
		kit.Row(c, i18n.T("shell.language"), "", func() {
			ui.Row(c).Disabled(disabled).Children(func() {
				// Languages are named in their own language, whatever the
				// interface's.
				if kit.Segmented(c, &language, []kit.SelectOption{
					{Value: "system", Label: i18n.T("shell.system")},
					{Value: i18n.Chinese, Label: "简体中文"},
					{Value: i18n.English, Label: "English"},
				}, i18n.T("shell.language")) {
					p.change(func(s *internalsettings.Settings) { s.Language = language })
				}
			})
		})
	})
	kit.Section(c, icons.PanelLeft, i18n.T("settingsDesign.defaultView"), "", func() {
		kit.Panel(c, func() {
			sidebar := p.env.SidebarStartup()
			kit.Row(c, i18n.T("settingsDesign.sidebar"), "", func() {
				ui.Row(c).Disabled(disabled).Children(func() {
					if kit.Segmented(c, &sidebar, []kit.SelectOption{
						{Value: "collapsed", Label: i18n.T("settingsDesign.sidebarIconsOnly")},
						{Value: "expanded", Label: i18n.T("settingsDesign.sidebarIconsAndNames")},
					}, i18n.T("settingsDesign.sidebar")) {
						// The choice is the window's own, kept beside the
						// Host's settings; a failed write keeps the old one.
						p.sidebarErr = ""
						if err := p.env.SetSidebarStartup(sidebar); err != nil {
							p.sidebarErr = i18n.T("shell.sidebarSaveFailed")
						}
					}
				})
			})
			kit.Row(c, i18n.T("settingsDesign.defaultPage"), "", func() { p.defaultPage(c, disabled) })
		})
		if p.sidebarErr != "" {
			kit.Notice(c, kit.Failure, "", p.sidebarErr, nil)
		}
	})
}

// defaultPage chooses the page the window opens on from the sidebar's own
// list, so a new page is never missing from it.
func (p *page) defaultPage(c *ui.Context, disabled bool) {
	catalog := shell.Catalog()
	options := make([]kit.SelectOption, len(catalog))
	for i, page := range catalog {
		options[i] = kit.SelectOption{Value: page.ID, Label: i18n.T(page.Title), Icon: page.Icon}
	}
	selected := p.settings.DefaultPage
	if !slices.ContainsFunc(catalog, func(page shell.PageInfo) bool { return page.ID == selected }) {
		selected = "overview"
	}
	changed, trigger := kit.Select(c, &selected, options)
	trigger.Disabled(disabled).Label(i18n.T("settingsDesign.defaultPage")).MaxWidth(320)
	for _, option := range options {
		if option.Value == selected {
			trigger.Tooltip(option.Label)
		}
	}
	if changed {
		p.change(func(s *internalsettings.Settings) { s.DefaultPage = selected })
	}
}

func (p *page) startup(c *ui.Context, disabled bool) {
	startup := internalsettings.StartupPreferences{StartMinimized: true}
	if p.settings.Startup != nil {
		startup = *p.settings.Startup
	}
	keep := startup.KeepUIRunning == nil || *startup.KeepUIRunning
	kit.Group(c, icons.Power, i18n.T("startup.title"), "", func() {
		// Each switch saves its own field, merged into the Host's current
		// startup preferences.
		save := func(edit func(*internalsettings.StartupPreferences)) {
			p.change(func(s *internalsettings.Settings) {
				if s.Startup == nil {
					s.Startup = &internalsettings.StartupPreferences{StartMinimized: true}
				}
				edit(s.Startup)
			})
		}
		logon, silent := startup.RunAtLogon, startup.StartMinimized
		if kit.Toggle(c, &logon, i18n.T("startup.logon"), i18n.T("startup.logonHint")).Disabled(disabled).Changed() {
			save(func(s *internalsettings.StartupPreferences) { s.RunAtLogon = logon })
		}
		if kit.Toggle(c, &silent, i18n.T("startup.silent"), i18n.T("startup.silentHint")).Disabled(disabled).Changed() {
			save(func(s *internalsettings.StartupPreferences) { s.StartMinimized = silent })
		}
		if kit.Toggle(c, &keep, i18n.T("startup.keepUI"), i18n.T("startup.keepUIHint")).Disabled(disabled).Changed() {
			value := keep
			save(func(s *internalsettings.StartupPreferences) { s.KeepUIRunning = &value })
		}
	})
	p.shortcuts.View(c)
	if p.working || p.reconnecting || p.closing {
		// The editor's Save and Undo live in the rail, outside this
		// fieldset: keep them unavailable while a preference saves.
		save, undo := p.env.Rail.Save, p.env.Rail.Undo
		save.Disabled, undo.Disabled = true, true
		p.env.Rail.Set(save, undo)
	}
}

func (p *page) permissions(c *ui.Context, busy bool) {
	dirty, active := p.env.Activity.State()
	disabled := busy || !p.loaded || dirty || active
	remembered := p.settings.Startup != nil && p.settings.Startup.PreferAdministrator
	status := "permission.standard"
	switch {
	case !p.loaded:
		status = "permission.loading"
	case p.administrator:
		status = "permission.administrator"
	}
	kit.Group(c, icons.ShieldCheck, i18n.T("permission.title"), "", func() {
		kit.Row(c, i18n.T(status), "", func() {
			label := "permission.restart"
			if p.restarting {
				label = "permission.restarting"
			}
			b := kit.Button(c, i18n.T(label), icons.ShieldCheck, p.restarting).Disabled(disabled || !p.loaded || p.administrator)
			if b.Clicked() {
				p.restart()
			}
		})
		helps := []string{i18n.T("permission.consent")}
		switch {
		case remembered && !p.administrator:
			helps = append(helps, i18n.T("permission.nextStart"))
		case !remembered && p.administrator:
			helps = append(helps, i18n.T("permission.untilExit"))
		}
		on := remembered
		if kit.Toggle(c, &on, i18n.T("permission.remember"), strings.Join(helps, "\n")).Disabled(disabled).Changed() {
			p.setAdministrator(on)
		}
	})
	kit.InlineError(c, p.permissionErr)
}

// permissionAllowed is the React page's guard for the administrator
// actions: no draft and no operation anywhere on the page.
func (p *page) permissionAllowed() bool {
	dirty, busy := p.env.Activity.State()
	return p.loaded && !p.working && !p.reconnecting && !p.closing && !dirty && !busy
}

// guard answers the helper's question before it changes anything: the
// window must hold no unsaved edit. It asks the UI thread, which owns the
// page's activity, and treats no answer within a second as "not clean".
func (p *page) guard() bool {
	answer := make(chan bool, 1)
	p.env.Post(func() {
		dirty, _ := p.env.Activity.State()
		answer <- !dirty && !p.closing
	})
	select {
	case clean := <-answer:
		return clean
	case <-time.After(time.Second):
		return false
	}
}

func (p *page) setAdministrator(enabled bool) {
	if !p.permissionAllowed() {
		return
	}
	p.working, p.err, p.permissionErr = true, "", ""
	core.Go(p.env, func() (desktop.PermissionResult, error) {
		return runPermission(p.env, desktop.PermissionAction{Enabled: enabled}, p.guard)
	}, func(result desktop.PermissionResult, err error) {
		p.working = false
		if err != nil {
			p.permissionErr = err.Error()
			return
		}
		var saved internalsettings.Settings
		if json.Unmarshal(result.Settings, &saved) == nil {
			p.settings = saved
		}
		p.env.Toast(i18n.T("common.saved"))
		p.env.Refresh()
	})
}

func (p *page) restart() {
	if !p.permissionAllowed() {
		return
	}
	p.working, p.restarting, p.err, p.permissionErr = true, true, "", ""
	core.Go(p.env, func() (desktop.PermissionResult, error) {
		return runPermission(p.env, desktop.PermissionAction{Restart: true}, p.guard)
	}, func(result desktop.PermissionResult, err error) {
		p.working = false
		switch {
		case err != nil:
			p.restarting, p.permissionErr = false, err.Error()
		case !result.Closing:
			// Already an administrator window: nothing restarts.
			p.restarting = false
			p.reconnect()
		default:
			// The Host committed the handoff and waits for this process to
			// end; the administrator window opens in its place.
			p.closing = true
			p.env.Activity.Report("global-settings", false, false, nil)
			if err := exitWindow(); err != nil {
				p.closing, p.restarting, p.permissionErr = false, false, err.Error()
			}
		}
	})
}
