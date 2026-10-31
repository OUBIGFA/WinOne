package convenience

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/internal/directorylinks"
)

// React source: features/directory-links-migration.tsx.

// migrationSelection is a migration's history action the history list
// chose: restore, recover or clean up a retained backup.
type migrationSelection struct {
	record directorylinks.Record
	action string
	index  int
}

// migrationJob is the Host's migration ticket (apps/host's
// directoryMigrationJob).
type migrationJob struct {
	ID        string                           `json:"id"`
	RequestID string                           `json:"requestId"`
	State     string                           `json:"state"`
	Progress  directorylinks.MigrationProgress `json:"progress"`
	Plan      *directorylinks.MigrationPlan    `json:"plan,omitempty"`
	Record    *directorylinks.Record           `json:"record,omitempty"`
	Error     string                           `json:"error,omitempty"`
	CanCancel bool                             `json:"canCancel"`
}

// migrationSize formats bytes in binary units.
func migrationSize(value float64) string {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return "—"
	}
	units := []string{"B", "KiB", "MiB", "GiB", "TiB"}
	index := 0
	for value >= 1024 && index < len(units)-1 {
		value /= 1024
		index++
	}
	decimals := 1
	if index == 0 {
		decimals = 0
	}
	return strconv.FormatFloat(value, 'f', decimals, 64) + " " + units[index]
}

func requestID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	h := hex.EncodeToString(b)
	return fmt.Sprintf("%s-%s-%s-%s-%s", h[:8], h[8:12], h[12:16], h[16:20], h[20:])
}

// linkMigration moves a folder to another place and leaves a link where
// it was, or reverses, recovers or cleans up such a move. The Host owns
// admission and the recovery journal; the window follows one ticket and
// never retries a write after a lost answer.
type linkMigration struct {
	env       *core.Env
	source    string
	directory string
	name      string
	chosen    *migrationSelection
	plan      *directorylinks.MigrationPlan
	job       *migrationJob
	pending   bool
	confirm   bool
	ack       bool
	err       string
	request   string
	closed    bool
	following bool
}

func newLinkMigration(env *core.Env) *linkMigration { return &linkMigration{env: env} }

func (m *linkMigration) busy() bool { return m.pending || m.job != nil && m.job.State == "running" }

func (m *linkMigration) mode() string {
	if m.chosen != nil {
		return m.chosen.action
	}
	return "migrate"
}

func (m *linkMigration) reset() {
	m.source, m.directory, m.name = "", "", ""
	m.chosen, m.plan, m.job = nil, nil, nil
	m.ack, m.confirm, m.err = false, false, ""
}

// Report holds the guard while a known job runs, even through a failed
// read of its state.
func (m *linkMigration) Report() {
	dirty := m.plan != nil || m.source != "" || m.directory != "" || m.name != "" || m.chosen != nil
	m.env.Activity.Report("directory-links-migration", dirty, m.busy(), m.reset)
}

// choose takes a history action.
func (m *linkMigration) choose(s migrationSelection) {
	m.chosen = &s
	m.plan, m.job, m.ack, m.err = nil, nil, false, ""
	m.source, m.directory, m.name = "", "", ""
	m.confirm = s.action != "restore"
}

// accept shows a job state; a finished job updates the history.
func (m *linkMigration) accept(value migrationJob, changed func([]directorylinks.Record)) error {
	if value.RequestID != m.request {
		return fmt.Errorf("%s", i18n.T("links.migrationUncertain"))
	}
	m.job = &value
	if value.State == "running" {
		return nil
	}
	if value.Plan != nil && value.State == "completed" {
		m.plan = value.Plan
	}
	if value.Record != nil {
		changed([]directorylinks.Record{*value.Record})
		if m.chosen != nil && m.chosen.record.ID == value.Record.ID {
			m.chosen.record = *value.Record
		}
	}
	if value.Error != "" {
		m.err = value.Error
	}
	if value.Record != nil && value.State == "completed" {
		m.source, m.directory, m.name = "", "", ""
		m.chosen, m.plan = nil, nil
	}
	return nil
}

// follow reads a running job's state four times a second.
func (m *linkMigration) follow(value migrationJob, changed func([]directorylinks.Record), done func()) {
	if err := m.accept(value, changed); err != nil {
		m.err = err.Error()
		done()
		return
	}
	if value.State != "running" || m.closed {
		done()
		return
	}
	id := value.ID
	core.Go(m.env, func() (migrationJob, error) {
		time.Sleep(250 * time.Millisecond)
		var next migrationJob
		err := m.env.Host.CallCompletingInto("directory-links.migration.status", map[string]any{"id": id}, &next)
		return next, err
	}, func(next migrationJob, err error) {
		if m.closed {
			done()
			return
		}
		if err != nil {
			m.err = err.Error()
			done()
			return
		}
		m.follow(next, changed, done)
	})
}

func (m *linkMigration) start(method string, args map[string]any, changed func([]directorylinks.Record)) {
	if m.busy() {
		return
	}
	m.pending, m.err, m.confirm, m.job = true, "", false, nil
	m.request = requestID()
	args["requestId"] = m.request
	// A spent execution token is never offered as a retry.
	if method != "migration.preview" {
		m.plan = nil
	}
	core.Go(m.env, func() (migrationJob, error) {
		var job migrationJob
		err := m.env.Host.CallCompletingInto("directory-links."+method, args, &job)
		return job, err
	}, func(job migrationJob, err error) {
		if err != nil {
			m.pending = false
			m.err = err.Error() + " " + i18n.T("links.migrationUncertain")
			return
		}
		m.follow(job, changed, func() { m.pending = false })
	})
}

func (m *linkMigration) readAgain(changed func([]directorylinks.Record)) {
	if m.pending {
		return
	}
	m.pending, m.err = true, ""
	params := map[string]any{"requestId": m.request}
	if m.job != nil && m.job.ID != "" {
		params = map[string]any{"id": m.job.ID}
	}
	core.Go(m.env, func() (migrationJob, error) {
		var job migrationJob
		err := m.env.Host.CallCompletingInto("directory-links.migration.status", params, &job)
		return job, err
	}, func(job migrationJob, err error) {
		if err != nil {
			m.pending = false
			m.err = err.Error()
			return
		}
		m.follow(job, changed, func() { m.pending = false })
	})
}

func (m *linkMigration) cancel(changed func([]directorylinks.Record)) {
	if m.job == nil || !m.job.CanCancel {
		return
	}
	id := m.job.ID
	resume := !m.pending
	if resume {
		m.pending = true
	}
	core.Go(m.env, func() (migrationJob, error) {
		var job migrationJob
		err := m.env.Host.CallCompletingInto("directory-links.migration.cancel", map[string]any{"id": id}, &job)
		return job, err
	}, func(job migrationJob, err error) {
		if err != nil {
			m.err = err.Error()
			if resume {
				m.pending = false
			}
			return
		}
		if resume {
			m.follow(job, changed, func() { m.pending = false })
		} else if err := m.accept(job, changed); err != nil {
			m.err = err.Error()
		}
	})
}

// Close cancels a running job the page leaves, as unmounting did.
func (m *linkMigration) Close() {
	m.closed = true
	if m.job != nil && m.job.State == "running" && m.job.CanCancel {
		id := m.job.ID
		go func() { _ = m.env.Host.CallCompletingInto("directory-links.migration.cancel", map[string]any{"id": id}, nil) }()
	}
}

func (m *linkMigration) pick(target *string) {
	m.pending, m.err = true, ""
	core.Go(m.env, func() (string, error) { return m.env.Dialogs.PickFolder(i18n.T("links.folder")) }, func(path string, err error) {
		m.pending = false
		if err != nil {
			m.err = err.Error()
		} else if path != "" {
			*target = path
			m.plan = nil
		}
	})
}

func (m *linkMigration) preview(changed func([]directorylinks.Record)) {
	m.ack, m.plan = false, nil
	if m.chosen != nil {
		m.start("migration.preview", map[string]any{"recordId": m.chosen.record.ID, "checkedAt": m.chosen.record.CheckedAt}, changed)
		return
	}
	m.start("migration.preview", map[string]any{"source": m.source, "directory": m.directory, "name": m.name}, changed)
}

func (m *linkMigration) execute(changed func([]directorylinks.Record)) {
	switch mode := m.mode(); mode {
	case "recover", "cleanup":
		if m.chosen == nil {
			return
		}
		path := ""
		if j := m.chosen.record.Migration; j != nil && m.chosen.index < len(j.Retained) {
			path = j.Retained[m.chosen.index].Path
		}
		m.start("migration."+mode, map[string]any{"id": m.chosen.record.ID, "index": m.chosen.index, "path": path, "confirmed": true}, changed)
	default:
		if m.plan != nil {
			m.start("migration.execute", map[string]any{"token": m.plan.ID, "confirmed": true, "acknowledged": m.ack}, changed)
		}
	}
}

func (m *linkMigration) detailPath() string {
	if m.chosen == nil {
		return ""
	}
	if m.chosen.action == "cleanup" {
		if j := m.chosen.record.Migration; j != nil && m.chosen.index < len(j.Retained) {
			return j.Retained[m.chosen.index].Path
		}
		return ""
	}
	return m.chosen.record.LinkPath
}

// folderField is a path input with its folder picker.
func (m *linkMigration) folderField(c *ui.Context, label string, value *string, locked bool) {
	kit.Label(c, label)
	ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
		if ui.TextInput(c, value).Grow(1).MinWidth(0).Label(label).Disabled(locked).Changed() {
			m.plan = nil
		}
		if kit.Button(c, i18n.T("links.folder"), icons.FolderOpen, false).Disabled(locked).Clicked() {
			m.pick(value)
		}
	})
}

func (m *linkMigration) View(c *ui.Context, disabled bool, changed func([]directorylinks.Record)) {
	locked := disabled || m.busy()
	mode := m.mode()
	ui.Column(c).FillWidth().Gap(12).Role(ui.RoleGroup).Label(i18n.T("links.migrationTitle")).Children(func() {
		kit.Help(c, i18n.T("links.migrationIntro"))
		kit.Help(c, i18n.T("links.migrationLimits"))
		if mode == "migrate" {
			m.folderField(c, i18n.T("links.migrationSource"), &m.source, locked)
			m.folderField(c, i18n.T("links.migrationDirectory"), &m.directory, locked)
			kit.Label(c, i18n.T("links.migrationName"))
			if ui.TextInput(c, &m.name).FillWidth().Placeholder(i18n.T("links.migrationNameHint")).Label(i18n.T("links.migrationName")).
				Disabled(locked).Changed() {
				m.plan = nil
			}
		} else {
			ui.Column(c).FillWidth().Gap(6).Children(func() {
				kit.Strong(c, i18n.T("links.migrationAction."+mode))
				ui.Text(c, m.detailPath()).Selectable().MinWidth(0)
				ui.Row(c).Children(func() {
					if kit.Button(c, i18n.T("links.migrationNew"), icons.Undo2, false).Disabled(locked).Clicked() {
						m.reset()
					}
				})
			})
		}
		ui.Row(c).Gap(8).Wrap().Children(func() {
			switch mode {
			case "migrate", "restore":
				missing := mode == "migrate" && (m.source == "" || m.directory == "")
				if kit.Button(c, i18n.T("links.migrationPreview"), icons.RefreshCw, false).Disabled(locked || missing).Clicked() {
					m.preview(changed)
				}
				label := "links.migrationStart"
				if mode == "restore" {
					label = "links.migrationRestore"
				}
				if kit.PrimaryButton(c, i18n.T(label), icons.Play, false).Disabled(locked || m.plan == nil).Clicked() {
					m.confirm = true
				}
			default:
				if kit.Button(c, i18n.T("links.migrationAction."+mode), nil, false).Disabled(locked).Clicked() {
					m.confirm = true
				}
			}
		})
		if p := m.plan; p != nil {
			kit.Section(c, nil, i18n.T("links.migrationPlan"), "", func() {
				ui.Row(c).FillWidth().Gap(10).Wrap().AlignItems(ui.Center).Children(func() {
					ui.Text(c, p.Source).Selectable().MinWidth(0)
					ui.Icon(c, icons.ArrowRight).FontSize(18).TextColor(c.Theme().TextMuted)
					ui.Text(c, p.Destination).Selectable().MinWidth(0)
				})
				kit.Label(c, i18n.T("links.migrationSize", "count", p.Files, "size", migrationSize(float64(p.Bytes))))
				for _, warning := range p.Warnings {
					kit.Notice(c, kit.Failure, "", warning, nil)
				}
			})
		}
		if j := m.job; j != nil {
			ui.Column(c).FillWidth().Gap(8).Role(ui.RoleStatus).Children(func() {
				state := i18n.T("links.migrationState." + j.State)
				if j.Plan != nil {
					state = i18n.T("links.migrationPlanReady")
				}
				if j.State == "running" {
					phase := j.Progress.Phase
					if phase == "" {
						phase = "checking"
					}
					state += " · " + i18n.T("links.migrationPhase."+phase)
				}
				kit.Status(c, state)
				if j.State == "running" {
					progress := float64(0)
					if j.Progress.TotalBytes > 0 {
						progress = math.Min(1, float64(j.Progress.Bytes)/float64(j.Progress.TotalBytes))
					}
					ui.Progress(c, progress).FillWidth().Label(i18n.T("links.migrationProgress"))
					kit.Help(c, fmt.Sprintf("%s / %s · %d / %d", migrationSize(float64(j.Progress.Bytes)), migrationSize(float64(j.Progress.TotalBytes)), j.Progress.Files, j.Progress.TotalFiles))
					ui.Row(c).Children(func() {
						if kit.Button(c, i18n.T("links.migrationCancel"), nil, false).Disabled(!j.CanCancel).Clicked() {
							m.cancel(changed)
						}
					})
				}
				if j.Record != nil && j.Record.Migration != nil && j.Record.Migration.Message != "" {
					kit.Notice(c, kit.Info, "", j.Record.Migration.Message, nil)
				}
			})
		}
		if m.err != "" {
			kit.Notice(c, kit.Failure, "", m.err, func() {
				if kit.Button(c, i18n.T("links.migrationReadAgain"), nil, false).Disabled(m.pending || m.request == "").Clicked() {
					m.readAgain(changed)
				}
			})
		}
	})
	m.confirmDialog(c, mode, changed)
}

func (m *linkMigration) confirmDialog(c *ui.Context, mode string, changed func([]directorylinks.Record)) {
	if !m.confirm {
		return
	}
	title := "links.migrationConfirm"
	if mode != "migrate" {
		title = "links.migrationAction." + mode
	}
	hint := "links.migrationConfirmHint"
	switch mode {
	case "cleanup":
		hint = "links.migrationCleanupHint"
	case "recover":
		hint = "links.migrationRecoverHint"
	}
	warned := m.plan != nil && len(m.plan.Warnings) > 0
	open := true
	if kit.Dialog(c, &open, kit.DialogOptions{
		Title: i18n.T(title), Message: i18n.T(hint), Cancel: i18n.T("links.cancel"),
		Confirm: i18n.T("links.migrationConfirmAction"), Danger: mode == "cleanup", Busy: m.busy(),
		ConfirmDisabled: warned && !m.ack,
		Body: func() {
			source := m.detailPath()
			if m.plan != nil && m.plan.Source != "" {
				source = m.plan.Source
			}
			ui.Text(c, source).Selectable().MinWidth(0)
			if m.plan != nil && m.plan.Destination != "" {
				ui.Text(c, m.plan.Destination).Selectable().MinWidth(0)
			}
			if warned {
				kit.Notice(c, kit.Failure, "", joinLines(m.plan.Warnings...), nil)
				ui.Checkbox(c, &m.ack, i18n.T("links.migrationAcknowledge"))
			}
		},
	}) {
		m.execute(changed)
	}
	if !open && !m.busy() {
		m.confirm = false
	}
}
