//go:build windows

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"winone/internal/ai"
	"winone/internal/chat"
)

// Cindy's Orca lifecycle/creation services own team admission and leases. This
// authenticated seam supplies real ordinary Chat sessions, not simulated Worker
// states. A companion Lead keeps its store; its Workers are ordinary Chat tasks.
func (g *chatGhosts) teamNative(ctx context.Context, raw json.RawMessage) (any, error) {
	var input struct {
		Method, TaskID, TeamID, Role, PermissionMode, Text, ClientID, OriginSessionID, Delivery, RequestID, Action, QueuedMessageID string
		Params                                                                                                                      json.RawMessage
		ExpectedRevision                                                                                                            uint64
		ExpectedCompletedAt                                                                                                         int64
		Cursor                                                                                                                      uint64
		Images                                                                                                                      []string
		Hidden, ToolsDisabled                                                                                                       bool
		Options                                                                                                                     struct {
			ID, AgentKind, WorkingDir, WorkspaceKind, Model, ProviderID, Effort, PermissionMode, Title, RemoteHostID, AgentDeviceID string
			FastMode                                                                                                                json.RawMessage
			VendorOptions                                                                                                           struct{ OrcaWorkflowID, OrcaLeadSessionID, OrcaWorkerID string }
		}
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	c := g.control
	if input.Method == "references" {
		// Original cleanup guards need every local native task, not the stub
		// session rows that merely bind plugin receipts and worktree metadata.
		rows := []map[string]any{}
		appendStore := func(store *chat.Store, source string, current bool) {
			for _, row := range store.Snapshot().Conversations {
				status := "active"
				if row.Archived {
					status = "archived"
				}
				rows = append(rows, map[string]any{"id": row.ID, "workingDir": row.Directory, "worktreePath": row.Directory, "status": status, "source": source, "currentDatabase": current})
			}
		}
		appendStore(c.store, "chat", true)
		if c.onebot != nil && c.onebot.sessions != nil && c.onebot.sessions.store != nil {
			appendStore(c.onebot.sessions.store, "bot", false)
		}
		return rows, nil
	}
	if input.Method == "settings" {
		return c.store.Preferences().RuntimeSettings(), nil
	}
	if input.Method == "changed" {
		g.pushEvent("team-changed", map[string]string{"sessionId": input.TaskID})
		return true, nil
	}
	if input.Method == "models" {
		config, err := c.aiConfiguration()
		if err != nil {
			return nil, err
		}
		return config.Providers, nil
	}
	if input.Method == "create" {
		opts := input.Options
		if opts.AgentKind != "pi" || opts.RemoteHostID != "" || opts.AgentDeviceID != "" {
			return nil, ghostTaskFail("UNSUPPORTED_CAPABILITY", "WinOne 协作使用本机 Pi")
		}
		leadControl, lead, err := g.targetConversation(opts.VendorOptions.OrcaLeadSessionID)
		if err != nil || leadControl == nil || lead.Archived || lead.OrcaRole != "lead" || lead.OrcaTeamID != opts.VendorOptions.OrcaWorkflowID {
			return nil, ghostTaskFail("TARGET_UNAVAILABLE", "主任务已不属于当前团队")
		}
		route, err := g.resolveTaskRoute(&ghostTaskRoute{AgentKind: "pi", ProviderID: opts.ProviderID, Model: opts.Model, Effort: opts.Effort, FastMode: opts.FastMode}, "")
		if err != nil {
			return nil, err
		}
		var fast bool
		_ = json.Unmarshal(route.FastMode, &fast)
		prepared := chat.Conversation{ID: opts.ID, Directory: opts.WorkingDir, Title: opts.Title, Model: ai.Selection{Provider: route.ProviderID, Model: route.Model}, Permission: opts.PermissionMode, ThinkingLevel: route.Effort, FastMode: fast, OrcaTeamID: opts.VendorOptions.OrcaWorkflowID, OrcaLeadID: lead.ID, OrcaWorkerID: opts.VendorOptions.OrcaWorkerID}
		row, err := c.store.NewWorkerConversation(prepared, lead)
		if err != nil {
			return nil, err
		}
		return g.teamView(row), nil
	}
	control, row, err := g.targetConversation(input.TaskID)
	if err != nil {
		return nil, err
	}
	switch input.Method {
	case "get":
		return g.teamView(row), nil
	case "directory":
		var body struct{ Directory string }
		if err := json.Unmarshal(input.Params, &body); err != nil {
			return nil, err
		}
		if !filepath.IsAbs(body.Directory) || strings.HasPrefix(body.Directory, `\\`) || strings.HasPrefix(body.Directory, "//") {
			return nil, ghostTaskFail("INVALID_PARAMS", "Worker 工作目录必须是本机绝对路径")
		}
		resolved, err := filepath.EvalSymlinks(body.Directory)
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(resolved)
		if err != nil || !info.IsDir() {
			return nil, ghostTaskFail("INVALID_PARAMS", "Worker 工作目录不可用")
		}
		return map[string]string{"directory": resolved}, nil
	case "role":
		updated, err := control.store.UpdateConversation(row.ID, func(value *chat.Conversation) error {
			if input.Role != "" && input.Role != "lead" {
				return errors.New("Invalid Orca role")
			}
			if input.Role == "lead" && value.OrcaRole == "worker" {
				return ghostTaskFail("PERMISSION_DENIED", "Worker 不能另建团队")
			}
			if input.Role == "lead" {
				value.OrcaRole, value.OrcaTeamID, value.OrcaWorkerPermission = "lead", input.TeamID, input.PermissionMode
			} else {
				value.OrcaRole = ""
				value.OrcaWorkerPermission = ""
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		return g.teamView(updated), nil
	case "identity":
		var body struct{ Title string }
		if err := json.Unmarshal(input.Params, &body); err != nil {
			return nil, err
		}
		updated, err := control.store.UpdateConversation(row.ID, func(value *chat.Conversation) error {
			if !value.TitleManual {
				value.Title = body.Title
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		return g.teamView(updated), nil
	case "facts":
		// Read-only inspection never opens or resumes a closed Pi runtime. The
		// sealed Bun service reads its Host-listed transcript with the existing
		// stable-branch reader when the live process has already been released.
		result := map[string]any{"row": g.teamView(row), "live": false, "activity": control.engine.Activity()[row.ID]}
		if state, err := control.engine.Command(ctx, row.ID, "execution.inspect", nil); err == nil {
			result["live"], result["state"] = true, json.RawMessage(state)
		}
		return result, nil
	case "stop":
		if row.Archived {
			return map[string]any{"stopped": true, "hadActiveTurn": false}, nil
		}
	}
	if control.engine == nil {
		return nil, ghostTaskFail("HOST_NOT_READY", "任务执行服务不可用")
	}
	if _, err = control.engine.Open(ctx, row.ID, 0); err != nil {
		return nil, err
	}
	defer control.engine.Release(row.ID, 0)
	switch input.Method {
	case "archive":
		_, err := control.engine.ConfigureConversationWithCheck(ctx, row.ID, func(snapshot json.RawMessage) error {
			var state struct {
				Running                       bool
				Pending, Approvals, Questions []json.RawMessage
			}
			if json.Unmarshal(snapshot, &state) != nil || state.Running || len(state.Pending)+len(state.Approvals)+len(state.Questions) > 0 {
				return ghostTaskFail("WORKER_STATE_CHANGED", "Worker has active or queued input")
			}
			return nil
		}, func(value *chat.Conversation) error { value.Archived = true; return nil })
		return err == nil, err
	case "send":
		originControl, origin, err := g.targetConversation(input.OriginSessionID)
		if err != nil || originControl == nil || row.Archived {
			return nil, ghostTaskFail("TARGET_UNAVAILABLE", "协作来源不可用")
		}
		related := row.OrcaRole == "worker" && row.OrcaLeadID == origin.ID && row.OrcaTeamID == origin.OrcaTeamID
		related = related || row.OrcaRole == "lead" && origin.OrcaLeadID == row.ID && row.OrcaTeamID == origin.OrcaTeamID
		if !related {
			return nil, ghostTaskFail("PERMISSION_DENIED", "协作输入不属于当前团队")
		}
		if !strings.HasPrefix(input.ClientID, "orca:") || input.Delivery != "queue" && input.Delivery != "steer" {
			return nil, ghostTaskFail("INVALID_REQUEST", "协作输入身份无效")
		}
		if control != g.control {
			digest := sha256.Sum256([]byte(input.Text))
			if err := g.control.onebot.grantInput(input.ClientID, oneBotInputGrant{SessionID: row.ID, ConnectionID: "orca:" + origin.OrcaWorkerID, Principal: "autonomous", Digest: hex.EncodeToString(digest[:])}); err != nil {
				return nil, err
			}
		}
		shown := input.Text
		value, err := control.engine.DispatchPresentedInput(ctx, row.ID, input.ClientID, input.Text, input.Delivery, chat.NativeInputPresentation{Text: &shown, Hidden: input.Hidden, ToolsDisabled: input.ToolsDisabled, OriginSessionID: origin.ID, ReviewRequest: json.RawMessage(`{"kind":"delegated-continuation"}`)}, input.Images)
		return json.RawMessage(value), err
	case "wait":
		return control.waitInputResult(ctx, row.ID, input.RequestID, input.Cursor)
	case "stop":
		state, err := control.engine.Command(ctx, row.ID, "execution.inspect", nil)
		if err != nil {
			return nil, err
		}
		var active struct {
			RequestID string
			Running   bool
			Execution json.RawMessage
		}
		if err = json.Unmarshal(state, &active); err != nil {
			return nil, err
		}
		if active.Running {
			_, err = control.engine.Command(ctx, row.ID, "abort.input", map[string]any{"id": active.RequestID, "execution": active.Execution})
		}
		return map[string]any{"stopped": err == nil, "hadActiveTurn": active.Running}, err
	case "queue":
		fields := map[string]any{}
		if len(input.Params) > 0 && json.Unmarshal(input.Params, &fields) != nil {
			return nil, errors.New("协作队列参数无效")
		}
		fields["action"], fields["expectedOriginSessionId"] = input.Action, input.OriginSessionID
		value, err := control.engine.Command(ctx, row.ID, "queue", fields)
		return json.RawMessage(value), err
	case "release":
		// Recheck the exact successful input and its completion credential inside
		// the existing session gate immediately before the reversible archive.
		var terminal struct {
			Status, RequestID string
			CompletedAt       int64
		}
		value, err := control.engine.Command(ctx, row.ID, "input.result", map[string]string{"id": input.RequestID})
		if err != nil {
			return nil, err
		}
		if json.Unmarshal(value, &terminal) != nil || terminal.Status != "completed" || terminal.CompletedAt != input.ExpectedCompletedAt || terminal.CompletedAt <= 0 {
			return nil, ghostTaskFail("STALE_REVISION", "Worker 没有匹配的成功完成凭据")
		}
		_, err = control.engine.ConfigureConversationWithCheck(ctx, row.ID, func(snapshot json.RawMessage) error {
			var state struct {
				RequestID                     string
				Running                       bool
				Pending, Approvals, Questions []json.RawMessage
			}
			if json.Unmarshal(snapshot, &state) != nil || state.Running || len(state.Pending)+len(state.Approvals)+len(state.Questions) > 0 || state.RequestID != input.RequestID {
				return ghostTaskFail("STALE_REVISION", "Worker 的输入或等待状态已变化")
			}
			return nil
		}, func(value *chat.Conversation) error {
			if value.Revision != input.ExpectedRevision {
				return ghostTaskFail("STALE_REVISION", "Worker 配置已变化")
			}
			value.Archived = true
			return nil
		})
		return err == nil, err
	}
	return nil, ghostTaskFail("INVALID_REQUEST", "Unknown native Orca operation")
}

// Reopening an archived Worker must restore its recorded working directory
// before Pi or file tools use it. This private request carries the Host's row,
// never a directory chosen by a plugin or a replacement fallback workspace.
func (g *chatGhosts) restoreWorkerWorktree(ctx context.Context, row chat.Conversation) error {
	if row.OrcaWorkerID == "" {
		return nil
	}
	params, _ := json.Marshal(map[string]string{"sessionId": row.ID, "workingDir": row.Directory})
	value, err := g.call(ctx, "worktree-readiness", params)
	if err != nil {
		return err
	}
	var result struct {
		Ready  bool
		Reason string
	}
	if err = json.Unmarshal(value, &result); err != nil {
		return err
	}
	if !result.Ready {
		return errors.New("工作树恢复未完成，请保留当前任务并重试：" + result.Reason)
	}
	return nil
}

func (g *chatGhosts) teamView(row chat.Conversation) map[string]any {
	value := ghostConversationView(row)
	value["id"], value["directory"], value["projectId"] = row.ID, row.Directory, row.ProjectID
	value["sessionDirectory"], value["sessionFile"] = row.SessionDirectory, row.SessionFile
	value["orcaRole"], value["orcaTeamId"], value["orcaLeadId"], value["orcaWorkerId"], value["workerPermissionMode"] = row.OrcaRole, row.OrcaTeamID, row.OrcaLeadID, row.OrcaWorkerID, row.OrcaWorkerPermission
	value["hasMessages"] = row.HasMessages
	for key, text := range map[string]string{"createdAt": row.CreatedAt, "updatedAt": row.UpdatedAt} {
		if at, err := time.Parse(time.RFC3339Nano, text); err == nil {
			value[key] = at.UnixMilli()
		}
	}
	return value
}
