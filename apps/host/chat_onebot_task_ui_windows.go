//go:build windows

package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"winone/internal/chat"
)

// A desktop task action is the owner's explicit click, not a model-supplied
// ownerAuthorized flag. Pin its caller and persist the same native lease used
// by typed tools before handing the action to the original task coordinator.
func (b *chatOneBot) taskUICall(ctx context.Context, runner *chat.Runner, method string, raw json.RawMessage) (json.RawMessage, error) {
	var input map[string]any
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	caller, ok := input["callerSessionId"].(string)
	if !ok || caller == "" {
		return nil, errors.New("请选择发起任务的 OneBot 会话")
	}
	if _, err := b.sessions.store.Conversation(caller); err != nil {
		return nil, err
	}
	delete(input, "ownerAuthorized")
	delete(input, "requestId")
	if method == "tasks.message" || method == "tasks.stop" || method == "tasks.trace" {
		var nonce [16]byte
		if _, err := rand.Read(nonce[:]); err != nil {
			return nil, err
		}
		requestID := "onebot-ui:" + hex.EncodeToString(nonce[:])
		digest := sha256.Sum256(raw)
		if err := b.grantInput(requestID, oneBotInputGrant{SessionID: caller, Owner: true, ConnectionID: "desktop", Digest: hex.EncodeToString(digest[:])}); err != nil {
			return nil, err
		}
		input["ownerAuthorized"] = true
		input["requestId"] = requestID
	}
	return runner.Call(ctx, method, input)
}
