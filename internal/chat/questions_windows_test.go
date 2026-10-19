//go:build windows

package chat

import (
	"context"
	"encoding/json"
	"testing"
)

// A pending question stays in the same replayable poll snapshot as permissions.
func TestPendingQuestionRetainsItsKindUntilExplicitResolution(t *testing.T) {
	p := &liveSession{gate: make(chan struct{}, 1), approvals: map[string]json.RawMessage{}, owners: map[uint32]bool{21: true}}
	s := &Service{sessions: map[string]*liveSession{"fixture": p}}
	s.event("fixture", p, []byte(`{"type":"approval","request":{"id":"q1","kind":"question","tool":"ask_user_question","args":{},"questions":[{"question":"Direction?","options":["A","B"]}]}}`))
	page, err := s.Poll("fixture", 0)
	if err != nil || len(page.Approvals) != 1 {
		t.Fatal(page, err)
	}
	var request struct{ ID, Kind string }
	if err := json.Unmarshal(page.Approvals[0], &request); err != nil {
		t.Fatal(err)
	}
	if request.Kind != "question" || request.ID != "q1" {
		t.Fatal("question type lost", request)
	}
	s.event("fixture", p, []byte(`{"type":"approval_cancelled","id":"q1"}`))
	page, err = s.Poll("fixture", page.Cursor)
	if err != nil || len(page.Approvals) != 0 {
		t.Fatal(page, err)
	}
	if _, err = s.Command(context.Background(), "fixture", "question", map[string]any{"id": "q1", "answers": []string{"B"}}); err == nil {
		t.Fatal("uninitialized question accepted")
	}
}
