//go:build windows

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
)

// oneBotBuiltinRoutinePrefix is Cindy's BUILTIN_ROUTINE_SCHEDULE_PREFIX: the
// schedule id of a built-in rule (`routine-builtin-follow-up-<bot>` and so on).
const oneBotBuiltinRoutinePrefix = "routine-builtin-"

// ownsSession reports whether a conversation lives in the teammates' store.
func (b *chatOneBot) ownsSession(id string) bool {
	b.startMu.Lock()
	sessions := b.sessions
	b.startMu.Unlock()
	if sessions == nil {
		return false
	}
	_, err := sessions.store.Conversation(id)
	return err == nil
}

// routinePolicy is schedule_notify / schedule_silence for the run currently
// executing in a teammate's chat: the Worker's routine scheduler checks that
// this accepted request is that run before changing its report policy.
func (b *chatOneBot) routinePolicy(ctx context.Context, id, action string) (json.RawMessage, int, error) {
	requestID := b.sessions.engine.ActiveRequest(id)
	if requestID == "" {
		return nil, 409, errors.New("No active automation turn")
	}
	runner, err := b.ensure(ctx)
	if err != nil {
		return nil, 500, err
	}
	value, err := runner.Call(ctx, "routines.policy", map[string]string{"sessionId": id, "requestId": requestID, "action": action})
	if err != nil {
		return nil, 409, err
	}
	return value, 200, nil
}

// authorizeRoutine is a private Worker callback, not a desktop or model method.
// A saved standing instruction can run only in its current canonical chat, with
// that chat's existing model and permission mode. The exact prompt is receipted.
func (b *chatOneBot) authorizeRoutine(ctx context.Context, raw json.RawMessage) (map[string]bool, error) {
	var input struct {
		SessionID  string `json:"sessionId"`
		RunID      string `json:"runId"`
		Prompt     string `json:"prompt"`
		ScheduleID string `json:"scheduleId"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	if input.SessionID == "" || input.RunID == "" || input.Prompt == "" || len(input.Prompt) > 256<<10 {
		return nil, errors.New("例行任务输入无效")
	}
	value, err := b.contextFor(ctx, map[string]string{"sessionId": input.SessionID})
	if err != nil {
		return nil, err
	}
	if value.Role != "canonical" || value.CanonicalSessionID != input.SessionID {
		return nil, errors.New("例行任务只能使用伙伴当前的主对话")
	}
	if _, err := b.sessions.store.Conversation(input.SessionID); err != nil {
		return nil, err
	}
	digest := sha256.Sum256([]byte(input.Prompt))
	// Cindy teammateTurnPrincipal: an owner-created routine is a standing
	// grant; built-in rules (follow-up, todo check) are Host templates and act
	// on the teammate's own initiative.
	grant := oneBotInputGrant{SessionID: input.SessionID, Owner: true, Channel: "routine", Principal: "entrusted", Digest: hex.EncodeToString(digest[:])}
	if strings.HasPrefix(input.ScheduleID, oneBotBuiltinRoutinePrefix) {
		grant.Owner, grant.Principal = false, "autonomous"
	}
	if err := b.grantInput(input.RunID, grant); err != nil {
		return nil, err
	}
	return map[string]bool{"ok": true}, nil
}
