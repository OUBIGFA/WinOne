//go:build windows

package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"time"
)

// Cindy teammate-authority (2d8d3c3): on an autonomous turn only explicit
// risks need the owner's visible, one-time decision. /policy issues a
// confirmation for the exact teammate tool call it classified as risky; the
// Worker shows its approval card and returns the confirmation with that one
// call. A confirmation is bound to the conversation, input receipt and tool,
// is consumed once, and records the confirmed call so the Worker's later
// authority check (tasks.authority) can tell it from an unconfirmed one.

// oneBotRiskTools are the teammate tools taskActionRisk can classify.
var oneBotRiskTools = map[string]bool{"message_session_task": true, "routine_save": true, "routine_delete": true, "routine_run_now": true, "schedule_set_pre_run_hook": true}

// oneBotRiskMethods maps each tool to the Worker method it calls.
var oneBotRiskMethods = map[string]string{"message_session_task": "tasks.message", "routine_save": "teammate.routines.save", "routine_delete": "teammate.routines.delete", "routine_run_now": "teammate.routines.runNow", "schedule_set_pre_run_hook": "teammate.routines.preRunHook"}

const oneBotRiskLifetime = 30 * time.Minute

type oneBotRiskConfirmation struct {
	SessionID   string
	RequestID   string
	Tool        string
	Fingerprint string
	Expires     time.Time
}

type oneBotRiskBook struct {
	mu        sync.Mutex
	pending   map[string]oneBotRiskConfirmation
	confirmed map[string]oneBotRiskConfirmation
}

func (r *oneBotRiskBook) prune(now time.Time) {
	for id, value := range r.pending {
		if now.After(value.Expires) {
			delete(r.pending, id)
		}
	}
	for key, value := range r.confirmed {
		if now.After(value.Expires) {
			delete(r.confirmed, key)
		}
	}
}

// issue returns the confirmation the Worker returns after the owner approved.
func (r *oneBotRiskBook) issue(sessionID, requestID, tool, fingerprint string) (string, error) {
	var raw [24]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	id := hex.EncodeToString(raw[:])
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.pending == nil {
		r.pending = map[string]oneBotRiskConfirmation{}
	}
	now := time.Now()
	r.prune(now)
	r.pending[id] = oneBotRiskConfirmation{SessionID: sessionID, RequestID: requestID, Tool: tool, Fingerprint: fingerprint, Expires: now.Add(oneBotRiskLifetime)}
	return id, nil
}

// consume admits one confirmed call of method and records it for authority checks.
func (r *oneBotRiskBook) consume(id, sessionID, requestID, tool, method, fingerprint string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	r.prune(now)
	value, ok := r.pending[id]
	if !ok || value.SessionID != sessionID || value.RequestID != requestID || value.Tool != tool || value.Fingerprint != fingerprint || oneBotRiskMethods[tool] != method {
		return errors.New("主人确认已失效，请重新发起这次操作")
	}
	delete(r.pending, id)
	if r.confirmed == nil {
		r.confirmed = map[string]oneBotRiskConfirmation{}
	}
	r.confirmed[id] = value
	return nil
}

// confirmedFor is valid only while this one admitted Worker call is in flight.
func (r *oneBotRiskBook) confirmedFor(id, sessionID, requestID, method string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	value, ok := r.confirmed[id]
	return ok && value.SessionID == sessionID && value.RequestID == requestID && oneBotRiskMethods[value.Tool] == method && time.Now().Before(value.Expires)
}

func (r *oneBotRiskBook) finish(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.confirmed, id)
}

// JSON object order is not authority. Canonicalize while preserving exact
// numbers, then bind both public/prepared arguments and current routine facts.
func oneBotRiskFingerprint(args, prepared, classification json.RawMessage) (string, error) {
	parts := make([]any, 0, 3)
	for _, raw := range []json.RawMessage{args, prepared, classification} {
		var value any
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if err := decoder.Decode(&value); err != nil {
			return "", err
		}
		parts = append(parts, value)
	}
	raw, err := json.Marshal(parts)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:]), nil
}
