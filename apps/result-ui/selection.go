package resultui

import (
	"github.com/egoist/mygo/ui"
	"winone/internal/resultwindow"
)

func (c *Controller) selectionActions() []resultwindow.SelectionAction {
	if c.Initial.Source == "quick-assistant" {
		return resultwindow.QuickAssistantActions(c.Initial.Actions)
	}
	if c.Initial.Actions == nil {
		return resultwindow.DefaultSelectionActions()
	}
	return c.Initial.Actions
}
func (c *Controller) actionLabel(action resultwindow.SelectionAction) string {
	if c.Panel.Language == "en-US" && action.EnglishName != "" {
		return action.EnglishName
	}
	return action.Name
}
func (c *Controller) ToolbarDimensions() (int, int) {
	count := 0
	for _, action := range c.selectionActions() {
		if action.Enabled {
			count++
		}
	}
	return 120 + 34*count, 44
}

// The only new view is the transient action strip. Every action switches to
// the existing Panel; source-specific presentation is a Panel option.
func (c *Controller) View(context *ui.Context) {
	if !c.Toolbar {
		c.Panel.View(context)
		c.selectedText = context.SelectedText()
		c.completeSelectionRead()
		return
	}
	p := c.Panel
	t := p.palette(context)
	context.SetTheme(t)
	windowSurface(ui.Row(context), t).AlignItems(ui.Center).Justify(ui.Center).Gap(4).Padding(4).Label(p.T("划词工具栏", "Selection toolbar")).Children(func() {
		// Widen only the logo's horizontal breathing room. The circular
		// container and the complete original bitmap retain their 22 DIP size.
		ui.Row(context).Size(34, 30).Shrink(0).Center().DragWindow().Cursor(ui.CursorGrab).Label(p.T("拖动工具栏", "Drag toolbar")).Children(func() {
			ui.Box(context).Size(22, 22).Shrink(0).Radius(11).Clip().DragWindow().Cursor(ui.CursorGrab).Children(func() {
				ui.Image(context, toolbarLogo()).Size(22, 22).Fit(ui.Contain).DragWindow().Cursor(ui.CursorGrab)
			})
		})
		// Keep the two end icons mirrored while giving the action area a visible
		// native drag handle, independent of the user's configurable actions.
		ui.Row(context).Size(18, 30).Shrink(0).Center().DragWindow().Cursor(ui.CursorGrab).Label(p.T("拖动划词助手", "Drag selection toolbar")).Children(func() {
			ui.Icon(context, svg("grip")).Size(12, 16).Shrink(0).TextColor(t.TextMuted).DragWindow().Cursor(ui.CursorGrab)
		})
		ui.Box(context).Width(1).Height(16).Shrink(0).Background(t.Border)
		ui.Row(context).Height(34).AlignItems(ui.Center).Gap(4).Padding(2, 3).Children(func() {
			for _, action := range c.selectionActions() {
				if !action.Enabled {
					continue
				}
				button := selectionButton(context, action.Icon, c.actionLabel(action), p.Busy)
				c.toolbarHint(button, c.actionLabel(action), t)
				if button.Clicked() {
					if action.Kind == "copy" {
						c.copy(p.Original)
						if p.Error == "" {
							c.Window.Close()
						}
					} else {
						c.openSelectionAction(action)
					}
				}
			}
		})
		ui.Box(context).Width(1).Height(16).Shrink(0).Background(t.Border)
		// Equal side slots mirror the logo; the launch icon is independent of the
		// user's configurable action order and remains at the toolbar's end.
		ui.Row(context).Size(34, 30).Shrink(0).Center().Label(p.T("快捷助手入口", "Quick assistant entry")).Children(func() {
			label := p.T("在快捷助手中打开", "Open in Quick assistant")
			button := selectionButton(context, "quick-assistant", label, p.Busy, 22)
			c.quickButtonID = button.ID()
			hint := label
			if p.Error != "" {
				hint = p.Error
			}
			c.toolbarHint(button, hint, t)
			if button.Clicked() && p.Actions.OpenQuick != nil {
				p.Actions.OpenQuick(p.Original)
			}
		})
	})
}

func (c *Controller) openSelectionAction(action resultwindow.SelectionAction) {
	if c.tooltip != nil {
		c.tooltip.close()
		c.tooltip = nil
	}
	c.Panel.Action, c.Initial.Action = action.ID, action.ID
	c.Panel.ActionLabel = c.actionLabel(action)
	c.Panel.ActionSelection = action.Selection
	c.Toolbar = false
	c.claimComposerHint()
	c.Window.SetResizable(true)
	c.Window.SetSkipTaskbar(c.SkipTaskbar())
	c.Window.SetSize(DefaultResultWidth, DefaultResultHeight)
	c.Window.SetMinimumSize(320, 300)
	if err := c.PrepareFrame(); err != nil {
		c.Panel.Error = err.Error()
		return
	}
	if err := PlaceSelectionWindow(c.Window.NativeHandle(), c.Initial.Anchor); err != nil {
		c.Panel.Error = err.Error()
		return
	}
	c.Window.Show()
	c.selectionResult()
}

func (c *Controller) selectionResult() {
	var selected *resultwindow.SelectionAction
	for _, action := range c.selectionActions() {
		if action.ID == c.Panel.Action {
			value := action
			selected = &value
			break
		}
	}
	if selected == nil {
		c.Panel.Error = c.Panel.T("不支持的划词操作", "Unsupported selection action")
		return
	}
	method, values, err := resultwindow.ActionRequest(*selected, c.Panel.Original, c.Panel.Target, c.Panel.Language)
	if err != nil {
		c.Panel.Error = err.Error()
		return
	}
	ctx, epoch := c.begin("selection")
	params := c.parameters(values)
	if selected.Kind == "translate" {
		params = c.translationParameters(values)
	}
	go func() {
		result, err := c.Client.Work(ctx, method, params, nil, c.progress(epoch, false))
		c.finish(epoch, func() { c.acceptResult(result) }, err)
	}()

}

// A toolbar hover changes only its face color. A fixed border and padding keep
// the icon still; the standard button retains keyboard and focus behavior.
func selectionButton(c *ui.Context, name, label string, disabled bool, iconSize ...float32) *ui.Element {
	size := float32(18)
	if len(iconSize) > 0 {
		size = iconSize[0]
	}
	t := c.Theme()
	button := ui.Button(c, "").Label(label).Size(30, 30).Padding((30-size)/2).Radius(6).Border(0, ui.Transparent).Background(ui.Transparent).TextColor(t.Text).Cursor(ui.CursorPointer).Disabled(disabled).Children(func() { ui.Icon(c, svg("toolbar-"+name)).Size(size, size) })
	if !disabled && (button.Hovered() || button.Pressed()) {
		button.Background(t.SurfaceHover)
	}
	return button
}
