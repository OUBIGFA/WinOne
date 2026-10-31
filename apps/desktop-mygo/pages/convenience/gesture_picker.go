package convenience

import (
	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

// strokeWindow is a top-level window the Host found.
type strokeWindow struct {
	Window      int64  `json:"window"`
	ProcessID   int    `json:"processId"`
	ProcessPath string `json:"processPath"`
	WindowClass string `json:"windowClass"`
	Title       string `json:"title"`
}

// pathPicker picks a program by its window: drag the crosshair onto it
// and let go, and the Host reads the window under the pointer then
// (stroke.inspect.target; no global hook is installed); or choose it from
// the list of open windows (stroke.windows.list).
//
// React source: StrokePathPicker in features/mouse-gestures-controls.tsx.
type pathPicker struct {
	env      *core.Env
	busy     bool
	err      string
	dragging bool
	windows  []strokeWindow
	open     bool
}

func (p *pathPicker) capture(pick func(strokeWindow)) {
	p.busy, p.err = true, ""
	core.Go(p.env, func() (strokeWindow, error) {
		var target strokeWindow
		err := p.env.Host.CallCompletingInto("stroke.inspect.target", nil, &target)
		return target, err
	}, func(target strokeWindow, err error) {
		p.busy = false
		if err != nil {
			p.err = err.Error()
			return
		}
		pick(target)
	})
}

func (p *pathPicker) list() {
	p.busy, p.err = true, ""
	core.Go(p.env, func() ([]strokeWindow, error) {
		var windows []strokeWindow
		err := p.env.Host.CallCompletingInto("stroke.windows.list", nil, &windows)
		return windows, err
	}, func(windows []strokeWindow, err error) {
		p.busy = false
		if err != nil {
			p.err = err.Error()
			return
		}
		p.windows, p.open = windows, true
	})
}

func (p *pathPicker) View(c *ui.Context, disabled bool, pick func(strokeWindow)) {
	ui.Column(c).FillWidth().Gap(8).Children(func() {
		ui.Row(c).Gap(8).Wrap().Children(func() {
			drag := kit.Button(c, i18n.T("stroke.addPath"), icons.Crosshair, false).Disabled(disabled || p.busy).
				Tooltip(i18n.T("stroke.dragPath"))
			if p.dragging {
				drag.Cursor(ui.CursorCrosshair)
			}
			// Pressing takes the pointer; its release, wherever it is,
			// names the window under it.
			drag.HandleInput(func(ev ui.InputEvent) bool {
				switch {
				case ev.Kind == ui.InputPointerDown && ev.Button == 0:
					p.dragging = true
					return true
				case ev.Kind == ui.InputPointerUp && p.dragging:
					p.dragging = false
					p.capture(pick)
					return true
				}
				return false
			})
			if kit.Button(c, i18n.T("stroke.chooseWindow"), icons.Monitor, false).Disabled(disabled || p.busy).Clicked() {
				p.list()
			}
		})
		if p.err != "" {
			kit.Notice(c, kit.Failure, "", p.err, nil)
		}
	})
	if !p.open {
		return
	}
	theme := c.Theme()
	kit.Dialog(c, &p.open, kit.DialogOptions{
		Title: i18n.T("stroke.windows"), Width: 576, Cancel: i18n.T("common.close"),
		Body: func() {
			ui.Row(c).Children(func() {
				if kit.Button(c, i18n.T("stroke.refreshWindows"), icons.RefreshCw, p.busy).Disabled(p.busy).Clicked() {
					p.list()
				}
			})
			if len(p.windows) == 0 {
				kit.Help(c, i18n.T("stroke.noWindows"))
			}
			ui.Column(c).FillWidth().Gap(2).Role(ui.RoleList).Children(func() {
				for _, target := range p.windows {
					b := ui.ButtonBase(c.Key(target.Window)).FillWidth().Padding(8).Justify(ui.Start).Radius(kit.PanelRadius).Label(target.Title)
					if b.Hovered() {
						b.Background(theme.SurfaceHover)
					}
					b.Children(func() {
						ui.Column(c).FillWidth().MinWidth(0).Gap(2).Children(func() {
							ui.Text(c, target.Title).SingleLine().Ellipsis("…").MinWidth(0)
							kit.Help(c, target.ProcessPath)
						})
					})
					if b.Clicked() {
						pick(target)
						p.open = false
					}
				}
			})
		},
	})
}
