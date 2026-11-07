//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"winone/internal/ai"
	"winone/internal/chat"
)

// legacyGhostPermission keeps the old errand vocabulary at the adapter edge.
// `plan` is Pi's read-only Ask session; it must never become full access.
func legacyGhostPermission(value string) (string, error) {
	switch value {
	case "plan":
		// Store creates Ask + the Host-owned read-only marker in the initial
		// catalog transaction. Do not briefly create an unrestricted Ask task.
		return "plan", nil
	case "":
		return "ask", nil
	case "acceptEdits", "ask", "auto", "bypassPermissions":
		return value, nil
	default:
		return "ask", nil
	}
}

func (g *chatGhosts) legacyRoute(route *ghostTaskRoute) (ghostTaskRoute, error) {
	if route == nil {
		return g.resolveTaskRoute(nil, "")
	}
	value := *route
	if value.AgentKind == "" {
		value.AgentKind = "pi"
	}
	if value.ProviderID == "" || value.Model == "" {
		base, err := g.resolveTaskRoute(nil, "")
		if err != nil {
			return ghostTaskRoute{}, err
		}
		if value.ProviderID == "" {
			value.ProviderID = base.ProviderID
		}
		if value.Model == "" {
			value.Model = base.Model
		}
	}
	if len(value.FastMode) == 0 {
		value.FastMode = json.RawMessage(`false`)
	}
	// Share ordinary provider/model/Fast admission and retain provider effort
	// words unchanged. The execution runtime owns its model's effort catalog.
	return g.resolveTaskRoute(&value, "")
}

func (g *chatGhosts) newLegacyConversation(directory, title string, route *ghostTaskRoute, permission string) (chat.Conversation, error) {
	resolved, err := g.legacyRoute(route)
	if err != nil {
		return chat.Conversation{}, err
	}
	var fast bool
	if len(resolved.FastMode) > 0 {
		if err = json.Unmarshal(resolved.FastMode, &fast); err != nil {
			return chat.Conversation{}, ghostTaskFail("ROUTE_UNAVAILABLE", "模型快速模式无效")
		}
	}
	projectID := ""
	if directory != "" {
		for _, project := range g.control.store.Snapshot().Projects {
			if strings.EqualFold(filepath.Clean(project.Path), filepath.Clean(directory)) {
				projectID, directory = project.ID, ""
				break
			}
		}
	}
	permission, err = legacyGhostPermission(permission)
	if err != nil {
		return chat.Conversation{}, err
	}
	return g.control.store.NewConfiguredConversation(projectID, directory, title,
		ai.Selection{Provider: resolved.ProviderID, Model: resolved.Model},
		permission, resolved.Effort, fast)
}

func (g *chatGhosts) newLegacyConversationFromRequest(directory, title string, route *ghostTaskRoute, permission string) (chat.Conversation, error) {
	return g.newLegacyConversation(directory, title, route, permission)
}

// These fields are a snapshot of Host-owned task preferences, never a plugin
// request override. Pointer/raw fields distinguish follow-default from an
// explicit empty effort or Fast=false when an existing task is reused.
type ghostLegacyTaskConfig struct {
	AgentKind, ProviderID, Model string
	Effort                       *string
	FastMode                     json.RawMessage
}

type ghostLegacyErrandRequest struct {
	GhostID, Origin, Message, Title, SessionKey, WorkingDir, SourceSessionID string
	PersistedContent, PermissionMode                                         string
	Route                                                                    *ghostTaskRoute
	TaskConfig                                                               *ghostLegacyTaskConfig
}

func legacyConversationPermission(row chat.Conversation) string {
	if row.LegacyPlan {
		return "plan"
	}
	return row.Permission
}

func legacyConversationRoute(row chat.Conversation) ghostTaskRoute {
	fast, _ := json.Marshal(row.FastMode)
	return ghostTaskRoute{AgentKind: "pi", ProviderID: row.Model.Provider, Model: row.Model.Model, Effort: row.ThinkingLevel, FastMode: fast}
}

func (g *chatGhosts) resolveLegacyErrandRoute(request ghostLegacyErrandRequest, existing chat.Conversation) (ghostTaskRoute, error) {
	if request.Route != nil && request.TaskConfig == nil {
		return g.legacyRoute(request.Route)
	}
	config := request.TaskConfig
	var route *ghostTaskRoute
	if existing.ID != "" && !existing.Archived && (config == nil || config.Model == "") {
		value := legacyConversationRoute(existing)
		route = &value
	} else if config != nil && config.ProviderID != "" && config.Model != "" {
		route = &ghostTaskRoute{AgentKind: "pi", ProviderID: config.ProviderID, Model: config.Model, FastMode: json.RawMessage(`false`)}
	} else if request.SourceSessionID != "" {
		if _, source, err := g.targetConversation(request.SourceSessionID); err == nil {
			value := legacyConversationRoute(source)
			route = &value
		} else {
			return ghostTaskRoute{}, ghostTaskFail("ROUTE_UNAVAILABLE", "发起任务已不可用，请重新发起或在插件设置中选择模型")
		}
	}
	if route == nil {
		value, err := g.legacyRoute(nil)
		if err != nil {
			return ghostTaskRoute{}, err
		}
		route = &value
	}
	if config != nil {
		if config.AgentKind != "" {
			route.AgentKind = config.AgentKind
		}
		if config.ProviderID != "" {
			route.ProviderID = config.ProviderID
		}
		if config.Model != "" {
			route.Model = config.Model
		}
		if config.Effort != nil {
			route.Effort = *config.Effort
		}
		if len(config.FastMode) > 0 {
			route.FastMode = config.FastMode
		}
	}
	return g.legacyRoute(route)
}

// ensureLegacyErrand implements only the old sessionKey mapping. Changed
// explicit settings allocate a new ordinary task, never overwrite its history
// or replace the mapping before the catalog write succeeds.
func (g *chatGhosts) ensureLegacyErrand(request ghostLegacyErrandRequest) (chat.Conversation, error) {
	key := g.errandKey(request.GhostID, request.SessionKey)
	g.mu.Lock()
	id := g.errands[key]
	g.mu.Unlock()
	conversation, _ := g.control.store.Conversation(id)
	if conversation.ID != "" && g.legacySessionBusy(conversation.ID) {
		return chat.Conversation{}, ghostTaskFail("BUSY", "插件的上一项任务还没完成")
	}
	route, err := g.resolveLegacyErrandRoute(request, conversation)
	if err != nil {
		return chat.Conversation{}, err
	}
	permission, err := legacyGhostPermission(request.PermissionMode)
	if err != nil {
		return chat.Conversation{}, err
	}
	directory := request.WorkingDir
	if directory != "" {
		if !filepath.IsAbs(directory) {
			return chat.Conversation{}, ghostTaskFail("INVALID_REQUEST", "工作目录必须是绝对路径")
		}
		directory, err = filepath.EvalSymlinks(directory)
		if err != nil {
			return chat.Conversation{}, ghostTaskFail("INVALID_REQUEST", "工作目录不存在或无法读取")
		}
		if info, e := os.Stat(directory); e != nil || !info.IsDir() {
			return chat.Conversation{}, ghostTaskFail("INVALID_REQUEST", "工作目录不是可用文件夹")
		}
	}
	wantDirectory := directory
	if wantDirectory == "" && conversation.ID != "" {
		wantDirectory = filepath.Join(g.control.store.Preferences().WorkspaceDirectory, conversation.ID)
	}
	var fast bool
	_ = json.Unmarshal(route.FastMode, &fast) // Shared route admission already checked it.
	if conversation.ID != "" && !conversation.Archived && !conversation.ModelDeferred &&
		conversation.Model.Provider == route.ProviderID && conversation.Model.Model == route.Model &&
		conversation.ThinkingLevel == route.Effort && conversation.FastMode == fast && legacyConversationPermission(conversation) == permission &&
		strings.EqualFold(filepath.Clean(conversation.Directory), filepath.Clean(wantDirectory)) {
		return conversation, nil
	}
	if conversation.ModelDeferred {
		return chat.Conversation{}, ghostTaskFail("BUSY", "任务正在切换模型，请稍后重试")
	}
	title := request.Title
	if title == "" {
		title = request.GhostID
	}
	conversation, err = g.newLegacyConversationFromRequest(directory, title, &route, request.PermissionMode)
	if err != nil {
		return chat.Conversation{}, err
	}
	g.mu.Lock()
	if g.errands == nil {
		g.errands = make(map[string]string)
	}
	g.errands[key] = conversation.ID
	if err = g.saveErrandsLocked(); err != nil {
		if id == "" {
			delete(g.errands, key)
		} else {
			g.errands[key] = id
		}
		g.mu.Unlock()
		// The newly created task is kept visible as recovery evidence; the old
		// stable mapping and all of its history remain authoritative.
		return chat.Conversation{}, ghostTaskFail("STORE_FAILED", "插件任务映射保存失败，旧任务与新建任务均已保留，请重试")
	}
	g.mu.Unlock()
	return conversation, nil
}

func (g *chatGhosts) legacySessionBusy(id string) bool {
	if g.control.engine == nil {
		return false
	}
	activity, busy := g.control.engine.Activity()[id]
	return busy && (activity.Paused || activity.Phase == "running" || activity.Phase == "needs-interaction")
}

func (g *chatGhosts) dispatchLegacyInput(ctx context.Context, c *chatControl, conversation chat.Conversation, text, persistedContent, sourceSessionID string) (string, bool, error) {
	if _, err := c.engine.Open(ctx, conversation.ID, 0); err != nil {
		return "", false, err
	}
	defer c.engine.Release(conversation.ID, 0)
	requestID := ghostRequestID()
	presentation := chat.NativeInputPresentation{OriginSessionID: sourceSessionID}
	if persistedContent != "" {
		presentation.ReviewRequest = json.RawMessage(persistedContent)
	}
	raw, err := c.engine.DispatchPresentedInput(ctx, conversation.ID, requestID, text, "queue", presentation)
	if err != nil {
		return "", false, err
	}
	var receipt struct{ Accepted, Queued bool }
	if err = json.Unmarshal(raw, &receipt); err != nil || !receipt.Accepted {
		return "", false, ghostTaskFail("DELIVERY_UNCERTAIN", "没有收到确定的输入受理回执，请打开任务核对；不要自动重发")
	}
	return requestID, receipt.Queued, nil
}

func (g *chatGhosts) startLegacyErrand(ctx context.Context, request ghostLegacyErrandRequest) (map[string]any, error) {
	conversation, err := g.ensureLegacyErrand(request)
	if err != nil {
		return map[string]any{"ok": false, "errorCode": errorCodeOf(err, "SESSION_UNAVAILABLE"), "message": err.Error()}, nil
	}
	if g.legacySessionBusy(conversation.ID) {
		return map[string]any{"ok": false, "errorCode": "BUSY", "message": "插件的上一项任务还没完成"}, nil
	}
	requestID, queued, err := g.dispatchLegacyInput(ctx, g.control, conversation, request.Message, request.PersistedContent, request.SourceSessionID)
	if err != nil {
		return map[string]any{"ok": false, "errorCode": "TURN_FAILED", "message": err.Error()}, nil
	}
	if queued {
		// Do not pick up the foreign active turn's answer as this errand's result.
		return map[string]any{"ok": false, "errorCode": "BUSY", "message": "目标任务恰好正忙，本次输入已排队但结果无法自动取回；请打开任务查看结果"}, nil
	}
	return map[string]any{"ok": true, "sessionId": conversation.ID, "requestId": requestID,
		"agentKind": "pi", "model": conversation.Model.Model}, nil
}

func (g *chatGhosts) waitLegacyErrand(ctx context.Context, sessionID, requestID string) (map[string]any, error) {
	conversation, err := g.control.store.Conversation(sessionID)
	if err != nil {
		return map[string]any{"ok": false, "errorCode": "SESSION_UNAVAILABLE", "message": err.Error()}, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 25*time.Minute)
	defer cancel()
	if _, err = g.control.engine.Open(ctx, sessionID, 0); err != nil {
		return map[string]any{"ok": false, "errorCode": "SESSION_UNAVAILABLE", "message": err.Error()}, nil
	}
	// Keep this result-read lease through the final query. The inner event
	// waiter releases its own lease and may otherwise retire the idle Worker.
	defer g.control.engine.Release(sessionID, 0)
	var terminal struct{ RequestID, Status, ResultText, OutputMessageID string }
	readTerminal := func() error {
		raw, err := g.control.engine.Command(ctx, sessionID, "input.result", map[string]string{"id": requestID})
		if err == nil {
			err = json.Unmarshal(raw, &terminal)
		}
		return err
	}
	if err = readTerminal(); err != nil {
		return map[string]any{"ok": false, "errorCode": "TURN_FAILED", "message": err.Error()}, nil
	}
	// A fast result can already have settled before errand-wait arrives, or
	// have been restored from the native transcript after Worker retirement.
	if terminal.Status == "" {
		if _, err = g.waitGhostTurn(ctx, conversation, requestID); err != nil {
			code := "TURN_FAILED"
			if errors.Is(err, context.DeadlineExceeded) {
				code = "TIMEOUT"
			}
			return map[string]any{"ok": false, "errorCode": code, "message": err.Error()}, nil
		}
		err = readTerminal()
	}
	if err != nil || terminal.RequestID != requestID || terminal.Status != "completed" {
		return map[string]any{"ok": false, "errorCode": "TURN_FAILED", "message": "本次派活没有可核实的完成回执，请打开任务查看"}, nil
	}
	text := terminal.ResultText
	if terminal.OutputMessageID != "" {
		// The terminal receipt intentionally bounds its text for transport.
		// Read its exact persisted message, so the original errand Slot can apply
		// its 64K limit and explicit truncation marker rather than silently losing
		// the tail at the native receipt's 60K boundary.
		raw, queryErr := g.control.engine.Command(ctx, sessionID, "state", map[string]int{"limit": 50})
		var page struct {
			Messages []struct {
				ID, Role, TurnID string
				Content          json.RawMessage
			}
		}
		found := false
		if queryErr == nil && json.Unmarshal(raw, &page) == nil {
			for _, message := range page.Messages {
				if message.ID != terminal.OutputMessageID || message.Role != "assistant" || message.TurnID != requestID {
					continue
				}
				if json.Unmarshal(message.Content, &text) != nil {
					var parts []struct{ Type, Text string }
					if json.Unmarshal(message.Content, &parts) != nil {
						break
					}
					text = ""
					for _, part := range parts {
						if part.Type == "text" {
							text += part.Text
						}
					}
				}
				found = true
				break
			}
		}
		if !found {
			return map[string]any{"ok": false, "errorCode": "TURN_FAILED", "message": "本次派活的原始回复无法读取，请打开任务查看；未以其他轮次回复代替"}, nil
		}
	}
	text = strings.TrimSpace(text)
	if text == "" {
		text = "(本轮没有可取回的文字回复)"
	}
	return map[string]any{"ok": true, "sessionId": conversation.ID, "text": text,
		"agentKind": "pi", "model": conversation.Model.Model}, nil
}

// runLegacyAgent retains the receiving task's model and permission authority.
// A plugin click ticket never substitutes the plugin's default configuration for
// the owner's task; only the original Agent Slot decides whether to reveal it.
func (g *chatGhosts) runLegacyAgent(ctx context.Context, raw json.RawMessage) (any, error) {
	var request struct{ GhostID, SourceSessionID, Mode, Prompt, PersistedContent, Title string }
	if err := json.Unmarshal(raw, &request); err != nil {
		return nil, err
	}
	failure := func(err error) (any, error) {
		return map[string]any{"ok": false, "errorCode": errorCodeOf(err, "SESSION_UNAVAILABLE"), "message": err.Error()}, nil
	}
	c, source, err := g.targetConversation(request.SourceSessionID)
	if err != nil {
		return failure(ghostTaskFail("NOT_FOUND", "原任务已不可用"))
	}
	if source.Archived {
		return failure(ghostTaskFail("ARCHIVED", "原任务已归档，请先恢复任务"))
	}
	if source.ModelDeferred {
		return failure(ghostTaskFail("BUSY", "原任务正在切换模型，请稍后重试"))
	}
	route := legacyConversationRoute(source)
	if _, err = g.legacyRoute(&route); err != nil {
		return failure(ghostTaskFail("MODEL_UNAVAILABLE", err.Error()))
	}
	disposition := "active"
	if _, live := c.engine.Activity()[source.ID]; !live {
		if source.HasMessages {
			disposition = "resumed"
		} else {
			disposition = "created"
		}
	}
	switch request.Mode {
	case "new":
		c = g.control
		source, err = g.newLegacyConversationFromRequest(source.Directory, request.Title, &route, legacyConversationPermission(source))
		if err != nil {
			return failure(err)
		}
		disposition = "created"
	case "fork":
		if !source.HasMessages {
			return failure(ghostTaskFail("NO_PRIOR_ASSISTANT", "原任务还没有可用于分叉的 Agent 回复"))
		}
		state, err := c.engine.Open(ctx, source.ID, 0)
		if err != nil {
			return failure(err)
		}
		defer c.engine.Release(source.ID, 0)
		var page struct {
			Messages []struct {
				ID, Role         string
				PresentationOnly bool
			}
		}
		if err = json.Unmarshal(state, &page); err != nil {
			return failure(err)
		}
		latest := ""
		for _, message := range page.Messages {
			if message.Role == "assistant" && !message.PresentationOnly {
				latest = message.ID
			}
		}
		if latest == "" {
			return failure(ghostTaskFail("NO_PRIOR_ASSISTANT", "原任务还没有可用于分叉的 Agent 回复"))
		}
		original := source
		if source, err = c.engine.Fork(ctx, source.ID, latest, request.Title); err != nil {
			return failure(ghostTaskFail("FORK_FAILED", err.Error()))
		}
		if source, err = c.store.UpdateConversation(source.ID, func(value *chat.Conversation) error {
			value.Model, value.Permission, value.LegacyPlan = original.Model, original.Permission, original.LegacyPlan
			value.ThinkingLevel, value.FastMode = original.ThinkingLevel, original.FastMode
			return nil
		}); err != nil {
			return failure(err)
		}
		disposition = "forked"
	case "continue":
	default:
		return failure(ghostTaskFail("INVALID_REQUEST", "Agent 回合模式无效"))
	}
	_, queued, err := g.dispatchLegacyInput(ctx, c, source, request.Prompt, request.PersistedContent, request.SourceSessionID)
	if err != nil {
		return failure(err)
	}
	if queued && request.Mode == "continue" {
		disposition = "queued"
	}
	return map[string]any{"ok": true, "sessionId": source.ID, "disposition": disposition}, nil
}

func (g *chatGhosts) findLegacyWorkspace(dirAbs string) (any, error) {
	if !filepath.IsAbs(dirAbs) {
		return nil, ghostTaskFail("INVALID_REQUEST", "工作区目录必须是绝对路径")
	}
	directory, err := filepath.EvalSymlinks(dirAbs)
	if err != nil {
		return nil, ghostTaskFail("DIR_NOT_FOUND", "工作区目录不存在或无法读取")
	}
	var selected chat.Conversation
	for _, row := range g.control.store.Snapshot().Conversations {
		// Plugin-created tasks are ordinary tasks too. Only archived/hidden
		// tasks and Orca Workers cannot be the visible workspace entry.
		if !row.Archived && !row.Hidden && row.OrcaRole != "worker" &&
			strings.EqualFold(filepath.Clean(row.Directory), filepath.Clean(directory)) &&
			(selected.ID == "" || row.UpdatedAt > selected.UpdatedAt) {
			selected = row
		}
	}
	if selected.ID != "" {
		return map[string]any{"sessionId": selected.ID}, nil
	}
	return map[string]any{}, nil
}

func (g *chatGhosts) createLegacyDraft(dirAbs, title string, route *ghostTaskRoute, permission string) (any, error) {
	conversation, err := g.newLegacyConversationFromRequest(dirAbs, title, route, permission)
	if err != nil {
		return nil, err
	}
	// No Open/dispatch/model input here: a workspace request creates a draft.
	return map[string]any{"sessionId": conversation.ID}, nil
}

func errorCodeOf(err error, fallback string) string {
	var failure *ghostTaskFailure
	if errors.As(err, &failure) && failure.Code != "" {
		return failure.Code
	}
	return fallback
}
