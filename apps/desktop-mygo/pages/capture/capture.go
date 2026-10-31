// Package capture is the Capture page: what happens after a capture, how
// its files are named, scrolling capture, text recognition and
// translation, and the capture shortcuts.
//
// React source: features/capture.tsx, capture-filename.tsx,
// capture-ai-settings.tsx and ai-model-select.tsx; main.tsx mounts the
// page with the module's state. Feature parity: docs/mygo-parity/capture.md.
// Changes from that layout: groups are panels of rows, success shows as a
// toast, the scroll interval steps with buttons beside the field, and
// loading shows placeholder rows.
package capture

import (
	"strconv"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/shell"
	"winone/desktopmygo/widgets"
)

func init() {
	shell.Register("capture", core.Settings, func(env *core.Env) core.Page { return newPage(env) })
}

// The page's tabs, in their order.
const (
	tabGeneral = iota
	tabFileName
	tabScroll
	tabAI
	tabShortcuts
)

// settings are the capture preferences this page edits. The Host's
// document has more (schema, revision, theme, toolbar), kept as they were
// by core.Preserved.
type settings struct {
	OutputDirectory  string `json:"outputDirectory"`
	CopyToClipboard  bool   `json:"copyToClipboard"`
	AskWhereToSave   bool   `json:"askWhereToSave"`
	OpenEditor       bool   `json:"openEditor"`
	PinAfterCapture  bool   `json:"pinAfterCapture"`
	ScrollDelay      int    `json:"scrollDelay"`
	ScrollFromTop    bool   `json:"scrollFromTop"`
	FileNameTemplate string `json:"fileNameTemplate"`
}

type document = core.Preserved[settings]

type page struct {
	env *core.Env
	tab int

	// saved is the Host's document as last read or saved; draft is the
	// page's edit of it. scrollText is the scroll interval as typed, kept
	// when it is not a valid interval so the user can correct it.
	saved, draft *document
	scrollText   string
	// disabled is the module's state the settings were read for.
	disabled bool
	loading  bool
	read     int // discards a read that a newer one replaced
	saving   bool
	choosing bool
	err      string

	scrollField ui.Handle
	name        fileName
	ai          *aiSection
	shortcuts   *widgets.Shortcuts
}

func newPage(env *core.Env) *page {
	p := &page{env: env}
	p.name.page = p
	p.disabled = p.moduleDisabled()
	p.reload()
	return p
}

// moduleDisabled reports whether the Host's snapshot shows the capture
// module turned off; its settings are then read without starting it.
func (p *page) moduleDisabled() bool {
	for _, module := range p.env.Snapshot().Modules {
		if module.ID == "capture" {
			return module.State == "disabled"
		}
	}
	return false
}

func (p *page) busy() bool { return p.saving || p.choosing }

// reload reads the settings again, dropping the draft.
func (p *page) reload() {
	p.read++
	owner, disabled := p.read, p.disabled
	p.loading, p.err = true, ""
	core.Go(p.env, func() (document, error) {
		if disabled {
			var value document
			err := p.env.Host.CallCompletingInto("capture.settings.get", nil, &value)
			return value, err
		}
		var snapshot struct {
			Settings document `json:"settings"`
		}
		err := p.env.Host.CallCompletingInto("capture.snapshot", nil, &snapshot)
		return snapshot.Settings, err
	}, func(value document, err error) {
		if owner != p.read {
			return
		}
		p.loading = false
		if err != nil {
			p.err = err.Error()
			return
		}
		p.accept(value)
	})
}

// accept makes value the saved baseline and the draft.
func (p *page) accept(value document) {
	saved, draft := value, value
	p.saved, p.draft = &saved, &draft
	p.scrollText = strconv.Itoa(value.Value.ScrollDelay)
}

// discard reverts the draft to the saved settings in place, without the
// Host, and clears the page's error.
func (p *page) discard() {
	if p.saved == nil {
		return
	}
	draft := *p.saved
	p.draft, p.err = &draft, ""
	p.scrollText = strconv.Itoa(draft.Value.ScrollDelay)
}

// validScroll reports whether the typed scroll interval is a whole number
// of milliseconds from 100 to 2000, the only intervals that can be saved.
func (p *page) validScroll() bool {
	_, ok := scrollDelay(p.scrollText)
	return ok
}

func (p *page) dirty() bool {
	return p.draft != nil && p.saved != nil && (p.draft.Value != p.saved.Value || !p.validScroll())
}

func (p *page) save() {
	if p.draft == nil || p.busy() || !p.validScroll() {
		return
	}
	p.saving, p.err = true, ""
	draft := *p.draft
	core.Go(p.env, func() (document, error) {
		var saved document
		err := p.env.Host.CallCompletingInto("capture.settings.save", draft, &saved)
		return saved, err
	}, func(saved document, err error) {
		p.saving = false
		if err != nil {
			p.err = err.Error()
			return
		}
		p.accept(saved)
		p.env.Toast(i18n.T("common.saved"))
	})
}

// browse chooses the save folder in Windows' folder picker. The page is
// busy until the picker closes, so leaving it waits for the answer.
func (p *page) browse() {
	if p.busy() || p.draft == nil {
		return
	}
	p.choosing, p.err = true, ""
	core.Go(p.env, func() (string, error) {
		return p.env.Dialogs.PickFolder(i18n.T("capture.pickFolderTitle"))
	}, func(path string, err error) {
		p.choosing = false
		switch {
		case err != nil:
			p.err = err.Error()
		case path != "" && p.draft != nil:
			p.draft.Value.OutputDirectory = path
		}
	})
}

func (p *page) View(c *ui.Context) {
	if disabled := p.moduleDisabled(); disabled != p.disabled {
		// The module was turned on or off: its settings come from
		// another source now, as React reloaded on the disabled prop.
		p.disabled = disabled
		p.reload()
	}
	p.env.Activity.Report("capture", p.dirty(), p.busy(), p.discard)
	if p.ai != nil {
		p.ai.report()
	}
	if p.shortcuts != nil && p.tab != tabShortcuts {
		// Its draft lives on while another tab shows.
		p.shortcuts.Report()
	}
	kit.Page(c, func() {
		kit.Tabs(c, &p.tab, []kit.Tab{
			{Label: i18n.T("capture.after"), Icon: icons.ScanLine},
			{Label: i18n.T("settingsDesign.captureNaming"), Icon: icons.FileType},
			{Label: i18n.T("settingsDesign.captureScrolling"), Icon: icons.ScrollText},
			{Label: i18n.T("settingsDesign.captureAI"), Icon: icons.Languages},
			{Label: i18n.T("shortcuts.title"), Icon: icons.SendHorizontal},
		}, p.env.RequestChange)
		if p.err != "" && p.draft != nil {
			kit.Notice(c, kit.Failure, "", p.err, func() {
				if kit.Button(c, i18n.T("capture.discard"), icons.RefreshCw, false).Disabled(p.busy()).Clicked() {
					p.reload()
				}
			})
		}
		switch p.tab {
		case tabAI:
			// Made when the tab first shows, then kept with its draft.
			if p.ai == nil {
				p.ai = newAISection(p.env)
			}
			p.ai.view(c)
		case tabShortcuts:
			if p.shortcuts == nil {
				p.shortcuts = widgets.NewShortcuts(p.env, "capture")
			}
			p.shortcuts.View(c)
		default:
			p.settingsTab(c)
		}
	})
	if p.tab != tabFileName {
		p.name.leave()
	}
}

// settingsTab shows one of the three tabs that edit the capture settings,
// and their Save and Undo.
func (p *page) settingsTab(c *ui.Context) {
	if p.draft == nil {
		p.unavailable(c)
		return
	}
	p.env.Rail.Set(
		core.Action{Label: i18n.T("capture.save"), Disabled: p.busy() || !p.dirty() || !p.validScroll(), Busy: p.saving, Run: p.save},
		core.Action{Disabled: p.busy() || !p.dirty(), Run: p.discard},
	)
	ui.Column(c).FillWidth().Gap(kit.GroupGap).Disabled(p.busy()).Children(func() {
		switch p.tab {
		case tabGeneral:
			p.general(c)
		case tabFileName:
			p.name.view(c)
		case tabScroll:
			p.scrolling(c)
		}
	})
	if !p.validScroll() && p.tab != tabScroll {
		kit.Notice(c, kit.Failure, "", i18n.T("capture.invalidScrollDelay"), func() {
			if kit.Button(c, i18n.T("capture.checkScrollDelay"), nil, false).Clicked() {
				// The draft stays; only the view changes, so no guard.
				p.tab = tabScroll
				p.scrollField.Focus(c)
			}
		})
	}
}

// unavailable stands for the settings until they are read.
func (p *page) unavailable(c *ui.Context) {
	if p.err != "" {
		kit.Notice(c, kit.Failure, "", p.err, func() {
			if kit.Button(c, i18n.T("common.retry"), icons.RefreshCw, p.loading).Disabled(p.busy()).Clicked() {
				p.reload()
			}
		})
		return
	}
	ui.Column(c).FillWidth().Gap(16).Children(func() {
		kit.Status(c, i18n.T("capture.loading"))
		kit.Skeleton(c, 5)
	})
}

func (p *page) general(c *ui.Context) {
	d := &p.draft.Value
	kit.Section(c, icons.Workflow, i18n.T("settingsDesign.captureWorkflow"), "", func() {
		chosen := 1
		if d.OpenEditor {
			chosen = 0
		}
		if kit.Choices(c, &chosen, []kit.Option{
			{Label: i18n.T("capture.editFirst"), Help: i18n.T("settingsDesign.captureEditHint")},
			{Label: i18n.T("capture.quickSave"), Help: i18n.T("settingsDesign.captureSaveHint")},
		}) {
			d.OpenEditor = chosen == 0
			if !d.OpenEditor {
				// Saving at once has no editor to ask where to save.
				d.AskWhereToSave = false
			}
		}
	})
	kit.Group(c, icons.ListChecks, i18n.T("capture.outputOptions"), "", func() {
		checkRow(c, &d.CopyToClipboard, i18n.T("capture.clipboard"))
		checkRow(c, &d.PinAfterCapture, i18n.T("capture.pin"))
	})
	kit.Group(c, icons.FolderOpen, i18n.T("capture.saveLocation"), "", func() {
		if d.OpenEditor {
			kit.Toggle(c, &d.AskWhereToSave, i18n.T("capture.chooseEveryTime"), "")
		}
		var choose ui.Element
		zone := kit.Stack(c, i18n.T("capture.directory"), "", func() {
			choose = kit.PathField(c, d.OutputDirectory, "", i18n.T("capture.chooseDirectory"), p.busy())
		})
		if choose.Clicked() {
			p.browse()
		}
		widgets.DropArea(c, p.env, zone, widgets.DropFolders, false, p.busy() || p.draft == nil, func(paths []string, err error) {
			switch {
			case err != nil:
				p.err = err.Error()
			case p.draft != nil:
				p.err = ""
				p.draft.Value.OutputDirectory = paths[0]
			}
		})
	})
}

// checkRow is a setting row holding a check box with its label.
func checkRow(c *ui.Context, value *bool, label string) {
	ui.Row(c).FillWidth().MinHeight(kit.RowMinHeight).Padding(10, 16).AlignItems(ui.Center).Children(func() {
		ui.Checkbox(c, value, label)
	})
}

func (p *page) scrolling(c *ui.Context) {
	d := &p.draft.Value
	kit.Group(c, icons.ScrollText, i18n.T("settingsDesign.captureScrolling"), "", func() {
		kit.Toggle(c, &d.ScrollFromTop, i18n.T("capture.fromTop"), "")
		p.scrollDelayRow(c)
	})
}

// Close implements core.Closer: leaving the page ends a shortcut
// recording and drops answers still on their way.
func (p *page) Close() {
	p.read++
	p.name.leave()
	if p.shortcuts != nil {
		p.shortcuts.Close()
	}
	if p.ai != nil {
		p.ai.read++
	}
}
