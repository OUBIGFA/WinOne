package wintray

import (
	"strconv"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

// draggedProgram is a list row being dragged to another place.
type draggedProgram struct{ id string }

// Widths of the list's control columns and the gap between them, shared by
// the headings and the rows (settings.css: 2.25rem 3.5rem 5rem, gap 1rem).
// The tray switch shows no text of its own: the column heading names it,
// and it keeps its name as label and tooltip, as React's settings surface
// hid the repeated label.
const (
	collectWidth = 36
	replaceWidth = 56
	orderWidth   = 80
	controlGap   = 16
)

// programList shows the programs in starting order: each with its icon,
// name, tray collection, replace action and position, chosen by a click,
// Enter or Space, and moved by dragging or by choosing its position.
func (p *page) programList(c *ui.Context) {
	apps := p.apps()
	ui.Scroll(c).Fill().Padding(12, 12, 12, 0).Children(func() {
		ui.Column(c).FillWidth().Gap(2).Role(ui.RoleList).Label(i18n.T("wintray.list")).FocusGroup(ui.Vertical).Children(func() {
			// Column headings, which assistive technology skips: each row
			// names its own controls.
			ui.Row(c).FillWidth().Padding(0, 8, 6, 8).Gap(8).Role(ui.RoleNone).Children(func() {
				ui.Box(c).Width(32).Shrink(0)
				kit.Caption(c, i18n.T("settingsDesign.programColumn")).Grow(1).MinWidth(0).SingleLine()
				ui.Row(c).Gap(controlGap).Shrink(0).Children(func() {
					for _, column := range []struct {
						key   string
						width float32
					}{{"settingsDesign.trayColumn", collectWidth}, {"settingsDesign.replaceColumn", replaceWidth}, {"settingsDesign.orderColumn", orderWidth}} {
						kit.Caption(c, i18n.T(column.key)).Width(column.width).SingleLine().TextAlign(ui.Center)
					}
				})
			})
			if len(apps) == 0 {
				kit.Help(c, i18n.T("wintray.empty")).TextAlign(ui.Center).FillWidth().Padding(48, 16)
				return
			}
			for i, app := range apps {
				p.programRow(c, i, app.Value)
			}
		})
	})
}

func (p *page) programRow(c *ui.Context, index int, app program) {
	t, palette := c.Theme(), kit.Colors(c)
	name := p.displayName(app)
	selected := app.ID == p.selected
	blocked := p.blocked()
	row := ui.Row(c.Key(app.ID)).FillWidth().Padding(8).Gap(8).AlignItems(ui.Center).Radius(t.Radius).
		Focusable().Role(ui.RoleListItem).Label(name).Selected(selected)
	// Selected marks the row for assistive technology; the list shows it
	// with the window's own selection fill rather than the accent.
	switch {
	case selected:
		row.Background(palette.Selected).TextColor(t.Text)
	case row.Hovered() && !blocked:
		row.Background(t.SurfaceHover).TextColor(t.Text)
	default:
		row.TextColor(t.Text)
	}
	if !blocked {
		row.Drag(&draggedProgram{app.ID})
	}
	row.Children(func() {
		if m := p.meta[app.ExePath]; m.icon != nil {
			ui.Image(c, m.icon).Size(32, 32).Shrink(0)
		} else {
			ui.Icon(c, icons.AppWindow).FontSize(32).TextColor(t.TextMuted).Shrink(0)
		}
		ui.Text(c, name).SingleLine().Ellipsis("…").Grow(1).Shrink(1).MinWidth(48).Tooltip(name)
		ui.Row(c).Gap(controlGap).Shrink(0).AlignItems(ui.Center).Children(func() {
			ui.Row(c).Width(collectWidth).Justify(ui.Center).Children(func() {
				on := app.CollectTrayIcon
				sw := ui.Switch(c, &on).Label(i18n.T("wintray.collect")).Tooltip(i18n.T("wintray.collect")).
					Disabled(blocked || !executable(app.ExePath))
				if sw.Changed() {
					p.update(app.ID, func(a *program) { a.CollectTrayIcon = on })
				}
			})
			ui.Row(c).Width(replaceWidth).Justify(ui.Center).Children(func() {
				replace := kit.IconButton(c, icons.FolderOpen, i18n.T("wintray.replaceNamed", "name", name)).
					Tooltip(i18n.T("wintray.replaceProgram")).Disabled(blocked)
				if replace.Clicked() {
					p.replace(app.ID)
				}
			})
			p.orderSelect(c, index, app, name, blocked)
		})
	})
	// A container takes its input with its children, once built.
	if dragged, ok := ui.Drop[*draggedProgram](row); ok {
		p.reorder(dragged.id, app.ID)
	}
	if _, over := ui.DragOver[*draggedProgram](row); over && !blocked {
		row.Border(1, t.Accent)
	}
	if !blocked && (row.Clicked() || row.Shortcut(0, ui.KeyEnter) || row.Shortcut(0, ui.KeySpace)) {
		p.selected = app.ID
	}
}

// orderSelect moves a program to a position chosen from 1 to the number
// of programs, for the keyboard as dragging is for the pointer.
func (p *page) orderSelect(c *ui.Context, index int, app program, name string, disabled bool) {
	apps := p.apps()
	options := make([]kit.SelectOption, len(apps))
	for i := range apps {
		options[i] = kit.SelectOption{Value: strconv.Itoa(i + 1), Label: strconv.Itoa(i + 1)}
	}
	position := strconv.Itoa(index + 1)
	ui.Box(c).Width(orderWidth).Shrink(0).Children(func() {
		changed, trigger := kit.Select(c, &position, options)
		trigger.MinWidth(0).FillWidth().Label(i18n.T("wintray.sortPosition", "name", name)).Disabled(disabled)
		if target, err := strconv.Atoi(position); changed && err == nil && target >= 1 && target <= len(apps) {
			p.reorder(app.ID, apps[target-1].Value.ID)
		}
	})
}

// programs is the list beside the selected program's settings: side by
// side with a divider the user moves, or one above the other in a narrow
// window. Both fill the window's height and scroll on their own.
func (p *page) programs(c *ui.Context) {
	palette := kit.Colors(c)
	_, height := c.Size()
	// The shell's frame takes the title bar (40), its padding above (24)
	// and below (40) the page, and this column's gap (16).
	available := height - 40 - 24 - 40 - 16 - p.header.Bounds(c).H
	available = max(available, 360)
	width := p.workspace.Bounds(c).W
	area := ui.Column(c).FillWidth().Height(available).BorderWidth(1, 0, 0, 0).BorderColor(palette.Hairline).Bind(&p.workspace)
	if width > 0 && width < 860 {
		// A list as high as a third of the area, at least 140, at most half.
		listHeight := min(available/2, max(140, available*0.34))
		area.Children(func() {
			ui.Column(c).FillWidth().Height(listHeight).Shrink(0).Children(func() { p.programList(c) })
			ui.Column(c).FillWidth().Grow(1).MinHeight(0).BorderWidth(1, 0, 0, 0).BorderColor(palette.Hairline).Children(func() { p.details(c) })
		})
		return
	}
	area.Children(func() {
		kit.Split(c, &p.split, kit.SplitOptions{Label: i18n.T("wintray.resizePanels"), MinFirst: 28, MinSecond: 30},
			func() { p.programList(c) }, func() { p.details(c) }).Grow(1).MinHeight(0)
	})
}
