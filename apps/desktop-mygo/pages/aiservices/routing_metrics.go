package aiservices

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

type traceUsage struct {
	Input        int64   `json:"in"`
	Output       int64   `json:"out"`
	CacheRead    int64   `json:"cache_read"`
	CacheWrite   int64   `json:"cache_write"`
	CacheUnknown bool    `json:"cache_unknown"`
	Credits      float64 `json:"credits"`
	Unit         string  `json:"unit"`
}

func (t *nativeTrace) stopReplay() {
	t.replayAPI.close()
	t.replayAPI.err = ""
	t.replay, t.replayRows = nil, nil
	t.selected, t.focused = nil, nil
}

func (t *nativeTrace) filterRoutes(rows []traceRoute) []traceRoute {
	var out []traceRoute
	for _, r := range rows {
		if t.status != "" && traceStatus(r) != t.status {
			continue
		}
		if len(t.purposes) > 0 && !t.purposes[routePurpose(r.Kind)] {
			continue
		}
		match := true
		for _, f := range [][2]string{{r.Provider, t.provider}, {r.Model, t.model}, {r.Agent, t.client}, {r.CallerKeyName + " " + r.CallerKey, t.caller}, {string(r.Raw), t.query}} {
			if !strings.Contains(strings.ToLower(f[0]), strings.ToLower(f[1])) {
				match = false
				break
			}
		}
		if match {
			out = append(out, r)
		}
	}
	return out
}

func (t *nativeTrace) metricsView(c *ui.Context) {
	if t.metrics == nil {
		t.metrics = map[string]bool{}
		for _, key := range []string{"duration", "ttft", "tokens", "cache", "speed", "cost"} {
			t.metrics[key] = true
		}
	}
	ui.Collapsible(c, i18n.T("aiRouting.metrics"), &t.metricsOpen, func() {
		ui.Row(c).FillWidth().Gap(12).Wrap().Children(func() {
			for _, key := range []string{"duration", "ttft", "tokens", "cache", "speed", "cost"} {
				on := t.metrics[key]
				if ui.Checkbox(c.Key(key), &on, i18n.T("aiRouting.metric."+key)).Changed() {
					t.metrics[key] = on
					t.saveViewPrefs()
				}
			}
		})
	})
}

// Token tiers are disjoint and include billable retries. Unknown cache usage
// stays unknown; it is not displayed as a zero-percent hit rate.
func traceCache(r traceRoute) (prompt, read int64) {
	for _, u := range r.Usage {
		if !u.CacheUnknown {
			prompt += u.Input + u.CacheRead + u.CacheWrite
			read += u.CacheRead
		}
	}
	return
}

// Match the upstream speed calculation, including buffered output and hidden
// reasoning. A lower bound is marked explicitly rather than shown as measured.
func traceSpeed(r traceRoute) (speed float64, floor bool) {
	if !r.Done || traceStatus(r) != "success" {
		return 0, false
	}
	think := r.Reasoning
	visible := think > 0 && r.FirstText <= 0 && r.FirstReasoning >= r.TTFT && r.Flow > 0 && think <= r.Output
	n, from := r.Output, r.TTFT
	if visible {
		from = r.FirstReasoning
	} else if think > 0 {
		n -= think
		from = r.FirstText
	}
	w := r.Millis - from
	valid := !(r.ReasoningHidden && r.FirstText <= 0) && n > 0 && r.TTFT > 0 && from > 0 && w >= 100 && n*1000 <= 10000*w
	valid = valid && !(r.Flow > 0 && r.Flow*4 < w) && !(w < 1000 && from > 4*w) && !(think > 0 && r.FirstText > 0 && n*(r.FirstText-r.TTFT) > 20*think*w)
	if valid {
		return float64(n) * 1000 / float64(w), false
	}
	n, w = r.Output-think, r.Millis-r.TTFT
	if r.TTFT > 0 && think > 0 && r.FirstText <= 0 && n >= 100 && w >= 1000 && n*1000 <= 10000*w && !(r.Flow > 0 && r.Flow*4 < w) {
		return float64(n) * 1000 / float64(w), true
	}
	if r.Output >= 1000 && r.Millis >= 1000 && r.Output*1000 <= 10000*r.Millis {
		return float64(r.Output) * 1000 / float64(r.Millis), true
	}
	return 0, false
}

func (t *nativeTrace) metricText(r traceRoute) string {
	var parts []string
	add := func(key, value string) {
		if t.metrics == nil || t.metrics[key] {
			parts = append(parts, i18n.T("aiRouting.metric."+key)+" "+value)
		}
	}
	if r.Done && r.Millis > 0 {
		add("duration", fmt.Sprintf("%.2f s", float64(r.Millis)/1000))
	}
	if r.Done && r.TTFT > 0 {
		add("ttft", fmt.Sprintf("%.2f s", float64(r.TTFT)/1000))
	}
	if r.Tokens > 0 {
		add("tokens", fmt.Sprint(r.Tokens))
	}
	if prompt, read := traceCache(r); r.Done && prompt > 0 {
		add("cache", fmt.Sprintf("%.1f%%", 100*float64(read)/float64(prompt)))
	}
	if v, floor := traceSpeed(r); v > 0 {
		prefix := ""
		if floor {
			prefix = "≥"
		}
		add("speed", fmt.Sprintf("%s%.0f tok/s", prefix, v))
	}
	if r.Priced {
		cost := fmt.Sprintf("$%.6f", r.Cost)
		if r.Unpriced > 0 {
			cost += " · " + i18n.T("aiRouting.partialPrice")
		}
		add("cost", cost)
	}
	for _, u := range r.Usage {
		if u.Credits > 0 {
			add("cost", fmt.Sprintf("%g %s", u.Credits, u.Unit))
		}
	}
	if !r.Done {
		parts = append(parts, i18n.T("aiRouting.received", "bytes", r.Bytes))
		if r.Phase != "" {
			parts = append(parts, r.Phase)
		}
	}
	return strings.Join(parts, " · ")
}

func traceTotals(rows []traceRoute) string {
	var running, failed, retried int
	var tokens int64
	var cost float64
	for _, r := range rows {
		if !r.Done {
			running++
		}
		if traceStatus(r) == "failed" {
			failed++
		}
		if len(r.Tries) > 1 {
			retried++
		}
		tokens += r.Tokens
		cost += r.Cost
	}
	return i18n.T("aiRouting.totals", "count", len(rows), "running", running, "retried", retried, "failed", failed, "tokens", tokens, "cost", fmt.Sprintf("%.6f", cost))
}

// The upstream stage is client → WinOne → candidate accounts. Every candidate
// and attempt comes from the trace snapshot; it never predicts a route.
func (t *nativeTrace) overviewView(c *ui.Context, p *page, rows []traceRoute) {
	mask := p.routing.accountMasker()
	var active []traceRoute
	for _, r := range rows {
		if !r.Done {
			active = append(active, r)
		}
	}
	if len(active) == 0 && len(rows) > 0 {
		active = rows[len(rows)-1:]
	}
	if t.focused != nil && t.replay == nil {
		active = []traceRoute{*t.focused}
	}
	ui.Row(c).FillWidth().Gap(10).Wrap().AlignItems(ui.Center).Children(func() {
		kit.Help(c, traceTotals(rows))
		ui.Spacer(c).Grow(1)
		if kit.GhostButton(c, i18n.T("aiRouting.latestFailure"), nil, false).Clicked() {
			for i := len(rows) - 1; i >= 0; i-- {
				if traceStatus(rows[i]) == "failed" {
					t.selectRoute(p, rows[i])
					break
				}
			}
		}
		if t.focused != nil && kit.GhostButton(c, i18n.T("aiRouting.followLatest"), nil, false).Clicked() {
			t.focused = nil
		}
	})
	width, _ := c.Size()
	nodeWidth, gap := float32(124), float32(22)
	if width < 760 {
		nodeWidth, gap = 86, 8
	}
	ui.Row(c).FillWidth().Gap(gap).Padding(22, 2).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Width(nodeWidth).Shrink(0).Gap(10).Children(func() {
			if len(active) == 0 {
				kit.Help(c, upstreamT("No request yet"))
			}
			for _, r := range active {
				ui.Column(c.Key(r.ID)).FillWidth().Gap(3).Padding(12, 8).Radius(12).Border(1, c.Theme().Border).Background(kit.Colors(c).Panel).Children(func() {
					ui.Icon(c, icons.Terminal).Size(26, 26).AlignSelf(ui.Center)
					if kit.GhostButton(c, mask(r.Agent), nil, false).Clicked() {
						t.selectRoute(p, r)
					}
					kit.Help(c, p.routing.memberLabel(r.Model)).MinWidth(0)
				})
			}
		})
		ui.Text(c, "→").TextColor(c.Theme().TextMuted)
		ui.Column(c).Width(nodeWidth).Shrink(0).Gap(4).Padding(12, 8).Radius(12).Border(1, c.Theme().Border).Background(kit.Colors(c).Panel).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icons.Sparkles).Size(26, 26)
			kit.Strong(c, "WinOne")
			kit.Help(c, upstreamT("Routing"))
		})
		ui.Text(c, "→").TextColor(c.Theme().TextMuted)
		ui.Column(c).Grow(1).MinWidth(0).Gap(7).Children(func() {
			for _, r := range active {
				var evidence traceEvidence
				_ = json.Unmarshal(r.Raw, &evidence)
				candidates := append(evidence.Order, evidence.Left...)
				if len(candidates) == 0 {
					for _, attempt := range r.Tries {
						candidates = append(candidates, traceCandidate{ID: attempt.ID, Model: attempt.Model})
					}
				}
				if len(candidates) == 0 {
					kit.Help(c, nativeT(traceStatus(r)))
				}
				for _, candidate := range candidates {
					label := strings.TrimSpace(candidate.Name + " · " + candidate.Who)
					if label == "·" || label == "" {
						label = p.routing.memberLabel(candidate.Model)
					}
					state := upstreamT("Waiting for a request")
					for _, attempt := range r.Tries {
						if attempt.ID == candidate.ID {
							state = nativeT("running")
							if attempt.Done {
								state = fmt.Sprintf("HTTP %d", attempt.Status)
							}
							if attempt.Error != "" {
								state = mask(attempt.Error)
							}
						}
					}
					row := ui.ButtonBase(c.Key(fmt.Sprint(r.ID)+candidate.ID)).FillWidth().Padding(9, 12).Radius(10).Border(1, c.Theme().Border).Background(kit.Colors(c).Panel).Label(mask(label))
					row.Children(func() {
						ui.Column(c).FillWidth().MinWidth(0).Gap(4).Children(func() {
							kit.Strong(c, mask(label))
							kit.Help(c, p.routing.memberLabel(candidate.Model))
							kit.Help(c, state)
						})
					})
					if row.Clicked() {
						t.selectRoute(p, r)
					}
				}
			}
		})
	})
}
