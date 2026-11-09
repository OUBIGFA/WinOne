package kit

import "github.com/egoist/mygo/ui"

// Tab is one tab of Tabs.
type Tab struct {
	Label string
	Icon  *ui.SVG
}

// Tabs is the first level of a page's tasks: icon and title on one line,
// the chosen one on a neutral surface above a hairline. Each title keeps
// one line; the group wraps when the window cannot fit all tasks in one row.
//
// A new choice goes through request, which applies it at once or after
// the user decides about unsaved changes; request nil applies it directly.
// It reports whether the choice changed in this frame.
func Tabs(c *ui.Context, selected *int, tabs []Tab, request func(apply func())) bool {
	t, p := c.Theme(), Colors(c)
	next := *selected
	parts := ui.TabsBase(c, &next, len(tabs))
	parts.List.FillWidth().Wrap().Gap(4).Padding(0, 0, 12, 0).BorderWidth(0, 0, 1, 0).BorderColor(p.Hairline).Children(func() {
		for i, tab := range tabs {
			chosen := i == *selected
			item := parts.Tab(i).Padding(8, 12).Radius(t.Radius).Gap(8).AlignItems(ui.Center).Shrink(0).MinWidth(0).
				Label(tab.Label).Tooltip(tab.Label)
			color := t.TextMuted
			if chosen || item.Hovered() {
				color = t.Text
			}
			if chosen {
				item.Background(p.Selected)
			} else if item.Hovered() {
				item.Background(t.SurfaceHover)
			}
			item.TextColor(color).Children(func() {
				if tab.Icon != nil {
					ui.Icon(c, tab.Icon).FontSize(16).Shrink(0)
				}
				label := ui.Text(c, tab.Label).SingleLine().MinWidth(0)
				if chosen {
					label.FontWeight(500)
				}

			})
		}
	})
	if next == *selected {
		return false
	}
	apply := func() { *selected = next }
	if request == nil {
		apply()
	} else {
		request(apply)
	}
	return true
}

// SubTabs is the second level: text only, an underline, no hairline of its
// own, so it reads as part of the panel it switches.
func SubTabs(c *ui.Context, selected *int, labels []string, request func(apply func())) bool {
	t := c.Theme()
	next := *selected
	parts := ui.TabsBase(c, &next, len(labels))
	parts.List.Gap(2).Wrap().Children(func() {
		for i, label := range labels {
			chosen := i == *selected
			item := parts.Tab(i).Padding(6, 10).Shrink(1).MinWidth(0).Label(label)
			color := t.TextMuted
			if chosen || item.Hovered() {
				color = t.Text
			}
			indicator := ui.Transparent
			if chosen {
				indicator = t.Accent
			}
			item.TextColor(color).Children(func() {
				ui.Text(c, label).SingleLine().MinWidth(0).FontSize(HelpSize(c))
				ui.Box(c).Absolute().Left(10).Right(10).Bottom(0).Height(2).Radius(1).Background(indicator)
			})
		}
	})
	if next == *selected {
		return false
	}
	apply := func() { *selected = next }
	if request == nil {
		apply()
	} else {
		request(apply)
	}
	return true
}
