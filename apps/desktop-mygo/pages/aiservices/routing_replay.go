package aiservices

import (
	"encoding/json"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
)

// The timeline follows Magpie's routing replay: retain event boundaries,
// compress each quiet interval to at most 1.8 s, and allow seeking backwards.
// It only projects recorded requests; playback never sends an AI request.
type routeReplay struct {
	routes       []traceRoute
	times, track []int64
	position     float64
	speed        int
	paused       bool
	last         time.Time
}

func routeMillis(s string) int64 {
	v, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return 0
	}
	return v.UnixMilli()
}

func newRouteReplay(rows []traceRoute) *routeReplay {
	r := &routeReplay{speed: 1, position: -500}
	for _, row := range rows {
		if !row.Done || routeMillis(row.Time) == 0 {
			continue
		}
		r.routes = append(r.routes, row)
		start := routeMillis(row.Time)
		end := start + row.Millis
		for _, tr := range row.Tries {
			s := routeMillis(tr.Start)
			r.times = append(r.times, s, s+tr.Millis)
			end = max(end, s+tr.Millis)
		}
		r.times = append(r.times, start, end)
	}
	if len(r.routes) == 0 {
		return nil
	}
	slices.SortFunc(r.routes, func(a, b traceRoute) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	slices.Sort(r.times)
	r.times = slices.Compact(r.times)
	r.track = make([]int64, len(r.times))
	for i := 1; i < len(r.times); i++ {
		r.track[i] = r.track[i-1] + min(r.times[i]-r.times[i-1], 1800)
	}
	return r
}
func (r *routeReplay) total() float64 { return float64(r.track[len(r.track)-1]) }
func (r *routeReplay) realAt() int64 {
	if r.position <= 0 {
		return r.times[0] + int64(r.position)
	}
	for i := 1; i < len(r.track); i++ {
		if r.position <= float64(r.track[i]) {
			dv := r.track[i] - r.track[i-1]
			if dv == 0 {
				return r.times[i]
			}
			return r.times[i-1] + int64((r.position-float64(r.track[i-1]))*float64(r.times[i]-r.times[i-1])/float64(dv))
		}
	}
	return r.times[len(r.times)-1]
}
func (r *routeReplay) frame() []traceRoute {
	now := r.realAt()
	var out []traceRoute
	for _, row := range r.routes {
		start := routeMillis(row.Time)
		if start > now {
			continue
		}
		end := start + row.Millis
		// Preserve unknown fields while replacing the recorded attempt states.
		var raw map[string]json.RawMessage
		if json.Unmarshal(row.Raw, &raw) != nil {
			continue
		}
		var tries []map[string]json.RawMessage
		json.Unmarshal(raw["tries"], &tries)
		var shown []map[string]json.RawMessage
		for i, tr := range row.Tries {
			s := routeMillis(tr.Start)
			e := s + tr.Millis
			end = max(end, e)
			if s > now || i >= len(tries) {
				continue
			}
			v := tries[i]
			if e > now {
				v["done"] = json.RawMessage(`false`)
				v["status"] = json.RawMessage(`0`)
				delete(v, "rest")
				delete(v, "again")
				delete(v, "error")
				delete(v, "fail")
			}
			shown = append(shown, v)
		}
		raw["tries"], _ = json.Marshal(shown)
		raw["done"], _ = json.Marshal(end <= now)
		if end > now {
			for _, key := range []string{"status", "error", "fail", "canceled"} {
				delete(raw, key)
			}
		}
		data, _ := json.Marshal(raw)
		var ghost traceRoute
		json.Unmarshal(data, &ghost)
		out = append(out, ghost)
	}
	return out
}

func routeEndpoint(row traceRoute) string {
	return "/api/gateway/route?" + url.Values{"id": {strconv.FormatInt(row.ID, 10)}, "day": {strings.Split(row.Time, "T")[0]}}.Encode()
}

func (t *nativeTrace) startReplay(p *page, rows []traceRoute) {
	t.stopReplay()
	t.selected, t.focused = nil, nil
	for _, row := range rows {
		if row.Done {
			t.replayRows = append(t.replayRows, row)
		}
	}
	t.replayRead = 0
	t.readReplay(p)
}

// History stores summaries. Fetch full evidence first so a replay never claims
// that a request had no attempts merely because its list row omitted them.
func (t *nativeTrace) readReplay(p *page) {
	if t.replayRead == len(t.replayRows) {
		t.replay = newRouteReplay(t.replayRows)
		t.replayRows = nil
		return
	}
	row := t.replayRows[t.replayRead]
	t.replayAPI.request(p, "GET", routeEndpoint(row), nil, func(data json.RawMessage) {
		var full traceRoute
		if !nativeDecode(&t.replayAPI, data, &full) {
			return
		}
		if full.ID != row.ID {
			t.replayAPI.err = nativeT("invalidResponse")
			return
		}
		t.replayRows[t.replayRead] = full
		t.replayRead++
		t.readReplay(p)
	})
}

func (t *nativeTrace) replayView(c *ui.Context, p *page) []traceRoute {
	t.replayAPI.notice(c, func() { t.readReplay(p) })
	if t.replayAPI.busy {
		kit.Status(c, i18n.T("aiRouting.replayLoading", "count", t.replayRead, "total", len(t.replayRows)))
		if kit.Button(c, i18n.T("common.cancel"), nil, false).Clicked() {
			t.replayAPI.close()
			t.replayRows = nil
		}
	}
	r := t.replay
	if r == nil {
		return t.routes
	}
	now := c.Now()
	if !r.last.IsZero() && !r.paused {
		r.position += min(float64(now.Sub(r.last).Milliseconds()), 250) * float64(r.speed)
	}
	r.last = now
	if r.position > r.total()+500 {
		t.stopReplay()
		return t.routes
	}
	kit.Group(c, nil, i18n.T("aiRouting.replaying"), time.UnixMilli(r.realAt()).Format("2006-01-02 15:04:05.000"), func() {
		ui.Column(c).FillWidth().Gap(12).Padding(12, 16).Children(func() {
			ui.Row(c).Gap(8).Wrap().Children(func() {
				label := i18n.T("aiRouting.pauseReplay")
				if r.paused {
					label = i18n.T("aiRouting.resumeReplay")
				}
				if kit.Button(c, label, nil, false).Clicked() {
					r.paused = !r.paused
				}
				if kit.Button(c, i18n.T("aiRouting.replaySpeed", "speed", r.speed), nil, false).Clicked() {
					r.speed *= 2
					if r.speed > 8 {
						r.speed = 1
					}
				}
				if kit.Button(c, i18n.T("aiRouting.stopReplay"), nil, false).Clicked() {
					t.stopReplay()
				}
			})
			position := max(0, min(r.total(), r.position))
			if ui.Slider(c, &position, 0, max(1, r.total())).FillWidth().Label(i18n.T("aiRouting.replayPosition")).Changed() {
				r.position = position
			}
		})
	})
	if t.replay == nil {
		return t.routes
	}
	if !r.paused {
		c.After(33 * time.Millisecond)
	}
	frame := r.frame()
	if t.selected != nil {
		id := t.selected.ID
		t.selected = nil
		for _, row := range frame {
			if row.ID == id {
				copy := row
				t.selected = &copy
				break
			}
		}
	}
	// A seek may precede the pinned request. Never leave its future state
	// visible in the detail dialog or the live overview.
	t.focused = t.selected
	return frame
}
