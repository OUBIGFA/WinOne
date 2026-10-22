//go:build windows

package chat

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestActivityIsOneNonblockingRuntimeProjection(t *testing.T) {
	service := NewService(context.Background(), nil, ServiceOptions{Configure: func(Conversation) (map[string]any, func(), error) {
		t.Fatal("activity started a worker")
		return nil, nil, nil
	}})
	service.sessions = map[string]*liveSession{
		"running":    {running: true, status: "running", gate: make(chan struct{}, 1)},
		"awaiting":   {running: true, approvals: map[string]json.RawMessage{"permission": json.RawMessage(`{}`)}},
		"paused":     {sessionPaused: true, status: "paused"},
		"completed":  {status: "completed"},
		"failed":     {running: true, status: "failed", failure: "recorded failure"},
		"idle":       {status: "idle"},
		"background": {backgroundRunning: true, status: "completed"},
		"stopped":    {status: "stopped", failure: "AbortError: operation cancelled"},
	}
	// Holding the prompt/command gate must not stop a catalog-wide status read.
	session := service.sessions["running"]
	if err := session.lock(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer session.unlock()
	read := make(chan map[string]ConversationActivity, 1)
	go func() { read <- service.Activity() }()
	var activity map[string]ConversationActivity
	select {
	case activity = <-read:
	case <-time.After(time.Second):
		t.Fatal("activity waited for a tool/command gate")
	}
	expected := map[string]string{"running": "running", "awaiting": "needs-interaction", "paused": "idle", "completed": "completed", "failed": "error", "idle": "idle", "background": "running", "stopped": "interrupted"}
	for id, phase := range expected {
		if activity[id].Phase != phase {
			t.Errorf("%s: got %s, want %s", id, activity[id].Phase, phase)
		}
	}
	if !activity["paused"].Paused || !activity["failed"].Attention || !activity["completed"].Attention {
		t.Fatal("lost paused/terminal attention evidence", activity)
	}
	delete(activity, "running")
	if len(service.Activity()) != len(expected) {
		t.Fatal("caller mutated runtime projection")
	}
	if _, exists := service.sessions["unopened"]; exists {
		t.Fatal("unopened session started")
	}
}

func TestTerminalActivitySurvivesIdleWorkerRetirement(t *testing.T) {
	for _, test := range []struct{ status, failure, phase string }{
		{"stopped", "AbortError", "interrupted"},
		{"completed", "", "completed"},
		{"failed", "connection lost", "error"},
		{"paused", "", "idle"},
	} {
		t.Run(test.status, func(t *testing.T) {
			service := NewService(context.Background(), nil, ServiceOptions{})
			p := &liveSession{gate: make(chan struct{}, 1), status: test.status, failure: test.failure, sessionPaused: test.status == "paused"}
			service.sessions["conversation"] = p
			service.closeIdle("conversation", p)
			if service.Active() || len(service.sessions) != 0 {
				t.Fatal("kept an idle runtime alive just to display its status")
			}
			if got := service.Activity()["conversation"]; got.Phase != test.phase || got.Paused != p.sessionPaused {
				t.Fatal("retirement erased outcome", got)
			}
			// Both Open and Reconfigure use this same admission point. A fresh
			// runtime must inherit only terminal metadata, never old event buffers.
			service.mu.Lock()
			reopened := service.newSessionLocked("conversation")
			service.mu.Unlock()
			if reopened == p || len(reopened.events) != 0 || service.Activity()["conversation"].Phase != test.phase {
				t.Fatal("reopening erased the outcome or retained the old runtime")
			}
			reopened.mu.Lock()
			reopened.status, reopened.failure, reopened.running, reopened.sessionPaused = "running", "", true, false
			reopened.mu.Unlock()
			if service.Activity()["conversation"].Phase != "running" {
				t.Fatal("cached outcome hid the next real run")
			}
			service.retire("conversation", p)
			if service.Activity()["conversation"].Phase != "running" {
				t.Fatal("late retirement overwrote a newer runtime")
			}
		})
	}
}

func TestBackgroundChildrenAreNotRetiredWhenParentIsIdle(t *testing.T) {
	service := NewService(context.Background(), nil, ServiceOptions{})
	session := &liveSession{gate: make(chan struct{}, 1), backgroundRunning: true, status: "completed"}
	service.sessions["background"] = session
	service.closeIdle("background", session)
	if session.closed || service.sessions["background"] != session {
		t.Fatal("idle parent retired an executing child")
	}
}
