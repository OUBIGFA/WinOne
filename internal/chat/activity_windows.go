//go:build windows

package chat

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ActivitySnapshot is the live projection Cindy's teammate follow-up reads
// (SessionActivitySnapshot in TaskWatch): the turn's phase, its timing and
// pending interactions. Interactions are the worker's approval requests; the
// follow-up reduces them to a one-line summary and never shows arguments.
type ActivitySnapshot struct {
	SessionID        string            `json:"sessionId"`
	Phase            string            `json:"phase"`
	StartedAtMs      int64             `json:"startedAtMs,omitempty"`
	LastActivityAtMs int64             `json:"lastActivityAtMs,omitempty"`
	TurnGeneration   uint64            `json:"turnGeneration"`
	ToolInFlight     bool              `json:"toolInFlight"`
	Interactions     []json.RawMessage `json:"interactions,omitempty"`
}

// activityState is the timing a snapshot needs beyond the session status.
// A turn starts when the session begins running; the generation counts turns.
type activityState struct {
	turnStartedAt, lastActivityAt int64
	generation                    uint64
	wasRunning                    bool
	tools                         map[string]bool
	signature                     string
}

func (a *activityState) trackTool(eventType, id string) {
	if id == "" {
		return
	}
	switch eventType {
	case "tool_execution_start":
		if a.tools == nil {
			a.tools = map[string]bool{}
		}
		a.tools[id] = true
	case "tool_execution_end":
		delete(a.tools, id)
	}
}

// retired keeps a closed runtime's last turn timing; no tool remains in flight.
func (a activityState) retired() activityState {
	a.tools, a.wasRunning = nil, false
	return a
}

// observeActivityLocked updates turn timing on every state change and reports
// whether the observable projection changed. Caller owns p.mu.
func (p *liveSession) observeActivityLocked() bool {
	now := time.Now().UnixMilli()
	p.activity.lastActivityAt = now
	if p.running && !p.activity.wasRunning {
		p.activity.generation++
		p.activity.turnStartedAt = now
	}
	p.activity.wasRunning = p.running
	signature := strings.Join([]string{p.activityLocked().Phase, strings.Join(p.approvalIDsLocked(), ","),
		strconv.FormatUint(p.activity.generation, 10), strconv.FormatBool(len(p.activity.tools) > 0)}, "|")
	if signature == p.activity.signature {
		return false
	}
	p.activity.signature = signature
	return true
}

func (p *liveSession) approvalIDsLocked() []string {
	ids := make([]string, 0, len(p.approvals))
	for id := range p.approvals {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (p *liveSession) snapshotLocked(id string) ActivitySnapshot {
	value := ActivitySnapshot{SessionID: id, Phase: p.activityLocked().Phase, StartedAtMs: p.activity.turnStartedAt,
		LastActivityAtMs: p.activity.lastActivityAt, TurnGeneration: p.activity.generation, ToolInFlight: len(p.activity.tools) > 0}
	for _, request := range p.approvalIDsLocked() {
		value.Interactions = append(value.Interactions, p.approvals[request])
	}
	return value
}

// ActivitySnapshots is a control-plane read like Activity: no session gate,
// worker contact or history read. Idle conversations are omitted unless asked
// for by id; retired runtimes keep their final phase and timing.
func (s *Service) ActivitySnapshots(ids ...string) []ActivitySnapshot {
	wanted := map[string]bool{}
	for _, id := range ids {
		wanted[id] = true
	}
	include := func(value ActivitySnapshot) bool {
		if len(ids) > 0 {
			return wanted[value.SessionID]
		}
		return value.Phase != "idle"
	}
	s.mu.Lock()
	sessions := make(map[string]*liveSession, len(s.sessions))
	for id, p := range s.sessions {
		sessions[id] = p
	}
	result := []ActivitySnapshot{}
	for id, outcome := range s.outcomes {
		if _, live := sessions[id]; live {
			continue
		}
		value := (&liveSession{status: outcome.status, failure: outcome.failure, sessionPaused: outcome.paused, activity: outcome.activity}).snapshotLocked(id)
		if include(value) {
			result = append(result, value)
		}
	}
	s.mu.Unlock()
	for id, p := range sessions {
		p.mu.Lock()
		value := p.snapshotLocked(id)
		p.mu.Unlock()
		if include(value) {
			result = append(result, value)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].SessionID < result[j].SessionID })
	return result
}
