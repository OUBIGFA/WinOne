package usage

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/kit"
)

// periodPicker is React's UsagePeriod: today, the last 7 or 30 days, all,
// or dates chosen in two fields, which change nothing until Apply; reversed
// dates are put in order, as Magpie's RangeOf reads an inclusive range.
type periodPicker struct {
	custom   bool
	from, to time.Time
	// seen is the period last shown, to follow a range set elsewhere.
	seen string
}

// sync follows the period: a range shows its dates; the first one starts
// with the last seven days, as React's fields did.
func (p *periodPicker) sync(value string, now time.Time) {
	if p.from.IsZero() {
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
		p.from, p.to = today.AddDate(0, 0, -6), today
		p.custom = strings.Contains(value, "..")
	}
	if value == p.seen {
		return
	}
	p.seen = value
	if first, last, ok := strings.Cut(value, ".."); ok {
		p.custom = true
		if day, ok := localDay(first); ok {
			p.from = day
		}
		if day, ok := localDay(last); ok {
			p.to = day
		}
	}
}

// view is the period's drop-down.
func (p *periodPicker) view(c *ui.Context, value string, change func(string)) {
	p.sync(value, c.Now())
	field(c, tr("usage.period"), func() {
		shown := value
		if p.custom {
			shown = "custom"
		}
		options := []kit.SelectOption{{Value: "today", Label: tr("usage.today")}, {Value: "7d", Label: tr("usage.week")},
			{Value: "30d", Label: tr("usage.month")}, {Value: "all", Label: tr("usage.all")}, {Value: "custom", Label: tr("usage.customPeriod")}}
		if choose(c, &shown, options) {
			p.custom = shown == "custom"
			if !p.custom {
				change(shown)
			}
		}
	})
}

// dates are the custom range's fields and Apply, below the controls.
func (p *periodPicker) dates(c *ui.Context, change func(string)) {
	if !p.custom {
		return
	}
	ui.Row(c).FillWidth().GapX(12).GapY(10).Wrap().AlignItems(ui.End).Children(func() {
		ui.Field(c, tr("usage.fromDate"), func() {
			ui.DateInput(c.Key("from"), &p.from).FillWidth().Label(tr("usage.fromDate"))
		}).Basis(160).Grow(1).Shrink(1).MinWidth(0)
		ui.Field(c, tr("usage.toDate"), func() {
			ui.DateInput(c.Key("to"), &p.to).FillWidth().Label(tr("usage.toDate"))
		}).Basis(160).Grow(1).Shrink(1).MinWidth(0)
		if kit.Button(c, tr("usage.applyDates"), nil, false).Disabled(p.from.IsZero() || p.to.IsZero()).Shrink(0).Clicked() {
			first, last := isoDay(p.from), isoDay(p.to)
			if last < first {
				first, last = last, first
			}
			change(first + ".." + last)
		}
	})
}
