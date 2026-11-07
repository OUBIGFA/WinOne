//go:build windows

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"winone/internal/chat"
)

type ghostTargetCandidate struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Detail string `json:"detail"`
}

func (g *chatGhosts) targetCandidates(ctx context.Context, kind string) ([]ghostTargetCandidate, error) {
	result := []ghostTargetCandidate{}
	switch kind {
	case "project":
		for _, project := range g.control.store.Snapshot().Projects {
			if info, err := os.Stat(project.Path); err == nil && info.IsDir() {
				result = append(result, ghostTargetCandidate{project.Path, project.Name, project.Path})
			}
		}
	case "task":
		for _, task := range g.control.store.VisibleSnapshot().Conversations {
			// OneBot-owned lanes use their companion target and admission policy.
			if !task.Archived && task.OneBotTaskID == "" {
				result = append(result, ghostTargetCandidate{task.ID, task.Title, task.Directory})
			}
		}
	case "bot":
		rows, err := g.companionSnapshots(ctx)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			if row.Status == "active" && row.Canonical != nil {
				result = append(result, ghostTargetCandidate{row.ID, row.Name, row.Description})
			}
		}
	default:
		return nil, ghostTaskFail("INVALID_PARAMS", "Invalid target kind")
	}
	return result, nil
}
func (g *chatGhosts) targetConversation(id string) (*chatControl, chat.Conversation, error) {
	if row, err := g.control.store.Conversation(id); err == nil {
		return g.control, row, nil
	}
	if b := g.control.onebot; b != nil && b.sessions != nil {
		if row, err := b.sessions.store.Conversation(id); err == nil {
			return b.sessions, row, nil
		}
	}
	return nil, chat.Conversation{}, ghostTaskFail("TASK_NOT_FOUND", "Target task not found")
}
func (g *chatGhosts) targetNative(ctx context.Context, raw json.RawMessage) (any, error) {
	var input struct {
		Method, TargetKind, TargetID, PluginID, TaskID, ClientID, Text, Context, Delivery, RequestID string
		Cursor                                                                                       uint64
		Images                                                                                       []ghostTaskImage
		Execution                                                                                    json.RawMessage
		Delegated                                                                                    bool
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	if input.Method == "list" {
		return g.targetCandidates(ctx, input.TargetKind)
	}
	if input.Method == "session" {
		candidates, err := g.targetCandidates(ctx, input.TargetKind)
		if err != nil {
			return nil, err
		}
		exists := false
		for _, candidate := range candidates {
			if candidate.ID == input.TargetID {
				exists = true
				break
			}
		}
		if !exists {
			return nil, ghostTaskFail("TARGET_UNAVAILABLE", "Target is no longer available")
		}
		if input.TargetKind == "task" {
			return input.TargetID, nil
		}
		rows, err := g.companionSnapshots(ctx)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			if row.ID == input.TargetID && row.Canonical != nil {
				return row.Canonical.ID, nil
			}
		}
		return nil, ghostTaskFail("TARGET_UNAVAILABLE", "Companion conversation is unavailable")
	}
	control, row, err := g.targetConversation(input.TaskID)
	if err != nil {
		if input.Method == "withdraw" {
			return true, nil
		}
		if input.Method == "settled" {
			return map[string]bool{"settled": true}, nil
		}
		return nil, err
	}
	if control.engine == nil {
		return nil, ghostTaskFail("HOST_NOT_READY", "Task engine unavailable")
	}
	if _, err = control.engine.Open(ctx, row.ID, 0); err != nil {
		return nil, err
	}
	defer control.engine.Release(row.ID, 0)
	switch input.Method {
	case "review":
		value, err := control.engine.Command(ctx, row.ID, "authorization.inspect", nil)
		if err != nil {
			return nil, err
		}
		var snapshot struct {
			Running    bool
			RequestID  string
			Execution  json.RawMessage
			Projection json.RawMessage
		}
		if err = json.Unmarshal(value, &snapshot); err != nil {
			return nil, err
		}
		var actual, expected struct {
			InstanceID string
			Generation uint64
		}
		if json.Unmarshal(snapshot.Execution, &actual) != nil || json.Unmarshal(input.Execution, &expected) != nil ||
			!snapshot.Running || snapshot.RequestID != input.RequestID || expected.InstanceID == "" || expected.Generation == 0 || actual != expected {
			return nil, ghostTaskFail("PERMISSION_DENIED", "Delegated task input changed")
		}
		fresh, err := control.store.Conversation(row.ID)
		if err != nil || fresh.Archived || fresh.Hidden {
			return nil, ghostTaskFail("PERMISSION_DENIED", "Delegated task is no longer active")
		}
		return map[string]any{"revision": fresh.Revision, "projection": snapshot.Projection,
			"session": map[string]any{"workingDir": fresh.Directory, "permissionMode": fresh.Permission, "planModeEnabled": fresh.LegacyPlan, "status": "active",
				"route": map[string]any{"agentKind": "pi", "providerId": fresh.Model.Provider, "model": fresh.Model.Model, "effort": fresh.ThinkingLevel, "fastMode": fresh.FastMode}}}, nil
	case "send":
		if input.PluginID == "" || !strings.HasPrefix(input.ClientID, "plugin-target:") || row.Archived || row.Hidden {
			return nil, ghostTaskFail("TARGET_UNAVAILABLE", "Invalid target delivery")
		}
		if input.Delivery != "queue" && input.Delivery != "steer" {
			return nil, ghostTaskFail("INVALID_PARAMS", "Invalid target delivery mode")
		}
		if control == g.control && row.OneBotTaskID != "" {
			return nil, ghostTaskFail("TARGET_UNAVAILABLE", "Use the owning companion target")
		}
		text := input.Text
		if input.Context != "" {
			text += "\n\n<reference_material>\n" + input.Context + "\n</reference_material>"
		}
		if control != g.control {
			// Plugin text remains an agent request, never an owner-authored IM turn.
			hash := sha256.Sum256([]byte(text))
			if err = g.control.onebot.grantInput(input.ClientID, oneBotInputGrant{SessionID: row.ID, ConnectionID: "plugin:" + input.PluginID, Principal: "autonomous", Digest: hex.EncodeToString(hash[:])}); err != nil {
				return nil, err
			}
		}
		files, err := saveGhostInputImages(row, input.Images)
		if err != nil {
			return nil, err
		}
		// A delegated receipt supplies scope to Auto review, never new owner
		// words. The private slot derives this flag from its persisted receipt.
		kind := "agent-request"
		if input.Delegated {
			kind = "delegated-continuation"
		}
		review, _ := json.Marshal(map[string]string{"kind": kind})
		value, err := control.engine.DispatchPresentedInput(ctx, row.ID, input.ClientID, text, input.Delivery, chat.NativeInputPresentation{Text: &input.Text, ReviewRequest: review}, files)
		return json.RawMessage(value), err
	case "wait":
		return control.waitInputResult(ctx, row.ID, input.RequestID, input.Cursor)
	case "withdraw", "settled":
		if !strings.HasPrefix(input.RequestID, "plugin-target:") {
			return nil, ghostTaskFail("INVALID_PARAMS", "Invalid target receipt")
		}
		value, err := control.engine.Command(ctx, row.ID, "execution.inspect", nil)
		if err != nil {
			return nil, err
		}
		var state struct {
			RequestID string
			Running   bool
			Execution json.RawMessage
			Pending   []struct{ ID string }
		}
		if err = json.Unmarshal(value, &state); err != nil {
			return nil, err
		}
		pending := false
		for _, item := range state.Pending {
			if item.ID == input.RequestID {
				pending = true
				break
			}
		}
		active := state.Running && state.RequestID == input.RequestID
		if input.Method == "settled" {
			return map[string]bool{"settled": !pending && !active}, nil
		}
		if pending {
			if _, err = control.engine.Command(ctx, row.ID, "queue", map[string]string{"id": input.RequestID, "action": "cancel"}); err != nil {
				return nil, err
			}
		}
		if active {
			if _, err = control.engine.Command(ctx, row.ID, "abort.input", map[string]any{"id": input.RequestID, "execution": state.Execution}); err != nil {
				return nil, err
			}
		}
		return true, nil
	}
	return nil, errors.New("Unknown native target operation")
}
