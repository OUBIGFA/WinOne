package chat

import (
	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
)

type workdirPluginDialog struct {
	open, loading, busy      bool
	taskID, directory, error string
	rows                     []struct {
		PluginID, Name string
		Enabled        bool
	}
	disabled map[string]bool
}

// Directory overrides use Cindy's actual store. They do not change global
// enabled state, task history or the owner's current composer draft.
func (p *Chat) openWorkdirPlugins() {
	row := p.conversation()
	if row == nil {
		return
	}
	d := &workdirPluginDialog{open: true, loading: true, taskID: row.ID, directory: row.Directory, disabled: map[string]bool{}}
	p.ui.workdirPlugins = d
	chatCall(p, "ghosts.workdir-preferences", map[string]string{"taskId": row.ID}, func(value struct {
		Disabled []string
		Plugins  []struct {
			PluginID, Name string
			Enabled        bool
		}
	}, err error) {
		if p.ui.workdirPlugins != d {
			return
		}
		d.loading = false
		if err != nil {
			d.error = err.Error()
			return
		}
		d.rows = value.Plugins
		for _, id := range value.Disabled {
			d.disabled[id] = true
		}
	})
}
func (p *Chat) workdirPluginsDialog(c *ui.Context) {
	d := p.ui.workdirPlugins
	if d == nil {
		return
	}
	if !d.open {
		p.ui.workdirPlugins = nil
		return
	}
	if kit.Dialog(c, &d.open, kit.DialogOptions{Title: i18n.T("chat.workdirPlugins"), Message: d.directory, Cancel: "", Confirm: i18n.T("chat.close"), Busy: d.busy, Body: func() {
		if d.loading {
			kit.Loader(c)
			return
		}
		if d.error != "" {
			kit.InlineError(c, d.error)
		}
		if len(d.rows) == 0 {
			kit.Help(c, i18n.T("chat.workdirPluginsEmpty"))
			return
		}
		for _, row := range d.rows {
			row := row
			allowed := !d.disabled[row.PluginID]
			if ui.Checkbox(c, &allowed, row.Name).Disabled(d.busy).Changed() {
				d.busy = true
				disable := !allowed
				chatCall(p, "ghosts.set-workdir-preference", map[string]any{"taskId": d.taskID, "pluginId": row.PluginID, "disabled": disable}, func(value struct{ Disabled []string }, err error) {
					d.busy = false
					if err != nil {
						d.error = err.Error()
						return
					}
					d.error = ""
					d.disabled = map[string]bool{}
					for _, id := range value.Disabled {
						d.disabled[id] = true
					}
				})
			}
			if !row.Enabled {
				kit.Status(c, i18n.T("chat.workdirPluginAsleep"))
			}
		}
	}}) {
		d.open = false
	}
}
