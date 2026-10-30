package overview

import (
	"time"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

type identity struct {
	ID   string     `json:"id"`
	Name core.Names `json:"name"`
}

// packageMetadata is a module or a runtime package; legacy module metadata
// omits kind.
type packageMetadata struct {
	Kind         string             `json:"kind,omitempty"`
	Version      string             `json:"version"`
	Module       *identity          `json:"module,omitempty"`
	Runtime      *identity          `json:"runtime,omitempty"`
	Dependencies []packageReference `json:"dependencies,omitempty"`
}

func (m packageMetadata) name() string {
	if m.Kind == "runtime" && m.Runtime != nil {
		return m.Runtime.Name.String()
	}
	if m.Module != nil {
		return m.Module.Name.String()
	}
	return ""
}

type preview struct {
	ID               string           `json:"id"`
	State            string           `json:"state"`
	Operation        string           `json:"operation,omitempty"`
	PreviousVersion  string           `json:"previousVersion,omitempty"`
	MigrationFrom    int              `json:"migrationFrom,omitempty"`
	MigrationTo      int              `json:"migrationTo,omitempty"`
	MigrationPaths   []string         `json:"migrationPaths,omitempty"`
	FileName         string           `json:"fileName"`
	Metadata         *packageMetadata `json:"metadata,omitempty"`
	Publisher        string           `json:"publisher,omitempty"`
	KeyFingerprint   string           `json:"keyFingerprint,omitempty"`
	ManifestSHA256   string           `json:"manifestSha256,omitempty"`
	CanInstall       bool             `json:"canInstall"`
	RestoresSettings bool             `json:"restoresSettings,omitempty"`
	Error            string           `json:"error,omitempty"`
	DependencyLock   *struct {
		Packages []packageReference `json:"packages"`
	} `json:"dependencyLock,omitempty"`
}

// installer installs or updates a module or runtime package: the Host
// checks the file's signature, compatibility and dependencies first, and
// the user confirms what it found.
type installer struct {
	page     *page
	open     bool
	preview  *preview
	err      string
	busy     bool
	choosing bool
	// generation discards answers about a file the user replaced.
	generation int
	file       string
	details    struct{ signature, dependencies, migration bool }
}

func (in *installer) button(c *ui.Context, disabled bool) {
	t := c.Theme()
	b := kit.Button(c, i18n.T("module.installPackage"), icons.Plus, in.choosing).Disabled(disabled || in.busy)
	// A package dropped on the button is checked as if chosen.
	if !disabled && !in.busy && !in.open {
		if b.FileDragOver() {
			b.Border(1, t.Accent)
		}
		if files := b.DroppedFiles(); files != nil {
			if len(files) == 1 {
				in.inspect(files[0])
			} else {
				in.page.err = i18n.T("fileDrop.single")
			}
		}
	}
	if b.Clicked() {
		in.choose()
	}
}

func (in *installer) choose() {
	in.choosing = true
	env := in.page.env
	core.Go(env, func() (string, error) {
		return env.Dialogs.PickFile(i18n.T("module.installPackage"), core.Filter{Name: i18n.T("module.packageFilter"), Extensions: []string{"winone-module", "winone-runtime", "zip"}})
	}, func(path string, err error) {
		in.choosing = false
		if err != nil {
			in.err, in.open = err.Error(), true
			return
		}
		if path != "" {
			in.inspect(path)
		}
	})
}

func (in *installer) inspect(path string) {
	in.generation++
	owner := in.generation
	in.err, in.preview, in.open, in.file = "", nil, true, path
	env := in.page.env
	core.Go(env, func() (preview, error) {
		var next preview
		err := env.Host.CallCompletingInto("modules.packagePreview.start", map[string]string{"path": path}, &next)
		return next, err
	}, func(next preview, err error) {
		if owner != in.generation {
			if err == nil {
				cancelPreview(env, next.ID)
			}
			return
		}
		if err != nil {
			in.err = err.Error()
			return
		}
		in.preview = &next
		in.poll(owner, next)
	})
}

// poll follows a check still running in the Host.
func (in *installer) poll(owner int, current preview) {
	if current.State != "checking" {
		return
	}
	env := in.page.env
	core.Go(env, func() (preview, error) {
		time.Sleep(200 * time.Millisecond)
		var next preview
		err := env.Host.CallCompletingInto("modules.packagePreview.get", map[string]string{"id": current.ID}, &next)
		return next, err
	}, func(next preview, err error) {
		if owner != in.generation || in.preview == nil || in.preview.ID != current.ID {
			return
		}
		if err != nil {
			in.err = err.Error()
			failed := current
			failed.State, failed.CanInstall = "failed", false
			in.preview = &failed
			return
		}
		in.preview = &next
		in.poll(owner, next)
	})
}

func cancelPreview(env *core.Env, id string) {
	if id == "" {
		return
	}
	go func() { _, _ = env.Host.CallCompleting("modules.packagePreview.cancel", map[string]string{"id": id}) }()
}

func (in *installer) cancel() {
	if in.busy {
		return
	}
	in.generation++
	if in.preview != nil {
		cancelPreview(in.page.env, in.preview.ID)
	}
	in.preview, in.open = nil, false
}

func (in *installer) install(c *ui.Context) {
	if in.preview == nil || !in.preview.CanInstall || in.busy {
		return
	}
	current := *in.preview
	operation, method := "install", "modules.installPackage"
	if current.Operation == "update" {
		operation, method = "update", "modules.updatePackage"
	}
	in.busy, in.err = true, ""
	env := in.page.env
	core.Go(env, func() (core.ModuleJob, error) {
		var result core.ModuleJob
		err := env.Host.CallCompletingInto(method, map[string]string{"id": current.ID}, &result)
		return result, err
	}, func(result core.ModuleJob, err error) {
		success := ""
		switch {
		case err != nil:
			in.err = err.Error()
		case result.State != "completed":
			in.err = result.Error
			if in.err == "" {
				in.err = i18n.T(map[string]string{"install": "module.installFailed", "update": "module.updateFailed"}[operation])
			}
		default:
			success = result.Warning
			if success == "" {
				switch {
				case current.Metadata != nil && current.Metadata.Kind == "runtime":
					success = i18n.T("sharedPackages.installed")
				case operation == "update":
					success = i18n.T("module.packageUpdated")
				default:
					success = i18n.T("module.packageInstalled")
				}
			}
			in.preview, in.open = nil, false
		}
		if in.err != "" && in.preview != nil {
			failed := *in.preview
			failed.CanInstall = false
			in.preview = &failed
		}
		// A failed switch can restore the old version or leave a recovery
		// entry: re-read the catalog either way, never showing success.
		in.page.reload(func() {
			in.busy = false
			in.page.env.Refresh()
			if success != "" {
				c.Toast(success)
			}
		})
	})
}

func (in *installer) dialog(c *ui.Context) {
	if !in.open {
		return
	}
	p := in.preview
	update := p != nil && p.Operation == "update"
	title, description, confirm := "module.installPackage", "module.installDescription", "module.confirmInstall"
	if update {
		title, description, confirm = "module.updatePackage", "module.updateDescription", "module.confirmUpdate"
	}
	checking := p == nil && in.err == "" || p != nil && p.State == "checking"
	open := in.open
	confirmed := kit.Dialog(c, &open, kit.DialogOptions{
		Title: i18n.T(title), Message: i18n.T(description), Confirm: i18n.T(confirm), Cancel: i18n.T("module.cancelInstall"),
		Busy: in.busy, ConfirmDisabled: p == nil || !p.CanInstall || checking, Width: 480,
		Body: func() { in.body(c, checking) },
		Actions: func() {
			failed := in.err != "" || p != nil && p.Error != ""
			if failed && in.file != "" && kit.Button(c, i18n.T("module.inspectAgain"), icons.RefreshCw, false).Disabled(in.busy).Clicked() {
				in.inspect(in.file)
			}
		},
	})
	if confirmed {
		in.install(c)
	}
	if !open {
		in.cancel()
	}
}

func (in *installer) body(c *ui.Context, checking bool) {
	t := c.Theme()
	p := in.preview
	if checking {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Role(ui.RoleStatus).Children(func() {
			kit.Loader(c)
			kit.Help(c, i18n.T("module.inspectPackage"))
		})
	}
	if message := in.err; message != "" || p != nil && p.Error != "" {
		if message == "" {
			message = p.Error
		}
		kit.Notice(c, kit.Failure, "", message, nil)
	}
	if p == nil || p.Metadata == nil {
		return
	}
	kit.Heading(c, p.Metadata.name())
	version := p.Metadata.Version
	if p.Operation == "update" {
		version = p.PreviousVersion + " → " + p.Metadata.Version
	}
	kit.Facts(c, []kit.Fact{
		{Label: i18n.T("module.packageVersion"), Value: version},
		{Label: i18n.T("module.packagePublisher"), Value: p.Publisher},
		{Label: i18n.T("module.packageFile"), Value: p.FileName},
	})
	ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, icons.Check).FontSize(kit.HelpSize(c)).TextColor(t.Success)
		kit.Help(c, i18n.T("module.signatureVerified"))
	})
	ui.Collapsible(c, i18n.T("module.signatureDetails"), &in.details.signature, func() {
		kit.Mono(c, p.KeyFingerprint).Selectable().TextColor(t.TextMuted)
		kit.Mono(c, p.ManifestSHA256).Selectable().TextColor(t.TextMuted)
	}).FontSize(kit.HelpSize(c))
	if p.DependencyLock != nil && len(p.DependencyLock.Packages) > 0 {
		ui.Collapsible(c, i18n.T("sharedPackages.dependencies")+" · "+itoa(len(p.DependencyLock.Packages)), &in.details.dependencies, func() {
			ui.Scroll(c).MaxHeight(144).Gap(4).Children(func() {
				for _, dependency := range p.DependencyLock.Packages {
					kit.Help(c, dependency.ID+" · "+dependency.Version).Selectable()
				}
			})
		}).FontSize(kit.HelpSize(c))
	}
	if p.MigrationFrom > 0 {
		kit.Help(c, i18n.T("module.migrationDescription", "from", p.MigrationFrom, "to", p.MigrationTo))
		if len(p.MigrationPaths) > 0 {
			ui.Collapsible(c, i18n.T("module.migrationFields"), &in.details.migration, func() {
				ui.Scroll(c).MaxHeight(128).Gap(4).Children(func() {
					for _, path := range p.MigrationPaths {
						kit.Mono(c, path).Selectable().TextColor(t.TextMuted)
					}
				})
			}).FontSize(kit.HelpSize(c))
		}
	}
	consequence := "module.installDisabled"
	switch {
	case p.Metadata.Kind == "runtime":
		consequence = "sharedPackages.installDescription"
	case p.MigrationFrom > 0 && p.Operation == "update":
		consequence = "module.migrationUpdated"
	case p.MigrationFrom > 0:
		consequence = "module.migrationInstalled"
	case p.Operation == "update":
		consequence = "module.updateRetained"
	case p.RestoresSettings:
		consequence = "module.installRetained"
	}
	kit.Help(c, i18n.T(consequence))
}

// removal removes an optional module's files (its settings stay) or an
// unused runtime package, after the Host checks it again.
type removal struct {
	page       *page
	open       bool
	busy       bool
	err        string
	id, kind   string
	manifest   string
	generation int
	preview    *struct {
		ID       string          `json:"id"`
		Metadata packageMetadata `json:"metadata"`
	}
}

func (r *removal) show(id, kind, manifestSHA256 string) {
	r.generation++
	owner := r.generation
	r.open, r.err, r.preview, r.id, r.kind, r.manifest = true, "", nil, id, kind, manifestSHA256
	env := r.page.env
	params := map[string]string{"id": id}
	if kind == "runtime" {
		params["kind"], params["manifestSha256"] = kind, manifestSHA256
	}
	type result = struct {
		ID       string          `json:"id"`
		Metadata packageMetadata `json:"metadata"`
	}
	core.Go(env, func() (result, error) {
		var next result
		err := env.Host.CallCompletingInto("modules.packageRemoval.preview", params, &next)
		return next, err
	}, func(next result, err error) {
		if owner != r.generation {
			if err == nil {
				cancelPreview(env, next.ID)
			}
			return
		}
		if err != nil {
			r.err = err.Error()
			return
		}
		r.preview = &next
	})
}

func (r *removal) cancel() {
	if r.busy {
		return
	}
	r.generation++
	if r.preview != nil {
		cancelPreview(r.page.env, r.preview.ID)
	}
	r.open, r.preview = false, nil
}

func (r *removal) remove(c *ui.Context) {
	if r.preview == nil || r.busy {
		return
	}
	id := r.preview.ID
	runtime := r.kind == "runtime"
	r.busy, r.err = true, ""
	env := r.page.env
	core.Go(env, func() (core.ModuleJob, error) {
		var result core.ModuleJob
		err := env.Host.CallCompletingInto("modules.removePackage", map[string]string{"id": id}, &result)
		return result, err
	}, func(result core.ModuleJob, err error) {
		r.busy = false
		switch {
		case err != nil:
			r.err, r.preview = err.Error(), nil
			return
		case result.State != "completed":
			r.err, r.preview = result.Error, nil
			if r.err == "" {
				r.err = i18n.T("module.removeFailed")
			}
			return
		}
		r.open, r.preview = false, nil
		success := result.Warning
		if success == "" {
			success = i18n.T("module.packageRemoved")
			if runtime {
				success = i18n.T("sharedPackages.removed")
			}
		}
		r.page.reload(func() {
			r.page.env.Refresh()
			c.Toast(success)
		})
	})
}

func (r *removal) dialog(c *ui.Context) {
	if !r.open {
		return
	}
	title, description := "module.removePackage", "module.removeDescription"
	if r.kind == "runtime" {
		title, description = "sharedPackages.remove", "sharedPackages.removeDescription"
	}
	open := r.open
	confirmed := kit.Dialog(c, &open, kit.DialogOptions{
		Title: i18n.T(title), Message: i18n.T(description), Confirm: i18n.T("module.confirmRemove"),
		Cancel: i18n.T("module.cancelInstall"), Danger: true, Busy: r.busy, ConfirmDisabled: r.preview == nil,
		Body: func() {
			if r.err != "" {
				kit.Notice(c, kit.Failure, "", r.err, nil)
			}
			if r.preview != nil {
				kit.Label(c, r.preview.Metadata.name()+" · "+r.preview.Metadata.Version)
			}
		},
	})
	if confirmed {
		r.remove(c)
	}
	if !open {
		r.cancel()
	}
}

// Close implements core.Closer: leaving Home releases a check in progress.
func (p *page) Close() {
	p.install.cancel()
	p.removal.cancel()
}
