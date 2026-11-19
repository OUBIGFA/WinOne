package shell

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

// The task stays at the bottom of every page. The narrow rail keeps all time
// digits readable by placing hours above minutes/seconds, not shrinking text.
// The countdown and progress stay outside the button: a button is a leaf to
// assistive technology, which reads nothing inside it.
func (s *Shell) timedPower(c *ui.Context, compact bool) {
	power := s.env.Power
	power.Frame(c)
	state := power.Status
	if state == nil || !state.Active || state.Task == nil {
		return
	}
	theme := c.Theme()
	label := i18n.T("timedPower." + state.Task.Action)
	if state.Task.DryRun {
		label = i18n.T("timedPower.dryRunTag")
	}
	ui.Box(c).FillWidth().Height(1).Margin(8, 0).Background(kit.Colors(c).Hairline)
	panel := ui.Column(c).FillWidth().Padding(8, 4).Radius(theme.Radius).Gap(6).AlignItems(ui.Center)
	if panel.Hovered() {
		panel.Background(theme.SurfaceHover)
	}
	open := false
	panel.Children(func() {
		button := ui.ButtonBase(c).FillWidth().Label(i18n.T("timedPower.openTask")).AlignItems(ui.Center)
		button.Children(func() {
			if !compact {
				ui.Text(c, label).SingleLine()
			} else {
				ui.Icon(c, icons.Timer).FontSize(18)
			}
		})
		if button.Clicked() {
			open = true
		}
		if power.Err != "" {
			ui.Text(c, i18n.T("timedPower.unavailable")).TextColor(theme.TextMuted)
		} else if state.State == "executing" {
			ui.Text(c, i18n.T("timedPower.executing")).TextColor(theme.TextMuted)
		} else {
			value := core.PowerRemaining(state.Deadline, time.Now())
			if compact {
				parts := strings.SplitN(value, ":", 2)
				value = parts[0] + "\n" + parts[1]
			}
			ui.Text(c, value).Font(kit.MonoFont).TextAlign(ui.Center).Role(ui.RoleTimer).Label(i18n.T("timedPower.remaining"))
			// Countdowns have an exact duration; calendar tasks show their remaining
			// time without inventing a starting time for a percentage.
			if state.Task.Mode == "countdown" && state.Task.DelaySeconds > 0 {
				value := 1 - max(0, time.Until(state.Deadline).Seconds())/float64(state.Task.DelaySeconds)
				ui.Progress(c, min(1, max(0, value))).FillWidth().Label(i18n.T("timedPower.progress"))
			}
		}
	})
	if panel.Clicked() {
		open = true
	}
	if open {
		s.env.Open("convenience", core.Intent{Tab: "timed-power"})
	}
}
