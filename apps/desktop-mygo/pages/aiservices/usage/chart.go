package usage

import (
	"math"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

// chartView is React's UsageChart: a trend chart's metric, style and
// split, the chart Magpie draws (ledger.go plans it, paint draws it), and
// its ranking beside it, whose rows pick the requests of a group.
type chartView struct {
	prefs *viewPrefs
	split string
	// emphasis is the ranking row under the pointer, which the chart
	// brings forward; tipWidth the tooltip's width in the last frame, to
	// keep it inside the chart.
	emphasis string
	tipWidth float32
}

func newChartView(prefs *viewPrefs) *chartView { return &chartView{prefs: prefs, split: "model"} }

// chartInput is what a chart shows and what its clicks do.
type chartInput struct {
	page *ledgerPage
	// filter is the ledger's, whose pick the ranking marks; nil for the
	// sessions' chart.
	filter *filter
	pick   func(dimension, id string)
	// day chooses a day by its column; nil when days are not chosen.
	day func(day string)
}

// plotHeight is the plot's height: lower in a narrow window, as React's
// stylesheet set it below 760 pixels.
func plotHeight(c *ui.Context) float32 {
	if w, _ := c.Size(); w <= 760 {
		return 260
	}
	return 310
}

func (ch *chartView) view(c *ui.Context, in chartInput) {
	page := in.page
	keyed := len(page.CallerKeys) > 0
	if ch.split == "callerKey" && !keyed {
		ch.split = "model"
	}
	dimension, metric := ch.split, ch.prefs.Metric
	picked := ""
	if f := in.filter; f != nil {
		switch dimension {
		case "modelAt":
			if f.Provider != "" && f.Model != "" {
				picked = f.Provider + "/" + f.Model
			}
		case "agent":
			picked = f.Source
		case "provider":
			picked = f.Provider
		case "model":
			picked = f.Model
		case "callerKey":
			picked = f.CallerKey
		}
	}
	ui.Column(c).FillWidth().Gap(14).Role(ui.RoleGroup).Label(tr("usage.trend")).Children(func() {
		kit.Heading(c, tr("usage.trend")).Role(ui.RoleHeading).Level(3)
		ui.Row(c).FillWidth().GapX(12).GapY(10).Wrap().AlignItems(ui.End).Children(func() {
			field(c, tr("usage.metric"), func() {
				options := make([]kit.SelectOption, len(chartMetrics))
				for i, m := range chartMetrics {
					key := "usage." + m
					if m == "tokens" {
						key = "usage.tokenTotal"
					}
					options[i] = kit.SelectOption{Value: m, Label: tr(key)}
				}
				if choose(c, &ch.prefs.Metric, options) {
					ch.prefs.save()
				}
			}).Basis(180)
			if kit.Segmented(c, &ch.prefs.Chart, []kit.SelectOption{
				{Value: "bars", Label: tr("usage.bars"), Icon: icons.ChartColumn},
				{Value: "line", Label: tr("usage.line"), Icon: icons.ChartLine},
			}, tr("usage.chartStyle")) {
				ch.prefs.save()
			}
			field(c, tr("usage.split"), func() {
				splits := []string{"model", "modelAt", "provider", "agent"}
				if keyed {
					splits = append(splits, "callerKey")
				}
				options := make([]kit.SelectOption, len(splits))
				for i, s := range splits {
					options[i] = kit.SelectOption{Value: s, Label: tr("usage." + s)}
				}
				choose(c, &ch.split, options)
			}).Basis(180)
		})
		pl := newChartPlan(page, dimension, metric, in.day != nil)
		ui.Row(c).FillWidth().GapX(20).GapY(16).Wrap().AlignItems(ui.Start).Children(func() {
			ui.Column(c).Basis(360).Grow(1.6).Shrink(1).MinWidth(0).Children(func() {
				ch.plot(c, &pl, in)
			})
			ui.Column(c).Basis(220).Grow(1).Shrink(1).MinWidth(0).Children(func() {
				list := page.By[dimension]
				legend := list
				if page.ChartBy != nil {
					legend = page.ChartBy[dimension]
				}
				rows := rankRows(list, legend, metric, len(list), false)
				ch.emphasis = rankList(c, rows, picked, false, func(id string) {
					if in.pick != nil {
						in.pick(dimension, id)
					}
				})
			})
		})
	})
}

// chartFont is the size of the axis' labels and of the tooltip.
func chartFont(c *ui.Context) float32 { return kit.CaptionSize(c) }

// widestLabel is the widest of the axis' labels, which the plot starts past.
func widestLabel(measure func(...ui.Span) (float32, float32), pl *chartPlan, size float32) float32 {
	widest := float32(0)
	for _, label := range pl.labels {
		if label == "" {
			continue
		}
		w, _ := measure(ui.Span{Text: label, Size: size, Features: "tnum"})
		widest = max(widest, w)
	}
	return widest
}

// plot is the chart itself, with what the pointer and the keyboard do on
// it: the tooltip of the point under the pointer and, by day, a button
// for each column that picks its day.
func (ch *chartView) plot(c *ui.Context, pl *chartPlan, in chartInput) {
	t, palette := c.Theme(), kit.Colors(c)
	height := plotHeight(c)
	size := chartFont(c)
	measure := func(spans ...ui.Span) (float32, float32) { return c.MeasureText(0, spans...) }
	widest := widestLabel(measure, pl, size)
	box := ui.Box(c.Key("plot")).FillWidth().Height(height).Role(ui.RoleImage).Label(lt("Usage trend"))
	if pl.selectable {
		box.Role(ui.RoleGroup)
	}
	bounds := box.Bounds()
	px, _, over := box.PointerPosition()
	g, laid := pl.geometry(bounds.W, height, widest)
	hover := -1
	if over && laid {
		hover = int(math.Floor(float64((px - g.left) / g.slot)))
		hover = max(0, min(len(pl.series)-1, hover))
	}
	line := ch.prefs.Chart == "line"
	box.Draw(func(p *ui.Painter, r ui.Rect) {
		ch.paint(p, r, pl, line, hover, size)
	})
	box.Children(func() {
		if len(pl.series) > 0 && pl.none != "" {
			ui.Column(c).Fill().Justify(ui.Center).AlignItems(ui.Center).Padding(0, 20).Children(func() {
				kit.Help(c, pl.none).TextAlign(ui.Center)
			})
			return
		}
		if !pl.shows() {
			return
		}
		if pl.selectable {
			// One stop of Tab, the arrows going from day to day, as a
			// toolbar's buttons; each says its day and whether it is picked.
			ui.Row(c).Absolute().Left(marginLeft(widest)).Right(marginRight).Top(marginTop).Bottom(marginBottom).
				FocusGroup(ui.Horizontal).Children(func() {
				for _, point := range pl.series {
					day := pointDay(point)
					b := ui.ButtonBase(c.Key(day)).Grow(1).Basis(0).MinWidth(0).FillHeight().Label(ledWhen(point, pl.bucket)).
						Role(ui.RoleToggleButton).Checked(day == pl.day).Cursor(ui.CursorPointer)
					if b.Clicked() && in.day != nil {
						in.day(day)
					}
				}
			})
		}
		if hover < 0 {
			return
		}
		ch.tooltip(c, pl, g, hover, size, t, palette)
	})
}

// tooltip is a point's figures beside its column, inside the chart: on
// its right, or its left where the right has no room.
func (ch *chartView) tooltip(c *ui.Context, pl *chartPlan, g chartGeometry, i int, size float32, t *ui.Theme, palette kit.Palette) {
	tp := pl.tip(i)
	x := g.left + g.slot*(float32(i)+0.5)
	w := ch.tipWidth
	if w <= 0 {
		w = 200
	}
	left := x + 14
	if x+14+w > g.w {
		left = x - w - 14
	}
	widest := float32(260)
	if pl.speed {
		widest = 400
	}
	box := ui.Column(c.Key("tip")).Absolute().Left(max(0, left)).Top(2).MinWidth(min(172, g.w)).MaxWidth(min(widest, g.w)).
		Padding(8, 10, 7, 10).Gap(2).Radius(9).Background(palette.Panel).Border(1, t.Border).FontSize(size + 1).PassThrough().
		Role(ui.RoleTooltip)
	ch.tipWidth = box.Bounds().W
	swatch := func(k tone) {
		ui.Box(c).Size(7, 7).Radius(2).Shrink(0).Background(colorOf(t, k))
	}
	box.Children(func() {
		ui.Text(c, tp.when).FontWeight(600).Margin(0, 0, 2, 0)
		if pl.speed {
			// Speeds are read across: the name in full, the speed and the
			// first token each in a column of their own (#2343).
			ui.Grid(c).FillWidth().ColumnTracks(ui.Fixed(7), ui.Fr(1), ui.FitContent(), ui.FitContent()).GapX(6).GapY(2).
				AlignItems(ui.Center).Children(func() {
				for k, r := range tp.rows {
					swatch(r.tone)
					ui.Text(c.Key(k), r.name).MinWidth(0)
					ui.Text(c.Key(-1-k), ledFormatLong(pl.metric, r.value)).NoWrap().FontFeatures("tnum").JustifySelf(ui.End)
					ttft := ""
					if v := ledTTFT(r.part.Timed, r.part.TTFT); v != 0 {
						ttft = lt("TTFT {ms}", "ms", ledTook(v))
					}
					ui.Text(c.Key(-1000-k), ttft).NoWrap().FontFeatures("tnum").TextColor(t.TextMuted).JustifySelf(ui.End)
				}
				all := lt("All")
				if tp.calls != "" {
					all += " · " + tp.calls
				}
				ui.Box(c).ColumnSpan(-1).Height(1).Background(t.Border).Margin(3, 0, 1, 0)
				ui.Text(c, all).ColumnSpan(2).TextColor(t.TextMuted).MinWidth(0)
				ui.Text(c, tp.total).NoWrap().FontFeatures("tnum").TextColor(t.TextMuted).JustifySelf(ui.End)
				ui.Text(c, tp.ttft).NoWrap().FontFeatures("tnum").TextColor(t.TextMuted).JustifySelf(ui.End)
			})
			return
		}
		for k, r := range tp.rows {
			ui.Row(c.Key(k)).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
				swatch(r.tone)
				ui.Text(c, r.name).SingleLine().Ellipsis("…").Grow(1).Shrink(1).MinWidth(0)
				ui.Text(c, ledFormatLong(pl.metric, r.value)).NoWrap().FontFeatures("tnum").Shrink(0).Margin(0, 0, 0, 12)
			})
		}
		total := tp.total
		if tp.calls != "" {
			total += " · " + tp.calls
		}
		ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Padding(3, 0, 0, 0).Margin(3, 0, 0, 0).
			BorderWidth(1, 0, 0, 0).BorderColor(t.Border).TextColor(t.TextMuted).Children(func() {
			ui.Text(c, lt("Total")).Grow(1).MinWidth(0)
			ui.Text(c, total).NoWrap().FontFeatures("tnum").Shrink(0).Margin(0, 0, 0, 12)
		})
	})
}

// paint draws the plan on the plot: the grid and its labels, the hovered
// and picked days, then the columns or the lines, the ranking's group in
// front while the pointer is on its row.
func (ch *chartView) paint(p *ui.Painter, r ui.Rect, pl *chartPlan, line bool, hover int, size float32) {
	t := p.Theme()
	widest := widestLabel(func(spans ...ui.Span) (float32, float32) { return p.MeasureText(0, spans...) }, pl, size)
	g, ok := pl.geometry(r.W, r.H, widest)
	if !ok {
		return
	}
	ox, oy := r.X, r.Y
	text := func(s string, x, y float32, align int, color ui.Color) {
		span := ui.Span{Text: s, Size: size, Color: color, Features: "tnum"}
		w, h := p.MeasureText(0, span)
		switch align {
		case 1: // right
			x -= w
		case 0: // center
			x -= w / 2
		}
		p.RichText(ox+x, oy+y-h/2, 0, span)
	}
	for i := range 5 {
		y := marginTop + g.height*float32(i)/4
		grid := (&ui.Path{}).MoveTo(ox+g.left, oy+y).LineTo(ox+g.w-marginRight, oy+y)
		p.StrokePath(grid.Cap(ui.ButtCap).Dash(0, 2, 3), 1, t.Border)
		text(pl.labels[i], g.left-6, y, 1, t.TextMuted)
	}
	for _, i := range pl.ticks(g) {
		text(ledTick(pl.series[i], pl.bucket), g.left+g.slot*(float32(i)+0.5), g.h-11, 0, t.TextMuted)
	}
	band := func(i int) {
		p.Fill(ui.Rect{X: ox + g.left + g.slot*float32(i), Y: oy + marginTop, W: g.slot, H: g.height}, t.SurfaceHover, 3)
	}
	if hover >= 0 {
		band(hover)
	}
	if pl.selectable && pl.selected >= 0 {
		band(pl.selected)
	}
	emphasis := ch.emphasis
	shade := func(k tone, key, day string) ui.Color {
		color := colorOf(t, k)
		if k == tonePill {
			// A speed column is the point's own speed: a track the marks
			// sit on, drawn strong enough to read as the track.
			color = t.SurfacePressed
		}
		dim := pl.day != "" && day != "" && day != pl.day
		if dim {
			color = colorOf(t, toneFaint)
		}
		if dim || emphasis != "" && key != emphasis {
			color = color.Alpha(0.22)
		}
		return color
	}
	if line {
		dot := float32(3)
		if g.slot >= 12 {
			dot = 4.5
		}
		for _, s := range pl.lines(g) {
			color := shade(s.tone, s.key, "")
			off := func(ps [][2]float32) [][2]float32 {
				out := make([][2]float32, len(ps))
				for i, pt := range ps {
					out[i] = [2]float32{ox + pt[0], oy + pt[1]}
				}
				return out
			}
			if s.banded {
				area := &ui.Path{}
				ledCurve(area, off(s.band[0]), true)
				under := off(s.band[1])
				for a, b := 0, len(under)-1; a < b; a, b = a+1, b-1 {
					under[a], under[b] = under[b], under[a]
				}
				ledCurve(area, under, false)
				area.Close()
				p.FillPath(area, color.Alpha(0.14))
			}
			path := &ui.Path{}
			for _, run := range s.runs {
				ledCurve(path, off(run), true)
			}
			width := float32(1.8)
			if s.dashed {
				width = 1.2
				path.Dash(0, 3, 3)
			}
			p.StrokePath(path, width, color)
			for _, d := range s.dots {
				dots := &ui.Path{}
				dots.Circle(ox+d[0], oy+d[1], dot/2)
				p.FillPath(dots, color)
			}
		}
		return
	}
	for _, s := range pl.segments(g) {
		color := shade(s.tone, s.key, s.day)
		switch s.kind {
		case segStem:
			stem := (&ui.Path{}).MoveTo(ox+s.x, oy+s.y).LineTo(ox+s.x, oy+s.y+s.h)
			p.StrokePath(stem.Cap(ui.ButtCap).Dash(0, 2, 2), 1, color.Alpha(0.45))
		case segMark:
			p.Fill(ui.Rect{X: ox + s.x, Y: oy + s.y, W: s.w, H: s.h}, color, 1.5)
		default:
			p.Fill(ui.Rect{X: ox + s.x, Y: oy + s.y, W: s.w, H: s.h}, color, 1)
		}
	}
}
