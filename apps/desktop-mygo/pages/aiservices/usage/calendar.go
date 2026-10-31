package usage

// The activity calendars' grading and layout, ported from yetone/magpie
// at ede1e32aec1a4f6bf06aeef6ed2e03d4218db226, internal/gui/assets/app.js
// (sessLevels, sessLegend, sessWeekdays, sessCalendar), as
// apps/desktop/frontend/src/features/magpie-session-calendar.ts carried
// them. MIT License, Copyright (c) 2026 yetone; the notice is
// third_party/licenses/upstream-magpie-ede1e32aec1a4f6bf06aeef6ed2e03d4218db226-0-LICENSE.txt.
// The quartiles, the Monday-first weeks and the month names' placing are
// upstream's; the cells are MyGo elements in a grid, as they were CSS grid
// items.

import (
	"math"
	"sort"
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/kit"
)

// sessLevels grades values 0 (none) to 4 by the quartiles of those above
// 0, as a calendar of contributions does.
func sessLevels(values []float64) func(float64) int {
	var nz []float64
	for _, v := range values {
		if v > 0 {
			nz = append(nz, v)
		}
	}
	if len(nz) == 0 {
		return func(float64) int { return 0 }
	}
	sort.Float64s(nz)
	q := func(p float64) float64 { return nz[min(len(nz)-1, int(math.Floor(p*float64(len(nz)))))] }
	a, b, c, top := q(0.25), q(0.5), q(0.75), nz[len(nz)-1]
	return func(v float64) int {
		switch {
		case v <= 0:
			return 0
		case v >= top:
			return 4
		case v <= a:
			return 1
		case v <= b:
			return 2
		case v <= c:
			return 3
		}
		return 4
	}
}

// levelColor is a level's shade: none in the plain surface, then the
// accent mixed in by a quarter, a half, three quarters and in full.
func levelColor(t *ui.Theme, level int) ui.Color {
	none := t.SurfaceHover
	switch level {
	case 1:
		return none.Mix(t.Accent, 0.22)
	case 2:
		return none.Mix(t.Accent, 0.45)
	case 3:
		return none.Mix(t.Accent, 0.70)
	case 4:
		return t.Accent
	}
	return none
}

// sessLegend is "Less", the five shades and "More".
func sessLegend(c *ui.Context, less, more string) {
	t := c.Theme()
	ui.Row(c).Gap(3).AlignItems(ui.Center).Role(ui.RoleNone).Children(func() {
		kit.Caption(c, less).Margin(0, 2, 0, 0)
		for level := range 5 {
			ui.Box(c.Key(level)).Size(10, 10).Radius(2.5).Background(levelColor(t, level))
		}
		kit.Caption(c, more).Margin(0, 0, 0, 2)
	})
}

// sessWeekdays are the short names of the days of the week, Monday first.
func sessWeekdays() [7]string {
	var names [7]string
	for i := range names {
		names[i] = formatDate(time.Date(2024, time.January, 1+i, 12, 0, 0, 0, time.Local), "EEE")
	}
	return names
}

// calDay is one day of a calendar.
type calDay struct {
	day   time.Time
	value float64
}

// sessCalendar is the range as weeks of days, Monday on top, each day as
// dark as it is busy among the others (Magpie's sessCalendar, without
// leading padding). tip says a day's figure, as the cell's title did.
func sessCalendar(c *ui.Context, days []calDay, tip func(calDay) string) {
	if len(days) == 0 {
		return
	}
	t := c.Theme()
	values := make([]float64, len(days))
	for i, d := range days {
		values[i] = d.value
	}
	lv := sessLevels(values)
	lead := (int(days[0].day.Weekday()) + 6) % 7
	weeks := (lead + len(days) + 6) / 7
	size := kit.CaptionSize(c)
	tracks := []ui.Track{ui.FitContent()}
	for range weeks {
		tracks = append(tracks, ui.Fr(1))
	}
	// At most 20 DIPs a week, as upstream's SESS_WEEK, past the names.
	ui.Grid(c).FillWidth().MaxWidth(2.6*size + float32(weeks)*20).ColumnTracks(tracks...).Gap(3).FontSize(size).
		TextColor(t.TextMuted).Role(ui.RoleGroup).Children(func() {
		names := sessWeekdays()
		for _, i := range []int{0, 2, 4} {
			ui.Text(c.Key("wd"+strconv.Itoa(i)), names[i]).NoWrap().FixedLineHeight(size).AlignSelf(ui.Center).Padding(0, 5, 0, 0).
				ColumnStart(1).RowStart(i + 2)
		}
		month, labelAt := -1, -9
		for i, d := range days {
			k := lead + i
			w, wd := k/7, k%7
			if (wd == 0 || i == 0) && int(d.day.Month()) != month {
				month = int(d.day.Month())
				// A month mostly gone when the weeks before begin is left
				// unnamed, so as not to crowd out the next one's name.
				if w-labelAt >= 3 {
					skeleton := "MMM"
					if d.day.Month() == time.January && weeks > 20 {
						skeleton = "yMMM"
					}
					ui.Text(c.Key("mo"+strconv.Itoa(i)), formatDate(d.day, skeleton)).NoWrap().ColumnStart(w+2).RowStart(1).
						Padding(0, 0, 1, 0)
					labelAt = w
				}
			}
			cell := ui.Box(c.Key(i)).AspectRatio(1).Radius(3).MinWidth(0).Background(levelColor(t, lv(d.value))).
				ColumnStart(w + 2).RowStart(wd + 2).Tooltip(tip(d)).Label(tip(d))
			if cell.Hovered() {
				cell.Border(1, t.Text)
			}
		}
	})
}
