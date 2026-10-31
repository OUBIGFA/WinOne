package usage

import (
	"encoding/json"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

// archiveView is React's RequestArchive: a request's archived bodies,
// read only when asked, in Magpie's bounded preview, and their export to
// a JSON file the user chooses, which the Host writes in full.
type archiveView struct {
	id     string
	record *archivePreview
	busy   bool
	err    string
	// failed is the action that failed, which Retry runs again.
	failed string
	saved  string
}

func (a *archiveView) read(u *Usage) {
	a.busy, a.err = true, ""
	id := a.id
	core.Go(u.env, func() (archivePreview, error) {
		var record archivePreview
		err := u.env.Host.CallCompletingInto("capture.ai.archive.read", map[string]string{"archive": id}, &record)
		return record, err
	}, func(record archivePreview, err error) {
		a.busy = false
		if err != nil {
			a.failed, a.err = "read", err.Error()
			return
		}
		a.record = &record
	})
}

// save exports the archive where the system's save dialog says; only that
// path reaches the Host, and a cancelled dialog sends nothing.
func (a *archiveView) save(u *Usage) {
	a.busy, a.err, a.saved = true, "", ""
	id := a.id
	core.Go(u.env, func() (string, error) {
		path, err := u.env.Dialogs.SaveFile(tr("usage.archiveDialog"), core.Filter{Name: tr("usage.archiveFilter"), Extensions: []string{"json"}}, "WinOne-request.json")
		if err != nil || path == "" {
			return "", err
		}
		if path, err = withExtension(path, ".json"); err != nil {
			return "", err
		}
		var receipt struct {
			State string `json:"state"`
			Path  string `json:"path"`
		}
		if err := u.env.Host.CallCompletingInto("capture.ai.archive.export.save", map[string]string{"archive": id, "path": path}, &receipt); err != nil {
			return "", err
		}
		if receipt.State != "saved" || receipt.Path != path {
			return "", core.Errorf(tr("usage.archiveUnconfirmed"))
		}
		return path, nil
	}, func(path string, err error) {
		a.busy = false
		if err != nil {
			a.failed, a.err = "save", err.Error()
			return
		}
		a.saved = path
	})
}

func (a *archiveView) view(c *ui.Context, u *Usage) {
	ui.Column(c).FillWidth().Gap(12).MinWidth(0).Children(func() {
		ui.Row(c).Gap(8).Wrap().Children(func() {
			if kit.Button(c, tr("usage.archiveView"), icons.Eye, false).Disabled(a.busy).Clicked() {
				a.read(u)
			}
			if kit.Button(c, tr("usage.archiveExport"), icons.Download, false).Disabled(a.busy).Clicked() {
				a.save(u)
			}
		})
		if a.busy {
			kit.Status(c, tr("usage.archiveLoading"))
		}
		if a.err != "" {
			kit.Notice(c, kit.Failure, "", a.err, func() {
				if kit.Button(c, tr("common.retry"), icons.RefreshCw, false).Disabled(a.busy).Clicked() {
					if a.failed == "save" {
						a.save(u)
					} else {
						a.read(u)
					}
				}
			})
		}
		if a.saved != "" {
			kit.Status(c, a.saved).Selectable()
		}
		record := a.record
		if record == nil {
			return
		}
		if record.Large {
			kit.Help(c, tr("usage.archiveLarge"))
		}
		for _, key := range []string{"request", "response"} {
			part := record.Request
			if key == "response" {
				part = record.Response
			}
			if part == nil {
				continue
			}
			ui.Column(c.Key(key)).FillWidth().Gap(8).MinWidth(0).Children(func() {
				kit.Strong(c, tr("usage.archive"+key)).Role(ui.RoleHeading).Level(4)
				if part.Truncated {
					kit.Help(c, tr("usage.archiveTruncated"))
				}
				if part.Omitted {
					kit.Help(c, tr("usage.archiveLarge"))
				} else {
					codeBlock(c, part.Body, 320)
				}
				if len(part.Headers) > 0 {
					headers, _ := json.MarshalIndent(part.Headers, "", "  ")
					kit.Label(c, tr("usage.archiveHeaders"))
					codeBlock(c, string(headers), 240)
				}
			})
		}
	})
}

// codeBlock shows text as it is, in the mono face, wrapped at the width
// and scrolling down past height; it can be selected and copied.
func codeBlock(c *ui.Context, text string, height float32) {
	t := c.Theme()
	ui.Scroll(c).FillWidth().MaxHeight(height).Padding(8, 10).Radius(kit.PanelRadius).Background(t.Surface).Children(func() {
		kit.Mono(c, text).Selectable().MinWidth(0)
	})
}
