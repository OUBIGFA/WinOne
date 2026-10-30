package kit

import (
	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/kit/icons"
)

// Tone is what a notice reports.
type Tone int

const (
	// Info is neutral information about the current state.
	Info Tone = iota
	// Failure is an error: what failed, why, and what to do next.
	Failure
	// Caution warns of a consequence before the user acts.
	Caution
)

// Notice is a message in the flow of a page, above what it concerns.
// Actions follow the message on their own line, so long translations
// never overlap them. The message can be selected and copied.
func Notice(c *ui.Context, tone Tone, title, message string, actions func()) ui.Element {
	t, p := c.Theme(), Colors(c)
	background, border, icon, iconColor := p.InfoSoft, p.Hairline, icons.Info, t.TextMuted
	switch tone {
	case Failure:
		background, border, icon, iconColor = p.DangerSoft, p.DangerLine, icons.CircleAlert, t.Danger
	case Caution:
		background, border, icon, iconColor = p.WarningSoft, t.Warning.Alpha(0.35), icons.TriangleAlert, t.Warning
	}
	n := ui.Row(c).FillWidth().Padding(12, 14).Gap(10).AlignItems(ui.Start).Background(background).
		Border(1, border).Radius(PanelRadius)
	// Screen readers hear a status as it appears; MyGo has no separate
	// alert role.
	n.Role(ui.RoleStatus)
	return n.Children(func() {
		ui.Icon(c, icon).FontSize(16).TextColor(iconColor).Shrink(0).Margin(2, 0, 0, 0)
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(4).Selectable().Children(func() {
			if title != "" {
				Strong(c, title)
			}
			if message != "" {
				ui.Text(c, message).FontSize(HelpSize(c)).LineHeight(1.5)
			}
			if actions != nil {
				ui.Row(c).Gap(8).Wrap().Margin(6, 0, 0, 0).Unselectable().Children(actions)
			}
		})
	})
}

// InlineError reports a failure next to the control or action it
// concerns; empty message shows nothing.
func InlineError(c *ui.Context, message string) ui.Element {
	if message == "" {
		return ui.Element{}
	}
	t := c.Theme()
	return ui.Row(c).Gap(6).AlignItems(ui.Start).Role(ui.RoleStatus).Children(func() {
		ui.Icon(c, icons.CircleAlert).FontSize(HelpSize(c)).TextColor(t.Danger).Shrink(0).Margin(2, 0, 0, 0)
		ui.Text(c, message).FontSize(HelpSize(c)).LineHeight(1.45).TextColor(t.Danger).Selectable().Shrink(1).MinWidth(0)
	})
}

// Status is a short state in the flow, as "已保存" beside an action.
func Status(c *ui.Context, message string) ui.Element {
	if message == "" {
		return ui.Element{}
	}
	return Help(c, message).Role(ui.RoleStatus)
}

// Empty is the state of a view with nothing to show yet: an icon, what
// is missing, how to add it, and the action that does.
func Empty(c *ui.Context, icon *ui.SVG, title, hint string, action func()) ui.Element {
	t := c.Theme()
	return ui.Column(c).FillWidth().Padding(40, 24).Gap(8).AlignItems(ui.Center).Children(func() {
		if icon != nil {
			ui.Icon(c, icon).FontSize(28).TextColor(t.TextMuted).Margin(0, 0, 4, 0)
		}
		Strong(c, title).TextAlign(ui.Center)
		if hint != "" {
			Help(c, hint).TextAlign(ui.Center).MaxWidth(420)
		}
		if action != nil {
			ui.Row(c).Gap(8).Margin(8, 0, 0, 0).Children(action)
		}
	})
}

// Skeleton stands in for rows while their content loads.
func Skeleton(c *ui.Context, rows int) ui.Element {
	p := Colors(c)
	return ui.Column(c).FillWidth().Gap(12).Role(ui.RoleStatus).Label("Loading").Children(func() {
		for i := range rows {
			width := []float32{100, 72, 86, 64}[i%4]
			ui.Box(c.Key(i)).Height(14).WidthPercent(width).Radius(4).Background(p.Skeleton)
		}
	})
}

// Badge is a short state in a pill: neutral, or colored by its meaning.
func Badge(c *ui.Context, text string, tone Tone) ui.Element {
	t, p := c.Theme(), Colors(c)
	background, color := t.Surface, t.TextMuted
	switch tone {
	case Failure:
		background, color = p.DangerSoft, t.Danger
	case Caution:
		background, color = p.WarningSoft, t.Warning
	}
	return ui.Text(c, text).FontSize(CaptionSize(c)).Padding(2, 8).Radius(999).Background(background).
		TextColor(color).SingleLine()
}

// SuccessBadge is a Badge for something that is on or went well.
func SuccessBadge(c *ui.Context, text string) ui.Element {
	t, p := c.Theme(), Colors(c)
	return ui.Text(c, text).FontSize(CaptionSize(c)).Padding(2, 8).Radius(999).Background(p.SuccessSoft).
		TextColor(t.Success).SingleLine()
}
