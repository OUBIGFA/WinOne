package usage

import (
	"math"
	"strings"
	"testing"
	"time"

	"winone/desktopmygo/i18n"
)

// The expected texts below are what Chromium's Intl and JavaScript wrote
// for the same values (node 24, ICU 77), the React window's formatters.
func TestFormatsFollowTheReactWindow(t *testing.T) {
	values := []float64{0, 7, 999, 999.95, 1000, 1234, 9999.95, 12345, 99999, 123456, 999950, 1234567, 99995000, 123456789, 1e12, 1.5e13}
	compactWant := map[string][]string{
		i18n.Chinese: {"0", "7", "999", "1000", "1000", "1234", "1万", "1.2万", "10万", "12.3万", "100万", "123.5万", "9999.5万", "1.2亿", "1万亿", "15万亿"},
		i18n.English: {"0", "7", "999", "1K", "1K", "1.2K", "10K", "12.3K", "100K", "123.5K", "1M", "1.2M", "100M", "123.5M", "1T", "15T"},
	}
	usdWant := map[string][2][]string{
		i18n.Chinese: {{"US$0.00", "US$0.02", "US$0.001234", "US$1,234.50", "US$0.123457", "US$0.00"}, {"US$0.00", "US$0.02", "US$1,234.57", "US$0.01"}},
		i18n.English: {{"$0.00", "$0.02", "$0.001234", "$1,234.50", "$0.123457", "$0.00"}, {"$0.00", "$0.02", "$1,234.57", "$0.01"}},
	}
	defer i18n.SetLocale(i18n.Chinese)
	for _, locale := range []string{i18n.Chinese, i18n.English} {
		i18n.SetLocale(locale)
		for i, v := range values {
			if got := fmtN(v); got != compactWant[locale][i] {
				t.Errorf("%s fmtN(%v) = %q, want %q", locale, v, got, compactWant[locale][i])
			}
		}
		for i, v := range []float64{0, 0.02, 0.001234, 1234.5, 0.1234567, 1e-7} {
			if got := usd(v, 6); got != usdWant[locale][0][i] {
				t.Errorf("%s usd6(%v) = %q", locale, v, got)
			}
		}
		for i, v := range []float64{0, 0.02, 1234.567, 0.005} {
			if got := usd(v, 2); got != usdWant[locale][1][i] {
				t.Errorf("%s usd2(%v) = %q", locale, v, got)
			}
		}
	}
	for _, c := range []struct {
		v      float64
		digits int
		want   string
	}{{0.125, 2, "0.13"}, {1.005, 2, "1.00"}, {2.5, 0, "3"}, {0.0005, 3, "0.001"}, {123.456, 1, "123.5"}, {0.1, 6, "0.100000"}} {
		if got := toFixed(c.v, c.digits); got != c.want {
			t.Errorf("toFixed(%v, %d) = %q, want %q", c.v, c.digits, got, c.want)
		}
	}
	if num(1234.5678) != "1,234.568" || num(math.NaN()) != "0" {
		t.Errorf("num %q", num(1234.5678))
	}
	if round(2.5) != 3 || round(-2.5) != -2 || round(0.49999999999999994) != 0 {
		t.Error("round is not Math.round")
	}
	for v, want := range map[float64]string{950: "950", 1000: "1K", 1234: "1.2K", 12345: "12.3K", 123456: "123K", 1234567: "1.23M", 1200000: "1.20M", 12345678: "12.3M"} {
		if got := fmtK(v); got != want {
			t.Errorf("fmtK(%v) = %q, want %q", v, got, want)
		}
	}
}

func shareOf(id string, input float64) share {
	return share{ID: id, totals: totals{Calls: 1, Input: input, Output: 20, CacheRead: 30, CacheWrite: 10, Cost: 0.02, Timed: 1, TTFT: 500, DecodeMs: 1000, DecodeOut: 20}}
}

func TestRankingsKeepMagpiesTopSevenAndOther(t *testing.T) {
	i18n.SetLocale(i18n.English)
	defer i18n.SetLocale(i18n.Chinese)
	var models []share
	for i := range 9 {
		models = append(models, shareOf("model-"+string(rune('0'+i)), float64(100-i)))
	}
	rows := rankRows(models, nil, "tokens", 7, false)
	if len(rows) != 8 || !rows[7].plain || rows[7].Name != "Other (2)" || rows[0].ID != "model-0" {
		t.Fatalf("rows %d last %+v", len(rows), rows[len(rows)-1])
	}
	if rows[7].Calls != 2 || rows[7].Input != 92+93 {
		t.Fatalf("Other sums %+v", rows[7].totals)
	}
	// Unfolded, every group stays a row that picks its requests.
	all := rankRows(models, nil, "tokens", 9, true)
	if len(all) != 9 || all[8].plain || all[8].ID != "model-8" {
		t.Fatalf("unfolded %d", len(all))
	}
	// Beside a chart, a group past its seven takes Other's color.
	beside := rankRows(models, models, "tokens", 9, false)
	if beside[6].tone != 6 || beside[7].tone != toneFaint {
		t.Fatalf("legend colors %v %v", beside[6].tone, beside[7].tone)
	}
	// Figures: requests, share, speed, first token and hit rate.
	if !strings.Contains(rows[0].detail, "1 request") || !strings.Contains(rows[0].detail, "TTFT 0.50 s") ||
		!strings.Contains(rows[0].detail, "hit rate 21%") || !strings.Contains(rows[0].detail, "20 tok/s") {
		t.Fatalf("detail %q", rows[0].detail)
	}
}

func ledgerPageFixture() *ledgerPage {
	base := totals{Calls: 2, Input: 80, Output: 20, Cost: 0.02}
	page := &ledgerPage{Bucket: "day", By: map[string][]share{"model": {{ID: "alpha", totals: base}, {ID: "beta", totals: base}}}}
	for i := range 3 {
		page.Series = append(page.Series, point{Time: time.Date(2026, 10, 20+i, 0, 0, 0, 0, time.Local).Format(time.RFC3339), totals: base,
			By: map[string]map[string]part{"model": {"alpha": {Calls: 1, Tokens: 60, Cost: 0.012}, "beta": {Calls: 1, Tokens: 40, Cost: 0.008}}}})
	}
	return page
}

func TestColumnsStackAndLinesBandEachGroup(t *testing.T) {
	pl := newChartPlan(ledgerPageFixture(), "model", "tokens", true)
	g, ok := pl.geometry(600, 300, 30)
	if !ok || pl.topV != 100 {
		t.Fatalf("geometry %v top %v", ok, pl.topV)
	}
	if segs := pl.segments(g); len(segs) != 6 {
		t.Fatalf("columns %d", len(segs))
	}
	lines := pl.lines(g)
	if len(lines) != 2 || !lines[0].banded || !lines[1].banded || lines[0].key != "beta" || lines[1].key != "alpha" {
		t.Fatalf("lines %+v", lines)
	}
	// alpha's band is from zero up to it; beta's from alpha up.
	if lines[1].band[1][0][1] != pl.y(g, 0) || lines[0].band[1][0][1] != pl.y(g, 60) || lines[0].band[0][0][1] != pl.y(g, 100) {
		t.Fatalf("bands %v %v", lines[1].band, lines[0].band)
	}
	if pl.selected != -1 || !pl.selectable {
		t.Fatalf("selection %v %v", pl.selectable, pl.selected)
	}
	tip := pl.tip(1)
	if len(tip.rows) != 2 || tip.rows[0].name != "alpha" || tip.total != "100" || tip.calls != lt("{n} requests", "n", "2") {
		t.Fatalf("tip %+v", tip)
	}
}

func TestSpeedLinesBreakAtUnmeasuredRepliesWithoutStacking(t *testing.T) {
	page := ledgerPageFixture()
	for i := range page.Series {
		p := &page.Series[i]
		p.DecodeOut, p.DecodeMs = 100, 1000
		alpha := part{Calls: 1, Tokens: 60, DecodeOut: 60, DecodeMs: 1000}
		if i == 1 {
			alpha.DecodeMs = 0
		}
		p.By = map[string]map[string]part{"model": {"alpha": alpha, "beta": {Calls: 1, Tokens: 40, DecodeOut: 40, DecodeMs: 1000}}}
	}
	pl := newChartPlan(page, "model", "speed", false)
	g, ok := pl.geometry(600, 300, 30)
	if !ok {
		t.Fatal("no geometry")
	}
	var alpha *lineSeries
	dashed := 0
	lines := pl.lines(g)
	for i := range lines {
		if lines[i].banded {
			t.Fatal("speeds were stacked")
		}
		if lines[i].dashed {
			dashed++
		}
		if lines[i].key == "alpha" {
			alpha = &lines[i]
		}
	}
	if dashed != 1 || alpha == nil || len(alpha.runs) != 2 || len(alpha.dots) != 2 {
		t.Fatalf("speed lines %+v", lines)
	}
	for _, s := range lines {
		for _, run := range s.runs {
			for _, pt := range run {
				if math.IsNaN(float64(pt[1])) || math.IsInf(float64(pt[1]), 0) {
					t.Fatalf("a point is not a number: %v", pt)
				}
			}
		}
	}
	// A column's speed is the point's own; each group is a mark on it.
	marks, tracks := 0, 0
	for _, s := range pl.segments(g) {
		switch s.kind {
		case segMark:
			marks++
		case segTrack:
			tracks++
		}
	}
	if tracks != 3 || marks != 5 {
		t.Fatalf("tracks %d marks %d", tracks, marks)
	}
}

func TestEmptyChartsSayWhy(t *testing.T) {
	i18n.SetLocale(i18n.Chinese)
	page := &ledgerPage{Series: []point{{Time: "2026-10-20T00:00:00Z"}}}
	for metric, want := range map[string]string{"tokens": "此范围内还没有请求记录。", "cost": "这些请求尚无已知价格", "speed": "此范围内没有已计时的流式回复"} {
		if pl := newChartPlan(page, "model", metric, true); pl.none != want || pl.shows() {
			t.Errorf("%s: %q", metric, pl.none)
		}
	}
	for max, want := range map[float64]float64{0: 4, 1: 1, 130: 200, 99: 100, 4000: 4000} {
		if got := ledAxis(max); got != want {
			t.Errorf("ledAxis(%v) = %v, want %v", max, got, want)
		}
	}
}

func TestTotalsLeaveUnknownCacheOutAndKeepSpeedsApart(t *testing.T) {
	i18n.SetLocale(i18n.Chinese)
	sum := totals{Calls: 2, Input: 180, Output: 20, CacheRead: 80, CacheWrite: 40, CacheUnknown: 100, CacheUnknownCalls: 1,
		DecodeOut: 120, DecodeMs: 1000, DecodeN: 1, FloorOut: 400, FloorMs: 2000, FloorN: 2}
	if rate, ok := cacheHitOf(sum); !ok || toFixed(rate*100, 1) != "40.0" {
		t.Fatalf("hit rate %v %v", rate, ok)
	}
	g, ok := ledGroupSpeed(sum)
	if !ok || g.main != "≥200 Token/秒 · 2 次回复" || g.other != "120 Token/秒 · 1 次计时" || g.v != 200 {
		t.Fatalf("speed %+v", g)
	}
	if !strings.Contains(g.title, "下限：2 次读自会话文件的回复") || !strings.Contains(g.title, "实测 1 次回复：从首字到结束") {
		t.Fatalf("speed title %q", g.title)
	}
	if got := cacheLeftOut(sum); got != "1 个请求未计入：其服务商不统计缓存" {
		t.Fatalf("left out %q", got)
	}
	if _, ok := cacheHitOf(totals{Input: 100, CacheUnknown: 100, CacheUnknownCalls: 2}); ok {
		t.Fatal("a hit rate of nothing")
	}
}

func TestRequestSpeedsSayWhyThereIsNone(t *testing.T) {
	r := row{Out: 1000, Ms: 2000, Status: 200}
	if s := requestSpeed(r); s.value != 500 || !s.floor || s.reason != "speedWholeFloor" {
		t.Fatalf("whole floor %+v", s)
	}
	failed := r
	failed.Status = 500
	empty := r
	empty.Out = 0
	untimed := r
	untimed.Out = 25
	measured := r
	measured.Speed = 88
	for want, x := range map[string]row{"speedFailed": failed, "speedNoOutput": empty, "speedNoFirstToken": untimed} {
		if s := requestSpeed(x); s.reason != want || s.value != 0 {
			t.Errorf("%s: %+v", want, s)
		}
	}
	if s := requestSpeed(measured); s.value != 88 || s.floor || s.reason != "" {
		t.Errorf("measured %+v", s)
	}
}

func TestQuotaCurvesUseTheRecordedReadings(t *testing.T) {
	now := time.Now()
	reset, start := now.Add(time.Hour).UTC().Format(time.RFC3339), now.Add(-time.Hour).UTC().Format(time.RFC3339)
	q := allowance{Provider: "fixture", User: "a@example.invalid", Windows: []quotaWindow{{Name: "Window", Used: 30, ResetsAt: reset}, {Name: "Family", Family: "x"}},
		History: []quotaLine{{Name: "Window", Points: []quotaReading{{At: start, Left: 90, Start: start, ResetsAt: reset}, {At: now.UTC().Format(time.RFC3339), Left: 70, Start: start, ResetsAt: reset}}}}}
	lines := quotaLines(q)
	if len(lines) != 1 || len(lines[0].points) != 2 {
		t.Fatalf("lines %+v", lines)
	}
	ms := float64(now.UnixNano()) / 1e6
	x0, x1 := quotaSpan(lines, "cycle", ms)
	if math.Abs(x0-(ms-3600e3)) > 2000 || math.Abs(x1-(ms+3600e3)) > 2000 {
		t.Fatalf("span %v %v", x0-ms, x1-ms)
	}
	runs := quotaPaths(lines[0].points, x0, x1, 300, 60)
	if len(runs) != 1 || len(runs[0]) != 2 || runs[0][1][1] != 60-0.7*60 {
		t.Fatalf("runs %v", runs)
	}
	if readings := quotaReadings(lines[0], ms); len(readings) != 2 || !strings.HasPrefix(readings[0], "70% · ") {
		t.Fatalf("readings %q", readings)
	}
	// A reset that moved further than the time between two readings
	// starts a cycle; a refill without resets does too.
	a := curvePoint{at: 0, left: 10, reset: 1000, start: none}
	if !quotaCycleBreak(a, curvePoint{at: 10, left: 90, reset: 1000 + 10 + 5*60e3 + 1, start: none}) ||
		!quotaCycleBreak(curvePoint{at: 0, left: 10, reset: none}, curvePoint{at: 10, left: 20, reset: none}) ||
		quotaCycleBreak(curvePoint{at: 0, left: 10, reset: none}, curvePoint{at: 10, left: 14, reset: none}) {
		t.Fatal("cycle breaks")
	}
}

func TestContextCellsShareTheWindowByLargestRemainder(t *testing.T) {
	p := prompt{Window: 1000, Tokens: 100, Parts: []promptPart{
		{Kind: "chat", Tokens: 60, Items: []promptItem{{Name: "1", Tag: "turn", Tokens: 60}}},
		{Kind: "system", Tokens: 40, Items: []promptItem{{Name: "prompt", Tokens: 40}}},
	}}
	cells := cellsOf(p, p.Window)
	if len(cells) != contextCells {
		t.Fatalf("cells %d", len(cells))
	}
	count := map[string]int{}
	for _, c := range cells {
		if c == nil {
			count["free"]++
		} else {
			count[c.kind]++
		}
	}
	// The system prompt comes first, as Magpie's parts are ordered.
	if count["system"] != 16 || count["chat"] != 24 || count["free"] != 360 || cells[0].kind != "system" {
		t.Fatalf("cells %v", count)
	}
	// A prompt past its window fills the grid; a tiny part keeps a cell.
	over := cellsOf(prompt{Tokens: 5000, Parts: []promptPart{{Kind: "tools", Tokens: 4999}, {Kind: "files", Tokens: 1}}}, 1000)
	n := map[string]int{}
	for _, c := range over {
		n[c.kind]++
	}
	if n["tools"] != 399 || n["files"] != 1 {
		t.Fatalf("overrun %v", n)
	}
}

func TestLevelsGradeByQuartiles(t *testing.T) {
	// Of 1…8 the quartiles are 3, 5 and 7, the top 8.
	lv := sessLevels([]float64{0, 1, 2, 3, 4, 5, 6, 7, 8})
	got := []int{lv(0), lv(1), lv(3), lv(4), lv(5), lv(6), lv(7), lv(8)}
	want := []int{0, 1, 1, 2, 2, 3, 3, 4}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("levels %v, want %v", got, want)
		}
	}
	if sessLevels(nil)(5) != 0 {
		t.Fatal("levels of nothing")
	}
}

func TestHeatmapTipsSayTheMetricFirst(t *testing.T) {
	i18n.SetLocale(i18n.Chinese)
	day := time.Date(2026, 10, 11, 0, 0, 0, 0, time.Local)
	d := heatDay{Date: "2026-10-11", Calls: 2, Tokens: 1250, Cost: 0.03}
	_, figures := heatTip(d, day, "calls")
	if figures != "2 次请求 · 1250 Token · ≈$0.030" {
		t.Fatalf("figures %q", figures)
	}
	if _, none := heatTip(heatDay{}, day, "tokens"); none != "没有请求" {
		t.Fatalf("empty %q", none)
	}
	if heatCost(heatDay{Unpriced: 1}) != "" || heatCost(heatDay{Cost: 120, Unpriced: 1}) != "$120+" {
		t.Fatal("heat cost")
	}
}
