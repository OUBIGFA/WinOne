package usage

// The allowance curves, ported from yetone/magpie at
// ede1e32aec1a4f6bf06aeef6ed2e03d4218db226, internal/gui/assets/app.js
// (quotaHistOf, quotaLines, quotaCycleBreak, quotaCycle, quotaSpan,
// quotaPaths, quotaTimeText, drawQuotaPlot, quotaCurve, quotaReadings,
// curveZoom), as apps/desktop/frontend/src/features/magpie-quota-curve.ts
// carried them. MIT License, Copyright (c) 2026 yetone; the notice is
// third_party/licenses/upstream-magpie-ede1e32aec1a4f6bf06aeef6ed2e03d4218db226-0-LICENSE.txt.
//
// Cycle breaks, the span, the even pace, the faint short windows, the
// readings and the zoom on a click, Enter or Space are upstream's; the
// plot is painted by MyGo, stretched across the card as its SVG was.

import (
	"math"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/kit"
)

func qt(key string, values ...any) string { return magpieText(quotaChinese, key, values...) }

// none is a reading's missing start or reset, as upstream's null.
var none = math.NaN()

func known(v float64) bool { return !math.IsNaN(v) }

// msOf is a time as milliseconds since 1970, NaN for none, as Date.parse.
func msOf(s string) float64 {
	if s == "" {
		return none
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return none
	}
	return float64(t.UnixNano()) / 1e6
}

// curvePoint is a reading: when, what was left, and its cycle's start and
// reset when the vendor told them.
type curvePoint struct {
	at, left, start, reset float64
}

// curveLine is a window's readings, as its card names it.
type curveLine struct {
	name, label string
	window      quotaWindow
	points      []curvePoint
}

// quotaLines are the lines of an account's windows, as its card lists
// them, with readings: none of an unlimited window or a family's; a
// pool's window under the pool's name and its own.
func quotaLines(sub allowance) []curveLine {
	var out []curveLine
	for _, w := range sub.Windows {
		if w.Unlimited || w.Family != "" {
			continue
		}
		name, label := w.Name, qt(w.Name)
		if w.Pool != "" {
			name, label = w.Pool+" · "+w.Name, w.Pool+" · "+qt(w.Name)
		}
		var line *quotaLine
		for i := range sub.History {
			if sub.History[i].Name == name {
				line = &sub.History[i]
				break
			}
		}
		if line == nil || len(line.Points) == 0 {
			continue
		}
		duplicate := false
		for _, o := range out {
			duplicate = duplicate || o.name == line.Name
		}
		if duplicate {
			continue
		}
		l := curveLine{name: name, label: label, window: w}
		for _, p := range line.Points {
			l.points = append(l.points, curvePoint{at: msOf(p.At), left: p.Left, start: msOf(p.Start), reset: msOf(p.ResetsAt)})
		}
		out = append(out, l)
	}
	return out
}

// quotaCycleBreak is whether b, read after a, is of its window started
// again, as magpie's record of them says (quota_history.go's newCycle).
func quotaCycleBreak(a, b curvePoint) bool {
	if !known(a.reset) && !known(b.reset) {
		return b.left > a.left+5
	}
	if !known(a.reset) || !known(b.reset) {
		return true
	}
	if a.reset <= b.at {
		return true
	}
	moved, slack := b.reset-a.reset, 5*60e3
	return moved > b.at-a.at+slack || moved < -slack
}

// quotaCycle is a line's current cycle: its start and its reset, the
// window's as the card has it, else its last reading's.
func quotaCycle(l curveLine, now float64) (start, reset float64) {
	last := l.points[len(l.points)-1]
	reset = last.reset
	if l.window.ResetsAt != "" {
		reset = msOf(l.window.ResetsAt)
	}
	start = last.start
	if known(last.start) && known(last.reset) && known(reset) {
		start = last.start + (reset - last.reset)
	}
	if !known(start) {
		// Its first reading of this cycle.
		i := len(l.points) - 1
		for i > 0 && !quotaCycleBreak(l.points[i-1], l.points[i]) {
			i--
		}
		start = l.points[i].at
	}
	if known(reset) && reset <= now {
		reset = none
	}
	return math.Min(start, now), reset
}

// quotaSpan is the time the plot shows: the last two days, or the cycles
// of every line to the latest reset.
func quotaSpan(lines []curveLine, rangeName string, now float64) (float64, float64) {
	if rangeName == "2d" {
		return now - 48*3600e3, now
	}
	x0, x1 := math.Inf(1), now
	for _, l := range lines {
		start, reset := quotaCycle(l, now)
		x0 = math.Min(x0, start)
		if known(reset) && reset != 0 {
			x1 = math.Max(x1, reset)
		}
	}
	if !(x0 < now) {
		x0 = now - 3600e3
	}
	return x0, x1
}

// quotaPaths are a line's readings in [x0, x1] on a w×h box, a new run
// at each cycle's start; the reading before x0 and the one after x1 go
// along, so the line reaches the box's edges. A run of one is a dot.
func quotaPaths(points []curvePoint, x0, x1 float64, w, h float32) [][][2]float32 {
	X := func(t float64) float32 { return float32((t - x0) / (x1 - x0) * float64(w)) }
	Y := func(v float64) float32 { return h - float32(math.Max(0, math.Min(100, v))/100)*h }
	var runs [][][2]float32
	var prev *curvePoint
	for i := range points {
		p := points[i]
		var next *curvePoint
		if i+1 < len(points) {
			next = &points[i+1]
		}
		if p.at < x0 && (next == nil || next.at < x0 || quotaCycleBreak(p, *next)) {
			continue
		}
		if prev != nil && p.at > x1 && prev.at > x1 {
			break
		}
		if prev == nil || quotaCycleBreak(*prev, p) {
			runs = append(runs, nil)
		}
		runs[len(runs)-1] = append(runs[len(runs)-1], [2]float32{X(p.at), Y(p.left)})
		prev = &points[i]
	}
	return runs
}

// quotaTimeText says when: "Now" within a minute, the time today, else
// the date and the time.
func quotaTimeText(at, now float64) string {
	if math.Abs(at-now) < 60e3 {
		return qt("Now")
	}
	d, n := time.UnixMilli(int64(at)).Local(), time.UnixMilli(int64(now)).Local()
	clock := formatDate(d, "HHmm")
	if d.Year() == n.Year() && d.YearDay() == n.YearDay() {
		return clock
	}
	return formatDate(d, "MMMd") + " " + clock
}

// quotaReadings are a line's last eight readings, newest first: what was
// left and when it was read, ↻ where the window started again (#802).
func quotaReadings(l curveLine, now float64) []string {
	pts := l.points[max(0, len(l.points)-8):]
	out := make([]string, len(pts))
	for i, p := range pts {
		mark := ""
		if i > 0 && quotaCycleBreak(pts[i-1], p) {
			mark = "↻ "
		}
		out[len(pts)-1-i] = mark + toFixed(round(p.left), 0) + "% · " + quotaTimeText(p.at, now)
	}
	return out
}

// quotaCurve is an account's windows over time, under its meters: the
// even pace dashed, what was left solid, now dotted; a click, Enter or
// Space makes it larger and back (curveZoom). big is its zoom, kept by
// the card. It shows nothing for the range "off" or without readings.
func quotaCurve(c *ui.Context, sub allowance, rangeName string, big *bool) {
	if rangeName == "off" {
		return
	}
	lines := quotaLines(sub)
	if len(lines) == 0 {
		return
	}
	t := c.Theme()
	now := float64(c.Now().UnixNano()) / 1e6
	x0, x1 := quotaSpan(lines, rangeName, now)
	span := qt("Cycle")
	if rangeName == "2d" {
		span = qt("2 days")
	}
	size := kit.HelpSize(c)
	ui.Column(c).FillWidth().Gap(4).Margin(10, 0, 0, 0).TextColor(t.TextMuted).FontSize(size).
		Tooltip(qt("Solid: what was left. Dashed: an even pace, full at the cycle's start to empty at its reset; a line above its dashes lasts the cycle, one below runs out before the reset.")).
		Children(func() {
			ui.Row(c).FillWidth().Gap(6).Justify(ui.SpaceBetween).AlignItems(ui.Baseline).Children(func() {
				ui.Text(c, qt("Left over time")).MinWidth(0)
				ui.Text(c, span).NoWrap()
			})
			height := float32(52)
			if *big {
				height = 168
			}
			plot := ui.ButtonBase(c).FillWidth().Height(height).Label(qt("Left over time") + " · " + span).Expanded(*big).
				Role(ui.RoleImage).Cursor(ui.CursorPointer)
			if plot.Clicked() {
				*big = !*big
			}
			plot.Draw(func(p *ui.Painter, r ui.Rect) {
				drawQuotaPlot(p, r, lines, x0, x1, now, t)
			})
			ui.Row(c).FillWidth().Gap(8).Justify(ui.SpaceBetween).Children(func() {
				ui.Text(c, quotaTimeText(x0, now)).NoWrap().FontFeatures("tnum")
				ui.Text(c, quotaTimeText(x1, now)).NoWrap().FontFeatures("tnum")
			})
			ui.Row(c).FillWidth().GapX(12).GapY(3).Wrap().Children(func() {
				for i, l := range lines {
					last := l.points[len(l.points)-1]
					left := toFixed(round(last.left), 0) + "%"
					tip := []string{qt("{name}: {n} left, read {when}", "name", l.label, "n", left, "when", quotaTimeText(last.at, now)), "", qt("Latest readings:")}
					tip = append(tip, quotaReadings(l, now)...)
					ui.Row(c.Key(l.name)).Gap(5).AlignItems(ui.Center).MinWidth(0).MaxWidthPercent(100).Tooltip(strings.Join(tip, "\n")).
						Children(func() {
							ui.Box(c).Size(7, 7).Radius(3.5).Shrink(0).Background(seriesColors[i%len(seriesColors)])
							ui.Text(c, l.label).SingleLine().Ellipsis("…").Shrink(1).MinWidth(0)
							ui.Text(c, left).FontWeight(600).FontFeatures("tnum").TextColor(t.Text).Shrink(0)
						})
				}
			})
		})
}

// drawQuotaPlot puts the lines' curves for [x0, x1] in r: the grid, the
// even burns, the lines, and now.
func drawQuotaPlot(p *ui.Painter, r ui.Rect, lines []curveLine, x0, x1, now float64, t *ui.Theme) {
	w, h := r.W, r.H
	line := func(ax, ay, bx, by float32, width float32, color ui.Color, dash ...float32) {
		path := (&ui.Path{}).MoveTo(r.X+ax, r.Y+ay).LineTo(r.X+bx, r.Y+by)
		if len(dash) > 0 {
			path.Cap(ui.ButtCap).Dash(0, dash...)
		}
		p.StrokePath(path, width, color)
	}
	for _, v := range []float32{0, 50, 100} {
		y := h - v/100*h
		line(0, y, w, y, 1, t.Border)
	}
	X := func(at float64) float32 { return float32((at - x0) / (x1 - x0) * float64(w)) }
	for i, l := range lines {
		color := seriesColors[i%len(seriesColors)]
		start, reset := quotaCycle(l, now)
		// A window far shorter than the span (a 5-hour one over a week)
		// is drawn faint, without its even burn.
		short := known(reset) && reset != 0 && reset-start < (x1-x0)/12
		if known(reset) && reset != 0 && start < reset && !short {
			line(X(start), 0, X(reset), h, 1, color.Alpha(0.45), 3, 3)
		}
		width, shade := float32(1.5), color
		if short {
			width, shade = 1, color.Alpha(0.45)
		}
		for _, run := range quotaPaths(l.points, x0, x1, w, h) {
			if len(run) == 1 {
				dot := &ui.Path{}
				dot.Circle(r.X+run[0][0], r.Y+run[0][1], width/2+0.5)
				p.FillPath(dot, shade)
				continue
			}
			path := (&ui.Path{}).MoveTo(r.X+run[0][0], r.Y+run[0][1])
			for _, pt := range run[1:] {
				path.LineTo(r.X+pt[0], r.Y+pt[1])
			}
			p.StrokePath(path, width, shade)
		}
	}
	if now >= x0 && now <= x1 {
		line(X(now), 0, X(now), h, 1, t.TextMuted, 1, 2)
	}
}
