//go:build windows

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	_ "golang.org/x/image/webp"
	"image"
	"image/jpeg"
	_ "image/png"
	"strings"
	"winone/internal/chat"
)

type ghostCompanionProfile struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Description        string `json:"description"`
	Avatar             string `json:"avatar"`
	Status             string `json:"status"`
	CanonicalSessionID string `json:"canonicalSessionId"`
	Sessions           []struct {
		ID         string `json:"id"`
		Role       string `json:"role"`
		ArchivedAt *int64 `json:"archivedAt"`
	} `json:"sessions"`
}
type ghostCompanionPending struct {
	SessionID   string          `json:"sessionId"`
	WorkingDir  string          `json:"workingDir"`
	Request     json.RawMessage `json:"request"`
	Fingerprint string          `json:"fingerprint"`
}
type ghostCompanionSnapshot struct {
	ID          string                  `json:"id"`
	Name        string                  `json:"name"`
	Description string                  `json:"description"`
	Avatar      string                  `json:"avatar"`
	Status      string                  `json:"status"`
	Canonical   *chat.Conversation      `json:"canonical,omitempty"`
	Activities  []chat.ActivitySnapshot `json:"activities"`
	Pending     []ghostCompanionPending `json:"pending"`
	sessions    map[string]*chatControl
}

func ghostPendingFingerprint(raw json.RawMessage) string {
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return ""
	}
	normalized, _ := json.Marshal(value)
	hash := sha256.Sum256(normalized)
	return hex.EncodeToString(hash[:])
}
func (g *chatGhosts) companionSnapshots(ctx context.Context) ([]ghostCompanionSnapshot, error) {
	b := g.control.onebot
	if b == nil {
		return nil, errors.New("HOST_NOT_READY")
	}
	runner, err := b.ensure(ctx)
	if err != nil {
		return nil, err
	}
	raw, err := runner.Call(ctx, "profiles.list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var profiles []ghostCompanionProfile
	if err = json.Unmarshal(raw, &profiles); err != nil {
		return nil, err
	}
	snapshots := []ghostCompanionSnapshot{}
	for _, profile := range profiles {
		if profile.Status == "archived" {
			continue
		}
		snapshot := ghostCompanionSnapshot{ID: profile.ID, Name: profile.Name, Description: profile.Description, Avatar: profile.Avatar, Status: profile.Status, Activities: []chat.ActivitySnapshot{}, Pending: []ghostCompanionPending{}, sessions: map[string]*chatControl{}}
		if row, e := b.sessions.store.Conversation(profile.CanonicalSessionID); e == nil && !row.Archived && !row.Hidden {
			snapshot.Canonical = &row
			snapshot.sessions[row.ID] = b.sessions
		}
		for _, link := range profile.Sessions {
			if link.Role == "history" || link.ArchivedAt != nil {
				continue
			}
			if row, e := b.sessions.store.Conversation(link.ID); e == nil && !row.Archived && !row.Hidden {
				snapshot.sessions[row.ID] = b.sessions
			}
		}
		for _, row := range g.control.store.Snapshot().Conversations {
			if row.OneBotOwnerID == profile.ID && !row.Archived && !row.Hidden {
				snapshot.sessions[row.ID] = g.control
			}
		}
		for id, control := range snapshot.sessions {
			conversation, lookup := control.store.Conversation(id)
			if lookup != nil {
				continue
			}
			for _, activity := range control.engine.ActivitySnapshots(id) {
				for _, request := range activity.Interactions {
					snapshot.Pending = append(snapshot.Pending, ghostCompanionPending{SessionID: id, WorkingDir: conversation.Directory, Request: request, Fingerprint: ghostPendingFingerprint(request)})
				}
				activity.Interactions = nil // Public activity never exposes a tool's arguments.
				snapshot.Activities = append(snapshot.Activities, activity)
			}
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots, nil
}

// Snapshot/answer share the same owner projection. No caller-provided session
// path or latest-task heuristic may redirect an answer to another companion.
func (g *chatGhosts) companionNative(ctx context.Context, op string, raw json.RawMessage) (any, error) {
	var input struct {
		CompanionID, SessionID, NativeID, RequestID, Fingerprint, Kind string
		Allow                                                          bool
		Answers                                                        []string
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	rows, err := g.companionSnapshots(ctx)
	if err != nil {
		return nil, err
	}
	if op == "companion-snapshot" {
		return rows, nil
	}
	var selected *ghostCompanionSnapshot
	for i := range rows {
		if rows[i].ID == input.CompanionID {
			selected = &rows[i]
			break
		}
	}
	if selected == nil {
		return nil, ghostTaskFail("TARGET_UNAVAILABLE", "TARGET_UNAVAILABLE")
	}
	if op == "companion-avatar" {
		fallback := strings.TrimSpace(selected.Name)
		if letters := []rune(fallback); len(letters) > 0 {
			fallback = string(letters[0])
		} else {
			fallback = "?"
		}
		if !strings.HasPrefix(selected.Avatar, "cindy-media://") {
			return map[string]any{"src": nil, "fallback": fallback}, nil
		}
		params, _ := json.Marshal(map[string]string{"url": selected.Avatar})
		media, err := g.control.onebot.mediaRead(params)
		if err != nil {
			return nil, err
		}
		// The existing managed media reader owns path and byte limits. Avatar
		// thumbnails use the original artwork, never a fabricated download URL.
		bytesJSON, _ := json.Marshal(media)
		var source struct {
			Base64 string `json:"base64"`
		}
		if err = json.Unmarshal(bytesJSON, &source); err != nil {
			return nil, err
		}
		decoded, err := decodeGhostAvatar(source.Base64)
		if err != nil {
			return nil, err
		}
		return map[string]any{"src": decoded, "fallback": fallback}, nil
	}
	if op == "companion-open" {
		if selected.sessions[input.SessionID] == nil {
			return nil, ghostTaskFail("TARGET_UNAVAILABLE", "TARGET_UNAVAILABLE")
		}
		target := map[string]string{"sessionId": input.SessionID}
		if selected.sessions[input.SessionID] == g.control.onebot.sessions {
			target["botId"] = selected.ID
		}
		g.pushEvent("reveal-session", target)
		return true, nil
	}
	if op != "companion-answer" {
		return nil, ghostTaskFail("INVALID_PARAMS", "INVALID_PARAMS")
	}
	control := selected.sessions[input.SessionID]
	if control == nil {
		return nil, ghostTaskFail("TARGET_UNAVAILABLE", "TARGET_UNAVAILABLE")
	}
	matched := false
	for _, pending := range selected.Pending {
		var request struct{ ID, RequestID, Kind string }
		_ = json.Unmarshal(pending.Request, &request)
		if pending.SessionID == input.SessionID && request.ID == input.NativeID && request.RequestID == input.RequestID && pending.Fingerprint == input.Fingerprint {
			if (request.Kind == "question") != (input.Kind == "question") {
				return nil, ghostTaskFail("INVALID_PARAMS", "INVALID_PARAMS")
			}
			matched = true
			break
		}
	}
	if !matched {
		return nil, ghostTaskFail("TARGET_UNAVAILABLE", "TARGET_UNAVAILABLE")
	}
	params := map[string]any{"id": input.NativeID, "allow": input.Allow, "answers": input.Answers, "scope": "once"}
	if input.RequestID != "" {
		params["requestId"] = input.RequestID
	}
	method := "permission"
	if input.Kind == "question" {
		method = "question"
	}
	result, err := control.engine.Command(ctx, input.SessionID, method, params)
	return json.RawMessage(result), err
}

func decodeGhostAvatar(encoded string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width < 1 || config.Height < 1 || int64(config.Width)*int64(config.Height) > 16_000_000 {
		return "", errors.New("Invalid companion avatar")
	}
	picture, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	if err = jpeg.Encode(&out, picture, &jpeg.Options{Quality: 80}); err != nil {
		return "", err
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(out.Bytes()), nil
}
