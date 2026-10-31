package settings

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/shell"
	"winone/desktopmygo/widgets"
	"winone/internal/previewtransfer"
)

func (p *page) profiles(c *ui.Context) {
	p.export.view(c)
	p.importer.view(c)
}

func profileFilter() core.Filter {
	return core.Filter{Name: i18n.T("profiles.fileFilter"), Extensions: []string{"winone"}}
}

// exporter writes the global preferences and module settings to a .winone
// file the user chooses.
type exporter struct {
	page    *page
	busy    bool
	written string
	err     string
}

func (e *exporter) report() { e.page.env.Activity.Report("profile-export", false, e.busy, nil) }

func (e *exporter) view(c *ui.Context) {
	kit.Section(c, icons.Download, i18n.T("profiles.export"), i18n.T("profiles.exportHint"), func() {
		ui.Row(c).FillWidth().GapX(16).GapY(8).Wrap().AlignItems(ui.Center).Children(func() {
			if kit.Button(c, i18n.T("profiles.exportFile"), icons.Download, e.busy).Clicked() {
				e.run()
			}
			if e.written != "" {
				kit.Status(c, i18n.T("profiles.exported", "path", e.written)).Selectable().Shrink(1).MinWidth(0)
			}
		})
		if e.err != "" {
			kit.Notice(c, kit.Failure, "", e.err, nil)
		}
	})
}

func (e *exporter) run() {
	if e.busy {
		return
	}
	env := e.page.env
	e.busy, e.written, e.err = true, "", ""
	core.Go(env, func() (string, error) {
		path, err := env.Dialogs.SaveFile(i18n.T("profiles.export"), profileFilter(), "WinOne.winone")
		if err != nil || path == "" {
			return "", err
		}
		if path, err = withProfileExtension(path); err != nil {
			return "", err
		}
		var result struct {
			Path string `json:"path"`
		}
		err = env.Host.CallInto("profiles.export", map[string]string{"path": path, "name": i18n.T("profiles.exportName")}, &result)
		return result.Path, err
	}, func(path string, err error) {
		e.busy = false
		if err != nil {
			e.err = err.Error()
			return
		}
		e.written = path
	})
}

// withProfileExtension adds .winone to a name typed without it. MyGo's
// save dialog has no default extension, so it asked before replacing only
// the name as typed: an existing file with the extension added is refused
// instead of replaced unasked. Another extension is kept; the Host rejects
// it.
func withProfileExtension(path string) (string, error) {
	if filepath.Ext(path) != "" {
		return path, nil
	}
	path += ".winone"
	if _, err := os.Stat(path); err == nil {
		return "", core.Errorf(i18n.T("profiles.exportExists", "path", path))
	}
	return path, nil
}

// The parts of a reviewed archive the page shows.
type profilePath struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Kind  string `json:"kind"`
	Value string `json:"value"`
	State string `json:"state"`
	Error string `json:"error,omitempty"`
}

type importModule struct {
	ID            string          `json:"id"`
	State         string          `json:"state"`
	RequiredPaths []string        `json:"requiredPaths"`
	Error         string          `json:"error,omitempty"`
	Settings      json.RawMessage `json:"settings,omitempty"`
}

type importPreview struct {
	Name                  string          `json:"name"`
	SourceHash            string          `json:"sourceHash"`
	ConfigurationRevision json.RawMessage `json:"configurationRevision"`
	ModuleRevision        json.RawMessage `json:"moduleRevision"`
	Preferences           struct {
		Theme          string          `json:"theme"`
		Language       string          `json:"language"`
		RunAtLogon     bool            `json:"runAtLogon"`
		StartMinimized bool            `json:"startMinimized"`
		DefaultPage    *string         `json:"defaultPage"`
		KeepUIRunning  *bool           `json:"keepUIRunning"`
		OpenShortcut   *string         `json:"openShortcut"`
		Enabled        map[string]bool `json:"enabled"`
		Shortcuts      json.RawMessage `json:"shortcuts"`
	} `json:"preferences"`
	CurrentShortcuts json.RawMessage `json:"currentShortcuts"`
	Plan             struct {
		Modules        []importModule `json:"modules"`
		ShortcutIssues []struct {
			Action string `json:"action"`
			Kind   string `json:"kind"`
			Error  string `json:"error"`
		} `json:"shortcutIssues"`
	} `json:"plan"`
	Paths []profilePath `json:"paths"`

	// Read once from the archive, in its own order.
	shortcuts, current []binding
	settings           map[string]value
}

// binding is one action's shortcut, kept in the archive's order.
type binding struct{ action, key string }

func bindings(raw json.RawMessage) []binding {
	parsed, err := parseValue(raw)
	if err != nil {
		return nil
	}
	var list []binding
	for _, member := range parsed.members {
		text, _ := member.value.scalar.(string)
		list = append(list, binding{member.key, text})
	}
	return list
}

var (
	knownModules = []string{"capture", "always-on-top", "wintray", "selection", "quick-assistant", "stroke"}
	knownActions = []string{"capture.region", "capture.ocr", "capture.translate", "always-on-top.toggle", "selection.show", "quick-assistant.show"}
	moduleStates = map[string]string{"ready": "profiles.ready", "needs_mapping": "profiles.mapping", "missing_module": "profiles.missing", "invalid": "profiles.invalid"}
	languages    = map[string]string{"system": "shell.system", "zh-CN": "profiles.chinese", "en-US": "profiles.english"}
)

// moduleName names a module as the window does elsewhere; an unknown one
// keeps its ID. React showed the shutdown timer by its ID too.
func moduleName(id string) string {
	switch {
	case id == "stroke":
		return i18n.T("stroke.title")
	case id == "timed-power":
		return i18n.T("timedPower.title")
	case slices.Contains(knownModules, id):
		return i18n.T("nav." + id)
	}
	return id
}

// pageName names a start page by the sidebar's title of it.
func pageName(id string) string {
	if id == "" {
		id = "overview"
	}
	for _, page := range shell.Catalog() {
		if page.ID == id {
			return i18n.T(page.Title)
		}
	}
	return i18n.T("nav." + id)
}

func pathName(path profilePath) string {
	switch {
	case path.ID == "capture.output":
		return i18n.T("profiles.captureFolder")
	case path.Label != "":
		return path.Label
	}
	return i18n.T("profiles.programLocation")
}

// importer reviews a .winone archive and applies the parts the user keeps:
// the Host checks it, the user maps local paths and confirms.
type importer struct {
	page    *page
	source  string
	preview *importPreview
	// mappings are the local paths chosen for the archive's references.
	mappings  map[string]string
	selected  []string
	global    bool
	reviewTab int
	detail    string
	busy      bool
	err       string
	notice    string
	// needsReview blocks applying until the archive was checked again, as
	// after a failed replacement, mapping or apply.
	needsReview bool
}

func (im *importer) report() {
	im.page.env.Activity.Report("profile-import", im.preview != nil, im.busy, im.discard)
}

func (im *importer) discard() {
	im.preview, im.needsReview, im.err, im.notice = nil, false, "", ""
}

// receivePreview reads the Host's review of an archive, which it hands out
// in chunks, checked against its digest; the transfer is always released
// (the Host also drops it after a while).
func receivePreview(env *core.Env, path string, mappings map[string]string) (*importPreview, error) {
	if mappings == nil {
		mappings = map[string]string{}
	}
	var descriptor previewtransfer.Descriptor
	if err := env.Host.CallInto("profiles.import.preview.start", map[string]any{"path": path, "bindings": mappings}, &descriptor); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	data, err := previewtransfer.Receive(ctx, descriptor, func(offset int) (previewtransfer.Chunk, error) {
		var chunk previewtransfer.Chunk
		err := env.Host.CallInto("profiles.import.preview.read", map[string]any{"token": descriptor.Token, "offset": offset}, &chunk)
		return chunk, err
	})
	_ = env.Host.CallInto("profiles.import.preview.release", map[string]string{"token": descriptor.Token}, nil)
	if err != nil {
		return nil, err
	}
	var preview importPreview
	if err := json.Unmarshal(data, &preview); err != nil {
		return nil, core.Errorf(err.Error())
	}
	preview.shortcuts = bindings(preview.Preferences.Shortcuts)
	preview.current = bindings(preview.CurrentShortcuts)
	preview.settings = map[string]value{}
	for _, module := range preview.Plan.Modules {
		if parsed, err := parseValue(module.Settings); err == nil && parsed.kind == objectValue {
			preview.settings[module.ID] = parsed
		}
	}
	return &preview, nil
}

// review shows a fresh check of path with mappings; reset starts the
// choices over for a newly chosen archive.
func (im *importer) review(path string, mappings map[string]string, next *importPreview, reset bool) {
	im.preview, im.source, im.mappings, im.needsReview = next, path, mappings, false
	if reset {
		im.selected = nil
		for _, module := range next.Plan.Modules {
			if module.State != "invalid" {
				im.selected = append(im.selected, module.ID)
			}
		}
		im.global, im.reviewTab, im.detail = true, 0, ""
		return
	}
	im.selected = slices.DeleteFunc(im.selected, func(id string) bool {
		return !slices.ContainsFunc(next.Plan.Modules, func(module importModule) bool { return module.ID == id })
	})
}

// choose checks an archive, chosen in the dialog when path is empty. A
// failed replacement keeps the current review, which then needs a fresh
// check before it can be applied.
func (im *importer) choose(path string) {
	if im.busy {
		return
	}
	im.busy, im.err, im.notice = true, "", ""
	env := im.page.env
	type chosen struct {
		path    string
		preview *importPreview
	}
	core.Go(env, func() (chosen, error) {
		if path == "" {
			picked, err := env.Dialogs.PickFile(i18n.T("profiles.chooseFile"), profileFilter())
			if err != nil || picked == "" {
				return chosen{}, err
			}
			path = picked
		}
		preview, err := receivePreview(env, path, nil)
		return chosen{path, preview}, err
	}, func(result chosen, err error) {
		im.busy = false
		if err != nil {
			im.err = err.Error()
			if path != "" || result.path != "" {
				im.needsReview = true
			}
			return
		}
		if result.path == "" {
			return
		}
		im.review(result.path, map[string]string{}, result.preview, true)
	})
}

// mapPath sets the local location of one of the archive's references,
// chosen in a dialog when value is empty, and checks the archive again.
func (im *importer) mapPath(path profilePath, value string) {
	if im.busy || im.preview == nil {
		return
	}
	im.busy, im.err = true, ""
	env := im.page.env
	source := im.source
	next := map[string]string{}
	for id, location := range im.mappings {
		next[id] = location
	}
	type mapped struct {
		value   string
		preview *importPreview
	}
	core.Go(env, func() (mapped, error) {
		if value == "" {
			picked, err := pickLocation(env, path.Kind)
			if err != nil || picked == "" {
				return mapped{}, err
			}
			value = picked
		}
		next[path.ID] = value
		preview, err := receivePreview(env, source, next)
		return mapped{value, preview}, err
	}, func(result mapped, err error) {
		im.busy = false
		if result.value != "" {
			// The chosen location stays even when the check failed.
			im.mappings = next
		}
		if err != nil {
			im.err, im.needsReview = err.Error(), true
			return
		}
		if result.value == "" {
			return
		}
		im.review(source, next, result.preview, false)
	})
}

func pickLocation(env *core.Env, kind string) (string, error) {
	switch kind {
	case "directory":
		return env.Dialogs.PickFolder(i18n.T("profiles.pickFolder"))
	case "program":
		return env.Dialogs.PickFile(i18n.T("profiles.pickProgram"), core.Filter{Name: i18n.T("profiles.programFilter"), Extensions: []string{"exe", "bat", "cmd", "ps1", "py", "pyw"}})
	}
	return env.Dialogs.PickFile(i18n.T("profiles.pickFile"), core.Filter{})
}

// refresh checks the same archive with the same mappings again.
func (im *importer) refresh() {
	if im.busy || im.preview == nil {
		return
	}
	im.busy, im.err = true, ""
	env := im.page.env
	source, mappings := im.source, im.mappings
	core.Go(env, func() (*importPreview, error) { return receivePreview(env, source, mappings) }, func(next *importPreview, err error) {
		im.busy = false
		if err != nil {
			im.err, im.needsReview = err.Error(), true
			return
		}
		im.review(source, mappings, next, false)
	})
}

func (im *importer) apply() {
	if im.busy || im.preview == nil {
		return
	}
	im.busy, im.err, im.notice = true, "", ""
	env := im.page.env
	preview := im.preview
	params := map[string]any{
		"path": im.source, "bindings": im.mappings, "sourceHash": preview.SourceHash,
		"configurationRevision": preview.ConfigurationRevision, "moduleRevision": preview.ModuleRevision,
		"modules": append([]string{}, im.selected...), "global": im.global,
	}
	if im.mappings == nil {
		params["bindings"] = map[string]string{}
	}
	core.Do(env, func() error {
		_, err := env.Host.CallCompleting("profiles.import.apply", params)
		return err
	}, func(err error) {
		im.busy = false
		if err != nil {
			im.err, im.needsReview = err.Error(), true
			return
		}
		im.preview, im.notice = nil, i18n.T("profiles.imported")
		env.Refresh()
	})
}

func (im *importer) chosen() []importModule {
	var modules []importModule
	if im.preview == nil {
		return nil
	}
	for _, module := range im.preview.Plan.Modules {
		if slices.Contains(im.selected, module.ID) {
			modules = append(modules, module)
		}
	}
	return modules
}

// paths returns the local locations the chosen modules need.
func (im *importer) paths() []profilePath {
	required := map[string]bool{}
	for _, module := range im.chosen() {
		for _, id := range module.RequiredPaths {
			required[id] = true
		}
	}
	var paths []profilePath
	for _, path := range im.preview.Paths {
		if required[path.ID] {
			paths = append(paths, path)
		}
	}
	return paths
}

// issues lists what keeps the import from applying, checking shortcuts
// against exactly what this import keeps and replaces.
func (im *importer) issues() []string {
	preview := im.preview
	var issues []string
	for _, issue := range preview.Plan.ShortcutIssues {
		if issue.Kind == "missing_module" || issue.Kind == "conflict" {
			continue
		}
		if slices.ContainsFunc(im.selected, func(id string) bool { return strings.HasPrefix(issue.Action, id+".") }) ||
			im.global && issue.Action == "application.open" {
			issues = append(issues, issue.Error)
		}
	}
	keys := slices.Clone(preview.current)
	set := func(action, key string) {
		for i := range keys {
			if keys[i].action == action {
				keys[i].key = key
				return
			}
		}
		keys = append(keys, binding{action, key})
	}
	if im.global && preview.Preferences.OpenShortcut != nil {
		set("application.open", *preview.Preferences.OpenShortcut)
	}
	for _, id := range im.selected {
		for _, b := range preview.shortcuts {
			if !strings.HasPrefix(b.action, id+".") || !slices.Contains(knownModules, id) {
				continue
			}
			if !slices.Contains(knownActions, b.action) {
				issues = append(issues, i18n.T("profiles.unsupported", "name", moduleName(id)))
			} else {
				set(b.action, b.key)
			}
		}
	}
	used := map[string]bool{}
	for _, b := range keys {
		key := canonicalShortcut(b.key)
		if key == "" {
			continue
		}
		if used[key] {
			issues = append(issues, i18n.T("profiles.conflict"))
		}
		used[key] = true
	}
	var unique []string
	for _, issue := range issues {
		if !slices.Contains(unique, issue) {
			unique = append(unique, issue)
		}
	}
	return unique
}

// canonicalShortcut compares chords whatever their order and case.
func canonicalShortcut(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	parts := strings.Split(value, "+")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	slices.Sort(parts)
	return strings.Join(parts, "+")
}

func (im *importer) blocked(issues []string) bool {
	if im.needsReview || !im.global && len(im.selected) == 0 || len(issues) > 0 {
		return true
	}
	for _, module := range im.chosen() {
		if module.State != "ready" && module.State != "missing_module" {
			return true
		}
	}
	for _, path := range im.paths() {
		if path.State != "ready" {
			return true
		}
	}
	return false
}

func (im *importer) view(c *ui.Context) {
	section := ui.Column(c).FillWidth().Gap(kit.HeadingGap).Children(func() {
		kit.SectionHeading(c, icons.Upload, i18n.T("profiles.import"), i18n.T("profiles.importHint"))
		ui.Row(c).Children(func() {
			if kit.Button(c, i18n.T("profiles.chooseFile"), icons.Upload, im.busy && im.preview == nil).Disabled(im.busy).Clicked() {
				im.choose("")
			}
		})
		if im.preview != nil {
			im.reviewView(c)
		}
		if im.err != "" {
			kit.Notice(c, kit.Failure, "", im.err, nil)
		}
		kit.Status(c, im.notice)
	})
	// The whole group takes one archive dropped from File Explorer; a
	// location row inside takes its own drops.
	widgets.DropArea(c, im.page.env, section, widgets.DropFiles, false, im.busy, func(paths []string, err error) {
		if err != nil {
			im.err = err.Error()
			return
		}
		im.choose(paths[0])
	})
}

func (im *importer) reviewView(c *ui.Context) {
	preview := im.preview
	paths := im.paths()
	issues := im.issues()
	ui.Column(c).FillWidth().Gap(16).Margin(8, 0, 0, 0).Children(func() {
		ui.Column(c).Gap(4).Children(func() {
			name := preview.Name
			if name == "" {
				name = i18n.T("profiles.import")
			}
			kit.Heading(c, name).Role(ui.RoleHeading).Level(3)
			kit.Help(c, im.source).Selectable()
		})
		ui.Column(c).FillWidth().Gap(12).Disabled(im.busy).Children(func() {
			// Alternate views of one pending import, not navigation: no
			// unsaved-change question when switching them.
			kit.SubTabs(c, &im.reviewTab, []string{i18n.T("profiles.contents"), i18n.T("profiles.details"), i18n.T("profiles.localPaths")}, nil)
			switch im.reviewTab {
			case 0:
				im.contents(c)
			case 1:
				im.details(c)
			default:
				im.locations(c, paths)
			}
		})
		if im.reviewTab != 2 && slices.ContainsFunc(paths, func(path profilePath) bool { return path.State != "ready" }) {
			ui.Row(c).Children(func() {
				if kit.Button(c, i18n.T("profiles.fixPaths"), nil, false).Disabled(im.busy).Clicked() {
					im.reviewTab = 2
				}
			})
		}
		if len(issues) > 0 {
			lines := slices.Clone(issues)
			if len(lines) > 1 {
				for i := range lines {
					lines[i] = "• " + lines[i]
				}
			}
			kit.Notice(c, kit.Failure, "", strings.Join(lines, "\n"), nil)
		}
		ui.Row(c).Gap(8).Wrap().Children(func() {
			if kit.PrimaryButton(c, i18n.T("profiles.confirm"), nil, im.busy).Disabled(im.busy || im.blocked(issues)).Clicked() {
				im.apply()
			}
			if kit.Button(c, i18n.T("profiles.review"), nil, false).Disabled(im.busy).Clicked() {
				im.refresh()
			}
			if kit.GhostButton(c, i18n.T("profiles.cancel"), nil, false).Disabled(im.busy).Clicked() {
				im.preview, im.err = nil, ""
			}
		})
	})
}

func onOff(on bool) string {
	if on {
		return i18n.T("profiles.on")
	}
	return i18n.T("profiles.off")
}

func (im *importer) contents(c *ui.Context) {
	preview := im.preview
	kit.Panel(c, func() {
		ui.Column(c).FillWidth().Padding(12, 16).Gap(6).Children(func() {
			ui.Checkbox(c, &im.global, i18n.T("profiles.global"))
			if !im.global {
				return
			}
			prefs := preview.Preferences
			ui.Column(c).PaddingStart(28).Gap(2).Children(func() {
				language := languages[prefs.Language]
				if language == "" {
					language = prefs.Language
				}
				kit.Help(c, strings.Join([]string{
					i18n.T("shell." + prefs.Theme), i18n.T(language),
					i18n.T("profiles.signIn", "value", onOff(prefs.RunAtLogon)),
					i18n.T("profiles.hidden", "value", onOff(prefs.StartMinimized)),
				}, " · "))
				page := i18n.T("profiles.keepCurrent")
				if prefs.DefaultPage != nil {
					page = pageName(*prefs.DefaultPage)
				}
				kit.Help(c, i18n.T("profiles.defaultPage", "value", page))
				keep := i18n.T("profiles.keepCurrent")
				if prefs.KeepUIRunning != nil {
					keep = onOff(*prefs.KeepUIRunning)
				}
				kit.Help(c, i18n.T("profiles.keepUI", "value", keep))
				open := i18n.T("profiles.keepCurrent")
				if prefs.OpenShortcut != nil {
					open = *prefs.OpenShortcut
					if open == "" {
						open = i18n.T("profiles.unset")
					}
				}
				kit.Help(c, i18n.T("profiles.openShortcut", "value", open))
			})
		})
		for _, module := range preview.Plan.Modules {
			ui.Column(c.Key(module.ID)).FillWidth().Padding(12, 16).Gap(6).Children(func() {
				selected := slices.Contains(im.selected, module.ID)
				on := selected
				if ui.Checkbox(c, &on, moduleName(module.ID)).Changed() {
					if on {
						im.selected = append(im.selected, module.ID)
					} else {
						im.selected = slices.DeleteFunc(im.selected, func(id string) bool { return id == module.ID })
					}
				}
				status := module.Error
				if status == "" {
					status = i18n.T(moduleStates[module.State])
				}
				if enabled, ok := preview.Preferences.Enabled[module.ID]; selected && ok && module.State != "missing_module" {
					after := "profiles.disabledAfter"
					if enabled {
						after = "profiles.enabledAfter"
					}
					status += " · " + i18n.T(after)
				}
				ui.Column(c).PaddingStart(28).Gap(2).Children(func() {
					kit.Help(c, status)
					if selected && module.ID == "wintray" {
						if apps, ok := preview.settings[module.ID].field("managedApps"); ok && apps.kind == arrayValue {
							kit.Help(c, i18n.T("profiles.replace", "count", len(apps.items)))
						}
					}
				})
			})
		}
	})
}

func (im *importer) details(c *ui.Context) {
	preview := im.preview
	var modules []importModule
	for _, module := range im.chosen() {
		if _, ok := preview.settings[module.ID]; ok {
			modules = append(modules, module)
		}
	}
	if len(modules) == 0 {
		kit.Help(c, i18n.T("profiles.noDetails"))
		return
	}
	detail := modules[0].ID
	if slices.ContainsFunc(modules, func(module importModule) bool { return module.ID == im.detail }) {
		detail = im.detail
	}
	options := make([]kit.SelectOption, len(modules))
	for i, module := range modules {
		options[i] = kit.SelectOption{Value: module.ID, Label: moduleName(module.ID)}
	}
	ui.Row(c).FillWidth().Gap(12).Wrap().AlignItems(ui.Center).Children(func() {
		kit.Label(c, i18n.T("profiles.module"))
		changed, trigger := kit.Select(c, &detail, options)
		trigger.Label(i18n.T("profiles.module"))
		if changed {
			im.detail = detail
		}
	})
	reviewSettings(c, preview.settings[detail], preview.Paths, false)
	for _, b := range preview.shortcuts {
		if strings.HasPrefix(b.action, detail+".") {
			key := b.key
			if key == "" {
				key = i18n.T("profiles.unset")
			}
			kit.Help(c, i18n.T("profiles.shortcut", "key", key))
		}
	}
}

func (im *importer) locations(c *ui.Context, paths []profilePath) {
	if len(paths) == 0 {
		kit.Help(c, i18n.T("profiles.noPaths"))
		return
	}
	kit.Section(c, nil, i18n.T("profiles.locations"), "", func() {
		kit.Panel(c, func() {
			for _, path := range paths {
				im.pathRow(c, path)
			}
		})
	})
}

func (im *importer) pathRow(c *ui.Context, path profilePath) {
	env := im.page.env
	name := pathName(path)
	row := kit.Stack(c.Key(path.ID), name, "", func() {
		ui.Column(c).FillWidth().Gap(6).Children(func() {
			// The whole path wraps, as React's did: its end, the file
			// name, is what tells the locations apart.
			ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
				shown := path.Value
				if shown == "" {
					shown = i18n.T("profiles.noLocation")
				}
				text := ui.Text(c, shown).Font(kit.MonoFont).FontSize(kit.HelpSize(c)).Grow(1).Shrink(1).Basis(0).MinWidth(0).Selectable()
				if path.Value == "" {
					text.TextColor(c.Theme().TextMuted)
				}
				choose := kit.Button(c, i18n.T("profiles.choose"), icons.FolderOpen, false).Shrink(0).Disabled(im.busy).
					Label(i18n.T("profiles.chooseFor", "name", name))
				if choose.Clicked() {
					im.mapPath(path, "")
				}
			})
			kit.InlineError(c, path.Error)
		})
	})
	// Each location takes a file or folder dropped on its own row.
	kind := widgets.DropFiles
	if path.Kind == "directory" {
		kind = widgets.DropFolders
	}
	widgets.DropArea(c, env, row, kind, false, im.busy, func(paths []string, err error) {
		if err != nil {
			im.err = err.Error()
			return
		}
		im.mapPath(path, paths[0])
	})
}
