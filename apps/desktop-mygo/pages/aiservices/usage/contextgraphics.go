package usage

// The context grid and sparkline, ported from yetone/magpie at
// ede1e32aec1a4f6bf06aeef6ed2e03d4218db226, internal/gui/assets/context.js
// (cellsOf, spark, fmtK), as
// apps/desktop/frontend/src/features/magpie-context-graphics.ts carried
// them. MIT License, Copyright (c) 2026 yetone; the notice is
// third_party/licenses/upstream-magpie-ede1e32aec1a4f6bf06aeef6ed2e03d4218db226-0-LICENSE.txt.
// The largest-remainder cells and the sparkline's scaling are upstream's;
// MyGo paints them.

import (
	"math"
	"sort"
	"strconv"
	"strings"
)

// contextParts are the kinds of a prompt, in their order.
var contextParts = []string{"system", "tools", "memory", "files", "results", "chat"}

// contextCells is how many cells the grid of a window has.
const contextCells = 400

// cell is one cell of the grid: the part it shows, -1 for free room.
type cell struct {
	kind string
	item int
}

// cellsOf are the cells of a prompt's grid, each the part and item it
// shows: the prompt's share of the window, its parts by largest
// remainder, so they add up and a part of any size has one at least. A
// window smaller than the prompt (unknown, or overrun) is the prompt.
func cellsOf(p prompt, window float64) []*cell {
	scale := math.Max(math.Max(window, p.Tokens), 1)
	used := int(math.Min(contextCells, round(p.Tokens/scale*contextCells)))
	// Each kind's first part, of those with tokens.
	var parts []promptPart
	for _, kind := range contextParts {
		for _, x := range p.Parts {
			if x.Kind == kind {
				if x.Tokens > 0 {
					parts = append(parts, x)
				}
				break
			}
		}
	}
	sum := 0.0
	for _, x := range parts {
		sum += x.Tokens
	}
	if sum == 0 {
		sum = 1
	}
	want := make([]float64, len(parts))
	got := make([]int, len(parts))
	left := used
	for i, x := range parts {
		want[i] = x.Tokens / sum * float64(used)
		got[i] = max(1, int(math.Floor(want[i])))
		left -= got[i]
	}
	order := make([]int, len(parts))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		return want[order[a]]-math.Floor(want[order[a]]) > want[order[b]]-math.Floor(want[order[b]])
	})
	for j := 0; left > 0 && len(order) > 0; j, left = (j+1)%len(order), left-1 {
		got[order[j]]++
	}
	for j := len(got) - 1; left < 0 && j >= 0; j-- {
		for left < 0 && got[j] > 1 {
			got[j]--
			left++
		}
	}
	var cells []*cell
	for pi, x := range parts {
		items := append([]promptItem(nil), x.Items...)
		sort.SliceStable(items, func(a, b int) bool { return items[a].Tokens > items[b].Tokens })
		isum := 0.0
		for _, it := range items {
			isum += it.Tokens
		}
		if isum == 0 {
			isum = 1
		}
		at, acc := 0, 0.0
		for c := 0; c < got[pi]; c++ {
			// The item whose share the cell's middle falls in.
			mid := (float64(c) + 0.5) / float64(got[pi]) * isum
			for at < len(items)-1 && acc+items[at].Tokens < mid {
				acc += items[at].Tokens
				at++
			}
			item := at
			if len(items) == 0 {
				item = -1
			}
			cells = append(cells, &cell{kind: x.Kind, item: item})
		}
	}
	for len(cells) < contextCells {
		cells = append(cells, nil)
	}
	return cells[:contextCells]
}

// fmtK writes a count of tokens short: 950, 1.2K, 12K, 3.4M.
func fmtK(n float64) string {
	n = math.Max(0, round(n))
	// toFixed, its decimals dropped when they are all zeros.
	short := func(x float64, d int) string {
		s := toFixed(x, d)
		if whole, decimals, ok := strings.Cut(s, "."); ok && strings.Trim(decimals, "0") == "" {
			return whole
		}
		return s
	}
	switch {
	case n >= 1e6:
		d := 2
		if n >= 1e7 {
			d = 1
		}
		return short(n/1e6, d) + "M"
	case n >= 1e3:
		d := 1
		if n >= 1e5 {
			d = 0
		}
		return short(n/1e3, d) + "K"
	}
	return strconv.FormatFloat(n, 'f', -1, 64)
}

// sparkGeometry places a session's prompts on the sparkline's 240×34 box,
// scaled to the session's peak so a session that never came near its
// window still shows how it grew; the window's line shows once near.
type sparkGeometry struct {
	top float64
	n   int
}

const (
	sparkW, sparkH, sparkPad = 240.0, 34.0, 3.0
)

func newSpark(points []ctxPoint, window float64) sparkGeometry {
	peak := 1.0
	for _, p := range points {
		peak = math.Max(peak, p.Tokens)
	}
	top := peak * 1.2
	if window != 0 && peak > window*0.6 {
		top = math.Max(window, peak)
	}
	return sparkGeometry{top: top, n: len(points)}
}

func (g sparkGeometry) x(i int) float64 {
	if g.n < 2 {
		return sparkW / 2
	}
	return sparkPad + float64(i)/float64(g.n-1)*(sparkW-sparkPad*2)
}

func (g sparkGeometry) y(v float64) float64 { return sparkH - sparkPad - v/g.top*(sparkH-sparkPad*2) }

// hit is a point's clickable stretch, in the box's units.
func (g sparkGeometry) hit(i int) (float64, float64) {
	w := sparkW / float64(g.n)
	return g.x(i) - math.Max(2, w/2), math.Max(4, w)
}
