package kit

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit/icons"
	"winone/modules/nativeui/design"
	"winone/modules/nativeui/hint"
)

// Spacing and corner radii of settings pages, in DIPs. Rows inside a group
// sit tight; groups stand apart, so a page reads as a few named tasks.
const (
	// PageWidth bounds a settings page's column for comfortable scanning.
	PageWidth = 880
	// WidePageWidth bounds work areas such as lists beside details.
	WidePageWidth = 1200
	// GroupGap separates groups on a page.
	GroupGap = 40
	// HeadingGap separates a group's heading from its rows.
	HeadingGap = 12
	// RowMinHeight is the height of a one-line setting row.
	RowMinHeight = 64
	// RowPadX and RowPadY are a panel row's inner padding; rows built by
	// hand inside a panel use them so every line keeps one inset.
	RowPadX = 24
	RowPadY = 16
	// IndentWidth moves dependent options under the row they depend on.
	IndentWidth = 32
	// ControlHeight is the height of buttons, inputs and selects.
	ControlHeight = 36
	// MarkRadius rounds only a check box glyph.
	MarkRadius = design.RadiusMark
	// MicroRadius rounds micro badges, compact tags and placeholders.
	MicroRadius = design.RadiusXS
	// ItemRadius rounds items nested inside panels, popovers and segmented tracks.
	ItemRadius = design.RadiusSM
	// ControlRadius rounds standard buttons, inputs, selects and nav rows.
	ControlRadius = design.RadiusMD
	// PanelRadius rounds groups, popovers and notices.
	PanelRadius = design.RadiusLG
	// DialogRadius rounds dialogs.
	DialogRadius = design.RadiusXL
	// PillRadius rounds full pills and circular icon buttons.
	PillRadius = design.RadiusFull
)

// Page lays out the column of a settings page; the shell supplies the
// scrolling and the margins.
func Page(c *ui.Context, fn func()) ui.Element {
	return ui.Column(c).FillWidth().MaxWidth(PageWidth).Margin(0, ui.Auto).Gap(GroupGap).Children(fn)
}

// WidePage is Page for work areas that use more width.
func WidePage(c *ui.Context, fn func()) ui.Element {
	return ui.Column(c).FillWidth().MaxWidth(WidePageWidth).Margin(0, ui.Auto).Gap(GroupGap).Children(fn)
}

// visibleMark starts a help text that stays on screen; see Visible.
const visibleMark = "\x1f"

// Visible keeps help on screen instead of behind a hover hint. Use it only
// for a risk, a limit, an irreversible consequence, an error's cause or
// the state of an empty group: other help moves into a hint by the name it
// explains, so a page reads by its names, not its sentences.
func Visible(help string) string {
	if help == "" {
		return ""
	}
	return visibleMark + help
}

// splitHelp returns help without its mark and whether it stays visible.
func splitHelp(help string) (string, bool) { return strings.CutPrefix(help, visibleMark) }

// HelpText returns help without Visible's mark, for elements that show or
// describe it themselves.
func HelpText(help string) string {
	text, _ := splitHelp(help)
	return text
}

// InfoMark is the small "i" beside a name whose help waits in a hover
// hint. It shows while shown is true (its row is hovered or focused) or
// while it has the pointer or the keyboard focus itself; focusable makes
// it a tab stop of its own, for rows whose control does not carry the
// help.
func InfoMark(c *ui.Context, help string, shown, focusable bool) ui.Element {
	t := c.Theme()
	m := ui.Row(c).Size(20, 20).Shrink(0).Radius(PillRadius).Justify(ui.Center).AlignItems(ui.Center).TextColor(t.TextMuted)
	if focusable {
		m = hint.Attach(c, m.Focusable().Label(i18n.T("notices.details")), help)
	}
	visible := float32(0)
	if shown || m.Hovered() || m.Focused() {
		visible = 1
	}
	return m.Opacity(m.Animate("reveal", visible, 150*time.Millisecond)).
		Children(func() { ui.Icon(c, icons.Info).FontSize(16) })
}

// Section is a group heading over any content: the title, with an
// optional sentence of purpose in a hover hint beside it. Without a title
// the sentence shows as the group's only text, as the state of an empty
// list. The icon argument remains compatible with callers.
func Section(c *ui.Context, icon *ui.SVG, title, description string, fn func()) ui.Element {
	return ui.Column(c).FillWidth().Gap(HeadingGap).Children(func() {
		if title != "" || description != "" {
			SectionHeading(c, icon, title, description)
		}
		fn()
	})
}

// SectionHeading is a Section's heading alone, for groups laid out by hand.
func SectionHeading(c *ui.Context, icon *ui.SVG, title, description string) ui.Element {
	text, visible := splitHelp(description)
	heading := ui.Column(c).Gap(4)
	shown := heading.Hovered() || heading.FocusWithin()
	return heading.Children(func() {
		if title == "" {
			// An empty title leaves the sentence alone, for a group whose
			// tab already names it.
			if text != "" {
				Help(c, text)
			}
			return
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			// Task names carry the hierarchy; repeated heading icons
			// compete with the actual actions and navigation.
			Heading(c, title).Role(ui.RoleHeading).Level(2).Shrink(1).MinWidth(0)
			if text != "" && !visible {
				InfoMark(c, text, shown, true)
			}
		})
		if text != "" && visible {
			Help(c, text)
		}
	})
}

// Group keeps the current heading and spacing, with a panel behind its
// related settings so each section remains distinct from the page canvas.
func Group(c *ui.Context, icon *ui.SVG, title, description string, rows func()) ui.Element {
	return Section(c, icon, title, description, func() { Panel(c, rows) })
}

// Panel restores the grouped surface while using the current shared palette
// and radius. Row padding keeps labels and controls clear of its boundary.
func Panel(c *ui.Context, rows func()) ui.Element {
	p := Colors(c)
	return ui.Column(c).FillWidth().Background(p.Panel).Border(1, p.Hairline).Radius(PanelRadius).
		Gap(1).Dividers(1, p.Hairline).Children(rows)
}

// rowName is a row's name with its help, the left side of Row, Toggle and
// Stack.
func rowName(c *ui.Context, label, help string, shown, markFocus bool) ui.Element {
	text, visible := splitHelp(help)
	if text == "" || visible {
		return ui.Column(c).Shrink(1).Gap(4).Children(func() {
			Label(c, label)
			if text != "" {
				Help(c, text)
			}
		})
	}
	return ui.Row(c).Shrink(1).Gap(8).AlignItems(ui.Center).Children(func() {
		Label(c, label).Shrink(1).MinWidth(0)
		InfoMark(c, text, shown, markFocus)
	})
}

// Row is a setting: its label on the left, its control on the right. In a
// narrow window the control moves below the text.
func Row(c *ui.Context, label, help string, control func()) ui.Element {
	// AlignContent centers the wrapped line: without it a row shorter than
	// RowMinHeight keeps its label against the top padding.
	row := ui.Row(c).FillWidth().MinHeight(RowMinHeight).Padding(RowPadY, RowPadX).GapX(24).GapY(8).
		AlignItems(ui.Center).AlignContent(ui.Center).Wrap()
	shown := row.Hovered() || row.FocusWithin()
	return row.Children(func() {
		rowName(c, label, help, shown, true).Basis(220).Grow(1)
		if control != nil {
			ui.Row(c).Shrink(0).Gap(8).AlignItems(ui.Center).Children(control)
		}
	})
}

// Stack is a setting whose control needs the row's width: a path, a long
// text, a list. The label sits above it. Without a label the control names
// itself, as a field does, and a hint's mark sits at the row's top end.
func Stack(c *ui.Context, label, help string, control func()) ui.Element {
	stack := ui.Column(c).FillWidth().Padding(RowPadY, RowPadX).Gap(12)
	shown := stack.Hovered() || stack.FocusWithin()
	text, visible := splitHelp(help)
	return stack.Children(func() {
		switch {
		case label != "":
			rowName(c, label, help, shown, true)
		case text != "" && visible:
			Help(c, text)
		case text != "":
			// Over the field's own name line, clear of the control.
			InfoMark(c, text, shown, true).Absolute().Top(RowPadY).Right(RowPadX)
		}
		control()
	})
}

// Indented holds the settings that depend on the row above them, as the
// options of a switch that is on.
func Indented(c *ui.Context, fn func()) ui.Element {
	return ui.Column(c).FillWidth().PaddingStart(IndentWidth).Children(fn)
}

// Toggle is a Row that is a switch as a whole: clicking its label toggles
// it as clicking the switch does, as an HTML label did for its control.
// Its help shows as the switch's hint. It returns the switch, whose
// Changed reports a toggle and which Disabled dims with its label.
func Toggle(c *ui.Context, value *bool, label, help string) ui.Element {
	t := c.Theme()
	text, visible := splitHelp(help)
	sw := ui.SwitchBase(c, value).FillWidth().Shrink(1).MinHeight(RowMinHeight).Padding(RowPadY, RowPadX).GapX(24).GapY(8).
		AlignItems(ui.Center).AlignContent(ui.Center).Wrap().Label(label).Cursor(ui.CursorPointer)
	if text != "" {
		sw.Description(text)
		if !visible {
			hint.Attach(c, sw, text)
		}
	}
	on := float32(0)
	if *value {
		on = 1
	}
	pos := sw.Animate("knob", on, 140*time.Millisecond)
	hovered := sw.Hovered()
	sw.Children(func() {
		rowName(c, label, help, hovered || sw.Focused(), false).Basis(220).Grow(1)
		// The track and knob of MyGo's Switch, drawn inside the row.
		off := t.Border.Mix(t.Text, 0.15)
		if hovered {
			off = t.Border.Mix(t.Text, 0.25)
		}
		ui.Box(c).Size(t.Space(9), t.Space(5)).Radius(t.Space(2.5)).Shrink(0).Background(off.Mix(t.Accent, pos)).
			Draw(func(p *ui.Painter, r ui.Rect) {
				in := t.Space(0.5)
				d := r.H - 2*in
				knob := ui.Rect{X: r.X + in + pos*(r.W-r.H), Y: r.Y + in, W: d, H: d}
				p.Shadow(knob, d/2, 0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.2))
				p.Fill(knob, ui.RGB(255, 255, 255), d/2)
			})
	})
	return sw
}

// CheckMark draws a check box as MyGo's Checkbox does, for a
// ui.CheckboxBase whose row holds more than a label: a picture and badges
// beside the name, all of which toggle it. hovered is the row's.
func CheckMark(c *ui.Context, on, hovered bool) ui.Element {
	t := c.Theme()
	box := ui.Box(c).Size(t.Space(4), t.Space(4)).Radius(MarkRadius).Shrink(0)
	switch {
	case on:
		box.Background(t.Accent)
	case hovered:
		box.Background(t.Background).Border(1, t.Accent)
	default:
		box.Background(t.Background).Border(1, t.Border.Mix(t.Text, 0.25))
	}
	if on {
		box.Draw(func(p *ui.Painter, r ui.Rect) {
			var path ui.Path
			path.MoveTo(r.X+r.W*0.22, r.Y+r.H*0.52).LineTo(r.X+r.W*0.42, r.Y+r.H*0.71).LineTo(r.X+r.W*0.78, r.Y+r.H*0.31)
			p.StrokePath(&path, t.Space(0.5), t.AccentText)
		})
	}
	return box
}

// PartialMark is CheckMark for a check box over several, some of them
// checked: a dash in the accent box.
func PartialMark(c *ui.Context) ui.Element {
	t := c.Theme()
	return ui.Box(c).Size(t.Space(4), t.Space(4)).Radius(MarkRadius).Shrink(0).Background(t.Accent).
		Draw(func(p *ui.Painter, r ui.Rect) {
			p.Line(r.X+r.W*0.25, r.Y+r.H/2, r.X+r.W*0.75, r.Y+r.H/2, t.Space(0.5), t.AccentText)
		})
}

// Option is one choice of Choices. Help follows the rules of a Row help:
// in a hint unless it is Visible.
type Option struct {
	Label, Help string
	Disabled    bool
}

// Choices is a radio list on a panel, one row per option, for choices
// whose consequences need a sentence. It returns whether the user chose
// another option.
func Choices(c *ui.Context, selected *int, options []Option) bool {
	before := *selected
	t, p := c.Theme(), Colors(c)
	ui.Column(c).FillWidth().Background(p.Panel).Border(1, p.Hairline).Radius(PanelRadius).Children(func() {
		ui.RadioGroup(c, func() {
			for i, option := range options {
				text, visible := splitHelp(option.Help)
				radio := ui.RadioBase(c.Key(i), selected, i).FillWidth().MinHeight(RowMinHeight-8).Padding(RowPadY, RowPadX).
					AlignItems(ui.Center).Disabled(option.Disabled).Label(option.Label)
				if text != "" {
					radio.Description(text)
					if !visible {
						hint.Attach(c, radio, text)
					}
				}
				on := *selected == i
				shown := radio.Hovered() || radio.Focused()
				radio.Children(func() {
					ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(4).Children(func() {
						ui.Row(c).Gap(16).AlignItems(ui.Center).Children(func() {
							dot := ui.Box(c).Size(16, 16).Radius(PillRadius).Shrink(0)
							if on {
								dot.Background(t.Accent)
							} else {
								dot.Background(t.Background).Border(1, t.Border.Mix(t.Text, 0.25))
							}
							dot.Children(func() {
								if on {
									ui.Box(c).Size(8, 8).Radius(PillRadius).Background(t.AccentText).Margin(ui.Auto)
								}
							})
							Label(c, option.Label).Shrink(1).MinWidth(0)
							if text != "" && !visible {
								InfoMark(c, text, shown, false)
							}
						})
						if text != "" && visible {
							Help(c, text).PaddingStart(32)
						}
					})
				})
			}
		}).FillWidth().Gap(1).Dividers(1, p.Hairline)
	})
	return *selected != before
}

// Fact is one label and value of Facts.
type Fact struct{ Label, Value string }

// Facts lists labeled values in two aligned columns, as a package version,
// publisher and file; values can be selected and copied.
func Facts(c *ui.Context, facts []Fact) ui.Element {
	t := c.Theme()
	return ui.Grid(c).FillWidth().ColumnTracks(ui.FitContent(), ui.Fr(1)).GapX(16).GapY(8).Children(func() {
		for i, fact := range facts {
			ui.Text(c.Key(i), fact.Label).FontSize(HelpSize(c)).TextColor(t.TextMuted).NoWrap()
			ui.Text(c.Key(-1-i), fact.Value).FontSize(HelpSize(c)).Selectable().MinWidth(0)
		}
	})
}

// Disclosure is a Panel whose rows fold away behind a header row, for an
// optional group on a page: the header chevron and title line up with the
// labels of rows inside panels, so a folded group reads as one more panel
// instead of a loose arrow beside the page headings. summary is an
// optional caption at the header end, such as a count. Rows follow Panel
// rules: build them with Row, Toggle or Stack, never bare text, so every
// line keeps the same RowPadX inset. Nested options inside a row keep
// ui.Collapsible.
func Disclosure(c *ui.Context, title, summary string, open *bool, rows func()) ui.Element {
	t, p := c.Theme(), Colors(c)
	return ui.Column(c).FillWidth().Background(p.Panel).Border(1, p.Hairline).Radius(PanelRadius).Children(func() {
		parts := ui.CollapsibleBase(c, open)
		tr := parts.Trigger.FillWidth().MinHeight(RowMinHeight).Padding(RowPadY, RowPadX).Gap(12).AlignItems(ui.Center).
			Radius(PanelRadius).Label(title).Cursor(ui.CursorPointer)
		if tr.Hovered() {
			tr.Background(t.SurfaceHover.Alpha(0.6))
		}
		progress := parts.Progress()
		tr.Children(func() {
			ui.Icon(c, icons.ChevronRight).FontSize(16).TextColor(t.TextMuted).Shrink(0).Rotate(90 * progress)
			Label(c, title).Grow(1).Shrink(1).MinWidth(0)
			if summary != "" {
				Caption(c, summary).SingleLine().Shrink(0)
			}
		})
		parts.Panel(func() {
			ui.Column(c).FillWidth().Gap(1).Dividers(1, p.Hairline).BorderWidth(1, 0, 0, 0).BorderColor(p.Hairline).Children(rows)
		})
	})
}
