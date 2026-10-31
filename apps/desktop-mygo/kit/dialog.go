package kit

import "github.com/egoist/mygo/ui"

// DialogOptions describe a dialog asking about one decision.
type DialogOptions struct {
	Title   string
	Message string
	// Error reports why the last attempt failed, inside the dialog.
	Error string
	// Confirm labels the action; empty shows only Cancel.
	Confirm string
	// Cancel labels the way back.
	Cancel string
	// Danger colors Confirm as an action that discards or removes.
	Danger bool
	// Busy shows Confirm working and keeps the dialog open.
	Busy bool
	// ConfirmDisabled keeps Confirm unavailable, as before a check ends.
	ConfirmDisabled bool
	// Body adds content between the message and the buttons.
	Body func()
	// Actions adds buttons before Cancel, as "check again".
	Actions func()
	// Width is the panel's width; zero is 420.
	Width float32
}

// Dialog shows a modal dialog while *open is true. It returns true in the
// frame the user confirms; Cancel, Escape and a click on the backdrop set
// *open to false (unless Busy).
func Dialog(c *ui.Context, open *bool, o DialogOptions) bool {
	confirmed := false
	if !*open {
		return false
	}
	t, p := c.Theme(), Colors(c)
	width := o.Width
	if width == 0 {
		width = 420
	}
	stay := *open
	ui.DialogBase(c, &stay, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, backdropAlpha(t)))
		// A long form scrolls between the title and the buttons, which
		// stay in view, as the panel never outgrows the window.
		panel.Width(width).MaxWidthPercent(92).MaxHeightPercent(88).Padding(24).Gap(12).Radius(12).Background(p.Panel).
			Border(1, p.Hairline).Shadow(0, 12, 32, 0, ui.RGBA(0, 0, 0, 2*shadowAlpha(t))).
			Role(ui.RoleDialog).Label(o.Title).Children(func() {
			Title(c, o.Title).FontSize(t.Rem(headingSize) + 2).Shrink(0)
			ui.Scroll(c).FillWidth().Shrink(1).MinHeight(0).Gap(12).Children(func() {
				if o.Message != "" {
					ui.Text(c, o.Message).LineHeight(1.5).TextColor(t.TextMuted).Selectable()
				}
				if o.Body != nil {
					o.Body()
				}
				InlineError(c, o.Error)
			})
			ui.Row(c).Shrink(0).Gap(8).Justify(ui.End).Wrap().Margin(8, 0, 0, 0).Children(func() {
				if o.Actions != nil {
					o.Actions()
				}
				if o.Cancel != "" && Button(c, o.Cancel, nil, false).Disabled(o.Busy).AutoFocus().Clicked() {
					*open = false
				}
				if o.Confirm == "" {
					return
				}
				var b ui.Element
				if o.Danger {
					b = DangerButton(c, o.Confirm, nil, o.Busy)
				} else {
					b = PrimaryButton(c, o.Confirm, nil, o.Busy)
				}
				b.Disabled(o.ConfirmDisabled || o.Busy)
				if b.Clicked() {
					confirmed = true
				}
			})
		})
	})
	if !stay && !o.Busy {
		*open = false
	}
	return confirmed
}

func backdropAlpha(t *ui.Theme) float32 {
	if t.Dark {
		return 0.5
	}
	return 0.28
}
