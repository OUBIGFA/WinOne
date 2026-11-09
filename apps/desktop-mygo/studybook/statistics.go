package studybook

import (
	"github.com/egoist/mygo/ui"
	"strconv"
	"winone/desktopmygo/core"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

type studyStatistics struct {
	Summary map[string]int64
	Days    []struct {
		Day   string `json:"day"`
		Count int64  `json:"count"`
	}
}

func (b *Book) loadStatistics() {
	if b.busy {
		return
	}
	b.busy = true
	b.err = ""
	core.Go(b.env, func() (studyStatistics, error) {
		var value studyStatistics
		err := b.request("statistics.getSummary", map[string]any{}, &value.Summary)
		if err == nil {
			err = b.request("statistics.getReviewDays", map[string]any{}, &value.Days)
		}
		return value, err
	}, func(value studyStatistics, err error) {
		if b.closed {
			return
		}
		b.busy = false
		if err != nil {
			b.err = err.Error()
			return
		}
		b.statistics = &value
	})
}
func (b *Book) statisticsView(c *ui.Context) {
	if kit.Button(c, text("refresh"), icons.RefreshCw, false).Clicked() {
		b.loadStatistics()
	}
	if b.statistics == nil {
		return
	}
	for _, key := range []string{"cardCount", "noteCount", "reviewCount", "newCount", "learningCount", "reviewCardCount", "suspendedCount"} {
		value := b.statistics.Summary[key]
		kit.Row(c, text(key), "", func() { ui.Text(c, strconv.FormatInt(value, 10)).Selectable() })
	}
	kit.Heading(c, text("reviewDays"))
	maximum := int64(1)
	for _, day := range b.statistics.Days {
		maximum = max(maximum, day.Count)
	}
	for _, day := range b.statistics.Days {
		day := day
		ui.Row(c.Key(day.Day)).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Text(c, day.Day).Width(110)
			ui.Box(c).Grow(1).MinWidth(0).Height(10).Background(c.Theme().Border).Children(func() {
				ui.Box(c).WidthPercent(100 * float32(day.Count) / float32(maximum)).Height(10).Background(c.Theme().TextMuted)
			})
			ui.Text(c, strconv.FormatInt(day.Count, 10)).Width(50)
		})
	}
	if len(b.statistics.Days) == 0 {
		kit.Help(c, text("noReviews"))
	}
	if kit.Button(c, text("allStatistics"), icons.ChartColumn, false).Clicked() {
		b.openTool("graphs", map[string]any{"search": "", "days": float64(0)})
	}
}
