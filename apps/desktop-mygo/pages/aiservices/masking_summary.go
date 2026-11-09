package aiservices

import (
	"encoding/json"
	"github.com/egoist/mygo/ui"
	"strings"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

type maskingSummary struct {
	Requests, N, Distinct, Warned, Blocked int
	Days                                   []struct {
		Day                   string
		Requests, N, Distinct int
	}
	Kinds, Agents, Providers []struct {
		Name        string
		N, Distinct int
	}
	Values []struct {
		Placeholder          string `json:"p"`
		Kind, By, Head, Last string
		Len, N, Requests     int
	}
}

// Match the upstream page order: aggregate card, interactive explanation,
// request/value list and the alert preference below it. All figures are read.
func (m *maskingRecords) upstreamSummary(c *ui.Context, locked bool) {
	var sum maskingSummary
	_ = json.Unmarshal(m.summary, &sum)
	ui.Column(c).FillWidth().Gap(12).Padding(18).Border(1, c.Theme().Border).Radius(14).Background(kit.Colors(c).Panel).Children(func() {
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			kit.Heading(c, upstreamT("Masking"))
			ui.Spacer(c).Grow(1)
			days := kit.Itoa(m.days)
			if days != "30" {
				days = "7"
			}
			if kit.Segmented(c, &days, options("7", upstreamT("7 days"), "30", upstreamT("30 days")), nativeT("summaryDays")) {
				m.days = 7
				if days == "30" {
					m.days = 30
				}
				m.loadSummary()
			}
		})
		ui.Row(c).FillWidth().Wrap().Gap(28).Children(func() {
			for _, value := range []struct {
				label string
				value int
			}{{nativeT("maskedValues"), sum.N}, {nativeT("distinctValues"), sum.Distinct}, {nativeT("requests"), sum.Requests}, {nativeT("blocked"), sum.Blocked}} {
				ui.Column(c).Basis(130).Grow(1).Gap(4).Children(func() { ui.Text(c, kit.Itoa(value.value)).FontSize(28).FontWeight(500); kit.Help(c, value.label) })
			}
		})
		if len(sum.Days) > 0 {
			maximum := 1
			for _, day := range sum.Days {
				maximum = max(maximum, day.N)
			}
			ui.Row(c).FillWidth().Height(64).Gap(4).AlignItems(ui.End).Children(func() {
				for _, day := range sum.Days {
					h := float32(day.N) * 56 / float32(maximum)
					ui.Box(c.Key(day.Day)).Grow(1).Height(max(float32(2), h)).Radius(3).Background(c.Theme().TextMuted).Tooltip(day.Day + " · " + kit.Itoa(day.N))
				}
			})
		}
		ui.Row(c).FillWidth().Wrap().Gap(8).Children(func() {
			for _, kind := range sum.Kinds {
				if kit.GhostButton(c, kind.Name+" · "+kit.Itoa(kind.N), nil, false).Clicked() {
					m.kind = kind.Name
					m.load(false)
				}
			}
		})
	})
	open := ui.Local(ui.Column(c), "masking-explanation", func() bool { return false })
	ui.Collapsible(c, upstreamT("How masking works"), open, func() {
		ui.Row(c).FillWidth().Gap(8).Wrap().Children(func() {
			kit.Help(c, upstreamT("Original"))
			ui.Icon(c, icons.ArrowRight).Size(14, 14)
			kit.Help(c, upstreamT("Masking"))
			ui.Icon(c, icons.ArrowRight).Size(14, 14)
			kit.Help(c, upstreamT("Provider"))
			ui.Icon(c, icons.ArrowRight).Size(14, 14)
			kit.Help(c, upstreamT("Restored"))
		})
		m.previewView(c, locked)
	})
}

func (m *maskingRecords) valuesList(c *ui.Context) {
	var sum maskingSummary
	if json.Unmarshal(m.summary, &sum) != nil {
		return
	}
	for _, value := range sum.Values {
		if m.kind != "" && value.Kind != m.kind || m.by != "" && value.By != m.by {
			continue
		}
		ui.Column(c.Key(value.Placeholder)).FillWidth().Gap(4).Padding(12, 16).BorderWidth(0, 0, 1, 0).BorderColor(c.Theme().Border).Children(func() {
			kit.Strong(c, value.Kind+" · "+strings.TrimSpace(value.Head))
			kit.Mono(c, value.Placeholder).Selectable()
			kit.Help(c, nativeT("requests")+" "+kit.Itoa(value.Requests)+" · ×"+kit.Itoa(value.N)+" · "+value.Last)
			if kit.GhostButton(c, upstreamT("By request"), nil, false).Clicked() {
				m.mode = ""
				m.kind = value.Kind
				m.load(false)
			}
		})
	}
}
