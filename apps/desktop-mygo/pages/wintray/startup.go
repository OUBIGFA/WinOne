package wintray

import (
	"encoding/json"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

// startup is the module's own startup: whether WinOne exits once the
// sign-in tasks are done, its waits, and resetting the managed programs.
func (p *page) startup(c *ui.Context, dirty bool) {
	settings := &p.draft.Value
	kit.Group(c, icons.Rocket, i18n.T("settingsDesign.launchBehavior"), "", func() {
		exit := settings.ExitAfterManagedAppsCompleted
		if kit.Toggle(c, &exit, i18n.T("wintray.exitWhenDone"), "").Changed() && !p.blocked() {
			settings.ExitAfterManagedAppsCompleted = exit
		}
	})
	kit.Group(c, icons.Timer, i18n.T("settingsDesign.waitTimes"), "", func() {
		p.numberRow(c, intervalKey, i18n.T("wintray.interval"), &settings.StartupIntervalSeconds, 0, 120, p.blocked())
		p.numberRow(c, windowWaitKey, i18n.T("wintray.windowWait"), &settings.CloseWindowRetrySeconds, 0, 120, p.blocked())
	})
	p.reset.view(c, p, dirty)
}

// resetPreview is what resetting would remove, with the revisions the
// reset must still find.
type resetPreview struct {
	ConfigurationRevision json.RawMessage `json:"configurationRevision"`
	ModuleRevision        json.RawMessage `json:"moduleRevision"`
	ProgramCount          int             `json:"programCount"`
}

// maintenance empties the managed programs, restores their windows and
// startup entries and turns the module off, after the user confirms what
// the Host will remove. WinOne's sign-in setting, other modules, the run
// history and recovery data stay.
type maintenance struct {
	preview *resetPreview
	busy    bool
	err     string
	done    bool
}

func (m *maintenance) working(p *page, busy bool) { m.busy, p.busy = busy, busy }

func (m *maintenance) review(p *page) {
	m.working(p, true)
	m.err, m.done = "", false
	core.Go(p.env, func() (resetPreview, error) {
		var preview resetPreview
		err := p.env.Host.CallCompletingInto("modules.resetWinTray.preview", nil, &preview)
		return preview, err
	}, func(preview resetPreview, err error) {
		m.working(p, false)
		if err != nil {
			m.err = err.Error()
			return
		}
		m.preview = &preview
	})
}

func (m *maintenance) reset(p *page) {
	if m.preview == nil || m.busy {
		return
	}
	preview := *m.preview
	m.working(p, true)
	m.err = ""
	core.Do(p.env, func() error {
		var result struct {
			State string `json:"state"`
			Error string `json:"error"`
		}
		params := map[string]any{"configurationRevision": preview.ConfigurationRevision, "moduleRevision": preview.ModuleRevision, "confirmed": true}
		if err := p.env.Host.CallCompletingInto("modules.resetWinTray", params, &result); err != nil {
			return err
		}
		if result.State != "completed" {
			if result.Error != "" {
				return core.Errorf(result.Error)
			}
			return core.Errorf(i18n.T("wintray.failed"))
		}
		return nil
	}, func(err error) {
		if err != nil {
			m.err = err.Error()
		} else {
			m.done = true
		}
		m.preview = nil
		// A reset turns the module off: read its configuration rather than
		// its stopped worker, and let the shell show it off.
		if err == nil {
			p.disabled = true
		}
		p.env.Refresh()
		p.reload(false, func() { m.working(p, false) })
	})
}

func (m *maintenance) view(c *ui.Context, p *page, dirty bool) {
	ui.Column(c).FillWidth().Gap(8).Children(func() {
		ui.Row(c).Children(func() {
			if kit.Button(c, i18n.T("wintray.resetPrograms"), icons.Trash2, false).Disabled(p.busy || dirty || p.recovering()).Clicked() {
				m.review(p)
			}
		})
		if m.err != "" {
			kit.Notice(c, kit.Failure, "", m.err, nil)
		}
		if m.done {
			kit.Status(c, i18n.T("wintray.resetDone"))
		}
	})
}

func (m *maintenance) dialog(c *ui.Context, p *page) {
	if m.preview == nil {
		return
	}
	open := true
	confirm := i18n.T("wintray.confirmReset")
	if m.busy {
		confirm = i18n.T("wintray.working")
	}
	confirmed := kit.Dialog(c, &open, kit.DialogOptions{
		Title: i18n.T("wintray.resetTitle"), Message: i18n.T("wintray.resetDescription", "count", m.preview.ProgramCount),
		Confirm: confirm, Cancel: i18n.T("wintray.cancelReset"), Danger: true, Busy: m.busy,
		Body: func() { kit.Help(c, i18n.T("wintray.resetRetained")) },
	})
	if confirmed {
		m.reset(p)
		return
	}
	if !open && !m.busy {
		m.preview = nil
	}
}
