package usage

// The request heatmap, ported from yetone/magpie at
// ede1e32aec1a4f6bf06aeef6ed2e03d4218db226, internal/gui/assets/app.js
// (ledHeatDate, ledHeatTip, drawLedHeat, ledHeatShow), as
// apps/desktop/frontend/src/features/magpie-usage-heatmap.ts carried it.
// MIT License, Copyright (c) 2026 yetone; the notice is
// third_party/licenses/upstream-magpie-ede1e32aec1a4f6bf06aeef6ed2e03d4218db226-0-LICENSE.txt.
//
// The 53 weeks from a Monday, the metric switch, the quartile shades, the
// month names' placing and the tip above a day are upstream's. The grid
// shrinks with the window instead of scrolling sideways, so the newest
// week, which upstream kept in sight by scrolling, always shows.

import (
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/kit"
)

func ht(key string, values ...any) string { return magpieText(heatChinese, key, values...) }

// heatView keeps where the day under the pointer is in the heatmap, from
// the frame that showed it, to put its tip above it.
type heatView struct {
	cell ui.Rect
}

// heatCost is the tip's cost, as upstream's fmtCost wrote it: a dollar
// sign, fewer decimals for more, a "+" when some calls had no price, and
// nothing for a day of no priced calls.
func heatCost(d heatDay) string {
	if d.Cost == 0 && d.Unpriced != 0 {
		return ""
	}
	digits := 3
	switch {
	case d.Cost >= 100:
		digits = 0
	case d.Cost >= 1:
		digits = 2
	}
	s := "$" + toFixed(d.Cost, digits)
	if d.Unpriced != 0 {
		s += "+"
	}
	return s
}

// heatTip is what a day's tip says: its date over its figures, the metric
// shown first.
func heatTip(d heatDay, day time.Time, metric string) (string, string) {
	date := formatDate(day, "yMMMdEEE")
	if d.Calls == 0 {
		return date, ht("No requests")
	}
	key := "{n} requests"
	if d.Calls == 1 {
		key = "{n} request"
	}
	parts := []string{ht("{n} tokens", "n", fmtN(d.Tokens)), ht(key, "n", num(d.Calls))}
	if cost := heatCost(d); cost != "" {
		parts = append(parts, "≈"+cost)
	}
	if order := map[string]int{"calls": 1, "cost": 2}[metric]; order > 0 && order < len(parts) {
		first := parts[order]
		parts = append([]string{first}, append(parts[:order:order], parts[order+1:]...)...)
	}
	return date, join(parts, " · ")
}

// heatmapView is React's UsageHeatmap; it shows nothing without a request
// in its 53 weeks.
func (u *Usage) heatmapView(c *ui.Context, h *heatmap) {
	if h == nil || h.From == "" || h.To == "" {
		return
	}
	at := map[string]heatDay{}
	busy := false
	for _, d := range h.Days {
		at[d.Date] = d
		busy = busy || d.Calls > 0
	}
	first, ok1 := localDay(h.From)
	last, ok2 := localDay(h.To)
	if !busy || !ok1 || !ok2 {
		return
	}
	type day struct {
		heatDay
		day time.Time
	}
	var days []day
	for d := first; !d.After(last); d = d.AddDate(0, 0, 1) {
		days = append(days, day{heatDay: at[isoDay(d)], day: d})
	}
	metric := u.prefs.Heatmap
	value := func(d heatDay) float64 {
		switch metric {
		case "cost":
			return d.Cost
		case "calls":
			return d.Calls
		}
		return d.Tokens
	}
	values := make([]float64, len(days))
	for i, d := range days {
		values[i] = value(d.heatDay)
	}
	lv := sessLevels(values)
	t, p := c.Theme(), kit.Colors(c)
	hv := &u.heat
	box := ui.Column(c.Key("heatmap")).FillWidth().Gap(14).Padding(14, 0).BorderWidth(1, 0, 1, 0).BorderColor(p.Hairline).
		Role(ui.RoleGroup).Label(tr("usage.heatmap"))
	origin := box.Bounds()
	hovered := -1
	box.Children(func() {
		ui.Row(c).FillWidth().GapX(12).GapY(10).Wrap().AlignItems(ui.Center).Justify(ui.SpaceBetween).Children(func() {
			ui.Column(c).Gap(2).MinWidth(0).Children(func() {
				kit.Strong(c, tr("usage.heatmap")).Role(ui.RoleHeading).Level(3)
				kit.Help(c, tr("usage.heatmapPeriod"))
			})
			options := []kit.SelectOption{{Value: "tokens", Label: ht("Tokens")}, {Value: "calls", Label: ht("Requests")}, {Value: "cost", Label: ht("Cost")}}
			if kit.Segmented(c, &u.prefs.Heatmap, options, tr("usage.metric")) {
				u.prefs.save()
			}
		})
		weeks := (len(days) + 6) / 7
		size := kit.HelpSize(c)
		tracks := []ui.Track{ui.FitContent()}
		for range weeks {
			tracks = append(tracks, ui.Fr(1))
		}
		// At most 21 DIPs a week past the day names, as upstream's grid.
		grid := ui.Grid(c).FillWidth().MaxWidth(3*size + float32(weeks)*21).ColumnTracks(tracks...).Gap(3).FontSize(size).
			TextColor(t.TextMuted).Role(ui.RoleImage).Label(ht("Usage heatmap"))
		grid.Children(func() {
			names := sessWeekdays()
			// Every row has its place at the edge, named every other day.
			for i := range 7 {
				text := ""
				if i%2 == 0 {
					text = names[i]
				}
				ui.Text(c.Key("wd"+strconv.Itoa(i)), text).NoWrap().FixedLineHeight(size).AlignSelf(ui.Center).
					Padding(0, 6, 0, 0).ColumnStart(1).RowStart(i + 2)
			}
			type label struct {
				text string
				week int
			}
			var labels []label
			month := -1
			for i, d := range days {
				w, wd := i/7, i%7
				if wd == 0 && int(d.day.Month()) != month {
					month = int(d.day.Month())
					// A month mostly gone in the first week is left
					// unnamed, as is one begun in the last week, with no
					// room for its name.
					if (len(labels) == 0 || w-labels[len(labels)-1].week >= 3) && !(w == 0 && d.day.Day() > 14) && w < weeks-1 {
						labels = append(labels, label{formatDate(d.day, "MMM"), w})
					}
				}
			}
			// A month's name spans its weeks up to the next name, cut there.
			for i, l := range labels {
				until := weeks
				if i+1 < len(labels) {
					until = labels[i+1].week
				}
				ui.Text(c.Key("mo"+strconv.Itoa(i)), l.text).SingleLine().Ellipsis("").MinWidth(0).ColumnStart(l.week+2).
					ColumnSpan(until-l.week).RowStart(1).Padding(0, 0, 2, 0)
			}
			for i, d := range days {
				// Each day is named by its date, which its tip says too.
				cell := ui.Box(c.Key(i)).AspectRatio(1).Radius(3).MinWidth(0).Background(levelColor(t, lv(values[i]))).
					ColumnStart(i/7 + 2).RowStart(i%7 + 2).Label(formatDate(d.day, "yMMMd"))
				if cell.Hovered() {
					hovered = i
					cell.Border(1, t.Text)
					b := cell.Bounds()
					hv.cell = ui.Rect{X: b.X - origin.X, Y: b.Y - origin.Y, W: b.W, H: b.H}
				}
			}
		})
		ui.Row(c).FillWidth().Justify(ui.End).Children(func() { sessLegend(c, ht("Less"), ht("More")) })
		if hovered < 0 {
			return
		}
		date, figures := heatTip(days[hovered].heatDay, days[hovered].day, metric)
		cell := hv.cell
		tip := ui.Column(c.Key("tip")).Absolute().Padding(6, 9).Gap(2).Radius(8).Background(p.Panel).Border(1, t.Border).
			FontSize(kit.HelpSize(c)).PassThrough().Role(ui.RoleTooltip)
		shown := tip.Bounds()
		left := max(6, min(origin.W-shown.W-6, cell.X+cell.W/2-shown.W/2))
		top := cell.Y - shown.H - 6
		if top < 4 {
			top = cell.Y + cell.H + 6
		}
		tip.Left(left).Top(top).Children(func() {
			ui.Text(c, date).FontWeight(600).NoWrap()
			ui.Text(c, figures).TextColor(t.TextMuted).NoWrap().FontFeatures("tnum")
		})
	})
}
