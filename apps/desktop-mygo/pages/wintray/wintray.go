// Package wintray is the Program management page: the programs WinOne
// starts at sign-in, how and when each starts, its tray behavior, and the
// module's own startup timing and reset.
//
// React source: features/wintray.tsx, wintray-program-actions.tsx,
// wintray-program-settings.tsx, wintray-maintenance.tsx,
// program-icon-cache.ts and wintray.css. Changes from that layout:
// success messages are toasts, and the status beside Save moved to the
// toolbar above the list.
package wintray

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
)

func init() {
	shell.Register("wintray", core.Workspace, func(env *core.Env) core.Page { return newPage(env) })
}

// programInfo reads a program's name and icon; tests replace it.
var programInfo = desktop.ProgramInfo

const (
	tabPrograms = iota
	tabStartup
)

// meta is what File Explorer shows for a program file.
type meta struct {
	name string
	icon *ui.Bitmap
}

type page struct {
	env *core.Env
	tab int

	saved *traySnapshot
	draft *core.Preserved[traySettings]
	// disabled is the module's state the settings were read for.
	disabled bool
	selected string
	busy     bool
	adding   bool
	// job is the module operation running, as "settings.save".
	job     string
	err     string
	loading bool

	// texts holds number fields being edited, by field: what is typed
	// stays as typed until it is a whole number in range.
	texts map[string]string
	// handles are the inputs their labels focus, by field.
	handles map[string]*ui.Handle
	// detailTab is the program settings' sub-tab, chosen again for each
	// program (detailFor).
	detailTab int
	detailFor string

	meta    map[string]meta
	reading map[string]bool

	split kit.SplitState
	// header and workspace measure the page, so the list and the details
	// fill the window and scroll on their own.
	header, workspace ui.Handle

	state       programState
	stopConfirm bool
	reset       maintenance
}

func newPage(env *core.Env) *page {
	p := &page{env: env, meta: map[string]meta{}, reading: map[string]bool{}, texts: map[string]string{}, handles: map[string]*ui.Handle{}}
	p.split.Percent = 40
	p.disabled = p.moduleDisabled()
	p.reload(false, nil)
	return p
}

// moduleDisabled reads the module's state from the shell's snapshot.
func (p *page) moduleDisabled() bool {
	for _, module := range p.env.Snapshot().Modules {
		if module.ID == "wintray" {
			return module.State == "disabled"
		}
	}
	return false
}

// reload reads the module's settings: from its worker when it runs, else
// from the configuration. preserve keeps the draft and takes only the
// worker's errors, as after a failed operation. done runs once read.
func (p *page) reload(preserve bool, done func()) {
	if !preserve {
		p.err = ""
	}
	method := "wintray.snapshot"
	if p.disabled {
		method = "wintray.settings.get"
	}
	p.loading = true
	core.Go(p.env, func() (traySnapshot, error) {
		var next traySnapshot
		err := p.env.Host.CallCompletingInto(method, nil, &next)
		return next, err
	}, func(next traySnapshot, err error) {
		p.loading = false
		defer func() {
			if done != nil {
				done()
			}
		}()
		if err != nil {
			p.err = err.Error()
			return
		}
		if preserve && p.saved != nil {
			p.saved.RecoveryError, p.saved.BackgroundError, p.saved.StartupError = next.RecoveryError, next.BackgroundError, next.StartupError
		} else {
			p.saved = &next
		}
		if !preserve {
			apps := next.Settings.Value.ManagedApps
			if !slices.ContainsFunc(apps, func(app core.Preserved[program]) bool { return app.Value.ID == p.selected }) {
				p.selected = ""
				if len(apps) > 0 {
					p.selected = apps[0].Value.ID
				}
			}
			draft := clone(next.Settings)
			p.draft = &draft
			clear(p.texts)
		}
		if next.RecoveryError != "" {
			p.err = next.RecoveryError
		} else if next.BackgroundError != "" {
			p.err = next.BackgroundError
		}
	})
}

func (p *page) dirty() bool {
	return p.saved != nil && p.draft != nil && (!same(p.saved.Settings, *p.draft) || len(p.texts) > 0 && p.anyInvalid())
}

func (p *page) recovering() bool { return p.saved != nil && p.saved.RecoveryError != "" }

// blocked is when editing stops: an operation runs or the settings must be
// recovered first.
func (p *page) blocked() bool { return p.busy || p.recovering() }

func (p *page) apps() []core.Preserved[program] {
	if p.draft == nil {
		return nil
	}
	return p.draft.Value.ManagedApps
}

func (p *page) active() *program {
	if p.draft == nil {
		return nil
	}
	for i := range p.draft.Value.ManagedApps {
		if p.draft.Value.ManagedApps[i].Value.ID == p.selected {
			return &p.draft.Value.ManagedApps[i].Value
		}
	}
	return nil
}

// update changes the program id in the draft, unless editing is blocked.
func (p *page) update(id string, edit func(*program)) {
	if p.blocked() || p.draft == nil {
		return
	}
	for i := range p.draft.Value.ManagedApps {
		if p.draft.Value.ManagedApps[i].Value.ID == id {
			edit(&p.draft.Value.ManagedApps[i].Value)
		}
	}
}

func (p *page) reorder(from, to string) {
	if p.blocked() || p.draft == nil || from == to {
		return
	}
	apps := p.draft.Value.ManagedApps
	source := slices.IndexFunc(apps, func(app core.Preserved[program]) bool { return app.Value.ID == from })
	target := slices.IndexFunc(apps, func(app core.Preserved[program]) bool { return app.Value.ID == to })
	if source < 0 || target < 0 {
		return
	}
	p.draft.Value.ManagedApps = move(apps, source, target)
}

// displayName is the saved name, else what the file says, else its name:
// an emptied name still identifies the row.
func (p *page) displayName(app program) string {
	if strings.TrimSpace(app.Name) != "" {
		return app.Name
	}
	if m := p.meta[app.ExePath]; m.name != "" {
		return m.name
	}
	if name := baseName(app.ExePath); name != "" {
		return name
	}
	return i18n.T("wintray.name")
}

func programFilter() core.Filter {
	return core.Filter{Name: i18n.T("wintray.programFilter"), Extensions: []string{"exe", "bat", "cmd", "ps1", "py", "pyw"}}
}

// remember keeps a program's name and icon, decoding the icon on the UI
// thread, which owns bitmaps. A file read without an icon keeps the icon
// read before.
func (p *page) remember(path string, info desktop.ProgramMetadata) {
	next := meta{name: info.Name, icon: p.meta[path].icon}
	if len(info.Icon) > 0 {
		if bitmap, err := ui.DecodeBitmap(info.Icon); err == nil {
			next.icon = bitmap
		}
	}
	p.meta[path] = next
}

// readIcons reads the names and icons of the programs not read yet on this
// visit; the profile's cache keeps them across visits.
func (p *page) readIcons() {
	for _, app := range p.apps() {
		path := app.Value.ExePath
		if p.reading[path] {
			continue
		}
		p.reading[path] = true
		directory := p.env.Directory
		core.Go(p.env, func() (desktop.ProgramMetadata, error) { return programInfo(directory, path) }, func(info desktop.ProgramMetadata, err error) {
			// A file that is unavailable for now keeps its cached icon.
			if err == nil {
				p.remember(path, info)
			}
		})
	}
}

// add adds programs chosen in the dialog, or paths dropped on the page,
// with their names and icons; one unreadable file adds none.
func (p *page) add(paths []string) {
	if p.busy || p.draft == nil || p.recovering() {
		return
	}
	p.busy, p.adding, p.err = true, true, ""
	env := p.env
	type added struct {
		paths []string
		infos []desktop.ProgramMetadata
	}
	core.Go(env, func() (added, error) {
		if paths == nil {
			picked, err := env.Dialogs.PickFiles(i18n.T("wintray.pickPrograms"), programFilter())
			if err != nil || len(picked) == 0 {
				return added{}, err
			}
			paths = picked
		}
		infos := make([]desktop.ProgramMetadata, len(paths))
		errs := make([]error, len(paths))
		done := make(chan struct{}, len(paths))
		for i, path := range paths {
			go func() {
				infos[i], errs[i] = programInfo(env.Directory, path)
				done <- struct{}{}
			}()
		}
		for range paths {
			<-done
		}
		for _, err := range errs {
			if err != nil {
				return added{}, err
			}
		}
		return added{paths, infos}, nil
	}, func(result added, err error) {
		p.busy, p.adding = false, false
		if err != nil {
			p.err = err.Error()
			return
		}
		if len(result.paths) == 0 || p.draft == nil {
			return
		}
		for i, path := range result.paths {
			p.remember(path, result.infos[i])
			p.reading[path] = true
			app := newProgram(path, result.infos[i].Name)
			p.draft.Value.ManagedApps = append(p.draft.Value.ManagedApps, app)
			p.selected = app.Value.ID
		}
	})
}

// replace points a program at another file chosen in the dialog.
func (p *page) replace(id string) {
	if p.busy {
		return
	}
	p.busy, p.err = true, ""
	env := p.env
	type chosen struct {
		path string
		info desktop.ProgramMetadata
	}
	core.Go(env, func() (chosen, error) {
		path, err := env.Dialogs.PickFile(i18n.T("wintray.pickProgram"), programFilter())
		if err != nil || path == "" {
			return chosen{}, err
		}
		info, err := programInfo(env.Directory, path)
		return chosen{path, info}, err
	}, func(result chosen, err error) {
		p.busy = false
		if err != nil {
			p.err = err.Error()
			return
		}
		if result.path == "" || p.draft == nil {
			return
		}
		p.remember(result.path, result.info)
		p.reading[result.path] = true
		for i := range p.draft.Value.ManagedApps {
			if app := &p.draft.Value.ManagedApps[i].Value; app.ID == id {
				*app = replaced(*app, result.path, result.info.Name)
			}
		}
	})
}

// runJob runs a module operation and follows its job to the end, then
// reads the settings again; a failure keeps the draft.
func (p *page) runJob(method string, params any) {
	if method == "settings.save" && (p.anyInvalid() || len(p.unnamed()) > 0) {
		return
	}
	if p.busy {
		return
	}
	p.busy, p.job, p.err = true, method, ""
	env := p.env
	core.Go(env, func() (core.ModuleJob, error) {
		var job core.ModuleJob
		raw, err := env.Host.CallCompleting("wintray."+method, params)
		if err != nil {
			return job, err
		}
		// A configuration commit completes without a job ID; a running
		// worker operation must keep its ID until its result arrives.
		if json.Unmarshal(raw, &job) != nil || string(raw) == "null" || job.State == "running" && job.ID == "" {
			return job, core.Errorf(i18n.T("wintray.unknownResult"))
		}
		deadline := time.Now().Add(5 * time.Minute)
		for job.State == "running" {
			if time.Now().After(deadline) {
				return job, core.Errorf(i18n.T("wintray.pending"))
			}
			time.Sleep(250 * time.Millisecond)
			id := job.ID
			if err := env.Host.CallCompletingInto("wintray.jobs.get", map[string]string{"id": id}, &job); err != nil {
				return job, err
			}
			if job.ID != id {
				return job, core.Errorf(i18n.T("wintray.unknownResult"))
			}
		}
		if job.State != "completed" {
			if job.Error != "" {
				return job, core.Errorf(job.Error)
			}
			return job, core.Errorf(i18n.T("wintray.failed"))
		}
		return job, nil
	}, func(_ core.ModuleJob, err error) {
		finish := func() { p.busy, p.job = false, "" }
		if err != nil {
			p.err = err.Error()
			p.reload(true, finish)
			return
		}
		message := map[string]string{"launch": "wintray.launched", "program.window": "wintray.windowChanged",
			"program.stop": "wintray.stopped", "recover": "wintray.recovered"}[method]
		if message == "" {
			message = "common.saved"
		}
		env.Toast(i18n.T(message))
		p.state.refresh()
		p.reload(false, finish)
	})
}

// fieldKeys are the number fields of the page and its programs, as texts
// keys them.
const intervalKey, windowWaitKey = "startup/interval", "startup/windowWait"

func programKey(id, field string) string { return id + "/" + field }

// anyInvalid reports a number field holding what is not a whole number in
// its range; such a field blocks saving even when it is hidden.
func (p *page) anyInvalid() bool { return len(p.invalidSections()) > 0 }

// invalidSections names where invalid numbers are: each program, then the
// startup settings.
func (p *page) invalidSections() []string {
	var sections []string
	invalid := func(key string, minimum, maximum int) bool {
		text, editing := p.texts[key]
		_, ok := parseWhole(text, minimum, maximum)
		return editing && !ok
	}
	for _, app := range p.apps() {
		a := app.Value
		tray, plan := a.TrayBehavior.Value, a.Schedule.Value
		bad := invalid(programKey(a.ID, "closeDelay"), 0, 600) || invalid(programKey(a.ID, "startDelay"), 0, 1440) ||
			invalid(programKey(a.ID, "autoExit"), 0, 1440) || invalid(programKey(a.ID, "days"), 1, 365) || invalid(programKey(a.ID, "runs"), 1, 1000) ||
			!inRange(tray.CloseDelaySeconds, 0, 600) || !inRange(plan.StartDelayMinutes, 0, 1440) || !inRange(plan.AutoExitMinutes, 0, 1440) ||
			plan.FrequencyEnabled && (!inRange(plan.FrequencyDays, 1, 365) || !inRange(plan.FrequencyRuns, 1, 1000))
		if bad {
			sections = append(sections, p.displayName(a))
		}
	}
	if p.draft != nil {
		s := p.draft.Value
		if invalid(intervalKey, 0, 120) || invalid(windowWaitKey, 0, 120) || !inRange(s.StartupIntervalSeconds, 0, 120) || !inRange(s.CloseWindowRetrySeconds, 0, 120) {
			sections = append(sections, i18n.T("wintray.advancedStartup"))
		}
	}
	return sections
}

// unnamed lists programs whose name was cleared.
func (p *page) unnamed() []string {
	var names []string
	for _, app := range p.apps() {
		if strings.TrimSpace(app.Value.Name) == "" {
			names = append(names, p.displayName(app.Value))
		}
	}
	return names
}

func (p *page) save() {
	if p.saved == nil || p.draft == nil {
		return
	}
	p.runJob("settings.save", map[string]any{"revision": p.saved.Revision, "settings": *p.draft})
}

func (p *page) View(c *ui.Context) {
	// The module was turned on or off elsewhere: read it the other way.
	if disabled := p.moduleDisabled(); disabled != p.disabled {
		p.disabled = disabled
		p.reload(false, nil)
	}
	dirty := p.dirty()
	p.env.Activity.Report("wintray", dirty, p.busy, func() { p.reload(false, nil) })
	invalid, unnamed := p.invalidSections(), p.unnamed()
	if p.draft != nil {
		p.env.Rail.Set(
			core.Action{Label: i18n.T("wintray.save"), Disabled: p.busy || !dirty || len(invalid) > 0 || len(unnamed) > 0 || p.recovering(),
				Busy: p.job == "settings.save", Run: p.save},
			core.Action{Disabled: p.busy || !dirty, Run: func() { p.reload(false, nil) }},
		)
	}
	p.readIcons()
	var page ui.Element
	kit.WidePage(c, func() {
		// One column, so the list's area knows the gap above it.
		page = ui.Column(c).FillWidth().Gap(16).Children(func() { p.content(c, dirty, invalid, unnamed) })
	})
	// Programs dropped from File Explorer anywhere on the list's tab are
	// added as if chosen.
	widgets.DropArea(c, p.env, page, widgets.DropFiles, true, p.busy || p.draft == nil || p.recovering() || p.tab != tabPrograms,
		func(paths []string, err error) {
			if err != nil {
				p.err = err.Error()
				return
			}
			p.add(paths)
		})
	p.stopDialog(c)
	p.reset.dialog(c, p)
}

func (p *page) content(c *ui.Context, dirty bool, invalid, unnamed []string) {
	ui.Column(c).FillWidth().Gap(16).Bind(&p.header).Children(func() {
		tabs := []kit.Tab{{Label: i18n.T("wintray.list"), Icon: icons.List}, {Label: i18n.T("wintray.advancedStartup"), Icon: icons.Power}}
		kit.Tabs(c, &p.tab, tabs, p.env.RequestChange)
		p.notices(c, dirty)
		if p.draft == nil {
			return
		}
		if p.tab == tabPrograms {
			ui.Row(c).FillWidth().GapX(16).GapY(8).Wrap().AlignItems(ui.Center).Children(func() {
				if kit.PrimaryButton(c, i18n.T("wintray.add"), icons.Plus, p.adding).Disabled(p.blocked()).Clicked() {
					p.add(nil)
				}
				status := ""
				switch {
				case p.busy:
					status = i18n.T("wintray.working")
				case dirty:
					status = i18n.T("wintray.dirty")
				}
				kit.Status(c, status)
			})
		}
		if len(invalid) > 0 {
			kit.InlineError(c, i18n.T("wintray.invalidNumbers", "sections", strings.Join(invalid, "、")))
		}
		if len(unnamed) > 0 {
			kit.InlineError(c, i18n.T("wintray.nameRequired", "programs", strings.Join(unnamed, ", ")))
		}
	})
	if p.draft == nil {
		if p.err == "" {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Role(ui.RoleStatus).Children(func() {
				kit.Loader(c)
				kit.Help(c, i18n.T("wintray.loading"))
			})
			kit.Skeleton(c, 4)
		}
		return
	}
	switch p.tab {
	case tabPrograms:
		p.programs(c)
	default:
		ui.Column(c).FillWidth().Gap(kit.GroupGap).Margin(16, 0, 0, 0).Disabled(p.blocked()).Children(func() { p.startup(c, dirty) })
	}
}

func (p *page) notices(c *ui.Context, dirty bool) {
	switch {
	case p.err != "" && p.draft != nil:
		kit.Notice(c, kit.Failure, "", p.err, func() {
			if kit.Button(c, i18n.T("wintray.discard"), nil, false).Disabled(p.busy).Clicked() {
				p.reload(false, nil)
			}
		})
	case p.err != "":
		kit.Notice(c, kit.Failure, "", p.err, func() {
			if kit.Button(c, i18n.T("common.retry"), icons.RefreshCw, p.loading).Disabled(p.busy).Clicked() {
				p.reload(false, nil)
			}
		})
	}
	if p.disabled {
		kit.Help(c, i18n.T("wintray.disabled"))
	}
	if p.saved == nil || p.disabled {
		return
	}
	if p.saved.RecoveryError != "" {
		ui.Row(c).Children(func() {
			if kit.Button(c, i18n.T("wintray.recover"), icons.RotateCcw, p.job == "recover").Disabled(p.busy).Clicked() {
				p.runJob("recover", nil)
			}
		})
	}
	if p.saved.StartupError != "" {
		kit.Notice(c, kit.Failure, "", p.saved.StartupError, func() {
			if kit.Button(c, i18n.T("wintray.retryStartup"), icons.RefreshCw, p.job == "startup.retry").Disabled(p.busy || dirty || p.recovering()).Clicked() {
				p.runJob("startup.retry", nil)
			}
		})
	}
}
