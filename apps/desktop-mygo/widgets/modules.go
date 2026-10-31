package widgets

import (
	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
)

// ModuleManifest names a module.
type ModuleManifest struct {
	ID           string     `json:"id"`
	Name         core.Names `json:"name"`
	Distribution string     `json:"distribution"`
}

// ModuleRuntime is the state of a module's process.
type ModuleRuntime struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	State  string `json:"state"`
	Detail string `json:"detail"`
}

// ModuleEntry is a module of the catalog.
type ModuleEntry struct {
	Manifest    ModuleManifest `json:"manifest"`
	Enabled     bool           `json:"enabled"`
	PendingStop bool           `json:"pendingStop"`
	Runtime     ModuleRuntime  `json:"runtime"`
	Blocked     bool           `json:"blocked,omitempty"`
	RecoveryID  string         `json:"recoveryId,omitempty"`
}

// PackageReference identifies an installed package.
type PackageReference struct {
	Kind           string `json:"kind"`
	ID             string `json:"id"`
	Version        string `json:"version"`
	ManifestSHA256 string `json:"manifestSha256"`
}

// SharedRuntime is a runtime package modules depend on.
type SharedRuntime struct {
	PackageReference
	Name       core.Names         `json:"name"`
	RequiredBy []PackageReference `json:"requiredBy"`
	CanRemove  bool               `json:"canRemove"`
	Error      string             `json:"error,omitempty"`
}

// ModuleSnapshot is the Host's module catalog (modules.snapshot).
type ModuleSnapshot struct {
	Revision           uint64        `json:"revision"`
	Modules            []ModuleEntry `json:"modules"`
	ConfigurationError string        `json:"configurationError,omitempty"`
	Cleanup            *struct {
		Pending bool   `json:"pending"`
		Error   string `json:"error,omitempty"`
	} `json:"cleanup,omitempty"`
	Packages *struct {
		Runtimes     []SharedRuntime               `json:"runtimes"`
		Dependencies map[string][]PackageReference `json:"dependencies"`
		Error        string                        `json:"error,omitempty"`
	} `json:"packages,omitempty"`
}

// Modules reads the module catalog and runs its lifecycle operations one
// at a time, reading the catalog again after each, so a failure never
// leaves a stale switch: Home lists every tool with it, Convenience shows
// one tool's switch per tab.
//
// React source: features/modules.tsx (ModulesPage, with and without only).
type Modules struct {
	env      *core.Env
	Snapshot *ModuleSnapshot
	Loading  bool
	LoadErr  string
	// Busy names the operation running: a module ID, "recovery" or
	// "cleanup"; one at a time, as the switches would race otherwise.
	Busy string
	Err  string
}

// NewModules reads the catalog.
func NewModules(env *core.Env) *Modules {
	m := &Modules{env: env}
	m.Reload(nil)
	return m
}

// Reload reads the catalog; done runs after, either way.
func (m *Modules) Reload(done func()) {
	m.Loading = true
	core.Go(m.env, func() (ModuleSnapshot, error) {
		var snapshot ModuleSnapshot
		err := m.env.Host.CallCompletingInto("modules.snapshot", nil, &snapshot)
		return snapshot, err
	}, func(snapshot ModuleSnapshot, err error) {
		m.Loading = false
		if err != nil {
			m.LoadErr = err.Error()
		} else {
			m.LoadErr, m.Snapshot = "", &snapshot
		}
		if done != nil {
			done()
		}
	})
}

// Finish ends an operation: show its result, read the catalog again and
// let the shell refresh the sidebar.
func (m *Modules) Finish(c *ui.Context, err error, success string) {
	if err != nil {
		m.Err = err.Error()
	} else if success != "" {
		m.env.Toast(success)
	}
	m.Reload(func() {
		m.Busy = ""
		m.env.Refresh()
	})
}

// SetEnabled turns a module on or off and follows its job.
func (m *Modules) SetEnabled(c *ui.Context, entry ModuleEntry, enabled bool) {
	if m.Snapshot == nil || m.Busy != "" {
		return
	}
	m.Busy, m.Err = entry.Manifest.ID, ""
	revision := m.Snapshot.Revision
	name := entry.Manifest.Name.String()
	core.Do(m.env, func() error {
		var job core.ModuleJob
		if err := m.env.Host.CallCompletingInto("modules.setEnabled", map[string]any{"id": entry.Manifest.ID, "enabled": enabled, "revision": revision}, &job); err != nil {
			return err
		}
		job, err := core.FollowJob(m.env, "modules", job)
		if err != nil {
			return err
		}
		if job.State != "completed" {
			if job.Error != "" {
				return core.Errorf(job.Error)
			}
			return core.Errorf(i18n.T("module.failed"))
		}
		return nil
	}, func(err error) {
		key := "module.disabled"
		if enabled {
			key = "module.enabled"
		}
		m.Finish(c, err, i18n.T(key, "name", name))
	})
}

// Run performs a module-service operation that returns a job.
func (m *Modules) Run(c *ui.Context, busy, method string, params any, failure, success string) {
	if m.Busy != "" {
		return
	}
	m.Busy, m.Err = busy, ""
	core.Do(m.env, func() error {
		var job core.ModuleJob
		if err := m.env.Host.CallCompletingInto("modules."+method, params, &job); err != nil {
			return err
		}
		if job.State != "completed" {
			if job.Error != "" {
				return core.Errorf(job.Error)
			}
			return core.Errorf(failure)
		}
		return nil
	}, func(err error) { m.Finish(c, err, success) })
}

// RecoverConfiguration retries reading the module configuration.
func (m *Modules) RecoverConfiguration(c *ui.Context) {
	m.Run(c, "recovery", "recoverConfiguration", nil, i18n.T("module.recoveryFailed"), i18n.T("module.recovered"))
}

// RecoverPackage restores or retries a blocked module package.
func (m *Modules) RecoverPackage(c *ui.Context, entry ModuleEntry) {
	if m.Snapshot == nil {
		return
	}
	id := entry.Manifest.ID
	m.Run(c, id, "recoverPackage", map[string]any{"id": id, "revision": m.Snapshot.Revision, "recoveryId": entry.RecoveryID},
		i18n.T("module.recoveryFailed"), i18n.T("module.packageRecovered", "name", entry.Manifest.Name.String()))
}

// ModuleStatus is the i18n key of a module's state.
func ModuleStatus(entry ModuleEntry) string {
	switch {
	case entry.PendingStop:
		return "moduleStatus.incomplete"
	case entry.Runtime.State == "faulted":
		return "moduleStatus.faulted"
	case entry.Runtime.State == "unavailable":
		return "moduleStatus.unavailable"
	case entry.Runtime.State == "starting" || entry.Runtime.State == "stopping":
		return "moduleStatus.working"
	case entry.Enabled:
		return "moduleStatus.enabled"
	}
	return "moduleStatus.disabled"
}

// Notices shows a failed read, operation or configuration, the last with
// its recovery.
func (m *Modules) Notices(c *ui.Context) {
	s := m.Snapshot
	switch {
	case s != nil && s.ConfigurationError != "":
		message := s.ConfigurationError
		if m.Err != "" {
			message = m.Err
		}
		kit.Notice(c, kit.Failure, "", message, func() {
			if kit.Button(c, i18n.T("module.retryRecovery"), nil, m.Busy == "recovery").Disabled(m.Busy != "").Clicked() {
				m.RecoverConfiguration(c)
			}
		})
	case m.Err != "":
		kit.Notice(c, kit.Failure, "", m.Err, nil)
	case m.LoadErr != "":
		kit.Notice(c, kit.Failure, "", m.LoadErr, nil)
	}
}

// Switch is one tool's switch at the top of its tab, with its failure and
// recovery, as ModulesPage only=X did. controlsDisabled holds the switch
// while the tab has unsaved changes or an operation running.
func (m *Modules) Switch(c *ui.Context, id string, controlsDisabled bool) {
	m.env.Activity.Report("module-availability:"+id, false, m.Busy != "", nil)
	ui.Column(c.Key("module:"+id)).FillWidth().Gap(12).Children(func() {
		m.Notices(c)
		s := m.Snapshot
		if s == nil {
			if m.Loading {
				kit.Status(c, i18n.T("module.loading"))
			} else if kit.Button(c, i18n.T("common.retry"), nil, m.Busy != "").Clicked() {
				m.Err = ""
				m.Reload(nil)
			}
			return
		}
		for _, entry := range s.Modules {
			if entry.Manifest.ID != id {
				continue
			}
			name := entry.Manifest.Name.String()
			status := i18n.T(ModuleStatus(entry))
			if m.Busy == id {
				status = i18n.T("module.updated")
			}
			label := i18n.T("module.enableTool", "name", name)
			kit.Panel(c, func() {
				kit.Row(c, label, status, func() {
					on := entry.Enabled && !entry.PendingStop
					if ui.Switch(c, &on).Label(label).
						Disabled(controlsDisabled || m.Busy != "" || entry.PendingStop || entry.Blocked || s.ConfigurationError != "").Changed() {
						m.SetEnabled(c, entry, on)
					}
				})
			})
			if (entry.Runtime.State == "faulted" || entry.Blocked) && entry.Runtime.Detail != "" {
				kit.Notice(c, kit.Failure, "", entry.Runtime.Detail, nil)
			}
			if entry.PendingStop && s.ConfigurationError == "" || entry.Blocked {
				ui.Row(c).Gap(8).Wrap().Children(func() {
					if entry.PendingStop && s.ConfigurationError == "" &&
						kit.Button(c, i18n.T("module.retryClose"), nil, false).Disabled(controlsDisabled || m.Busy != "").Clicked() {
						m.SetEnabled(c, entry, false)
					}
					if entry.Blocked {
						label := "module.retryPackage"
						if entry.RecoveryID != "" {
							label = "module.restorePackage"
						}
						if kit.Button(c, i18n.T(label), nil, m.Busy == id).Disabled(controlsDisabled || m.Busy != "").Clicked() {
							m.RecoverPackage(c, entry)
						}
					}
				})
			}
		}
	})
}
