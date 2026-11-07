package kit

import (
	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit/icons"
)

// Toasts keeps MyGo's existing timing, hover/focus pause, action and Escape
// lifecycle, adding only the explicit native close control required by WinOne.
func Toasts(c *ui.Context) {
	ui.ToastViewportBase(c, func(viewport ui.Element, toasts []ui.Toast) {
		viewport.Padding(16).AlignItems(ui.End).Gap(8)
		for _, toast := range toasts {
			parts := ui.ToastBase(c, toast)
			parts.Root.Row().AlignItems(ui.Center).Gap(10).Padding(10, 14).MaxWidth(440).MaxWidthPercent(92).Radius(PanelRadius).Border(1, Colors(c).Hairline).Background(Colors(c).Panel)
			parts.Root.Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
					if toast.Title != "" {
						ui.Text(c, toast.Title).MinWidth(0)
					}
					if toast.Description != "" {
						Help(c, toast.Description).MinWidth(0)
					}
				})
				if toast.Action != "" {
					parts.ActionButton().Padding(4, 8).Label(toast.Action).Children(func() { ui.Text(c, toast.Action) })
				}
				parts.CloseButton().Size(24, 24).Label(i18n.T("notices.closeToast")).Tooltip(i18n.T("notices.closeToast")).Children(func() { ui.Icon(c, icons.X).FontSize(14) })
			})
		}
	})
}
