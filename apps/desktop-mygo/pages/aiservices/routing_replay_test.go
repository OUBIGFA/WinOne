package aiservices

import (
	"encoding/json"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestRoutingReplayRetainsAttemptBoundariesAndCanSeekBack(t *testing.T) {
	var a, b traceRoute
	json.Unmarshal([]byte(`{"id":1,"time":"2026-11-07T00:00:00Z","done":true,"ms":3000,"tries":[{"id":"one","start":"2026-11-07T00:00:00Z","ms":1000,"done":true,"status":429,"rest":{"why":"quota"}},{"id":"two","start":"2026-11-07T00:00:02Z","ms":1000,"done":true,"status":200}]}`), &a)
	json.Unmarshal([]byte(`{"id":2,"time":"2026-11-07T01:00:00Z","done":true,"ms":1000,"tries":[]}`), &b)
	r := newRouteReplay([]traceRoute{b, a})
	if r.total() != 5800 {
		t.Fatalf("quiet gap not compressed: %v", r.total())
	}
	r.position = 500
	frame := r.frame()
	if len(frame) != 1 || frame[0].Done || len(frame[0].Tries) != 1 || frame[0].Tries[0].Done || frame[0].Tries[0].Status != 0 || frame[0].Tries[0].Rest != nil {
		t.Fatalf("future evidence leaked: %+v", frame)
	}
	r.position = 2500
	frame = r.frame()
	if len(frame[0].Tries) != 2 || !frame[0].Tries[0].Done || frame[0].Tries[1].Done {
		t.Fatalf("attempt transition lost: %+v", frame[0].Tries)
	}
	r.position = r.total()
	if frame = r.frame(); len(frame) != 2 || !frame[0].Done || !frame[1].Done {
		t.Fatal("replay did not finish")
	}
	r.position = -500
	if len(r.frame()) != 0 {
		t.Fatal("seek backwards retained future requests")
	}
	if a.Tries[0].Status != 429 {
		t.Fatal("replay changed the original record")
	}
}

func TestReplaySeekingBeforeSelectionClearsFutureDetails(t *testing.T) {
	_, h, p, _ := setup(t)
	var row traceRoute
	json.Unmarshal([]byte(`{"id":1,"time":"2026-11-07T00:00:00Z","done":true,"ms":3000}`), &row)
	tr := &p.routing.trace
	tr.replay = newRouteReplay([]traceRoute{row})
	tr.replay.paused = true
	tr.selected, tr.focused = &row, &row
	tt := ui.NewTester(func(c *ui.Context) { tr.replayView(c, p) }, 1000, 800)
	settle(h, tt)
	if tr.selected != nil || tr.focused != nil {
		t.Fatal("seek before the request retained future details")
	}
}

func TestRoutingMetricsKeepCacheTiersAndReasoningSeparate(t *testing.T) {
	r := traceRoute{Done: true, Status: 200, Output: 1500, Reasoning: 1000, Millis: 12000, TTFT: 2000, FirstText: 7000, Flow: 10000, Usage: []traceUsage{{Input: 100, CacheRead: 600, CacheWrite: 300}, {Input: 999, CacheUnknown: true}}}
	if prompt, read := traceCache(r); prompt != 1000 || read != 600 {
		t.Fatalf("cache tiers: %d/%d", read, prompt)
	}
	if speed, floor := traceSpeed(r); speed != 100 || floor {
		t.Fatalf("reasoning counted as answer speed: %v floor=%v", speed, floor)
	}
	r.Flow = 1
	if _, floor := traceSpeed(r); !floor {
		t.Fatal("buffered reply shown as measured speed")
	}
	r.Canceled = true
	if speed, _ := traceSpeed(r); speed != 0 {
		t.Fatal("canceled request shown as an answered speed")
	}
}
