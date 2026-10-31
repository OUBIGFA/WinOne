package convenience

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/widgets"
	"winone/internal/directorylinks"
)

// React source: features/directory-links-scan.tsx and
// directory-links-archive.tsx.

// dropSources makes a pane take dropped files and folders as sources, as
// the React window's page drop did with Explorer's paths.
func dropSources(c *ui.Context, env *core.Env, pane ui.Element, disabled bool, add func([]string), fail func(string)) {
	if pane.FileDragOver() && !disabled {
		pane.Border(1, c.Theme().Accent)
	}
	if files := pane.DroppedFiles(); len(files) > 0 && !disabled {
		add(append([]string(nil), files...))
	}
}

// dropTarget makes the destination pane take a dropped folder as the
// destination.
func dropTarget(c *ui.Context, pane ui.Element, disabled bool, open func([]string)) {
	if pane.FileDragOver() && !disabled {
		pane.Border(1, c.Theme().Accent)
	}
	if files := pane.DroppedFiles(); len(files) > 0 && !disabled {
		open(append([]string(nil), files...))
	}
}

// linkScan registers the links an explicitly chosen folder holds.
type linkScan struct {
	env       *core.Env
	directory string
	recursive bool
	busy      bool
	err       string
	result    *directorylinks.ScanResult
	issues    bool
}

func newLinkScan(env *core.Env) *linkScan { return &linkScan{env: env, recursive: true} }

func (s *linkScan) Report() {
	s.env.Activity.Report("directory-links-scan", false, s.busy, nil)
}

func (s *linkScan) scan(changed func([]directorylinks.Record)) {
	directory, recursive := s.directory, s.recursive
	s.busy, s.err, s.result = true, "", nil
	core.Go(s.env, func() (directorylinks.ScanResult, error) {
		var result directorylinks.ScanResult
		err := s.env.Host.CallCompletingInto("directory-links.scan", map[string]any{"directory": directory, "recursive": recursive}, &result)
		return result, err
	}, func(result directorylinks.ScanResult, err error) {
		s.busy = false
		if err != nil {
			s.err = err.Error()
			return
		}
		s.result = &result
		changed(result.Records)
	})
}

func (s *linkScan) View(c *ui.Context, disabled bool, changed func([]directorylinks.Record)) {
	locked := s.busy || disabled
	ui.Column(c).FillWidth().Gap(12).Role(ui.RoleGroup).Label(i18n.T("links.findExisting")).Children(func() {
		kit.Help(c, i18n.T("links.scanHint"))
		if s.err != "" {
			kit.Notice(c, kit.Failure, "", s.err, nil)
		}
		kit.Label(c, i18n.T("links.scanDirectory"))
		field := ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center)
		field.Children(func() {
			ui.TextInput(c, &s.directory).Grow(1).MinWidth(0).Label(i18n.T("links.scanDirectory")).Disabled(locked)
			if kit.Button(c, i18n.T("links.folder"), icons.FolderOpen, false).Disabled(locked).Clicked() {
				core.Go(s.env, func() (string, error) { return s.env.Dialogs.PickFolder(i18n.T("links.scanDirectory")) }, func(path string, err error) {
					if err != nil {
						s.err = err.Error()
					} else if path != "" {
						s.directory = path
					}
				})
			}
		})
		widgets.DropArea(c, s.env, field, widgets.DropFolders, false, locked, func(paths []string, err error) {
			if err != nil {
				s.err = err.Error()
				return
			}
			s.directory, s.err = paths[0], ""
		})
		ui.Checkbox(c, &s.recursive, i18n.T("links.recursive")).Disabled(locked)
		ui.Row(c).Gap(8).Children(func() {
			if kit.Button(c, i18n.T("links.scan"), icons.Search, s.busy).Disabled(locked || s.directory == "").Clicked() {
				s.scan(changed)
			}
			if s.busy && kit.Button(c, i18n.T("links.cancelScan"), nil, false).Clicked() {
				go func() {
					if err := s.env.Host.CallCompletingInto("directory-links.scan.cancel", nil, nil); err != nil {
						s.env.Post(func() { s.err = err.Error() })
					}
				}()
			}
		})
		if r := s.result; r != nil {
			message := i18n.T("links.scanResult", "count", len(r.Records), "scanned", r.Scanned)
			if r.Truncated {
				message += "\n" + i18n.T("links.scanLimit")
			}
			if r.Cancelled {
				message += "\n" + i18n.T("links.scanCancelled")
			}
			kit.Notice(c, kit.Info, "", message, nil)
			if len(r.Issues) > 0 {
				ui.Collapsible(c, i18n.T("links.scanIssues", "count", len(r.Issues)), &s.issues, func() {
					for _, issue := range r.Issues {
						kit.InlineError(c, issue.Path+"："+issue.Error)
					}
				}).FontSize(kit.HelpSize(c))
			}
		}
	})
}

// linkArchive saves the selected relationships to a file, and imports a
// saved one after the Host checked it against this computer's disks.
type linkArchive struct {
	env       *core.Env
	busy      bool
	err       string
	notice    string
	preview   *directorylinks.HistoryImportPreview
	open      bool
	skippedOn bool
}

func (a *linkArchive) Report() {
	a.env.Activity.Report("directory-links-archive", a.preview != nil, a.busy, func() { a.preview, a.open = nil, false })
}

func (a *linkArchive) exportHistory(ids []string) {
	a.busy, a.err, a.notice = true, "", ""
	path := ""
	core.Do(a.env, func() error {
		directory, err := a.env.Dialogs.PickFolder(i18n.T("links.archiveExport"))
		if err != nil || directory == "" {
			return err
		}
		var result struct {
			Path string `json:"path"`
		}
		err = a.env.Host.CallCompletingInto("directory-links.history.export", map[string]any{"ids": ids, "directory": directory}, &result)
		path = result.Path
		return err
	}, func(err error) {
		a.busy = false
		switch {
		case err != nil:
			a.err = err.Error()
		case path != "":
			a.notice = i18n.T("links.archiveSaved", "path", path)
		}
	})
}

func (a *linkArchive) previewHistory() {
	a.busy, a.err, a.notice, a.preview = true, "", "", nil
	var preview *directorylinks.HistoryImportPreview
	core.Do(a.env, func() error {
		path, err := a.env.Dialogs.PickFile(i18n.T("links.archiveImport"), core.Filter{})
		if err != nil || path == "" {
			return err
		}
		var value directorylinks.HistoryImportPreview
		if err := a.env.Host.CallCompletingInto("directory-links.history.preview", map[string]any{"path": path}, &value); err != nil {
			return err
		}
		preview = &value
		return nil
	}, func(err error) {
		a.busy = false
		if err != nil {
			a.err = err.Error()
			return
		}
		a.preview, a.open = preview, preview != nil
	})
}

// importHistory commits the checked preview. Its token is spent even when
// the import fails: a backup is opened again rather than retried blind.
func (a *linkArchive) importHistory(changed func([]directorylinks.Record)) {
	if a.preview == nil || a.preview.ID == "" || a.busy {
		return
	}
	token := a.preview.ID
	a.busy, a.err = true, ""
	core.Go(a.env, func() ([]directorylinks.Record, error) {
		var rows []directorylinks.Record
		err := a.env.Host.CallCompletingInto("directory-links.history.import", map[string]any{"token": token, "confirmed": true}, &rows)
		return rows, err
	}, func(rows []directorylinks.Record, err error) {
		a.busy = false
		a.preview, a.open = nil, false
		if err != nil {
			a.err = err.Error()
			return
		}
		changed(rows)
		a.notice = i18n.T("links.archiveImported", "count", len(rows))
	})
}

func (a *linkArchive) View(c *ui.Context, ids []string, disabled bool, changed func([]directorylinks.Record)) {
	locked := a.busy || disabled
	ui.Column(c).FillWidth().Gap(8).Role(ui.RoleGroup).Label(i18n.T("links.archiveTitle")).Children(func() {
		ui.Row(c).Gap(8).Wrap().Children(func() {
			if kit.Button(c, i18n.T("links.archiveExport"), icons.Download, false).Disabled(locked || len(ids) == 0).Clicked() {
				a.exportHistory(ids)
			}
			if kit.Button(c, i18n.T("links.archiveImport"), icons.Upload, false).Disabled(locked).Clicked() {
				a.previewHistory()
			}
		})
		if a.err != "" {
			kit.Notice(c, kit.Failure, "", a.err, nil)
		}
		if a.notice != "" {
			kit.Notice(c, kit.Info, "", a.notice, nil)
		}
	})
	if a.preview == nil {
		return
	}
	p := a.preview
	if kit.Dialog(c, &a.open, kit.DialogOptions{
		Title: i18n.T("links.archivePreview"), Message: i18n.T("links.archiveHint"), Width: 560,
		Cancel: i18n.T("links.cancel"), Confirm: i18n.T("links.archiveConfirm"), Busy: a.busy,
		ConfirmDisabled: p.ID == "" || len(p.Records) == 0,
		Body: func() {
			kit.Label(c, i18n.T("links.archiveCounts", "count", len(p.Records), "issues", len(p.Issues)))
			for _, row := range p.Records {
				ui.Column(c.Key(row.ID)).FillWidth().Gap(2).Children(func() {
					ui.Text(c, row.Name+" · "+i18n.T("links.status."+row.Status)).MinWidth(0)
					kit.Help(c, row.LinkPath+" → "+row.TargetPath).Selectable()
				})
			}
			if len(p.Issues) > 0 {
				ui.Collapsible(c, i18n.T("links.archiveSkipped"), &a.skippedOn, func() {
					for _, issue := range p.Issues {
						kit.Help(c, issue.Path+"："+issue.Error)
					}
				}).FontSize(kit.HelpSize(c))
			}
		},
	}) {
		a.importHistory(changed)
	}
	if !a.open && !a.busy {
		a.preview = nil
	}
}

// joinLines shows several messages in one notice.
func joinLines(lines ...string) string { return strings.Join(lines, "\n") }
