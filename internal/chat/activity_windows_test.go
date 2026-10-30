//go:build windows

package chat

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// Teammate follow-up reads this projection: each observable change (phase,
// pending approval, new turn, tool in flight) is reported once, and timing
// survives a retired runtime.
func TestActivitySnapshotsFollowTurnsToolsAndApprovals(t *testing.T) {
	changed := make(chan string, 32)
	service := NewService(context.Background(), nil, ServiceOptions{OnActivity: func(id string) { changed <- id }})
	service.mu.Lock()
	p := service.newSessionLocked("task")
	idle := service.newSessionLocked("other")
	service.mu.Unlock()
	// An attached window keeps both runtimes from retiring between events.
	p.owners[1], idle.owners[1] = true, true
	expect := func(step, phase string) ActivitySnapshot {
		t.Helper()
		select {
		case id := <-changed:
			if id != "task" {
				t.Fatalf("%s: change reported for %s", step, id)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("%s: no activity change reported", step)
		}
		snapshots := service.ActivitySnapshots("task")
		if len(snapshots) != 1 || snapshots[0].Phase != phase {
			t.Fatalf("%s: got %+v, want phase %s", step, snapshots, phase)
		}
		return snapshots[0]
	}
	service.event("task", p, json.RawMessage(`{"type":"execution_started"}`))
	first := expect("turn started", "running")
	if first.TurnGeneration != 1 || first.StartedAtMs == 0 || first.ToolInFlight {
		t.Fatalf("turn timing: %+v", first)
	}
	service.event("task", p, json.RawMessage(`{"type":"pi_event","eventType":"tool_execution_start","toolCallId":"call-1"}`))
	if !expect("tool started", "running").ToolInFlight {
		t.Fatal("tool in flight was not reported")
	}
	service.event("task", p, json.RawMessage(`{"type":"pi_event","eventType":"tool_execution_end","toolCallId":"call-1"}`))
	if expect("tool ended", "running").ToolInFlight {
		t.Fatal("finished tool stayed in flight")
	}
	// Streaming output keeps the projection unchanged: no push per token.
	service.event("task", p, json.RawMessage(`{"type":"pi_event","eventType":"message_update"}`))
	select {
	case id := <-changed:
		t.Fatalf("streamed text reported a change for %s", id)
	case <-time.After(100 * time.Millisecond):
	}
	service.event("task", p, json.RawMessage(`{"type":"approval","request":{"id":"ask-1","kind":"question","questions":[{"question":"Continue?"}]}}`))
	waiting := expect("question asked", "needs-interaction")
	if len(waiting.Interactions) != 1 || !json.Valid(waiting.Interactions[0]) {
		t.Fatalf("pending interaction: %+v", waiting)
	}
	service.event("task", p, json.RawMessage(`{"type":"approval_cancelled","id":"ask-1"}`))
	expect("question answered", "running")
	p.mu.Lock()
	p.running, p.status = false, "completed"
	p.notifyLocked()
	p.mu.Unlock()
	expect("turn completed", "completed")

	all := service.ActivitySnapshots()
	if len(all) != 1 || all[0].SessionID != "task" {
		t.Fatalf("an idle conversation entered the full read: %+v", all)
	}
	if named := service.ActivitySnapshots("other"); len(named) != 1 || named[0].Phase != "idle" {
		t.Fatalf("a requested idle conversation was omitted: %+v", named)
	}

	service.event("task", p, json.RawMessage(`{"type":"execution_started"}`))
	if second := expect("next turn", "running"); second.TurnGeneration != 2 {
		t.Fatalf("turn generation: %+v", second)
	}
	service.event("task", p, json.RawMessage(`{"type":"pi_event","eventType":"tool_execution_start","toolCallId":"call-2"}`))
	expect("second tool", "running")
	service.retire("task", p)
	retired := service.ActivitySnapshots("task")
	if len(retired) != 1 || retired[0].TurnGeneration != 2 || retired[0].ToolInFlight || retired[0].StartedAtMs == 0 {
		t.Fatalf("retired runtime lost its turn: %+v", retired)
	}
}
