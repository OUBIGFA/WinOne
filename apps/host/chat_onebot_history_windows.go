//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"winone/internal/chat"
)

// Search scope comes from the durable Bot links and the Host's Chat index.
// Renderer-supplied paths, session ids and source filters are never forwarded.
func (b *chatOneBot) searchHistory(ctx context.Context, runner *chat.Runner, raw json.RawMessage) (json.RawMessage, error) {
	var input struct {
		BotID string `json:"botId"`
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	b.sessionMu.Lock()
	defer b.sessionMu.Unlock()
	profileRaw, err := runner.Call(ctx, "profiles.get", map[string]string{"botId": input.BotID})
	if err != nil {
		return nil, err
	}
	var profile struct {
		Sessions []struct {
			ID   string `json:"id"`
			Role string `json:"role"`
		} `json:"sessions"`
	}
	if err = json.Unmarshal(profileRaw, &profile); err != nil {
		return nil, err
	}
	sessions := []chat.Conversation{}
	for _, link := range profile.Sessions {
		if link.Role == "group" || link.Role == "channel" {
			continue
		}
		if b.sessions == nil {
			return nil, errors.New("OneBot 历史索引尚未就绪")
		}
		conversation, err := b.sessions.store.Conversation(link.ID)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, conversation)
	}
	return runner.Call(ctx, "history.query", map[string]any{"botId": input.BotID, "query": input.Query, "limit": input.Limit, "sessions": sessions})
}
