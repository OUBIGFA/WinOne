//go:build windows

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"winone/internal/chat"
)

type oneBotTaskSession struct {
	ID              string          `json:"id"`
	Title           string          `json:"title"`
	WorkingDir      string          `json:"workingDir"`
	Model           string          `json:"model"`
	ProviderID      string          `json:"providerId"`
	AgentKind       string          `json:"agentKind"`
	Effort          string          `json:"effort"`
	FastMode        json.RawMessage `json:"fastMode"`
	PermissionMode  string          `json:"permissionMode"`
	ParentSessionID string          `json:"parentSessionId"`
	TaskID          string          `json:"taskId"`
	BotID           string          `json:"botId"`
}

// Only the authenticated OneBot worker calls this bridge. Renderer/model calls
// cannot publish an arbitrary conversation or forge a terminal execution.
func (b *chatOneBot) serveTask(w http.ResponseWriter, r *http.Request, raw []byte) {
	var input struct {
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}
	var value any
	err := json.Unmarshal(raw, &input)
	if err == nil {
		value, err = b.taskNative(r.Context(), input.Method, input.Params)
	}
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error(), "errorCode": "NATIVE_TASK_FAILED"})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"value": value})
}

func (b *chatOneBot) taskConversation(id string) (*chatControl, chat.Conversation, error) {
	if b.sessions != nil {
		if row, err := b.sessions.store.Conversation(id); err == nil {
			return b.sessions, row, nil
		}
	}
	row, err := b.control.store.Conversation(id)
	if err != nil {
		return nil, row, err
	}
	if row.OneBotTaskID == "" {
		return nil, row, errors.New("该会话不属于 OneBot 独立任务")
	}
	return b.control, row, nil
}

func (b *chatOneBot) taskNative(ctx context.Context, method string, raw json.RawMessage) (any, error) {
	if method == "canonical" {
		// The coordinator recovers a missing main chat through the same
		// compare-and-set path as the owner's page, never its own executor.
		runner, err := b.ensure(ctx)
		if err != nil {
			return nil, err
		}
		return b.createCanonicalSession(ctx, runner, raw)
	}
	var input struct {
		SessionID      string           `json:"sessionId"`
		RequestID      string           `json:"requestId"`
		PermissionMode string           `json:"permissionMode"`
		Route          oneBotModelRoute `json:"route"`
		Selection      struct {
			ID       string          `json:"id"`
			Effort   string          `json:"effort"`
			FastMode json.RawMessage `json:"fastMode"`
		} `json:"selection"`
		TargetSessionID     string                  `json:"targetSessionId"`
		ClientID            string                  `json:"clientId"`
		Message             string                  `json:"message"`
		PersistedContent    *string                 `json:"persistedContent"`
		ToolsDisabled       bool                    `json:"toolsDisabled"`
		Attachments         []oneBotGroupAttachment `json:"attachments"`
		AutoReviewUserText  json.RawMessage         `json:"autoReviewUserText"`
		DeliveryMode        string                  `json:"deliveryMode"`
		DispatcherSessionID string                  `json:"dispatcherSessionId"`
		Held                bool                    `json:"held"`
		Action              string                  `json:"action"`
		Text                string                  `json:"text"`
		NativeID            string                  `json:"nativeId"`
		Kind                string                  `json:"kind"`
		Allow               bool                    `json:"allow"`
		Answers             []string                `json:"answers"`
		DelegationID        string                  `json:"delegationId"`
		ReplyTaskID         string                  `json:"replyTaskId"`
		ReplyChildSessionID string                  `json:"replyChildSessionId"`
		ReplyKind           string                  `json:"replyKind"`
		InputClientIDs      []string                `json:"inputClientIds"`
		Prepared            *oneBotTaskSession      `json:"prepared"`
		Cursor              uint64                  `json:"cursor"`
		Execution           json.RawMessage         `json:"execution"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	switch method {
	case "validateModel", "resolveModel":
		cfg, err := b.control.aiConfiguration()
		if err != nil {
			return nil, err
		}
		route := input.Route
		if method == "resolveModel" {
			provider, model, ok := strings.Cut(input.Selection.ID, "/")
			if !ok {
				return nil, errors.New("任务模型必须包含供应商")
			}
			route = oneBotModelRoute{Harness: "pi", ProviderID: provider, Model: model, Effort: input.Selection.Effort, FastMode: input.Selection.FastMode}
		}
		if route.Harness != "pi" {
			return false, nil
		}
		selected, fast, err := oneBotRouteSelection(route, cfg)
		if method == "validateModel" {
			return err == nil, nil
		}
		if err != nil {
			return nil, err
		}
		route.ProviderID = selected.Provider
		route.FastMode, _ = json.Marshal(fast)
		return route, nil
	case "authority":
		grant, err := b.inputGrant(input.SessionID, input.RequestID)
		if err != nil {
			return false, nil
		}
		// Cindy autonomousActionAllowed: on its own initiative a teammate may
		// advance or stop the background tasks it started (the Worker checks
		// ownership); permission decisions and resuming a pause stay with the owner.
		autonomous := grant.principal() == "autonomous" && (input.Action == "tasks.stop" || input.Action == "tasks.message" && input.ReplyKind != "approve" && input.ReplyKind != "deny" && input.ReplyKind != "resume")
		if !grant.Owner && !autonomous {
			return false, nil
		}
		row, err := b.sessions.store.Conversation(input.SessionID)
		if err != nil {
			return false, nil
		}
		active, err := b.contextFor(ctx, map[string]string{"sessionId": input.SessionID})
		return err == nil && row.Permission == input.PermissionMode && oneBotPermission(active.Capabilities.Permissions) == input.PermissionMode, nil
	case "artifacts":
		control, row, err := b.taskConversation(input.SessionID)
		if err != nil {
			return nil, err
		}
		if len(input.InputClientIDs) > 4096 {
			return nil, errors.New("任务回执数量过多")
		}
		files := []map[string]string{}
		positions := map[string]int{}
		seen := map[string]bool{}
		for _, id := range input.InputClientIDs {
			if seen[id] {
				continue
			}
			seen[id] = true
			snapshot, _, err := control.store.TurnReview(row.ID, id)
			if errors.Is(err, chat.ErrReviewTurnNotFound) {
				continue
			}
			if err != nil {
				return nil, err
			}
			for _, file := range snapshot.Files {
				absolute := filepath.FromSlash(file.Path)
				if !filepath.IsAbs(absolute) {
					absolute = filepath.Join(snapshot.Root, absolute)
				}
				status := "modified"
				switch file.Status {
				case "A", "?":
					status = "added"
				case "D":
					status = "deleted"
				case "R":
					status = "renamed"
				}
				artifact := map[string]string{"path": file.Path, "absolutePath": absolute, "status": status}
				if index, ok := positions[absolute]; ok {
					files[index] = artifact
				} else {
					positions[absolute] = len(files)
					files = append(files, artifact)
				}
			}
		}
		return files, nil
	case "source":
		grant, err := b.inputGrant(input.SessionID, input.RequestID)
		if err != nil || !grant.Owner {
			return nil, nil
		}
		result, err := b.sessions.engine.Command(ctx, input.SessionID, "input.source", map[string]string{"id": input.RequestID})
		return json.RawMessage(result), err
	case "session":
		control, row, err := b.taskConversation(input.SessionID)
		if err != nil {
			return nil, err
		}
		source := "desktop"
		if control == b.sessions {
			source = "bot"
		}
		status := "active"
		if row.Archived {
			status = "archived"
		}
		return map[string]any{"id": row.ID, "title": row.Title, "workingDir": row.Directory, "model": row.Model.Model, "providerId": row.Model.Provider, "agentKind": "pi", "effort": row.ThinkingLevel, "fastMode": row.FastMode, "permissionMode": row.Permission, "status": status, "source": source}, nil
	case "dispatch":
		if input.Prepared != nil {
			prepared := input.Prepared
			if prepared.ID != input.TargetSessionID || prepared.AgentKind != "pi" {
				return nil, errors.New("任务会话身份无效")
			}
			active, err := b.contextFor(ctx, map[string]string{"sessionId": prepared.ParentSessionID})
			if err != nil {
				return nil, err
			}
			if active.BotID != prepared.BotID || oneBotPermission(active.Capabilities.Permissions) != prepared.PermissionMode {
				return nil, errors.New("任务伙伴或权限已变化")
			}
			cfg, err := b.control.aiConfiguration()
			if err != nil {
				return nil, err
			}
			selected, fast, err := oneBotRouteSelection(oneBotModelRoute{Harness: "pi", ProviderID: prepared.ProviderID, Model: prepared.Model, Effort: prepared.Effort, FastMode: prepared.FastMode}, cfg)
			if err != nil {
				return nil, err
			}
			_, err = b.control.store.AdmitTaskConversation(chat.Conversation{ID: prepared.ID, Title: prepared.Title, Directory: prepared.WorkingDir, Model: selected, ThinkingLevel: prepared.Effort, FastMode: fast, Permission: prepared.PermissionMode, OneBotTaskID: prepared.TaskID, OneBotOwnerID: prepared.BotID, OneBotParentID: prepared.ParentSessionID})
			if err != nil {
				return nil, err
			}
		}
		control, row, err := b.taskConversation(input.TargetSessionID)
		if err != nil {
			return nil, err
		}
		if row.Archived {
			return nil, errors.New("任务已归档，未重新启动")
		}
		groupLane := false
		if control == b.sessions {
			active, err := b.contextFor(ctx, map[string]string{"sessionId": row.ID})
			if err != nil {
				return nil, err
			}
			groupLane = active.Role == "group"
			// Completion is a host-authored continuation. Local group rounds,
			// however, carry the owner's standing request to the selected members.
			// They use each member's permissions, not an IM guest's read-only policy.
			digest := sha256.Sum256([]byte(input.Message))
			replyTaskID := ""
			if input.ReplyTaskID != "" {
				child, err := b.control.store.Conversation(input.ReplyChildSessionID)
				if err != nil {
					return nil, err
				}
				active, err := b.contextFor(ctx, map[string]string{"sessionId": row.ID})
				if err != nil {
					return nil, err
				}
				if child.OneBotTaskID != input.ReplyTaskID || child.OneBotOwnerID != active.BotID {
					return nil, errors.New("任务交互通知归属无效")
				}
				replyTaskID = input.ReplyTaskID
			}
			grant := oneBotInputGrant{SessionID: row.ID, ConnectionID: "task-completion", ReplyTaskID: replyTaskID, Digest: hex.EncodeToString(digest[:])}
			if groupLane {
				grant.ConnectionID, grant.Owner, grant.Group, grant.Principal = "local-group", true, true, "entrusted"
			}
			if err = b.grantInput(input.ClientID, grant); err != nil {
				return nil, err
			}
		}
		var attachments []string
		if len(input.Attachments) > 0 {
			if !groupLane {
				return nil, errors.New("只有群专线可以接收群聊附件")
			}
			attachments, err = b.groupAttachments(row.ID, input.ClientID, input.Attachments)
			if err != nil {
				return nil, err
			}
		}
		if control.engine == nil {
			return nil, errors.New("Chat 执行器尚未就绪")
		}
		if _, err = control.engine.Open(ctx, row.ID, 0); err != nil {
			return nil, err
		}
		defer control.engine.Release(row.ID, 0)
		mode := input.DeliveryMode
		if mode == "" {
			mode = "queue"
		}
		result, err := control.engine.DispatchPresentedInput(ctx, row.ID, input.ClientID, input.Message, mode, chat.NativeInputPresentation{Text: input.PersistedContent, Hidden: control == b.sessions || input.DispatcherSessionID == "" && (strings.HasPrefix(input.ClientID, "bot-delegation-unpause:") || strings.HasPrefix(input.ClientID, "bot-delegation-resume:")), ReviewRequest: input.AutoReviewUserText, OriginSessionID: input.DispatcherSessionID, ToolsDisabled: input.ToolsDisabled}, attachments)
		return json.RawMessage(result), err
	case "timeline", "wait", "abort", "inspect", "hold", "pause", "resume", "queue", "interaction":
		control, row, err := b.taskConversation(input.SessionID)
		if err != nil {
			return nil, err
		}
		if control.engine == nil {
			return nil, errors.New("Chat 执行器尚未就绪")
		}
		if _, err = control.engine.Open(ctx, row.ID, 0); err != nil {
			return nil, err
		}
		defer control.engine.Release(row.ID, 0)
		if method == "interaction" {
			if input.Kind != "permission" && input.Kind != "question" {
				return nil, errors.New("不支持的任务交互类型")
			}
			result, err := control.engine.Command(ctx, row.ID, input.Kind, map[string]any{"id": input.NativeID, "requestId": input.RequestID, "allow": input.Allow, "answers": input.Answers, "scope": "once"})
			return json.RawMessage(result), err
		}
		if method == "inspect" {
			result, err := control.engine.Command(ctx, row.ID, "execution.inspect", nil)
			return json.RawMessage(result), err
		}
		if method == "hold" {
			result, err := control.engine.Command(ctx, row.ID, "task.hold", map[string]any{"held": input.Held, "execution": input.Execution})
			return json.RawMessage(result), err
		}
		if method == "pause" || method == "resume" {
			_, err := control.engine.Command(ctx, row.ID, method, nil)
			if err != nil {
				return nil, err
			}
			result, err := control.engine.Command(ctx, row.ID, "execution.inspect", nil)
			return json.RawMessage(result), err
		}
		if method == "queue" {
			_, err := control.engine.Command(ctx, row.ID, "queue", map[string]any{"id": input.RequestID, "action": input.Action, "text": input.Text})
			if err != nil {
				return nil, err
			}
			result, err := control.engine.Command(ctx, row.ID, "execution.inspect", nil)
			return json.RawMessage(result), err
		}
		if method == "timeline" {
			result, err := control.engine.Command(ctx, row.ID, "timeline.append", raw)
			return json.RawMessage(result), err
		}
		if method == "abort" {
			result, err := control.engine.Command(ctx, row.ID, "abort.input", map[string]any{"id": input.RequestID, "execution": input.Execution})
			return json.RawMessage(result), err
		}
		// Event-driven wait with a durable per-input query: another user turn ending
		// later cannot replace this task's result. Reopening never resubmits a prompt.
		result, err := control.engine.Command(ctx, row.ID, "input.result", map[string]string{"id": input.RequestID})
		if err != nil {
			return nil, err
		}
		var terminal struct {
			Status string `json:"status"`
		}
		if err = json.Unmarshal(result, &terminal); err != nil {
			return nil, err
		}
		snapshot, err := control.engine.Command(ctx, row.ID, "execution.inspect", nil)
		if err != nil {
			return nil, err
		}
		if terminal.Status != "" {
			return map[string]any{"terminal": json.RawMessage(result), "cursor": input.Cursor, "state": json.RawMessage(snapshot)}, nil
		}
		wait, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		page, err := control.engine.WaitEvents(wait, row.ID, input.Cursor)
		if err != nil && !errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return map[string]any{"cursor": page.Cursor, "state": json.RawMessage(snapshot)}, nil
	default:
		return nil, errors.New("不支持的 OneBot 原生任务操作")
	}
}
