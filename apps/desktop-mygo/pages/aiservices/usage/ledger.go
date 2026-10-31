package usage

// The Usage ledger's figures and chart geometry, ported from yetone/magpie
// at 42ec2700bd2a4cf8d4ceb76a95767ac221e283e5 (internal/gui/assets/app.js
// and plugins/usage/page.js: ledGroupSpeed, ledAxis, ledCategories,
// drawLedColumns, drawLedRank, ledLines, ledCurve), as
// apps/desktop/frontend/src/features/magpie-ledger-charts.ts carried them
// into WinOne. MIT License, Copyright (c) 2026 yetone; the notice is
// third_party/licenses/upstream-magpie-42ec2700bd2a4cf8d4ceb76a95767ac221e283e5-0-LICENSE.txt.
//
// The arithmetic, the ordering, the axis, the colors by rank, the margins,
// the stacking and the tooltip's contents follow upstream line by line.
// What changed is only the medium: these functions compute the plan of a
// chart, which chart.go paints with MyGo's painter instead of building
// SVG elements, and rank.go builds as MyGo elements.

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"
)

// ledShown is how many groups a chart tells apart; the rest are "Other".
const ledShown = 7

// tone is a mark's color: one of the seven series colors by rank, or
// one of the neutral colors the chart uses for what is no group's own.
type tone int

const (
	toneFaint tone = -1 // "Other", and days a selection dims
	tonePill  tone = -2 // a speed column's own track
	toneMuted tone = -3 // the point's own speed line
)

// seriesColors are the ledger's seven series colors (--usage-c1…c7),
// muted so they read as data rather than decoration in both themes.
var seriesColors = [7]ui.Color{
	ui.RGB(0x62, 0x6f, 0xb0), ui.RGB(0x4e, 0x92, 0xaf), ui.RGB(0x55, 0x94, 0x77), ui.RGB(0xb6, 0x8a, 0x49),
	ui.RGB(0xac, 0x6b, 0x91), ui.RGB(0x86, 0x74, 0xac), ui.RGB(0x71, 0x81, 0x8c),
}

// colorOf is a tone in the theme.
func colorOf(t *ui.Theme, k tone) ui.Color {
	switch k {
	case toneFaint, toneMuted:
		return t.TextMuted
	case tonePill:
		return t.Surface
	}
	return seriesColors[int(k)%len(seriesColors)]
}

func lt(key string, values ...any) string { return magpieText(ledgerChinese, key, values...) }

// allTokens is what went in and out and through the cache.
func allTokens(x totals) float64 { return x.Input + x.Output + x.CacheRead + x.CacheWrite }

// cacheHitOf is a total's cache hit rate, leaving out the calls whose
// vendor counts no cache; false when nothing is left (#3464).
func cacheHitOf(x totals) (float64, bool) {
	prompt := x.Input + x.CacheRead + x.CacheWrite - x.CacheUnknown
	if prompt > 0 {
		return x.CacheRead / prompt, true
	}
	return 0, false
}

// cacheLeftOut says what a hit rate leaves out, when it leaves anything.
func cacheLeftOut(x totals) string {
	if x.CacheUnknownCalls == 0 {
		return ""
	}
	key := "{n} requests left out: their vendor doesn't count the cache"
	if x.CacheUnknownCalls == 1 {
		key = "{n} request left out: its vendor doesn't count the cache"
	}
	return lt(key, "n", num(x.CacheUnknownCalls))
}

// speedOf is tokens a second over a time: 0 for none.
func speedOf(out, ms float64) float64 {
	if ms > 0 {
		return 1000 * out / ms
	}
	return 0
}

// ledSpeed is how fast the timed replies wrote after their first token
// (usage.Totals.Speed); ledFloor the least the replies of a group that has
// no first token wrote at (usage.Totals.Floor).
func ledSpeed(x totals) float64 { return speedOf(x.DecodeOut, x.DecodeMs) }
func ledFloor(x totals) float64 { return speedOf(x.FloorOut, x.FloorMs) }

// groupSpeed is a group's speed as every ranking and the speed card say
// it: the measured speed and its count, the session files' floor and its
// count, the one of more replies first.
type groupSpeed struct {
	v           float64 // what a speed ranking orders and bars by
	text        string  // both, as one line
	main, other string  // the first and the second, "" with one
	title       string  // how each was measured
}

// ledGroupSpeed is Magpie's rule (Fallmonkey on Discord): with replies
// whose speed was measured, their speed and how many they are; with
// replies read from session files, the floor they wrote at and how many;
// with both, both, the one of more replies first; else nothing (false).
func ledGroupSpeed(x totals) (groupSpeed, bool) {
	v, f, kv, kf := ledSpeed(x), ledFloor(x), x.DecodeN, x.FloorN
	if v == 0 && f == 0 {
		return groupSpeed{}, false
	}
	tok := func(s float64) string { return lt("{n} tok/s", "n", num(round(s))) }
	count := func(k float64, timed bool) string {
		if k == 0 {
			return ""
		}
		key := "{k} replies"
		switch {
		case timed && k == 1:
			key = "{k} timed reply"
		case timed:
			key = "{k} timed replies"
		case k == 1:
			key = "{k} reply"
		}
		return lt(key, "k", num(k))
	}
	vTitle := ""
	if v != 0 && kv != 0 {
		key := "Measured on {k} replies, from their first token to their end; the others tell no speed"
		switch {
		case f != 0 && kv == 1:
			key = "Measured on {k} reply, from its first token to its end"
		case f != 0:
			key = "Measured on {k} replies, from their first token to their end"
		case kv == 1:
			key = "Measured on {k} reply, from its first token to its end; the others tell no speed"
		}
		vTitle = lt(key, "k", num(kv))
	}
	fTitle := ""
	if f != 0 && kf != 0 {
		key := "At least: {k} replies read from session files, which don't say when their answers began, so time before that is counted too"
		if kf == 1 {
			key = "At least: {k} reply read from a session file, which doesn't say when its answer began, so time before that is counted too"
		}
		fTitle = lt(key, "k", num(kf))
	}
	type figure struct {
		s                  float64
		head, count, title string
	}
	var measured, floor *figure
	if v != 0 {
		measured = &figure{v, tok(v), count(kv, true), vTitle}
	}
	if f != 0 {
		floor = &figure{f, "≥" + tok(f), count(kf, false), fTitle}
	}
	// The larger sample first; a tie keeps the measured one first.
	a, b := measured, floor
	if a == nil {
		a, b = floor, nil
	} else if b != nil && kf > kv {
		a, b = floor, measured
	}
	say := func(p *figure) string {
		if p.count != "" {
			return p.head + " · " + p.count
		}
		return p.head
	}
	g := groupSpeed{v: a.s, main: say(a), title: a.title}
	g.text = g.main
	if b != nil {
		g.other = say(b)
		g.text = lt("{a}; {b}", "a", g.main, "b", g.other)
		if g.title != "" && b.title != "" {
			g.title += "\n" + b.title
		} else if b.title != "" {
			g.title = b.title
		}
	}
	return g, true
}

// ledTTFT is the mean wait for the first token, 0 for none.
func ledTTFT(timed, ttft float64) float64 {
	if timed > 0 {
		return ttft / timed
	}
	return 0
}

// ledTook writes milliseconds as seconds.
func ledTook(ms float64) string { return toFixed(ms/1000, 2) + " s" }

// ledValue is a metric of a point or a share, as the chart counts it.
func ledValue(metric string, x totals) float64 {
	switch metric {
	case "speed":
		return ledSpeed(x)
	case "cost":
		return x.Cost
	case "calls":
		return x.Calls
	}
	return allTokens(x)
}

// ledRankValue is a share's metric in the ranking: a speed is what its
// row says first.
func ledRankValue(metric string, x totals) float64 {
	if metric == "speed" {
		g, _ := ledGroupSpeed(x)
		return g.v
	}
	return ledValue(metric, x)
}

// ledPart is the metric of a point's part.
func ledPart(metric string, p part) float64 {
	switch metric {
	case "speed":
		return speedOf(p.DecodeOut, p.DecodeMs)
	case "cost":
		return p.Cost
	case "calls":
		return p.Calls
	}
	return p.Tokens
}

// ledFormat is a metric as its axis says it; ledFormatLong as its values do.
func ledFormat(metric string, v float64) string {
	switch metric {
	case "cost":
		return usd(v, 2)
	case "calls", "speed":
		return num(round(v))
	}
	return fmtN(round(v))
}

func ledFormatLong(metric string, v float64) string {
	switch metric {
	case "speed":
		if v > 0 {
			return lt("{n} tok/s", "n", num(round(v)))
		}
		return "—"
	case "cost":
		return usd(v, 6)
	}
	return num(round(v))
}

// ledAxis is an axis's top with four steps under it: round numbers, the
// top at least the largest value.
func ledAxis(max float64) float64 {
	if !(max > 0) {
		return 4
	}
	e := math.Pow(10, math.Floor(math.Log10(max/4)))
	for _, m := range []float64{1, 2, 2.5, 5, 10} {
		if m*e*4 >= max {
			return m * e * 4
		}
	}
	return 10 * e * 4
}

// category is a share the chart or the ranking tells apart, with its color.
type category struct {
	share
	tone tone
}

// ledCategories is what a chart tells apart, most first: the top shown
// in the colors of their rank, the rest together as "Other".
func ledCategories(list []share, metric string, shown int) (top, rest []category) {
	shown = max(ledShown, shown)
	sorted := make([]category, len(list))
	for i, x := range list {
		sorted[i] = category{share: x}
	}
	sort.SliceStable(sorted, func(i, j int) bool {
		a, b := ledRankValue(metric, sorted[i].totals), ledRankValue(metric, sorted[j].totals)
		if a != b {
			return a > b
		}
		return allTokens(sorted[i].totals) > allTokens(sorted[j].totals)
	})
	n := min(shown, len(sorted))
	for i := range sorted[:n] {
		sorted[i].tone = tone(i % len(seriesColors))
	}
	for i := range sorted[n:] {
		sorted[n+i].tone = toneFaint
	}
	return sorted[:n], sorted[n:]
}

// pointTime reads a point's time: RFC 3339 from the ledger, or a local
// date and time without a zone, as the sessions' days are.
func pointTime(s string) time.Time {
	if t, ok := parseTime(s); ok {
		return t
	}
	t, _ := time.ParseInLocation("2006-01-02T15:04:05", s, time.Local)
	return t
}

// ledWhen is when a point is: the hour, the day or the week it stands for.
func ledWhen(p point, bucket string) string {
	d := pointTime(p.Time)
	if bucket == "hour" {
		return formatDate(d, "yMMddHHmm")
	}
	day := formatDate(d, "yMMdd")
	if bucket == "week" {
		return lt("week of {label}", "label", day)
	}
	return day
}

// ledTick is a point's label on the axis.
func ledTick(p point, bucket string) string {
	d := pointTime(p.Time)
	if bucket == "hour" {
		return twoDigits(d.Hour()) + ":00"
	}
	return formatDate(d, "Md")
}

func twoDigits(n int) string { return fmt.Sprintf("%02d", n) }

// pointDay is a point's day, YYYY-MM-DD.
func pointDay(p point) string {
	if len(p.Time) >= 10 {
		return p.Time[:10]
	}
	return p.Time
}

// ledCurve adds a smooth line through ps to path, as d3's curveMonotoneX
// draws it (Fritsch–Carlson): it bends between points but never past one,
// so a quiet hour isn't drawn dipping under zero or a peak over its value.
// move starts it with a move, else it goes on from where the path is.
func ledCurve(path *ui.Path, ps [][2]float32, move bool) {
	n := len(ps)
	if n == 0 {
		return
	}
	if move {
		path.MoveTo(ps[0][0], ps[0][1])
	} else {
		path.LineTo(ps[0][0], ps[0][1])
	}
	if n == 1 {
		return
	}
	x := func(i int) float64 { return float64(ps[i][0]) }
	y := func(i int) float64 { return float64(ps[i][1]) }
	s := make([]float64, n-1)
	m := make([]float64, n)
	for i := 0; i < n-1; i++ {
		s[i] = (y(i+1) - y(i)) / (x(i+1) - x(i))
	}
	sign := func(v float64) float64 {
		switch {
		case v > 0:
			return 1
		case v < 0:
			return -1
		}
		return 0
	}
	for i := 1; i < n-1; i++ {
		h0, h1 := x(i)-x(i-1), x(i+1)-x(i)
		p := (s[i-1]*h1 + s[i]*h0) / (h0 + h1)
		if s[i-1]*s[i] <= 0 {
			m[i] = 0
		} else {
			m[i] = (sign(s[i-1]) + sign(s[i])) * math.Min(math.Min(math.Abs(s[i-1]), math.Abs(s[i])), math.Abs(p)/2)
		}
	}
	if n > 2 {
		m[0] = (3*s[0] - m[1]) / 2
		m[n-1] = (3*s[n-2] - m[n-2]) / 2
	} else {
		m[0], m[n-1] = s[0], s[0]
	}
	for i := 0; i < n-1; i++ {
		xa, ya, xb, yb := x(i), y(i), x(i+1), y(i+1)
		dx := (xb - xa) / 3
		path.CubeTo(float32(xa+dx), float32(ya+dx*m[i]), float32(xb-dx), float32(yb-dx*m[i+1]), float32(xb), float32(yb))
	}
}

// chartPlan is what a chart shows, before its size is known: the series,
// the groups told apart and their colors, the axis and the selection.
type chartPlan struct {
	series        []point
	bucket, day   string
	split, metric string
	top           []category
	totals        []float64
	speed         bool
	topV          float64
	labels        [5]string
	// selectable charts choose a day by its column (day buckets with a
	// day filter); selected is the chosen day's column, -1 for none.
	selectable bool
	selected   int
	// none is what an empty chart says instead.
	none string
}

// newChartPlan plans the chart of l by split and metric (drawLedColumns'
// first half). The chart's groups are the whole period's (chartBy) while
// a day is picked, so its colors stay as the day changes.
func newChartPlan(l *ledgerPage, split, metric string, selectable bool) chartPlan {
	by := l.By
	if l.ChartBy != nil {
		by = l.ChartBy
	}
	pl := chartPlan{series: l.Series, bucket: l.Bucket, day: l.Day, split: split, metric: metric, speed: metric == "speed", selected: -1}
	pl.top, _ = ledCategories(by[split], metric, ledShown)
	pl.totals = make([]float64, len(pl.series))
	most := 0.0
	for i, p := range pl.series {
		pl.totals[i] = ledValue(metric, p.totals)
		most = math.Max(most, pl.totals[i])
		if pl.speed {
			// Speeds don't add up: each group's mark is at its own speed,
			// which may be past the point's own.
			for _, c := range pl.top {
				most = math.Max(most, ledPart("speed", p.By[split][c.ID]))
			}
		}
	}
	if !(most > 0) {
		switch {
		case metric == "cost":
			pl.none = lt("No known price for these requests")
		case pl.speed:
			pl.none = lt("No streamed replies timed in this period")
		default:
			pl.none = lt("Nothing in this period")
		}
		return pl
	}
	pl.topV = ledAxis(most)
	for i := range pl.labels {
		pl.labels[i] = ledFormat(metric, pl.topV*(1-float64(i)/4))
	}
	pl.selectable = l.Bucket == "day" && selectable
	if pl.selectable {
		for i, p := range pl.series {
			if pointDay(p) == l.Day {
				pl.selected = i
				break
			}
		}
	}
	return pl
}

// shows reports whether the plan has columns to draw.
func (pl *chartPlan) shows() bool { return pl.none == "" && len(pl.series) > 0 }

// chartGeometry is a plan laid out on a plot: margins, the plot's
// inner box and each point's slot.
type chartGeometry struct {
	w, h           float32
	left           float32
	inner, height  float32 // the plotting area's width and height
	slot, barWidth float32
}

const (
	marginRight  = 6
	marginTop    = 8
	marginBottom = 22
)

// marginLeft is where the plot starts: past the widest axis label, so
// none reaches past the chart's edge ("8000 万" is wider than "80M").
func marginLeft(widest float32) float32 {
	if widest > 0 {
		return float32(math.Ceil(float64(widest))) + 6
	}
	return 44
}

// geometry lays the plan out on a w×h plot; false when it is too small
// or has nothing to draw.
func (pl *chartPlan) geometry(w, h, widest float32) (chartGeometry, bool) {
	n := len(pl.series)
	if w < 100 || !pl.shows() {
		return chartGeometry{}, false
	}
	g := chartGeometry{w: w, h: h, left: marginLeft(widest)}
	g.inner = w - g.left - marginRight
	g.height = h - marginTop - marginBottom
	g.slot = g.inner / float32(n)
	g.barWidth = float32(math.Max(2, math.Min(30, float64(g.slot)*0.68)))
	return g, true
}

// y is where a value is on the plot.
func (pl *chartPlan) y(g chartGeometry, v float64) float32 {
	return marginTop + g.height - float32(float64(g.height)*v/pl.topV)
}

// ticks returns the points whose day or hour the x axis names: as many
// as fit, 62 DIPs apart at least.
func (pl *chartPlan) ticks(g chartGeometry) []int {
	n := len(pl.series)
	room := max(2, int(math.Floor(float64(g.inner)/62)))
	every := int(math.Ceil(float64(n) / float64(room)))
	var out []int
	for i := 0; i < n; i += every {
		out = append(out, i)
	}
	return out
}

// segKind is what a segment of the columns chart is.
type segKind int

const (
	segBar   segKind = iota // a group's part of a column
	segTrack                // a speed column: the point's own speed
	segStem                 // a leader from a speed column up to a mark past it
	segMark                 // a group's speed on its column
)

// segment is one mark of the columns chart.
type segment struct {
	kind       segKind
	key, day   string
	tone       tone
	x, y, w, h float32 // a stem's x is its line's, y and h its ends
}

// segments are the columns: each point's part of each group told apart,
// stacked from the bottom in the ranking's colors, and what is left of
// its total as "Other"; for speed, the point's own speed as a track with
// each group's mark at its own speed on it.
func (pl *chartPlan) segments(g chartGeometry) []segment {
	var segs []segment
	for i, p := range pl.series {
		x := g.left + g.slot*(float32(i)+0.5) - g.barWidth/2
		day := pointDay(p)
		y0 := 0.0
		draw := func(key string, t tone, kind segKind, v float64) {
			if !(v > 0) {
				return
			}
			top, bottom := pl.y(g, y0+v), pl.y(g, y0)
			segs = append(segs, segment{kind: kind, key: key, day: day, tone: t, x: x, w: g.barWidth, y: top, h: float32(math.Max(0.5, float64(bottom-top)))})
			y0 += v
		}
		by := p.By[pl.split]
		if pl.speed {
			if pl.totals[i] > 0 {
				draw("\x00all", tonePill, segTrack, pl.totals[i])
			}
			for _, c := range pl.top {
				v := ledPart("speed", by[c.ID])
				if !(v > 0) {
					continue
				}
				if v > pl.totals[i] {
					// A group faster than the point's own leaves its mark
					// above the column: a leader ties the two, stopping at
					// the column's top within a pixel of it.
					top := pl.y(g, pl.totals[i])
					end := float32(math.Min(float64(pl.y(g, v)+1.5), float64(top)))
					segs = append(segs, segment{kind: segStem, key: c.ID, day: day, tone: c.tone, x: x + g.barWidth/2, y: end, h: top - end})
				}
				segs = append(segs, segment{kind: segMark, key: c.ID, day: day, tone: c.tone, x: x, w: g.barWidth, y: pl.y(g, v) - 1.5, h: 3})
			}
			continue
		}
		stacked := 0.0
		for _, c := range pl.top {
			v := ledPart(pl.metric, by[c.ID])
			stacked += v
			draw(c.ID, c.tone, segBar, v)
		}
		draw("\x00other", toneFaint, segBar, math.Max(0, pl.totals[i]-stacked))
	}
	return segs
}

// lineSeries is one group of the lines chart: its unbroken runs, each a
// smooth curve, a dot for a run of one point, and the band under it of a
// stacked chart.
type lineSeries struct {
	key    string
	tone   tone
	runs   [][][2]float32
	dots   [][2]float32
	band   [2][][2]float32 // its top, and the line under it, left to right
	banded bool
	dashed bool
}

// lines are the chart as lines (ledLines): tokens, cost and requests stay
// stacked, each line where its part of the column ended, its band filled
// under it; speeds don't add up, so each group's line is at its own
// speed, broken where it had none, over the point's own speed dashed.
// They are in painting order: a band never covers the line under it.
func (pl *chartPlan) lines(g chartGeometry) []lineSeries {
	x0 := g.left + g.slot/2
	at := func(i int, v float64) [2]float32 { return [2]float32{x0 + g.slot*float32(i), pl.y(g, v)} }
	series := func(key string, t tone, vals []float64, known []bool, base []float64, dashed bool) (lineSeries, bool) {
		s := lineSeries{key: key, tone: t, dashed: dashed}
		var run [][2]float32
		end := func() {
			if len(run) == 1 && !dashed {
				s.dots = append(s.dots, run[0])
			}
			if len(run) > 0 {
				s.runs = append(s.runs, run)
			}
			run = nil
		}
		for i, v := range vals {
			if !known[i] {
				end()
				continue
			}
			run = append(run, at(i, v))
		}
		end()
		if len(s.runs) == 0 {
			return s, false
		}
		if base != nil {
			s.banded = true
			for i := range vals {
				s.band[0] = append(s.band[0], at(i, vals[i]))
				s.band[1] = append(s.band[1], at(i, base[i]))
			}
		}
		return s, true
	}
	n := len(pl.series)
	all := func([]float64) []bool {
		known := make([]bool, n)
		for i := range known {
			known[i] = true
		}
		return known
	}
	var out []lineSeries
	if pl.speed {
		known := make([]bool, n)
		for i, v := range pl.totals {
			known[i] = v > 0
		}
		if s, ok := series("\x00all", toneMuted, pl.totals, known, nil, true); ok {
			out = append(out, s)
		}
		for _, c := range pl.top {
			vals, known := make([]float64, n), make([]bool, n)
			for i, p := range pl.series {
				vals[i] = ledPart("speed", p.By[pl.split][c.ID])
				known[i] = vals[i] != 0
			}
			if s, ok := series(c.ID, c.tone, vals, known, nil, false); ok {
				out = append(out, s)
			}
		}
		return out
	}
	// From the bottom up: each one's part on what is under it, "Other"
	// last; each goes under those before it.
	type stack struct {
		key  string
		tone tone
		vals []float64
	}
	parts := make([]stack, 0, len(pl.top)+1)
	for _, c := range pl.top {
		vals := make([]float64, n)
		for i, p := range pl.series {
			vals[i] = ledPart(pl.metric, p.By[pl.split][c.ID])
		}
		parts = append(parts, stack{c.ID, c.tone, vals})
	}
	other := make([]float64, n)
	for i := range other {
		sum := 0.0
		for _, p := range parts {
			sum += p.vals[i]
		}
		other[i] = math.Max(0, pl.totals[i]-sum)
	}
	parts = append(parts, stack{"\x00other", toneFaint, other})
	under := make([]float64, n)
	for _, p := range parts {
		over := make([]float64, n)
		any := false
		for i := range over {
			over[i] = under[i] + p.vals[i]
			any = any || p.vals[i] > 0
		}
		if any {
			if s, ok := series(p.key, p.tone, over, all(over), under, false); ok {
				out = append([]lineSeries{s}, out...)
			}
		}
		under = over
	}
	return out
}

// tipRow is one line of a point's tooltip.
type tipRow struct {
	name  string
	tone  tone
	value float64
	part  part
}

// tip is what a point's tooltip says (drawLedColumns' pointer handler):
// when it is, each group's figure most first, and the total.
type tip struct {
	when  string
	rows  []tipRow
	total string
	calls string
	ttft  string
}

func (pl *chartPlan) tip(i int) tip {
	p := pl.series[i]
	out := tip{when: ledWhen(p, pl.bucket)}
	by := p.By[pl.split]
	var rows []tipRow
	stacked := 0.0
	for _, c := range pl.top {
		name := c.Name
		if name == "" {
			name = c.ID
		}
		if name == "" {
			name = "—"
		}
		v := ledPart(pl.metric, by[c.ID])
		stacked += v
		rows = append(rows, tipRow{name: lt(name), tone: c.tone, value: v, part: by[c.ID]})
	}
	if !pl.speed {
		rows = append(rows, tipRow{name: lt("Other"), tone: toneFaint, value: math.Max(0, pl.totals[i]-stacked)})
	}
	sort.SliceStable(rows, func(a, b int) bool { return rows[a].value > rows[b].value })
	for _, r := range rows {
		if r.value > 0 {
			out.rows = append(out.rows, r)
		}
	}
	if pl.metric != "calls" && p.Calls != 0 {
		key := "{n} requests"
		if p.Calls == 1 {
			key = "{n} request"
		}
		out.calls = lt(key, "n", num(p.Calls))
	}
	out.total = ledFormatLong(pl.metric, pl.totals[i])
	if pl.speed {
		if t := ledTTFT(p.Timed, p.TTFT); t != 0 {
			out.ttft = lt("TTFT {ms}", "ms", ledTook(t))
		}
	}
	return out
}

// rankRow is one row of a ranking (drawLedRank): a group with its share,
// its other figures, and a click that lists only its requests.
type rankRow struct {
	share
	tone   tone
	plain  bool    // the sum of those left out, which no click picks
	value  string  // the metric, right of the name
	bar    float64 // the bar's length, in percent of the leader's
	detail string  // the other figures, below the bar
	errors string  // its failed requests, said apart in red
	loops  string  // its replies ended for looping, in red too
	title  string  // everything it says, for its tooltip
}

// rankRows ranks list by metric: the top shown, and the rest summed as
// one plain row. A ranking beside a chart takes the chart's colors from
// its legend, the groups the chart tells apart, so a day picked keeps
// each group's color and a group the chart counts as "Other" is shown in
// Other's; without one, the rows take the colors of their rank. compact
// rows say less, as the overview's breakdowns do.
func rankRows(list, legend []share, metric string, shown int, compact bool) []rankRow {
	top, rest := ledCategories(list, metric, shown)
	if len(top) == 0 {
		return nil
	}
	if legend != nil {
		colors, _ := ledCategories(legend, metric, ledShown)
		for i := range top {
			top[i].tone = toneFaint
			for _, c := range colors {
				if c.ID == top[i].ID {
					top[i].tone = c.tone
				}
			}
		}
	}
	sum := 0.0
	for _, x := range append(append([]category(nil), top...), rest...) {
		sum += ledValue(metric, x.totals)
	}
	if sum == 0 {
		sum = 1
	}
	leader := math.Max(1, ledRankValue(metric, top[0].totals))
	one := func(x category, plain bool) rankRow {
		v := ledRankValue(metric, x.totals)
		gs, hasSpeed := ledGroupSpeed(x.totals)
		r := rankRow{share: x.share, tone: x.tone, plain: plain, bar: math.Min(100, math.Max(1.5, 100*v/leader))}
		if r.Name == "" {
			r.Name = x.ID
		}
		if r.Name == "" {
			r.Name = "—"
		}
		r.Name = lt(r.Name)
		switch {
		case metric != "speed":
			r.value = ledFormatLong(metric, v)
		case hasSpeed:
			r.value = gs.text
		default:
			r.value = "—"
		}
		requests := func() string {
			key := "{n} requests"
			if x.Calls == 1 {
				key = "{n} request"
			}
			return lt(key, "n", num(x.Calls))
		}
		share := toFixed(round(100*v/sum), 0) + "%"
		var more, brief, extra []string
		if metric != "calls" {
			more = append(more, requests())
			brief = append(brief, requests())
		}
		if metric != "tokens" {
			more = append(more, lt("{n} tokens", "n", fmtN(allTokens(x.totals))))
		}
		if metric != "cost" && x.Cost != 0 {
			more = append(more, "≈"+usd(x.Cost, 6))
			brief = append(brief, "≈"+usd(x.Cost, 6))
		}
		if x.Credited != 0 {
			// Two decimals at most, as (+n.toFixed(2)).toLocaleString() wrote them.
			credits := "<0.01"
			if !(x.Credits > 0 && x.Credits < 0.01) {
				rounded, _ := strconv.ParseFloat(toFixed(x.Credits, 2), 64)
				credits = num(rounded)
			}
			more = append(more, lt("{n} credits", "n", credits))
			extra = append(extra, lt("What {n} of these requests cost in their plan's credits, as the replies said", "n", num(x.Credited)))
		}
		if metric != "speed" && hasSpeed {
			more = append(more, gs.text)
		}
		if hasSpeed && gs.title != "" {
			extra = append(extra, gs.title)
		}
		if t := ledTTFT(x.Timed, x.TTFT); t != 0 {
			more = append(more, lt("TTFT {ms}", "ms", ledTook(t)))
		}
		if hit, ok := cacheHitOf(x.totals); ok && !compact {
			more = append(more, lt("hit rate {p}", "p", toFixed(round(100*hit), 0)+"%"))
		}
		if metric != "speed" {
			more = append(more, share)
			brief = append(brief, share)
		}
		r.title = join(more, " · ")
		if compact {
			r.detail = join(brief, " · ")
		} else {
			r.detail = r.title
		}
		if x.Errors != 0 {
			r.errors = lt("{n} failed", "n", toString(x.Errors))
		}
		if x.Loops != 0 {
			r.loops = lt("{n} looped", "n", num(x.Loops))
			extra = append(extra, lt("Replies magpie ended because the model kept repeating itself"))
		}
		if len(extra) > 0 {
			r.title += "\n" + join(extra, "\n")
		}
		return r
	}
	rows := make([]rankRow, 0, len(top)+1)
	for _, x := range top {
		rows = append(rows, one(x, false))
	}
	if len(rest) > 0 {
		other := category{share: share{ID: "\x00other", Name: lt("Other ({n})", "n", len(rest))}, tone: toneFaint}
		for _, x := range rest {
			other.add(x.totals)
		}
		row := one(other, true)
		// "Other ({n})" is already in the window's language.
		row.Name = other.Name
		rows = append(rows, row)
	}
	return rows
}

func join(parts []string, sep string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += sep
		}
		out += p
	}
	return out
}
