package usage

import (
	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/kit"
)

// rankList builds a ranking's rows (Magpie's drawLedRank, planned by
// rankRows): each group's color, name and figure, a bar against the
// leader's, and its other figures below, its failures in red. A row picks
// the group's requests; picked is the one the filter picked. spacious
// rows are the overview breakdowns'. It returns the group under the
// pointer, which a chart beside it brings forward.
func rankList(c *ui.Context, rows []rankRow, picked string, spacious bool, choose func(id string)) string {
	t := c.Theme()
	hovered := ""
	ui.Column(c).FillWidth().Gap(1).Role(ui.RoleList).Children(func() {
		for _, r := range rows {
			on := picked != "" && picked == r.ID
			var row ui.Element
			if r.plain {
				row = ui.Column(c.Key(r.ID)).Role(ui.RoleListItem)
			} else {
				row = ui.ButtonBase(c.Key(r.ID)).Column().AlignItems(ui.Stretch).Checked(on)
			}
			row.FillWidth().Radius(8).Gap(0).Tooltip(r.title)
			if spacious {
				row.Padding(9, 4)
			} else {
				row.Padding(6, 8, 7, 8)
			}
			track := t.SurfaceHover
			switch {
			case on:
				row.Background(t.SurfacePressed)
				track = t.Background
			case row.Hovered() && !r.plain:
				row.Background(t.Surface)
			}
			if row.Hovered() {
				hovered = r.ID
			}
			row.Children(func() {
				ui.Row(c).FillWidth().Gap(7).AlignItems(ui.Center).Children(func() {
					ui.Box(c).Size(8, 8).Radius(2).Shrink(0).Background(colorOf(t, r.tone))
					ui.Text(c, r.Name).FontWeight(500).Grow(1).Shrink(1).MinWidth(0)
					ui.Text(c, r.value).FontWeight(600).NoWrap().FontFeatures("tnum").Shrink(0).Margin(0, 0, 0, 8)
				})
				bar := ui.Box(c).Height(4).Radius(2).Background(track).Clip().Margin(5, 0, 4, 15)
				if spacious {
					bar.Height(5).Margin(7, 0, 5, 15)
				}
				bar.Children(func() {
					ui.Box(c).WidthPercent(float32(r.bar)).FillHeight().Radius(2).Background(colorOf(t, r.tone))
				})
				spans := []ui.Span{{Text: r.detail}}
				for _, bad := range []string{r.errors, r.loops} {
					if bad != "" {
						if spans[0].Text != "" {
							spans = append(spans, ui.Span{Text: " · "})
						}
						spans = append(spans, ui.Span{Text: bad, Color: t.Danger})
					}
				}
				ui.RichText(c, spans...).FontSize(kit.HelpSize(c)).LineHeight(1.5).TextColor(t.TextMuted).FontFeatures("tnum").
					Margin(0, 0, 0, 15).MinWidth(0)
			})
			if !r.plain && row.Clicked() {
				choose(r.ID)
			}
		}
	})
	return hovered
}

// tile is one figure of a row of totals.
type tile struct {
	label, value, detail, title string
}

// tiles are the totals between two hairlines, each its name, its figure
// and a line of detail, as many to a row as fit.
func tiles(c *ui.Context, items []tile) {
	t, p := c.Theme(), kit.Colors(c)
	ui.Row(c).FillWidth().GapX(20).GapY(22).Wrap().Padding(18, 0).BorderWidth(1, 0, 1, 0).BorderColor(p.Hairline).
		AlignItems(ui.Start).Children(func() {
		for _, it := range items {
			box := ui.Column(c.Key(it.label)).Basis(125).Grow(1).Shrink(1).MinWidth(0).Gap(6)
			if it.title != "" {
				box.Tooltip(it.title)
			}
			box.Children(func() {
				kit.Help(c, it.label)
				ui.Text(c, it.value).FontSize(t.Rem(20.0 / 14)).FontWeight(500).LineHeight(1.2).FontFeatures("tnum").Selectable()
				if it.detail != "" {
					kit.Help(c, it.detail)
				}
			})
		}
	})
}

// usageTotals are React's UsageTotals: requests, tokens, cost, credits,
// speed, first-token wait and cache hit rate. They use the ledger's own
// arithmetic: vendors that count no cache do not dilute the hit rate, and
// measured speeds and lower bounds stay apart.
func usageTotals(c *ui.Context, sum totals) {
	items := []tile{
		{label: tr("usage.calls"), value: num(sum.Calls), detail: tr("usage.errors") + " · " + num(sum.Errors)},
		{label: tr("usage.input"), value: num(sum.Input), detail: tr("usage.cache") + " · " + num(sum.CacheRead+sum.CacheWrite)},
		{label: tr("usage.output"), value: num(sum.Output)},
		{label: tr("usage.cost"), value: usd(sum.Cost, 6)},
	}
	if sum.Credited != 0 {
		items = append(items, tile{label: tr("usage.credits"), value: num(sum.Credits)})
	}
	speedTile := tile{label: tr("usage.speed"), value: "—"}
	if g, ok := ledGroupSpeed(sum); ok {
		speedTile.value, speedTile.detail, speedTile.title = g.main, g.other, g.title
	}
	items = append(items, speedTile)
	ttft := "—"
	if sum.Timed != 0 {
		ttft = toFixed(sum.TTFT/sum.Timed/1000, 2) + " s"
	}
	items = append(items, tile{label: tr("usage.ttft"), value: ttft})
	hit := "—"
	if rate, ok := cacheHitOf(sum); ok {
		hit = toFixed(rate*100, 1) + "%"
	}
	items = append(items, tile{label: tr("usage.hitRate"), value: hit, detail: cacheLeftOut(sum)})
	ui.Column(c).FillWidth().Gap(8).Children(func() {
		tiles(c, items)
		if sum.Unpriced > 0 {
			kit.Help(c, tr("usage.unpriced", "count", sum.Unpriced))
		}
	})
}
