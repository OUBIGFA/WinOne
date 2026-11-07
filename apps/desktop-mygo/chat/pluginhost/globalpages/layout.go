package globalpages

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

func tr(key string, args ...any) string { return i18n.T("pluginViews."+key, args...) }
func pageIcon(d Declaration) *ui.SVG {
	switch d.Icon {
	case "globe":
		return icons.Globe
	case "code":
		return icons.Code
	case "folder":
		return icons.Folder
	case "database":
		return icons.Database
	case "chart-column":
		return icons.ChartColumn
	case "image":
		return icons.Image
	case "message-circle":
		return icons.MessageCircle
	case "calendar-days":
		return icons.CalendarDays
	}
	return icons.Puzzle
}

type tabDrag struct {
	owner *Controller
	id    string
}

// Layout goes around the shell's content, not its title bar or main navigation.
// Global tabs stay independent of task selection; task tabs are projected only
// for Session. Empty/parked groups take no width. Narrow frames stack the same
// groups without rewriting their saved edges or concealing any contribution.
func (c *Controller) Layout(ctx *ui.Context, onChatRoute bool, content func()) {
	if c.closed {
		content()
		return
	}
	if !c.loaded || c.language != i18n.Locale() {
		c.Refresh()
	}
	if c.root == nil {
		c.root = mygo.FocusedWindow()
		if c.root == nil {
			for _, window := range mygo.Windows() {
				if window.Page() == nil {
					c.root = window
					break
				}
			}
		}
	}
	c.bodyShown = map[string]bool{}
	if c.loaded && c.root != nil {
		for _, tab := range append([]Tab(nil), c.state.Tabs...) {
			if c.state.available(tab) && tab.Location.Mode == "window" && c.windows[tab.ID] == nil {
				c.detach(tab.ID)
			}
		}
	}
	bounds := c.area.Bounds(ctx)
	width, height := bounds.W, bounds.H
	if width <= 0 {
		width = 960
	}
	if height <= 0 {
		height = 720
	}
	groups := map[string][]Tab{}
	if onChatRoute {
		for _, side := range sides {
			groups[side] = c.state.group(side, c.session)
		}
	}
	root := ui.Column(ctx.Key("plugin-views-layout")).Fill().MinWidth(0).MinHeight(0).Bind(&c.area)
	root.Children(func() {
		if tab := c.state.findTab(c.maximized); tab != nil && c.state.available(*tab) && tab.Location.Mode == "sidebar" && onChatRoute {
			c.groupView(ctx, tab.Location.Side, groups[tab.Location.Side]).Fill()
			return
		}
		c.maximized = ""
		// A small window cannot simultaneously give left/right groups and
		// Chat's own conversation list their minimum readable widths.
		stacked := width < 800
		if stacked {
			count := 0
			for _, side := range sides {
				if len(groups[side]) > 0 {
					count++
				}
			}
			panelHeight := min(180, height*.48/float32(max(count, 1)))
			for _, side := range []string{"top", "left", "right"} {
				if len(groups[side]) > 0 {
					c.groupView(ctx, side, groups[side]).FillWidth().Height(panelHeight).Shrink(0)
				}
			}
			ui.Column(ctx.Key("content")).Grow(1).Basis(0).FillWidth().MinHeight(0).MinWidth(0).Children(content)
			if len(groups["bottom"]) > 0 {
				c.groupView(ctx, "bottom", groups["bottom"]).FillWidth().Height(panelHeight).Shrink(0)
			}
			return
		}
		if len(groups["top"]) > 0 {
			c.groupView(ctx, "top", groups["top"]).FillWidth().Height(c.groupSize("top", height, 96, height*.35)).Shrink(0)
			c.resizeHandle(ctx, "top", height*.35)
		}
		ui.Row(ctx).Grow(1).Basis(0).FillWidth().MinWidth(0).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
			limit := max(120, (width-400)/float32(max(1, boolInt(len(groups["left"]) > 0)+boolInt(len(groups["right"]) > 0))))
			if len(groups["left"]) > 0 {
				c.groupView(ctx, "left", groups["left"]).Width(c.groupSize("left", width, c.minimumWidth(groups["left"]), limit)).FillHeight().Shrink(0)
				c.resizeHandle(ctx, "left", limit)
			}
			ui.Column(ctx.Key("content")).Grow(1).Basis(0).FillHeight().MinWidth(0).MinHeight(0).Children(content)
			if len(groups["right"]) > 0 {
				c.resizeHandle(ctx, "right", limit)
				c.groupView(ctx, "right", groups["right"]).Width(c.groupSize("right", width, c.minimumWidth(groups["right"]), limit)).FillHeight().Shrink(0)
			}
		})
		if len(groups["bottom"]) > 0 {
			c.resizeHandle(ctx, "bottom", height*.35)
			c.groupView(ctx, "bottom", groups["bottom"]).FillWidth().Height(c.groupSize("bottom", height, 96, height*.35)).Shrink(0)
		}
	})
	if c.dragSource != "" {
		c.edgeDropZones(ctx, root)
	}
	for id, body := range c.bodies {
		body.SetVisible(c.bodyShown[id])
	}
}
func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
func (c *Controller) minimumWidth(tabs []Tab) float32 {
	minimum := float32(120)
	for _, tab := range tabs {
		if d, exists := c.state.declaration(tab.PluginID, tab.Page); exists {
			minimum = max(minimum, d.MinWidth)
		}
	}
	return minimum
}
func (c *Controller) groupSize(side string, total, minimum, maximum float32) float32 {
	size := c.state.Sizes[side]
	if size == 0 {
		size = total * .22
		for _, tab := range c.state.group(side, c.session) {
			if d, exists := c.state.declaration(tab.PluginID, tab.Page); exists && d.DefaultFraction > 0 {
				size = total * d.DefaultFraction
				break
			}
		}
	}
	return min(maximum, max(minimum, size))
}
func (c *Controller) resizeHandle(ctx *ui.Context, side string, maximum float32) {
	horizontal := side == "left" || side == "right"
	handle := ui.Box(ctx.Key("resize-" + side)).Focusable().Role(ui.RoleSplitter).Label(tr("resize", "side", tr(side))).Background(kit.Colors(ctx).Hairline).Shrink(0)
	if horizontal {
		handle.Width(5).FillHeight().Cursor(ui.CursorResizeEW)
	} else {
		handle.Height(5).FillWidth().Cursor(ui.CursorResizeNS)
	}
	move := func(delta float32) {
		if side == "right" || side == "bottom" {
			delta = -delta
		}
		area := c.groups[side].Bounds(ctx)
		size, minimum := area.H, float32(96)
		if horizontal {
			size, minimum = area.W, c.minimumWidth(c.state.group(side, c.session))
		}
		c.state.Sizes[side] = min(maximum, max(minimum, size+delta))
	}
	if dx, dy, dragged := handle.Dragged(); dragged {
		if horizontal {
			move(dx)
		} else {
			move(dy)
		}
	}
	if _, _, ended := handle.DragEnded(); ended {
		c.save()
	}
	if horizontal {
		if handle.Shortcut(0, ui.KeyLeft) {
			move(-16)
			c.save()
		}
		if handle.Shortcut(0, ui.KeyRight) {
			move(16)
			c.save()
		}
	} else {
		if handle.Shortcut(0, ui.KeyUp) {
			move(-16)
			c.save()
		}
		if handle.Shortcut(0, ui.KeyDown) {
			move(16)
			c.save()
		}
	}
}
func (c *Controller) groupView(ctx *ui.Context, side string, tabs []Tab) ui.Element {
	handle := c.groups[side]
	if handle == nil {
		handle = &ui.Handle{}
		c.groups[side] = handle
	}
	selected := c.state.selectedTab(side, c.session, tabs)
	return ui.Column(ctx.Key("plugin-group-"+side)).MinWidth(0).MinHeight(0).Border(1, kit.Colors(ctx).Hairline).Background(kit.Colors(ctx).Panel).Bind(handle).Label(tr(side)).Children(func() {
		c.tabBar(ctx, tabs, selected.ID, side, nil)
		ui.Column(ctx.Key("body")).Grow(1).Basis(0).FillWidth().MinWidth(0).MinHeight(0).Children(func() {
			body := c.bodies[selected.ID]
			d := c.state.declarations[declarationKey(selected.PluginID, selected.Page)]
			if body == nil {
				body = newPageBody(c, c.env, d, selected.Session)
				c.bodies[selected.ID] = body
			}
			c.bodyShown[selected.ID] = true
			body.SetVisible(true)
			body.View(ctx)
		})
	})
}
func (c *Controller) tabBar(ctx *ui.Context, tabs []Tab, selected, side string, window *detachedWindow) {
	bar := ui.Row(ctx.Key("plugin-tab-bar")).FillWidth().MinWidth(0).Gap(4).Padding(5).Wrap().AlignItems(ui.Center)
	bar.Children(func() {
		for _, tab := range tabs {
			d := c.state.declarations[declarationKey(tab.PluginID, tab.Page)]
			row := ui.Row(ctx.Key(tab.ID)).Grow(1).Basis(110).MinWidth(0).MaxWidth(260).Border(1, kit.Colors(ctx).Hairline).Drag(tabDrag{c, tab.ID}).DragData(transfer.TextData(d.Title))
			if row.Dragging() {
				c.dragSource = tab.ID
			} else if c.dragSource == tab.ID {
				c.dragSource = ""
			}
			row.Children(func() {
				button := ui.ButtonBase(ctx).Grow(1).MinWidth(0).Padding(5, 7).Gap(5).AlignItems(ui.Center).Label(d.Title).Tooltip(d.Title)
				if selected == tab.ID {
					button.Background(kit.Colors(ctx).Selected)
				}
				button.Children(func() {
					ui.Icon(ctx, pageIcon(d)).FontSize(14).Shrink(0)
					if d.Unread != nil {
						ui.Icon(ctx, icons.BellRing).FontSize(13).Shrink(0).Tooltip(d.Unread.Summary).Label(tr("unread"))
					}
					ui.Text(ctx, d.Title).FontSize(kit.HelpSize(ctx)).SingleLine().Ellipsis("…").MinWidth(0)
				})
				if button.Clicked() {
					if window != nil {
						window.selected = tab.ID
						window.win.Invalidate()
					} else {
						c.state.selectTab(tab)
					}
					c.save()
				}
				if kit.IconButton(ctx, icons.X, tr("closeTab", "name", d.Title)).Size(24, 26).Clicked() {
					c.CloseTab(tab.ID)
				}
			})
			if dragged, dropped := ui.Drop[tabDrag](row); dropped && dragged.owner == c {
				if window != nil {
					c.joinWindow(dragged.id, window)
				} else {
					c.Move(dragged.id, side)
				}
				c.state.reorder(dragged.id, tab.ID)
				c.save()
			}
			row.ContextMenu(func(menu *ui.Menu) { c.tabMenu(menu, tab, window) })
		}
		if window == nil {
			if kit.IconButton(ctx, icons.Maximize, tr("maximize")).Clicked() {
				if c.maximized == selected {
					c.maximized = ""
				} else {
					c.maximized = selected
				}
			}
			if kit.IconButton(ctx, icons.SquareArrowOutUpRight, tr("openWindow")).Clicked() {
				c.detach(selected)
			}
		} else if kit.IconButton(ctx, icons.PanelRightOpen, tr("returnWindow")).Clicked() {
			c.Move(selected, side)
		}
		kit.IconButton(ctx, icons.Plus, tr("add")).Menu(func(menu *ui.Menu) {
			for _, d := range c.declarationsForMenu() {
				if menu.Item(d.Title).Chosen() {
					c.OpenAt(d.PluginID, d.Page, Location{Mode: "sidebar", Side: side})
				}
			}
		})
	})
	if dragged, dropped := ui.Drop[tabDrag](bar); dropped && dragged.owner == c {
		if window != nil {
			c.joinWindow(dragged.id, window)
		} else {
			c.Move(dragged.id, side)
		}
	}
}
func (c *Controller) declarationsForMenu() []Declaration {
	var rows []Declaration
	for _, d := range c.state.declarations {
		if d.available() && (d.Page == "main" || d.Page == "panel" && d.Position != "tab") {
			rows = append(rows, d)
		}
	}
	return rows
}
func (c *Controller) tabMenu(menu *ui.Menu, tab Tab, window *detachedWindow) {
	for _, side := range sides {
		if menu.Item(tr("move", "side", tr(side))).Chosen() {
			c.Move(tab.ID, side)
		}
	}
	if menu.Item(tr("openWindow")).Chosen() {
		c.detach(tab.ID)
	}
	if window != nil && menu.Item(tr("returnWindow")).Chosen() {
		c.Move(tab.ID, tab.Location.Side)
	}
	if menu.Item(tr("resetPlacement")).Chosen() {
		c.RestoreDefault(tab.PluginID, tab.Page)
	}
	if menu.Item(tr("closeTab", "name", c.state.declarations[declarationKey(tab.PluginID, tab.Page)].Title)).Chosen() {
		c.CloseTab(tab.ID)
	}
}
func (c *Controller) edgeDropZones(ctx *ui.Context, root ui.Element) {
	for _, side := range sides {
		zone := ui.Column(ctx.Key("plugin-drop-"+side)).Absolute().MinWidth(0).MinHeight(0).Justify(ui.Center).AlignItems(ui.Center).Border(1, ctx.Theme().Border).Background(kit.Colors(ctx).Panel).Label(tr("move", "side", tr(side)))
		switch side {
		case "left":
			zone.Left(0).TopPercent(25).BottomPercent(25).Width(68)
		case "right":
			zone.Right(0).TopPercent(25).BottomPercent(25).Width(68)
		case "top":
			zone.Top(0).LeftPercent(25).RightPercent(25).Height(52)
		case "bottom":
			zone.Bottom(0).LeftPercent(25).RightPercent(25).Height(52)
		}
		zone.Children(func() { kit.Caption(ctx, tr(side)) })
		if dragged, dropped := ui.Drop[tabDrag](zone); dropped && dragged.owner == c {
			c.Move(dragged.id, side)
			c.dragSource = ""
		}
	}
	_ = root
}

// Navigation uses the same visible projection for full and icon-only sidebars.
// Hiding an entry edits only this UI preference, never plugin enabled state.
func (c *Controller) MainNavigation() []Declaration {
	var rows []Declaration
	for _, d := range sortedMainViews(c.state.declarations) {
		if c.state.navigationVisible(d.PluginID) {
			rows = append(rows, d)
		}
	}
	return rows
}

func (c *Controller) Navigation(ctx *ui.Context, collapsed bool) {
	for _, d := range c.MainNavigation() {
		button := kit.GhostButton(ctx.Key("plugin-main:"+d.PluginID), "", pageIcon(d), false).FillWidth().MinWidth(0).Label(d.Title).Tooltip(d.Title).Justify(ui.Start)
		if !collapsed {
			button.Children(func() {
				ui.Text(ctx, d.Title).SingleLine().Ellipsis("…").MinWidth(0)
				if d.Unread != nil {
					ui.Icon(ctx, icons.BellRing).FontSize(13).Shrink(0).Tooltip(d.Unread.Summary).Label(tr("unread"))
				}
			})
		} else if d.Unread != nil {
			button.Tooltip(d.Title + " · " + tr("unread") + " · " + d.Unread.Summary)
		}
		if c.main != nil && c.main.plugin == d.PluginID {
			button.Background(kit.Colors(ctx).Selected)
		}
		if button.Clicked() {
			c.Open(d.PluginID, "main")
		}
		button.ContextMenu(func(menu *ui.Menu) {
			if menu.Item(tr("openPage")).Chosen() {
				c.OpenAt(d.PluginID, "main", Location{Mode: "page"})
			}
			if menu.Item(tr("openSidebar")).Chosen() {
				c.OpenAt(d.PluginID, "main", Location{Mode: "sidebar", Side: defaultLocation(d).Side})
			}
			if menu.Item(tr("openWindow")).Chosen() {
				c.OpenAt(d.PluginID, "main", Location{Mode: "window", Side: defaultLocation(d).Side})
			}
			if menu.Item(tr("resetPlacement")).Chosen() {
				c.RestoreDefault(d.PluginID, "main")
			}
		})
	}
}

// Dashboard mirrors the dynamic entries added above the sidebar footer. Home
// must not omit an installed application that its navigation offers.
func (c *Controller) Dashboard(ctx *ui.Context) {
	rows := c.MainNavigation()
	if len(rows) == 0 {
		return
	}
	ui.Column(ctx).FillWidth().Gap(8).Margin(16, 0).Children(func() {
		for _, d := range rows {
			if kit.GhostButton(ctx.Key("home-plugin:"+d.PluginID), d.Title, pageIcon(d), false).FillWidth().MinWidth(0).Justify(ui.Start).Clicked() {
				c.Open(d.PluginID, "main")
			}
		}
	})
}

// ManagementControls keeps low-frequency placement and navigation choices in
// plugin details, alongside the existing Host-managed plugin configuration.
func (c *Controller) ManagementControls(ctx *ui.Context, plugin string) {
	if d, exists := c.state.declaration(plugin, "main"); exists {
		visible := c.state.navigationVisible(plugin)
		if kit.Toggle(ctx, &visible, tr("showNavigation"), "").Changed() {
			c.SetNavigationVisible(plugin, visible)
		}
		ui.Row(ctx).FillWidth().Wrap().Gap(6).Children(func() {
			if kit.Button(ctx, tr("openPage"), pageIcon(d), false).Disabled(!d.available()).Clicked() {
				c.OpenAt(plugin, "main", Location{Mode: "page"})
			}
			if kit.Button(ctx, tr("openSidebar"), icons.PanelRightOpen, false).Disabled(!d.available()).Clicked() {
				c.OpenAt(plugin, "main", Location{Mode: "sidebar", Side: defaultLocation(d).Side})
			}
			if kit.Button(ctx, tr("openWindow"), icons.SquareArrowOutUpRight, false).Disabled(!d.available()).Clicked() {
				c.OpenAt(plugin, "main", Location{Mode: "window", Side: defaultLocation(d).Side})
			}
			if kit.Button(ctx, tr("resetPlacement"), icons.RotateCcw, false).Clicked() {
				c.RestoreDefault(plugin, "main")
			}
		})
	}
}
