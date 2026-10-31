package selection

import (
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/widgets"
)

// maxActions is the Host's limit of toolbar actions.
const maxActions = 24

// Widths of the position and visibility columns, so the column headings
// stand over their controls in every row.
const (
	positionWidth = 76
	showWidth     = 52
)

// actionIcons are the icons an action can take, in the picker's order,
// by the names the Host stores (resultwindow.ValidateActions).
var actionIcons = []struct {
	name string
	icon *ui.SVG
}{
	{"copy", icons.Copy}, {"translate", icons.Languages}, {"explain", icons.MessageCircleQuestion},
	{"summarize", icons.ListCheck}, {"polish", icons.PenLine}, {"code", icons.CodeXml},
	{"conversation", icons.MessageCircleMore}, {"service", icons.PlugZap}, {"book", icons.BookOpen},
	{"idea", icons.Lightbulb}, {"search", icons.Search}, {"document", icons.FileText},
	{"check", icons.CheckCheck}, {"write", icons.NotebookPen}, {"mail", icons.Mail},
	{"globe", icons.Globe}, {"learn", icons.GraduationCap}, {"calculate", icons.Calculator},
	{"terminal", icons.Terminal}, {"braces", icons.Braces}, {"tasks", icons.ListChecks},
	{"text", icons.TextCursorInput}, {"case", icons.CaseSensitive}, {"wand", icons.WandSparkles},
	{"astroid", icons.Astroid},
}

// actionIcon returns the icon of an action, Sparkles for one it does not
// know.
func actionIcon(name string) *ui.SVG {
	for _, entry := range actionIcons {
		if entry.name == name {
			return entry.icon
		}
	}
	return icons.Sparkles
}

// draggedAction is what a row carries while it is dragged to a new place.
type draggedAction struct{ id string }

// displayName is an action's name in the window's language: English names
// are optional, so the English window falls back to the Chinese one.
func displayName(a *action) string {
	if i18n.Locale() == i18n.English && a.EnglishName != "" {
		return a.EnglishName
	}
	return a.Name
}

func (p *page) actions() []core.Preserved[action] { return p.draft.Value.Actions }

// move puts the action at from at position to, as the position list and
// dragging do.
func (p *page) move(from, to int) {
	actions := p.draft.Value.Actions
	if from == to || from < 0 || to < 0 || from >= len(actions) || to >= len(actions) {
		return
	}
	moved := actions[from]
	actions = append(actions[:from], actions[from+1:]...)
	actions = append(actions[:to], append([]core.Preserved[action]{moved}, actions[to:]...)...)
	p.draft.Value.Actions = actions
}

func (p *page) index(id string) int {
	for i := range p.draft.Value.Actions {
		if p.draft.Value.Actions[i].Value.ID == id {
			return i
		}
	}
	return -1
}

// add appends a custom AI action and opens it with its name selected, as
// React did, up to the Host's limit.
func (p *page) add() {
	if len(p.actions()) >= maxActions {
		return
	}
	id := "custom-" + uuid()
	p.draft.Value.Actions = append(p.draft.Value.Actions, core.Preserved[action]{Value: action{
		ID: id, Name: i18n.T("selection.newAction"), Icon: "polish", Kind: "ai", Enabled: true,
	}})
	p.expanded, p.focus = id, id
}

// uuid returns a random version 4 UUID in lower case, as
// crypto.randomUUID did: the Host takes action IDs of lower-case letters,
// digits and hyphens.
func uuid() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	s := hex.EncodeToString(b[:])
	return s[0:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:32]
}

func (p *page) remove(id string) {
	if i := p.index(id); i >= 0 {
		actions := p.draft.Value.Actions
		p.draft.Value.Actions = append(actions[:i:i], actions[i+1:]...)
	}
	p.expanded = ""
}

func (p *page) toolbar(c *ui.Context) {
	ui.Column(c).FillWidth().Gap(kit.HeadingGap).Children(func() {
		kit.SectionHeading(c, nil, i18n.T("settingsDesign.toolbarActions"), i18n.T("selection.toolbarHint"))
		if p.aiErr != "" {
			kit.Notice(c, kit.Failure, "", p.aiErr, func() {
				if kit.Button(c, i18n.T("common.retry"), icons.RefreshCw, false).Clicked() {
					p.loadAI()
				}
			})
		}
		kit.Panel(c, func() {
			p.columnHeadings(c)
			actions := p.actions()
			for i := range actions {
				p.actionRow(c, i, len(actions))
			}
			// Rows change the list only once all are built, so no row is
			// built twice or skipped in this frame.
			if change := p.change; change != nil {
				p.change = nil
				change()
			}
		}).Role(ui.RoleList)
		ui.Row(c).Margin(6, 0, 0, 0).Children(func() {
			if kit.Button(c, i18n.T("selection.add"), icons.Plus, false).Disabled(len(p.actions()) >= maxActions).Clicked() {
				p.add()
			}
		})
	})
	if p.quick {
		title := i18n.T("settingsDesign.quickToolbarHelp")
		ui.Column(c).FillWidth().Gap(6).Role(ui.RoleGroup).Label(title).Children(func() {
			kit.Strong(c, title)
			for _, key := range []string{"settingsDesign.quickToolbarUse", "settingsDesign.quickToolbarDetails"} {
				ui.Row(c).Gap(8).AlignItems(ui.Start).Children(func() {
					kit.Help(c, "•").Shrink(0)
					kit.Help(c, i18n.T(key)).Shrink(1).MinWidth(0)
				})
			}
		})
	}
}

// columnHeadings names the columns of the action rows; the controls in
// each row carry their own names for assistive technology.
func (p *page) columnHeadings(c *ui.Context) {
	ui.Row(c).FillWidth().Padding(8, 16, 8, 8).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Width(16).Shrink(0)
		kit.Caption(c, i18n.T("settingsDesign.action")).Grow(1).MinWidth(0).SingleLine().Padding(0, 8).Role(ui.RoleNone)
		kit.Caption(c, i18n.T("selection.position")).Width(positionWidth).Shrink(0).SingleLine().Role(ui.RoleNone)
		kit.Caption(c, i18n.T("selection.show")).Width(showWidth).Shrink(0).SingleLine().Role(ui.RoleNone)
	})
}

// actionRow is one action: a handle to drag it, its name that opens its
// editor, its position and whether it shows, and the editor while open.
func (p *page) actionRow(c *ui.Context, index, count int) {
	t := c.Theme()
	a := &p.draft.Value.Actions[index].Value
	id := a.ID
	name := displayName(a)
	shown := name
	if shown == "" {
		shown = i18n.T("selection.newAction")
	}
	item := ui.Column(c.Key(id)).FillWidth().Role(ui.RoleListItem)
	dragged, over := ui.DragOver[draggedAction](item)
	if dropped, ok := ui.Drop[draggedAction](item); ok && dropped.id != id {
		from := p.index(dropped.id)
		p.change = func() { p.move(from, index) }
	}
	from := -1
	if over && dragged.id != id {
		from = p.index(dragged.id)
	}
	if p.focus == id {
		item.ScrollIntoView()
	}
	item.Children(func() {
		if from >= 0 {
			// Where the dragged action would go: above this one when it
			// comes from below, under it when it comes from above.
			line := ui.Box(c).Absolute().Left(8).Right(8).Height(2).Radius(1).Background(t.Accent)
			if from > index {
				line.Top(-1)
			} else {
				line.Bottom(-1)
			}
		}
		open := p.expanded == id
		header := ui.Row(c).FillWidth().Padding(6, 16, 6, 8).Gap(12).AlignItems(ui.Center).Drag(draggedAction{id})
		if header.Dragging() {
			header.Opacity(0.4)
		}
		var parts ui.CollapsibleParts
		header.Children(func() {
			ui.Icon(c, icons.GripVertical).FontSize(16).TextColor(t.TextMuted).Shrink(0).Cursor(ui.CursorGrab).
				Tooltip(i18n.T("selection.dragToReorder"))
			parts = ui.CollapsibleBase(c, &open)
			trigger := parts.Trigger.Grow(1).Shrink(1).MinWidth(0).MinHeight(36).Padding(6, 8).Gap(12).Radius(t.Radius).
				AlignItems(ui.Center).Label(i18n.T("selection.editAction", "name", shown))
			if trigger.Hovered() {
				trigger.Background(t.SurfaceHover)
			}
			trigger.Children(func() {
				ui.Icon(c, actionIcon(a.Icon)).FontSize(16).Shrink(0)
				label := ui.Text(c, shown).SingleLine().Grow(1).Shrink(1).MinWidth(0).Tooltip(shown)
				if open {
					label.FontWeight(600)
				}
				ui.Icon(c, icons.ChevronDown).FontSize(16).TextColor(t.TextMuted).Shrink(0).Rotate(180 * parts.Progress())
			})
			position := strconv.Itoa(index)
			options := make([]kit.SelectOption, count)
			for i := range options {
				options[i] = kit.SelectOption{Value: strconv.Itoa(i), Label: strconv.Itoa(i + 1)}
			}
			ui.Row(c).Width(positionWidth).Shrink(0).Children(func() {
				changed, trigger := kit.Select(c, &position, options)
				trigger.MinWidth(0).FillWidth().Label(shown + " · " + i18n.T("selection.position"))
				if changed {
					if to, err := strconv.Atoi(position); err == nil {
						p.change = func() { p.move(index, to) }
					}
				}
			})
			ui.Row(c).Width(showWidth).Shrink(0).Children(func() {
				ui.Switch(c, &a.Enabled).Label(i18n.T("selection.showAction", "name", name))
			})
		})
		if open != (p.expanded == id) {
			if open {
				p.expanded = id
			} else {
				p.expanded = ""
			}
		}
		if panel := parts.Panel(func() { p.editor(c, a, name) }); panel.Valid() {
			panel.Padding(4, 16, 20, 44).Gap(16)
		}
	})
	if p.focus == id {
		p.focus = ""
	}
}

// editor edits an action: its name in the window's language, its icon,
// and for AI actions the model and the instruction; custom actions can
// be deleted.
func (p *page) editor(c *ui.Context, a *action, name string) {
	english := i18n.Locale() == i18n.English
	ui.Row(c).FillWidth().GapX(16).GapY(12).Wrap().AlignItems(ui.Start).Children(func() {
		field(c, i18n.T("selection.actionName"), func() {
			value := name
			in := ui.TextInput(c, &value)
			if p.focus == a.ID {
				in.AutoFocus().SetTextSelection(0, len([]rune(value)))
			}
			if in.Changed() {
				if english {
					a.EnglishName = value
				} else {
					a.Name = value
				}
			}
		})
		field(c, i18n.T("selection.icon"), func() {
			var options []kit.SelectOption
			for _, entry := range actionIcons {
				if p.quick && entry.name == "copy" {
					continue
				}
				options = append(options, kit.SelectOption{Value: entry.name, Label: i18n.T("selection.icons." + entry.name), Icon: entry.icon})
			}
			_, trigger := kit.Select(c, &a.Icon, options)
			trigger.MinWidth(0).FillWidth()
		})
	})
	if a.Kind != "copy" && p.aiLoaded {
		var value *widgets.AISelection
		if a.Selection != nil && a.Selection.Provider != "" {
			value = a.Selection
		}
		if next, changed := widgets.AIModelSelect(c, widgets.AIModelSelectOptions{
			Env: p.env, Providers: p.providers, Value: value, Label: name,
			InheritLabel: i18n.T("ai.followDefaultService"), Disabled: p.busy(),
		}); changed {
			a.Selection = next
		}
	}
	if a.Kind != "copy" {
		ui.Field(c, i18n.T("selection.prompt"), func() {
			ui.TextArea(c, &a.Prompt).Lines(3, 12)
		}).FillWidth()
	}
	if strings.HasPrefix(a.ID, "custom-") {
		ui.Row(c).Children(func() {
			if kit.Button(c, i18n.T("selection.remove"), icons.Trash2, false).Clicked() {
				id := a.ID
				p.change = func() { p.remove(id) }
			}
		})
	}
}

// field is a control with its name above it, which a click on the name
// focuses, in a column that shares a row with another and wraps below it
// when narrow.
func field(c *ui.Context, label string, control func()) {
	ui.Field(c, label, control).Basis(220).Grow(1).Shrink(1).MinWidth(0)
}
