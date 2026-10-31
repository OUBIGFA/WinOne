// Package selection is the Selection assistant page and the Quick
// assistant page: one editor of an assistant's toolbar actions (order,
// visibility, names, icons, models and instructions, custom actions), its
// general preferences and its shortcut. Each assistant saves its own
// settings.
//
// React source: features/selection.tsx (one component for both, by
// moduleId) with ai-model-select.tsx and ui/path-drop.ts; main.tsx mounts
// both with their module's state. Feature parity:
// docs/mygo-parity/selection.md and quick-assistant.md. Changes from that
// layout: rows can also be dragged to reorder (the position list stays,
// for the keyboard), success shows as a toast, loading shows placeholder
// rows, and dropped programs are added by file name.
package selection

import (
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/shell"
	"winone/desktopmygo/widgets"
)

func init() {
	shell.Register("selection", core.Settings, func(env *core.Env) core.Page { return newPage(env, "selection") })
	shell.Register("quick-assistant", core.Settings, func(env *core.Env) core.Page { return newPage(env, "quick-assistant") })
}

// The page's tabs, in their order.
const (
	tabToolbar = iota
	tabGeneral
	tabShortcuts
)

// action is a toolbar action as the Host stores it
// (resultwindow.SelectionAction); fields it adds later are kept by
// core.Preserved.
type action struct {
	Selection   *widgets.AISelection `json:"selection,omitempty"`
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	EnglishName string               `json:"englishName,omitempty"`
	Icon        string               `json:"icon"`
	Kind        string               `json:"kind"`
	Prompt      string               `json:"prompt,omitempty"`
	Enabled     bool                 `json:"enabled"`
}

// preferences are the fields of both assistants' settings. Each document
// has only some of them (the Selection assistant its triggers and
// exclusions, the Quick assistant its window behavior), and the Host
// refuses fields it does not know: absent ones stay absent (nil).
type preferences struct {
	Automatic     *bool                    `json:"automatic,omitempty"`
	Keyboard      *bool                    `json:"keyboard,omitempty"`
	ExcludedApps  *[]string                `json:"excludedApps,omitempty"`
	AlwaysOnTop   *bool                    `json:"alwaysOnTop,omitempty"`
	ShowReasoning *bool                    `json:"showReasoning,omitempty"`
	Actions       []core.Preserved[action] `json:"actions,omitempty"`
}

type document = core.Preserved[preferences]

type page struct {
	env    *core.Env
	module string // "selection" or "quick-assistant"
	quick  bool
	tab    int
	// shown is the tab of the last frame, to read the AI services again
	// each time the toolbar shows, as React did.
	shown int

	saved, draft *document
	// excluded is the exclusion list as typed, one program per line.
	excluded string
	loading  bool
	working  bool
	read     int
	err      string
	opening  bool

	// expanded is the action whose editor is open, one at a time;
	// focus is a newly added action whose name takes the focus.
	expanded, focus string
	// excludedField is the exclusion list's text area.
	excludedField ui.Handle
	// change is a move or removal a row asked for, applied once every row
	// is built.
	change func()

	providers []widgets.AIProvider
	aiLoaded  bool
	aiErr     string
	aiRead    int

	shortcuts *widgets.Shortcuts
}

func newPage(env *core.Env, module string) *page {
	p := &page{env: env, module: module, quick: module == "quick-assistant", shown: -1}
	p.load()
	return p
}

// moduleDisabled reports whether the Host's snapshot shows the assistant
// turned off.
func (p *page) moduleDisabled() bool {
	for _, module := range p.env.Snapshot().Modules {
		if module.ID == p.module {
			return module.State == "disabled"
		}
	}
	return false
}

func (p *page) busy() bool { return p.working || p.loading }

// load reads the settings, dropping the draft; Undo and discarding
// changes read them again too.
func (p *page) load() {
	p.read++
	owner := p.read
	p.loading, p.err = true, ""
	core.Go(p.env, func() (document, error) {
		var value document
		err := p.env.Host.CallCompletingInto(p.module+".settings.get", nil, &value)
		return value, err
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
	saved, draft := value, clone(value)
	p.saved, p.draft = &saved, &draft
	p.excluded = ""
	if value.Value.ExcludedApps != nil {
		p.excluded = strings.Join(*value.Value.ExcludedApps, "\n")
	}
}

// clone copies a document so editing one leaves the other as it was.
func clone(d document) document {
	d.Value.Actions = slices.Clone(d.Value.Actions)
	for i := range d.Value.Actions {
		if s := d.Value.Actions[i].Value.Selection; s != nil {
			copied := *s
			d.Value.Actions[i].Value.Selection = &copied
		}
	}
	if apps := d.Value.ExcludedApps; apps != nil {
		copied := slices.Clone(*apps)
		d.Value.ExcludedApps = &copied
	}
	for _, field := range []**bool{&d.Value.Automatic, &d.Value.Keyboard, &d.Value.AlwaysOnTop, &d.Value.ShowReasoning} {
		if *field != nil {
			copied := **field
			*field = &copied
		}
	}
	return d
}

// pending is the draft as it would be saved: for the Selection assistant,
// with the exclusion list read from its text, one program per line.
func (p *page) pending() document {
	draft := clone(*p.draft)
	if !p.quick {
		var apps []string
		for _, line := range strings.Split(p.excluded, "\n") {
			if line = strings.TrimSpace(line); line != "" {
				apps = append(apps, line)
			}
		}
		if apps == nil {
			apps = []string{}
		}
		// An empty list is no change from a document that had none.
		if len(apps) > 0 || p.saved.Value.ExcludedApps != nil {
			draft.Value.ExcludedApps = &apps
		}
	}
	return draft
}

// dirty compares the draft as it would be saved with the saved settings,
// field by field; both share the fields this page does not declare.
func (p *page) dirty() bool {
	if p.draft == nil || p.saved == nil {
		return false
	}
	return !reflect.DeepEqual(p.pending().Value, p.saved.Value)
}

func (p *page) save() {
	if p.draft == nil || p.busy() {
		return
	}
	p.working, p.err = true, ""
	pending := p.pending()
	core.Go(p.env, func() (document, error) {
		var saved document
		err := p.env.Host.CallCompletingInto(p.module+".settings.save", pending, &saved)
		return saved, err
	}, func(saved document, err error) {
		p.working = false
		if err != nil {
			p.err = err.Error()
			return
		}
		p.read++ // a read still on its way is older than this answer
		p.loading = false
		p.accept(saved)
		p.env.Toast(i18n.T("common.saved"))
	})
}

// loadAI reads the AI services the action pickers offer. A failure is
// shown above the actions; the services read before stay offered.
func (p *page) loadAI() {
	p.aiRead++
	owner := p.aiRead
	p.aiErr = ""
	core.Go(p.env, func() ([]widgets.AIProvider, error) {
		var configuration struct {
			Providers []widgets.AIProvider `json:"providers"`
		}
		err := p.env.Host.CallCompletingInto("capture.ai.configuration.get", nil, &configuration)
		return configuration.Providers, err
	}, func(providers []widgets.AIProvider, err error) {
		if owner != p.aiRead {
			return
		}
		if err != nil {
			p.aiErr = err.Error()
			return
		}
		p.providers, p.aiLoaded = providers, true
	})
}

func (p *page) View(c *ui.Context) {
	p.env.Activity.Report(p.module, p.dirty(), p.working, p.load)
	if p.shortcuts != nil && p.tab != tabShortcuts {
		// Its draft lives on while another tab shows.
		p.shortcuts.Report()
	}
	kit.Page(c, func() {
		p.tabBar(c)
		if p.tab == tabToolbar && p.shown != tabToolbar {
			p.loadAI()
		}
		p.shown = p.tab
		// The settings' state and errors (also opening the Quick
		// assistant's) show above every tab, as in React.
		if p.draft == nil && p.err == "" {
			key := "selection.loading"
			if p.quick {
				key = "quickAssistant.loading"
			}
			kit.Status(c, i18n.T(key))
		}
		if p.err != "" {
			kit.Notice(c, kit.Failure, "", p.err, func() {
				if !p.dirty() && kit.Button(c, i18n.T("common.retry"), icons.RefreshCw, p.loading).Disabled(p.busy()).Clicked() {
					p.load()
				}
			})
		}
		if p.tab == tabShortcuts {
			if p.shortcuts == nil {
				p.shortcuts = widgets.NewShortcuts(p.env, p.module)
			}
			p.shortcuts.View(c)
			return
		}
		if p.draft == nil {
			if p.err == "" {
				kit.Skeleton(c, 5)
			}
			return
		}
		p.env.Rail.Set(
			core.Action{Label: i18n.T("selection.save"), Disabled: !p.dirty() || p.busy(), Busy: p.working, Run: p.save},
			core.Action{Disabled: !p.dirty() || p.busy(), Run: p.load},
		)
		ui.Column(c).FillWidth().Gap(kit.GroupGap).Disabled(p.busy()).Children(func() {
			if p.tab == tabToolbar {
				p.toolbar(c)
			} else {
				p.general(c)
			}
		})
	})
}

// tabBar is the tabs and, for the Quick assistant, the button that opens
// it, at the end of the same line.
func (p *page) tabBar(c *ui.Context) {
	tabs := []kit.Tab{
		{Label: i18n.T("selection.toolbar"), Icon: icons.SlidersHorizontal},
		{Label: i18n.T("selection.general"), Icon: icons.Settings},
		{Label: i18n.T("shortcuts.title"), Icon: icons.SendHorizontal},
	}
	if !p.quick {
		kit.Tabs(c, &p.tab, tabs, p.env.RequestChange)
		return
	}
	// The button ends the tab row, on the same hairline; the tabs shorten
	// their titles before it would wrap.
	ui.Row(c).FillWidth().AlignItems(ui.Stretch).Children(func() {
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Children(func() {
			kit.Tabs(c, &p.tab, tabs, p.env.RequestChange)
		})
		ui.Row(c).Shrink(0).PaddingStart(12).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(kit.Colors(c).Hairline).
			Children(func() {
				b := kit.Button(c, i18n.T("quickAssistant.open"), icons.Astroid, p.opening).Disabled(p.moduleDisabled() || p.busy())
				if b.Clicked() {
					p.open()
				}
			})
	})
}

func (p *page) general(c *ui.Context) {
	d := &p.draft.Value
	if p.moduleDisabled() {
		kit.Help(c, i18n.T("module.disabledTitle", "name", i18n.T("nav."+p.module)))
	}
	if p.quick {
		kit.Group(c, icons.AppWindow, i18n.T("settingsDesign.windowBehavior"), "", func() {
			onTop := d.AlwaysOnTop != nil && *d.AlwaysOnTop
			if kit.Toggle(c, &onTop, i18n.T("quickAssistant.alwaysOnTop"), "").Changed() {
				d.AlwaysOnTop = &onTop
			}
			// Reasoning shows unless it was turned off.
			reasoning := d.ShowReasoning == nil || *d.ShowReasoning
			if kit.Toggle(c, &reasoning, i18n.T("quickAssistant.showReasoning"), "").Changed() {
				d.ShowReasoning = &reasoning
			}
		})
		return
	}
	kit.Group(c, icons.MousePointerClick, i18n.T("settingsDesign.selectionTriggers"), "", func() {
		automatic := d.Automatic != nil && *d.Automatic
		if kit.Toggle(c, &automatic, i18n.T("selection.automatic"), "").Changed() {
			d.Automatic = &automatic
		}
		kit.Indented(c, func() {
			keyboard := d.Keyboard != nil && *d.Keyboard
			help := ""
			if !automatic {
				help = i18n.T("settingsDesign.selectionKeyboardRequires")
			}
			if kit.Toggle(c, &keyboard, i18n.T("selection.keyboard"), help).Disabled(!automatic).Changed() {
				d.Keyboard = &keyboard
			}
		})
	})
	kit.Group(c, icons.Ban, i18n.T("selection.excluded"), "", func() {
		zone := kit.Stack(c, "", "", func() {
			ui.TextArea(c, &p.excluded).Label(i18n.T("selection.excluded")).Lines(5, 14).Bind(&p.excludedField)
			kit.Help(c, i18n.T("selection.excludedHint"))
		})
		widgets.DropArea(c, p.env, zone, widgets.DropFiles, true, p.busy(), func(paths []string, err error) {
			if err != nil {
				p.err = err.Error()
				return
			}
			p.err = ""
			p.excluded = appendPrograms(p.excluded, paths)
		})
	})
}

// appendPrograms adds the programs of dropped files to the exclusion
// list, once each. The list takes file names, as notepad.exe, which the
// Host requires; React added whole paths, which it then refused.
func appendPrograms(text string, paths []string) string {
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		if line = strings.TrimSuffix(line, "\r"); line != "" {
			lines = append(lines, line)
		}
	}
	for _, path := range paths {
		if name := filepath.Base(path); !slices.Contains(lines, name) {
			lines = append(lines, name)
		}
	}
	return strings.Join(lines, "\n")
}

// Close implements core.Closer: leaving the page ends a shortcut
// recording and drops answers still on their way.
func (p *page) Close() {
	p.read++
	p.aiRead++
	if p.shortcuts != nil {
		p.shortcuts.Close()
	}
}
