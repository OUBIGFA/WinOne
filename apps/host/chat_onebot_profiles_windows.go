//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"winone/internal/chat"
)

// projectProfiles joins two existing authorities: the Worker owns Bot links;
// Chat owns conversation titles, archival state and timestamps. It never
// infers membership from directories or exposes an unrelated Chat session.
// The caller holds sessionMu against opening/deleting linked conversations.
func (b *chatOneBot) projectProfiles(raw json.RawMessage, list bool) (json.RawMessage, error) {
	var profiles []map[string]json.RawMessage
	if list {
		if err := json.Unmarshal(raw, &profiles); err != nil {
			return nil, err
		}
	} else {
		var profile map[string]json.RawMessage
		if err := json.Unmarshal(raw, &profile); err != nil {
			return nil, err
		}
		profiles = []map[string]json.RawMessage{profile}
	}
	conversations := map[string]chat.Conversation{}
	if b.sessions != nil {
		for _, conversation := range b.sessions.store.Snapshot().Conversations {
			conversations[conversation.ID] = conversation
		}
	}
	for _, profile := range profiles {
		var links []struct {
			ID       string `json:"id"`
			Role     string `json:"role"`
			RouteKey string `json:"routeKey"`
		}
		if err := json.Unmarshal(profile["sessions"], &links); err != nil {
			return nil, err
		}
		sessions := make([]map[string]any, 0, len(links))
		for _, link := range links {
			conversation, ok := conversations[link.ID]
			if !ok {
				return nil, errors.New("OneBot 关联会话不在历史索引中")
			}
			updated, err := time.Parse(time.RFC3339Nano, conversation.UpdatedAt)
			if err != nil {
				return nil, fmt.Errorf("OneBot 会话更新时间无效: %w", err)
			}
			kind := link.Role
			if kind == "canonical" {
				kind = "chat"
			}
			if conversation.Archived && kind != "channel" && kind != "group" {
				kind = "history"
			}
			status := "active"
			if conversation.Archived {
				status = "archived"
			}
			session := map[string]any{
				"id": link.ID, "role": link.Role, "routeKey": link.RouteKey,
				"title": conversation.Title, "kind": kind, "updatedAt": updated.UnixMilli(), "status": status,
			}
			// A private-chat topic opens on its root message; a group lane belongs to its group.
			if link.Role == "thread" {
				if root, client, ok := strings.Cut(strings.TrimPrefix(link.RouteKey, "dm-thread:"), ":"); ok && strings.HasPrefix(link.RouteKey, "dm-thread:") {
					session["threadRoot"] = map[string]string{"rootSessionId": root, "rootClientId": client}
				}
			}
			if link.Role == "group" {
				if group, _, ok := strings.Cut(strings.TrimPrefix(link.RouteKey, "group:"), ":"); ok || strings.HasPrefix(link.RouteKey, "group:") {
					session["groupId"] = group
				}
			}
			sessions = append(sessions, session)
		}
		projected, err := json.Marshal(sessions)
		if err != nil {
			return nil, err
		}
		profile["sessions"] = projected
	}
	if !list {
		return json.Marshal(profiles[0])
	}
	return json.Marshal(profiles)
}
