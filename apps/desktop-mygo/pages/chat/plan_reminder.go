package chat

import (
	"math"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/chat/model"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

// currentPlan is the latest turn's plan, nil when that turn has none.
func currentPlan(items []*model.Item) *model.Plan {
	for i := len(items) - 1; i >= 0; i-- {
		if items[i].Kind == "response" {
			return model.PlanOf(items[i].Response)
		}
	}
	return nil
}

// planReminder floats the latest turn's plan above the input while the
// turn runs or is paused and its inline card is out of view, as Cindy's
// ChatPlanReminder: a pill with the current step, which opens the steps;
// dismissing it hides this plan's reminder until the turn plans anew.
func (p *Chat) planReminder(c *ui.Context, plan *model.Plan) {
	s := &p.ui.items
	if plan == nil || !(p.view.Running || isPaused(p.view)) || s.planDismissed == s.planAnchor || len(plan.Workflow.Items) == 0 {
		s.planOpen = false
		return
	}
	// A card not laid out yet counts as in view, as React's reminder
	// waits for the card it measures.
	card, viewport := s.planCard.Bounds(c), p.ui.transcript.Bounds(c)
	if card.H == 0 || card.Y+card.H > viewport.Y && card.Y < viewport.Y+viewport.H {
		s.planOpen = false
		return
	}
	t, pal := c.Theme(), kit.Colors(c)
	items := plan.Workflow.Items
	completed, active, pending := 0, -1, -1
	for i, item := range items {
		switch item.Status {
		case "completed":
			completed++
		case "in_progress":
			if active < 0 {
				active = i
			}
		case "pending":
			if pending < 0 {
				pending = i
			}
		}
	}
	done := completed >= len(items)
	current := min(completed, len(items)-1)
	if active >= 0 {
		current = active
	} else if pending >= 0 {
		current = pending
	}
	step := current + 1
	if done {
		step = len(items)
	}
	label := i18n.T("chat.planStep", "current", step, "total", len(items))
	ui.Row(c.Key("plan-reminder")).Attach(ui.AnchorTop, ui.AnchorBottom).Top(-10).Children(func() {
		pill := ui.ButtonBase(c).Gap(8).AlignItems(ui.Center).Padding(6, 14).Radius(99).Border(1, pal.Hairline).Background(pal.Panel).
			Label(i18n.T("chat.planTitle") + " · " + label).Expanded(s.planOpen)
		if pill.Hovered() {
			pill.Background(t.SurfaceHover)
		}
		pill.Children(func() {
			if done {
				ui.Icon(c, icons.CircleCheck).FontSize(14).TextColor(t.Text)
			} else {
				progressRing(c, float32(step)/float32(len(items)))
			}
			ui.Text(c, label).FontSize(kit.HelpSize(c)).SingleLine()
		})
		if pill.Clicked() {
			s.planOpen = !s.planOpen
		}
		// The steps open above the pill, over the transcript, as Cindy's
		// flyout, so the composer stays in view.
		ui.PopoverBase(c, pill, &s.planOpen, func(panel ui.Element) {
			panel.AttachTo(pill, ui.AnchorTop, ui.AnchorBottom).Top(-8).Width(360).MaxWidthPercent(92).Padding(10, 14).Gap(2).Radius(12).
				Background(pal.Panel).Border(1, pal.Hairline).Shadow(0, 8, 24, 0, ui.RGBA(0, 0, 0, 0.16)).Children(func() {
				ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
					ui.Text(c, i18n.T("chat.planTitle")).FontWeight(600).Grow(1)
					if kit.IconButton(c, icons.X, i18n.T("chat.planDismiss")).Clicked() {
						s.planDismissed, s.planOpen = s.planAnchor, false
					}
				})
				ui.Scroll(c).FillWidth().MaxHeight(280).Children(func() {
					planSteps(c, items, p.view.Running && !isPaused(p.view))
				})
			})
		})
	})
}

// planSteps lists a plan's steps with their state.
func planSteps(c *ui.Context, items []model.WorkflowItem, running bool) {
	t := c.Theme()
	for i, item := range items {
		ui.Row(c.Key("plan-step-"+kit.Itoa(i))).FillWidth().Gap(10).AlignItems(ui.Start).Padding(4, 0).Children(func() {
			icon, color := icons.Circle, t.TextMuted
			switch item.Status {
			case "completed":
				icon = icons.CircleCheck
			case "in_progress":
				icon, color = icons.CircleDot, t.Text
			}
			if item.Status == "in_progress" && running {
				ui.Spinner(c).Size(16, 16).Shrink(0).Margin(2, 0, 0, 0)
			} else {
				ui.Icon(c, icon).FontSize(16).TextColor(color).Shrink(0).Margin(2, 0, 0, 0)
			}
			text := ui.Text(c, item.Step).LineHeight(1.45).MinWidth(0)
			if item.Status == "completed" {
				text.TextColor(t.TextMuted)
			}
		})
	}
}

// progressRing is the pill's ring, filled to progress from the top.
func progressRing(c *ui.Context, progress float32) {
	t := c.Theme()
	ui.Box(c).Size(16, 16).Shrink(0).Draw(func(pt *ui.Painter, r ui.Rect) {
		cx, cy, radius := r.X+r.W/2, r.Y+r.H/2, r.W/2-1
		ring := func(to float32) *ui.Path {
			var path ui.Path
			steps := 48
			for i := 0; i <= steps; i++ {
				a := -math.Pi/2 + 2*math.Pi*float64(i)/float64(steps)*float64(to)
				x, y := cx+radius*float32(math.Cos(a)), cy+radius*float32(math.Sin(a))
				if i == 0 {
					path.MoveTo(x, y)
				} else {
					path.LineTo(x, y)
				}
			}
			return &path
		}
		pt.StrokePath(ring(1), 2, t.Border)
		if progress > 0 {
			pt.StrokePath(ring(min(progress, 1)), 2, t.Text)
		}
	})
}
