package shell

import (
	"math"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/internal/branding"
)

var logo, _ = ui.DecodeBitmap(branding.Logo)

const titleBarHeight = 40

func kitTheme(c *ui.Context, dark bool) *ui.Theme { return kit.Theme(c.Preferences(), dark) }

func kitDialog(c *ui.Context, open *bool, title, message, err, confirm, cancel string) bool {
	return kit.Dialog(c, open, kit.DialogOptions{Title: title, Message: message, Error: err, Confirm: confirm, Cancel: cancel, Danger: true})
}

// titleBar draws the window's caption: the logo and name, the page shown,
// the page's refresh, and the window's own buttons. Windows still draws
// the frame, its corners, shadow and resizing edges.
func (s *Shell) titleBar(c *ui.Context) {
	t, p := c.Theme(), kit.Colors(c)
	current := spec(s.page)
	conversation := current.layout == core.Conversation
	ui.Row(c).FillWidth().Height(titleBarHeight).AlignItems(ui.Stretch).Background(t.Background).
		BorderWidth(0, 0, 1, 0).BorderColor(p.Hairline).Children(func() {
		ui.Row(c).Grow(1).MinWidth(0).Padding(0, 12, 0, 16).Gap(10).AlignItems(ui.Center).DragWindow().Children(func() {
			if logo != nil {
				ui.Image(c, logo).Size(22, 22).Fit(ui.Contain).Shrink(0)
			}
			ui.Text(c, "WinOne").FontWeight(600).SingleLine().Shrink(0)
			if !conversation {
				ui.Box(c).Width(1).Height(16).Background(p.Hairline).Shrink(0)
				kit.Help(c, i18n.T(current.title)).SingleLine().MinWidth(0)
			}
		})
		if !conversation {
			ui.Row(c).Padding(0, 4).AlignItems(ui.Center).Shrink(0).Children(func() {
				dirty, busy := s.env.Activity.State()
				label := i18n.T("common.refresh")
				if dirty || busy {
					label = i18n.T("settingsDesign.refreshAfterEditing")
				}
				var b ui.Element
				if s.loading {
					b = ui.ButtonBase(c).Size(kit.ControlHeight, kit.ControlHeight).Justify(ui.Center).AlignItems(ui.Center).
						Disabled(true).Label(label).Children(func() { kit.Loader(c) })
				} else {
					b = kit.IconButton(c, icons.RefreshCw, label).Disabled(dirty || busy)
				}
				if b.Clicked() {
					s.refresh()
				}
			})
		}
		ui.Row(c).AlignItems(ui.Stretch).Shrink(0).Children(func() {
			s.windowButton(c, icons.Minus, i18n.T("window.minimize"), false, s.window.Minimize)
			if s.window.IsMaximized() {
				s.windowButton(c, icons.Copy, i18n.T("window.restore"), false, s.window.ToggleMaximize).MaximizeButton()
			} else {
				s.windowButton(c, icons.Square, i18n.T("window.maximize"), false, s.window.ToggleMaximize).MaximizeButton()
			}
			s.windowButton(c, icons.X, i18n.T("window.close"), true, s.window.Close)
		})
	})
	if s.loadErr != "" && s.loaded && !conversation {
		ui.Row(c).FillWidth().Padding(8, 16).Children(func() { kit.InlineError(c, s.loadErr) })
	}
}

func (s *Shell) windowButton(c *ui.Context, icon *ui.SVG, label string, closing bool, run func()) ui.Element {
	t := c.Theme()
	b := ui.ButtonBase(c).Width(46).Justify(ui.Center).AlignItems(ui.Center).Label(label).Tooltip(label).FocusRing(false)
	switch {
	case closing && b.Pressed():
		b.Background(t.Danger.Mix(t.Text, 0.15)).TextColor(ui.Hex("#ffffff"))
	case closing && b.Hovered():
		b.Background(ui.Hex("#c42b1c")).TextColor(ui.Hex("#ffffff"))
	case b.Pressed():
		b.Background(t.SurfacePressed)
	case b.Hovered():
		b.Background(t.SurfaceHover)
	}
	b.Children(func() { ui.Icon(c, icon).FontSize(16) })
	if b.Clicked() {
		run()
	}
	return b
}

// sidebar lists the pages: icons only when narrow (the default on
// opening), names beside them when wider. Its right edge drags to any width
// from the icon rail to 360 DIPs; the button above folds and unfolds it.
func (s *Shell) sidebar(c *ui.Context) {
	t, p := c.Theme(), kit.Colors(c)
	compact := s.sidebarWidth < compactWidth
	ui.Column(c).Width(s.sidebarWidth).Shrink(0).FillHeight().Background(p.Sidebar).
		Role(ui.RoleGroup).Label(i18n.T("nav.label")).Children(func() {
		ui.Column(c).Padding(8, 8, 4).Children(func() {
			label := i18n.T("navigation.collapseSidebar")
			if compact {
				label = i18n.T("navigation.expandSidebar")
			}
			b := ui.ButtonBase(c).Size(48, 40).Justify(ui.Center).AlignItems(ui.Center).Radius(t.Radius).
				Label(label).Tooltip(label)
			if b.Hovered() {
				b.Background(t.SurfaceHover)
			}
			b.Children(func() { ui.Icon(c, icons.PanelLeft).FontSize(18).TextColor(t.TextMuted) })
			if b.Clicked() {
				if compact {
					s.sidebarWidth = s.ui.ExpandedWidth
				} else {
					s.sidebarWidth = collapsedWidth
				}
			}
		})
		ui.Scroll(c).Grow(1).MinHeight(0).Padding(4, 8).Gap(2).FocusGroup(ui.Vertical).Children(func() {
			for _, item := range pages() {
				if !item.footer {
					s.navItem(c, item, compact)
				}
			}
		})
		ui.Column(c).Padding(4, 8, 12).Gap(2).FocusGroup(ui.Vertical).Children(func() {
			for _, item := range pages() {
				if !item.footer {
					continue
				}
				if item.separated {
					ui.Box(c).Height(1).Margin(6, 8).Background(p.Hairline)
				}
				s.navItem(c, item, compact)
			}
		})
	})
	s.sidebarHandle(c)
}

func (s *Shell) navItem(c *ui.Context, item pageSpec, compact bool) {
	t, p := c.Theme(), kit.Colors(c)
	title := i18n.T(item.title)
	selected := item.id == s.page
	row := ui.ButtonBase(c.Key(item.id)).Height(44).Padding(0, 0, 0, 14).Gap(12).AlignItems(ui.Center).
		Radius(t.Radius).Label(title).Selected(selected)
	if compact {
		row.Tooltip(title)
	}
	switch {
	case selected:
		row.Background(p.Selected)
	case row.Pressed():
		row.Background(t.SurfacePressed)
	case row.Hovered():
		row.Background(t.SurfaceHover)
	}
	row.Children(func() {
		color := t.TextMuted
		if selected {
			color = t.Text
		}
		ui.Icon(c, item.icon).FontSize(19).TextColor(color).Shrink(0)
		if !compact {
			text := ui.Text(c, title).SingleLine().MinWidth(0).Shrink(1)
			if selected {
				text.FontWeight(600)
			}
		}
		if selected {
			// Windows' navigation marks the chosen page with a short bar at
			// its leading edge; the row keeps its place and size.
			ui.Box(c).Absolute().Left(0).Top(14).Bottom(14).Width(3).Radius(2).Background(t.Accent)
		}
	})
	if row.Clicked() {
		s.requestPage(item.id, nil)
	}
}

// sidebarHandle is the sidebar's right edge, which drags its width.
func (s *Shell) sidebarHandle(c *ui.Context) {
	p := kit.Colors(c)
	handle := ui.Box(c).Width(5).Shrink(0).FillHeight().Margin(0, 0, 0, -3).Cursor(ui.CursorResizeEW).
		Focusable().Label(i18n.T("navigation.resizeSidebar")).Role(ui.RoleSplitter)
	line := p.Hairline
	if handle.Hovered() || handle.Dragging() {
		line = c.Theme().Border
	}
	handle.Draw(func(painter *ui.Painter, r ui.Rect) {
		painter.Fill(ui.Rect{X: r.X + 2, Y: r.Y, W: 1, H: r.H}, line, 0)
	})
	if dx, _, ok := handle.Dragged(); ok {
		s.sidebarWidth = float32(math.Max(collapsedWidth, math.Min(maxSidebarWidth, float64(s.sidebarWidth+dx))))
	}
	if _, _, ended := handle.DragEnded(); ended {
		s.rememberSidebar()
	}
	if handle.Shortcut(0, ui.KeyLeft) {
		s.sidebarWidth = max(collapsedWidth, s.sidebarWidth-16)
		s.rememberSidebar()
	}
	if handle.Shortcut(0, ui.KeyRight) {
		s.sidebarWidth = min(maxSidebarWidth, s.sidebarWidth+16)
		s.rememberSidebar()
	}
}

// setSidebarStartup stores how the sidebar opens; the current sidebar
// follows at once, as the React window's setting did.
func (s *Shell) setSidebarStartup(value string) error {
	if value != "expanded" {
		value = "collapsed"
	}
	next := s.ui
	next.SidebarStartup = value
	if err := next.save(s.opts.Directory); err != nil {
		return err
	}
	s.ui = next
	s.sidebarWidth = collapsedWidth
	if value == "expanded" {
		s.sidebarWidth = s.ui.ExpandedWidth
	}
	return nil
}

func (s *Shell) rememberSidebar() {
	if s.sidebarWidth >= compactWidth {
		s.ui.ExpandedWidth = max(minExpandedWidth, s.sidebarWidth)
		_ = s.ui.save(s.opts.Directory)
	}
}

// content frames the page: settings pages scroll in a centered column
// with the save rail floating at the right; conversations fill it.
func (s *Shell) content(c *ui.Context) {
	current := spec(s.page)
	// Pages talk to the Host from their first frame: wait for the first
	// snapshot, which starts the Host when needed, as every request of
	// the previous window waited for that start.
	if s.instance == nil && s.loaded {
		if current.make != nil {
			s.instance = current.make(s.env)
		} else {
			s.instance = unavailable(s.env, current)
		}
	}
	s.env.Rail.Clear()
	ui.Box(c).Grow(1).MinWidth(0).FillHeight().Children(func() {
		if current.layout == core.Conversation {
			ui.Column(c.Key(s.page)).Fill().Children(func() { s.pageView(c) })
			return
		}
		width, _ := c.Size()
		margin := float32(32)
		if width-s.sidebarWidth < 640 {
			margin = 16
		}
		// The rail floats over the content's right margin; keep text
		// clear of it without reserving a column.
		railRoom := float32(56)
		scroller := ui.Scroll(c.Key(s.page)).Fill().TrackScroll(&s.scroll).Padding(24, margin+railRoom, 40, margin).
			Children(func() {
				if s.snapshot.RuntimeWarning != "" || s.opts.StartupWarning != "" {
					ui.Column(c).FillWidth().MaxWidth(kit.WidePageWidth).Margin(0, ui.Auto, 16, ui.Auto).Children(func() {
						warning := s.snapshot.RuntimeWarning
						if s.opts.StartupWarning != "" && !s.snapshot.Administrator && s.snapshot.Settings.Startup != nil && s.snapshot.Settings.Startup.PreferAdministrator {
							if warning != "" {
								warning += "\n"
							}
							warning += s.opts.StartupWarning
						}
						if warning != "" {
							kit.Notice(c, kit.Info, "", warning, nil)
						}
					})
				}
				s.pageView(c)
			})
		_ = scroller
		s.rail(c)
	})
}

// pageView builds the page, or what stands for it until the Host answers.
func (s *Shell) pageView(c *ui.Context) {
	switch {
	case s.instance != nil:
		s.instance.View(c)
	case s.loadErr != "":
		kit.Page(c, func() {
			kit.Notice(c, kit.Failure, i18n.T("common.failed"), s.loadErr, func() {
				if kit.Button(c, i18n.T("common.reconnect"), icons.RefreshCw, s.loading).Clicked() {
					s.refresh()
				}
			})
		})
	default:
		kit.Page(c, func() { kit.Skeleton(c, 6) })
	}
}

// rail is the column of actions at the content's bottom right: Save, Undo,
// a gap, then back to top and to bottom when the page scrolls. Save and
// Undo show on every settings page at the same place, disabled when the
// page has no manual save, so they never move.
func (s *Shell) rail(c *ui.Context) {
	rail := s.env.Rail
	ui.Column(c).Absolute().Right(16).Bottom(16).Gap(8).AlignItems(ui.Center).Children(func() {
		save, undo := rail.Save, rail.Undo
		if !rail.Active() {
			save, undo = core.Action{Label: i18n.T("navigation.saveChanges"), Disabled: true}, core.Action{Disabled: true}
		}
		if save.Label == "" {
			save.Label = i18n.T("navigation.saveChanges")
		}
		saveIcon := icons.Save
		var b ui.Element
		if save.Busy {
			b = ui.PrimaryButton(c, "").Size(kit.ControlHeight+4, kit.ControlHeight+4).Padding(0).Justify(ui.Center).
				AlignItems(ui.Center).Disabled(true).Label(save.Label).Children(func() { kit.Loader(c) })
		} else {
			b = kit.OutlineIconButton(c, saveIcon, save.Label, true).Disabled(save.Disabled || save.Run == nil)
		}
		if b.Clicked() && save.Run != nil {
			save.Run()
		}
		u := kit.OutlineIconButton(c, icons.RotateCcw, i18n.T("navigation.undoChanges"), false).
			Disabled(undo.Disabled || undo.Run == nil)
		if u.Clicked() && undo.Run != nil {
			undo.Run()
		}
		if s.scroll.MaxY > 1 {
			ui.Box(c).Height(kit.ControlHeight)
			top := kit.OutlineIconButton(c, icons.ArrowUpToLine, i18n.T("navigation.scrollToTop"), false).Disabled(s.scroll.Y <= 1)
			if top.Clicked() {
				s.scroll.ScrollTo(0, 0, ui.Smooth)
			}
			bottom := kit.OutlineIconButton(c, icons.ArrowDownToLine, i18n.T("navigation.scrollToBottom"), false).
				Disabled(s.scroll.Y >= s.scroll.MaxY-1)
			if bottom.Clicked() {
				s.scroll.ScrollTo(0, s.scroll.MaxY, ui.Smooth)
			}
		}
	})
}
