package kit

import "github.com/egoist/mygo/ui"

// Spacing of settings pages, in DIPs. Rows inside a group sit tight; groups
// stand apart, so a page reads as a few named tasks.
const (
	// PageWidth bounds a settings page's column for comfortable scanning.
	PageWidth = 880
	// WidePageWidth bounds work areas such as lists beside details.
	WidePageWidth = 1200
	// GroupGap separates groups on a page.
	GroupGap = 32
	// HeadingGap separates a group's heading from its rows.
	HeadingGap = 10
	// RowMinHeight is the height of a one-line setting row.
	RowMinHeight = 48
	// ControlHeight is the height of buttons, inputs and selects.
	ControlHeight = 32
	// PanelRadius rounds groups, popovers and notices.
	PanelRadius = 8
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

// Section is a group heading over any content: an optional icon in the
// text's muted color, the title, and an optional sentence of purpose.
func Section(c *ui.Context, icon *ui.SVG, title, description string, fn func()) ui.Element {
	return ui.Column(c).FillWidth().Gap(HeadingGap).Children(func() {
		SectionHeading(c, icon, title, description)
		fn()
	})
}

// SectionHeading is a Section's heading alone, for groups laid out by hand.
func SectionHeading(c *ui.Context, icon *ui.SVG, title, description string) ui.Element {
	t := c.Theme()
	return ui.Column(c).Gap(2).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if icon != nil {
				ui.Icon(c, icon).FontSize(t.Rem(headingSize)).TextColor(t.TextMuted)
			}
			Heading(c, title).Role(ui.RoleHeading).Level(2)
		})
		if description != "" {
			Help(c, description)
		}
	})
}

// Group is a Section whose content is setting rows on a panel, one
// hairline between each two.
func Group(c *ui.Context, icon *ui.SVG, title, description string, rows func()) ui.Element {
	return Section(c, icon, title, description, func() { Panel(c, rows) })
}

// Panel is the face grouped rows sit on.
func Panel(c *ui.Context, rows func()) ui.Element {
	p := Colors(c)
	return ui.Column(c).FillWidth().Background(p.Panel).Border(1, p.Hairline).Radius(PanelRadius).
		Gap(1).Dividers(1, p.Hairline).Children(rows)
}

// Row is a setting: its label and optional help on the left, its control
// on the right. In a narrow window the control moves below the text.
func Row(c *ui.Context, label, help string, control func()) ui.Element {
	return ui.Row(c).FillWidth().MinHeight(RowMinHeight).Padding(10, 16).GapX(16).GapY(8).
		AlignItems(ui.Center).Wrap().Children(func() {
		ui.Column(c).Basis(220).Grow(1).Shrink(1).Gap(2).Children(func() {
			Label(c, label)
			if help != "" {
				Help(c, help)
			}
		})
		if control != nil {
			ui.Row(c).Shrink(0).Gap(8).AlignItems(ui.Center).Children(control)
		}
	})
}

// Stack is a setting whose control needs the row's width: a path, a long
// text, a list. The label and help sit above it.
func Stack(c *ui.Context, label, help string, control func()) ui.Element {
	return ui.Column(c).FillWidth().Padding(12, 16).Gap(8).Children(func() {
		if label != "" || help != "" {
			ui.Column(c).Gap(2).Children(func() {
				if label != "" {
					Label(c, label)
				}
				if help != "" {
					Help(c, help)
				}
			})
		}
		control()
	})
}

// Indented holds the settings that depend on the row above them, as the
// options of a switch that is on.
func Indented(c *ui.Context, fn func()) ui.Element {
	return ui.Column(c).FillWidth().PaddingStart(28).Children(fn)
}

// Toggle is a Row with a switch; it returns the switch, whose Changed
// reports a click.
func Toggle(c *ui.Context, value *bool, label, help string) ui.Element {
	var sw ui.Element
	Row(c, label, help, func() { sw = ui.Switch(c, value).Label(label) })
	return sw
}

// Option is one choice of Choices.
type Option struct {
	Label, Help string
	Disabled    bool
}

// Choices is a radio list on a panel: one row per option, its help below
// its label, for choices whose consequences need a sentence. It returns
// whether the user chose another option.
func Choices(c *ui.Context, selected *int, options []Option) bool {
	before := *selected
	t, p := c.Theme(), Colors(c)
	ui.Column(c).FillWidth().Background(p.Panel).Border(1, p.Hairline).Radius(PanelRadius).Children(func() {
		ui.RadioGroup(c, func() {
			for i, option := range options {
				radio := ui.RadioBase(c.Key(i), selected, i).FillWidth().Padding(10, 16).Gap(12).
					AlignItems(ui.Start).Disabled(option.Disabled).Label(option.Label)
				if option.Help != "" {
					radio.Description(option.Help)
				}
				on := *selected == i
				radio.Children(func() {
					dot := ui.Box(c).Size(16, 16).Radius(8).Shrink(0).Margin(2, 0, 0, 0)
					if on {
						dot.Background(t.Accent)
					} else {
						dot.Background(t.Background).Border(1, t.Border.Mix(t.Text, 0.25))
					}
					dot.Children(func() {
						if on {
							ui.Box(c).Size(6, 6).Radius(3).Background(t.AccentText).Margin(ui.Auto)
						}
					})
					ui.Column(c).Grow(1).Shrink(1).Gap(2).Children(func() {
						Label(c, option.Label)
						if option.Help != "" {
							Help(c, option.Help)
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

// Facts lists labeled values in two aligned columns, as a package's
// version, publisher and file; values can be selected and copied.
func Facts(c *ui.Context, facts []Fact) ui.Element {
	t := c.Theme()
	return ui.Grid(c).FillWidth().ColumnTracks(ui.FitContent(), ui.Fr(1)).GapX(16).GapY(6).Children(func() {
		for i, fact := range facts {
			ui.Text(c.Key(i), fact.Label).FontSize(HelpSize(c)).TextColor(t.TextMuted).NoWrap()
			ui.Text(c.Key(-1-i), fact.Value).FontSize(HelpSize(c)).Selectable().MinWidth(0)
		}
	})
}
