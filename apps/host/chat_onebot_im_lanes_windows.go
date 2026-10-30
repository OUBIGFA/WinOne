//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"

	"winone/internal/chat"
)

// Lane reads expose only registered OneBot records to the private Worker.
// Activity is an in-memory read; browsing connections does not start Pi.
func (b *chatOneBot) laneSessions() (any, error) {
	if b.sessions == nil || b.sessions.engine == nil {
		return nil, errors.New("OneBot 会话服务尚未就绪")
	}
	return map[string]any{"conversations": b.sessions.store.Snapshot().Conversations, "activity": b.sessions.engine.Activity()}, nil
}

// The original Cindy lane service validates the connection-specific scope.
// This private native boundary additionally requires a real companion link,
// never a renderer-supplied path, and keeps the transcript and draft intact.
func (b *chatOneBot) controlLane(ctx context.Context, action string, raw json.RawMessage) (any, error) {
	var input struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	runner, err := b.ensure(ctx)
	if err != nil {
		return nil, err
	}
	linked, err := runner.Call(ctx, "sessions.lookup", map[string]string{"sessionId": input.SessionID})
	if err != nil {
		return nil, err
	}
	var link struct {
		Role string `json:"role"`
	}
	if err = json.Unmarshal(linked, &link); err != nil {
		return nil, err
	}
	if link.Role != "canonical" && link.Role != "channel" || action == "lane-archive" && link.Role != "channel" {
		return nil, errors.New("此会话不属于可操作的 IM 专线")
	}
	row, err := b.sessions.store.Conversation(input.SessionID)
	if err != nil {
		return nil, err
	}
	if action == "lane-archive" {
		if err = b.sessions.engine.CloseSession(ctx, row.ID); err != nil {
			return nil, err
		}
		if _, err = b.sessions.store.UpdateConversation(row.ID, func(c *chat.Conversation) error { c.Archived = true; return nil }); err != nil {
			return nil, err
		}
	} else if action == "lane-restart" {
		if row.Archived {
			return nil, errors.New("会话已归档，不能重新开始")
		}
		if _, err = b.sessions.engine.Open(ctx, row.ID, 0); err != nil {
			return nil, err
		}
		defer b.sessions.engine.Release(row.ID, 0)
		// One command owns the native input gate across abort and context reset.
		// Reset uses Pi's durable context head; the visible history is not cleared.
		if _, err = b.sessions.engine.Command(ctx, row.ID, "context.reset", nil); err != nil {
			return nil, err
		}
	} else {
		return nil, errors.New("不支持的渠道会话操作")
	}
	if err = b.revokeSessionGrants([]string{row.ID}); err != nil {
		return nil, err
	}
	return map[string]bool{"ok": true}, nil
}
