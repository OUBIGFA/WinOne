package capture

import (
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

// fileName edits the file name template and shows the name it gives,
// as React's CaptureFileName did: the example is asked of the Host 150 ms
// after the last change, once the template is not empty or the user has
// focused or edited it; a newer change discards an older answer.
type fileName struct {
	page *page
	// active is true while the tab shows; leaving it drops what is on
	// its way, as unmounting the React component did.
	active    bool
	requested bool
	// asked is the template and attempt of the last request.
	asked      string
	attempt    int
	generation int
	example    string
	err        string
	input      ui.Handle
}

func (f *fileName) leave() {
	if f.active {
		f.active = false
		f.generation++
	}
}

func (f *fileName) view(c *ui.Context) {
	d := &f.page.draft.Value
	if !f.active {
		f.active, f.requested = true, d.FileNameTemplate != ""
		f.asked, f.example, f.err = "", "", ""
	}
	label := i18n.T("capture.fileNameTemplate")
	kit.Group(c, icons.FileText, label, "", func() {
		kit.Stack(c, "", "", func() {
			in := ui.TextInput(c, &d.FileNameTemplate).Placeholder("WinOne-{timestamp}-OK").Label(label).Bind(&f.input)
			if in.Changed() || in.Focused() {
				f.requested = true
			}
			in.Error(f.err)
			kit.Help(c, i18n.T("capture.fileNameHint"))
			if f.example != "" {
				kit.Status(c, i18n.T("capture.fileNameExample")+": "+f.example).Selectable()
			}
			if f.err != "" {
				kit.InlineError(c, f.err)
				ui.Row(c).Children(func() {
					if kit.Button(c, i18n.T("common.retry"), icons.RefreshCw, false).Clicked() {
						f.attempt++
						f.input.Focus(c)
					}
				})
			}
		})
	})
	if key := d.FileNameTemplate + "\x00" + strconv.Itoa(f.attempt); f.requested && key != f.asked {
		f.asked = key
		f.preview(d.FileNameTemplate)
	}
}

func (f *fileName) preview(template string) {
	f.generation++
	owner := f.generation
	f.example, f.err = "", ""
	env := f.page.env
	core.Do(env, func() error { time.Sleep(150 * time.Millisecond); return nil }, func(error) {
		if owner != f.generation {
			return
		}
		core.Go(env, func() (string, error) {
			var result struct {
				Name *string `json:"name"`
			}
			if err := env.Host.CallCompletingInto("capture.filename.preview", map[string]string{"template": template}, &result); err != nil {
				return "", err
			}
			if result.Name == nil || strings.TrimSpace(*result.Name) == "" {
				return "", core.Errorf(i18n.T("capture.fileNameFailed"))
			}
			return *result.Name, nil
		}, func(name string, err error) {
			if owner != f.generation {
				return
			}
			if err != nil {
				f.err = err.Error()
				return
			}
			f.example = name
		})
	})
}
