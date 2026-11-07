//go:build windows

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"winone/internal/chat"
)

// The plugin slots' requests that touch WinOne itself. The slots already checked
// the plugin's declaration, click ticket and rate; these only do the named work on
// real conversations, under each conversation's own permission mode.

const ghostTurnBudget = 30 * time.Minute

func ghostRequestID() string {
	data := make([]byte, 16)
	_, _ = rand.Read(data)
	return "plugin-" + hex.EncodeToString(data)
}

// dispatchGhostTurn records a durable native input before returning. Waiting is
// separate so legacy errand can acknowledge the real session and acceptance
// before the plugin waits for the model's terminal result.
func (g *chatGhosts) dispatchGhostTurn(ctx context.Context, conversation chat.Conversation, text, persistedContent, sourceSessionID string) (string, error) {
	if _, err := g.control.engine.Open(ctx, conversation.ID, 0); err != nil {
		return "", err
	}
	defer g.control.engine.Release(conversation.ID, 0)
	request := ghostRequestID()
	presentation := chat.NativeInputPresentation{OriginSessionID: sourceSessionID}
	if persistedContent != "" {
		presentation.ReviewRequest = json.RawMessage(persistedContent)
	}
	if _, err := g.control.engine.DispatchPresentedInput(ctx, conversation.ID, request, text, "queue", presentation); err != nil {
		return "", err
	}
	return request, nil
}

func (g *chatGhosts) waitGhostTurn(ctx context.Context, conversation chat.Conversation, request string) (string, error) {
	if _, err := g.control.engine.Open(ctx, conversation.ID, 0); err != nil {
		return "", err
	}
	defer g.control.engine.Release(conversation.ID, 0)
	ctx, cancel := context.WithTimeout(ctx, ghostTurnBudget)
	defer cancel()
	answer := ""
	cursor := uint64(0)
	for {
		page, err := g.control.engine.WaitEvents(ctx, conversation.ID, cursor)
		if err != nil {
			return answer, err
		}
		for _, entry := range page.Events {
			var event struct {
				Type, RequestID, Status, Error string
				Message                        struct {
					Role    string `json:"role"`
					Content []struct {
						Type, Text string
					} `json:"content"`
				} `json:"message"`
			}
			if json.Unmarshal(entry.Data, &event) != nil {
				continue
			}
			if event.Type == "message_end" && event.Message.Role == "assistant" {
				parts := []string{}
				for _, part := range event.Message.Content {
					if part.Type == "text" {
						parts = append(parts, part.Text)
					}
				}
				if joined := strings.Join(parts, ""); joined != "" {
					answer = joined
				}
			}
			if (event.Type == "settled" || event.Type == "input_settled") && event.RequestID == request {
				if event.Status != "" && event.Status != "completed" && event.Status != "succeeded" {
					if event.Error == "" {
						event.Error = event.Status
					}
					return answer, errors.New(event.Error)
				}
				return answer, nil
			}
		}
		cursor = page.Cursor
	}
}

// runGhostTurn is retained for the old single-call operation. New errand
// callers use dispatchGhostTurn/waitGhostTurn so hooks can report acceptance.
func (g *chatGhosts) runGhostTurn(ctx context.Context, conversation chat.Conversation, text string, wait bool) (string, error) {
	request, err := g.dispatchGhostTurn(ctx, conversation, text, "", "")
	if err != nil || !wait {
		return "", err
	}
	return g.waitGhostTurn(ctx, conversation, request)
}

// ghostConversation preserves the old ordinary default route. Configured
// legacy lanes use newLegacyConversation so their model and permission snapshot
// are validated before the first catalog write.
func (g *chatGhosts) ghostConversation(directory, title string) (chat.Conversation, error) {
	return g.newLegacyConversation(directory, title, nil, "ask")
}

func (g *chatGhosts) errandKey(pluginID, key string) string { return pluginID + "\x00" + key }

func (g *chatGhosts) ghostTask(ctx context.Context, raw json.RawMessage) (any, error) {
	// Dispatch namespaces before decoding their payloads: native task creation
	// uses project as a boolean, while the older project query uses a string.
	var envelope struct {
		Op string `json:"op"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, err
	}
	if envelope.Op == "plugin-native" {
		return g.pluginNative(ctx, raw)
	}
	if envelope.Op == "plugin-target" {
		return g.targetNative(ctx, raw)
	}
	if envelope.Op == "team-native" {
		return g.teamNative(ctx, raw)
	}
	var input struct {
		Op              string          `json:"op"`
		Request         json.RawMessage `json:"request"`
		DirAbs          string          `json:"dirAbs"`
		Title           string          `json:"title"`
		Project         string          `json:"project"`
		TaskID          string          `json:"taskId"`
		Plugin          string          `json:"pluginId"`
		SourceSessionID string          `json:"sourceSessionId"`
		Route           *ghostTaskRoute `json:"route"`
		PermissionMode  string          `json:"permissionMode"`
		Routine         string          `json:"routineId"`
		Event           json.RawMessage `json:"event"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	c := g.control
	switch input.Op {
	case "tool-scope", "tool-release", "tool-authorize", "mcp-message":
		return g.mcpTask(ctx, input.Op, raw)
	case "file-review":
		var request struct {
			SessionID, InstanceID, RequestID string
			Execution, Action                json.RawMessage
		}
		if err := json.Unmarshal(raw, &request); err != nil {
			return nil, err
		}
		control, _, err := g.targetConversation(request.SessionID)
		if err != nil || control.engine == nil {
			return nil, ghostTaskFail("HOST_NOT_READY", "会话权限不可用")
		}
		return control.engine.Command(ctx, request.SessionID, "file.review", map[string]any{"instanceId": request.InstanceID, "requestId": request.RequestID, "execution": request.Execution, "action": request.Action})
	case "agent-turn":
		return g.runLegacyAgent(ctx, input.Request)
	case "errand-start":
		var request ghostLegacyErrandRequest
		if err := json.Unmarshal(input.Request, &request); err != nil {
			return nil, err
		}
		return g.startLegacyErrand(ctx, request)
	case "errand-wait":
		var request struct {
			SessionID, RequestID string
		}
		if err := json.Unmarshal(input.Request, &request); err != nil {
			return nil, err
		}
		return g.waitLegacyErrand(ctx, request.SessionID, request.RequestID)
	case "errand":
		var request struct {
			GhostID, Origin, Message, Title, SessionKey, WorkingDir string
			SourceSessionID                                         string `json:"sourceSessionId"`
		}
		if err := json.Unmarshal(input.Request, &request); err != nil {
			return nil, err
		}
		failure := func(code string, err error) (any, error) {
			return map[string]any{"ok": false, "errorCode": code, "message": err.Error()}, nil
		}
		g.mu.Lock()
		id := g.errands[g.errandKey(request.GhostID, request.SessionKey)]
		g.mu.Unlock()
		conversation, err := c.store.Conversation(id)
		if err != nil {
			title := request.Title
			if title == "" {
				title = request.GhostID
			}
			if conversation, err = g.ghostConversation(request.WorkingDir, title); err != nil {
				return failure("SESSION_UNAVAILABLE", err)
			}
			g.mu.Lock()
			key:=g.errandKey(request.GhostID,request.SessionKey);previous:=g.errands[key]
			g.errands[key]=conversation.ID
			saveErr:=g.saveErrandsLocked()
			if saveErr!=nil {if previous==""{delete(g.errands,key)}else{g.errands[key]=previous}}
			g.mu.Unlock()
			if saveErr!=nil{return failure("HOST_NOT_READY",saveErr)}
		}
		if activity, busy := c.engine.Activity()[conversation.ID]; busy && activity.Phase != "" && activity.Phase != "idle" {
			return failure("BUSY", errors.New("插件的上一项任务还没完成"))
		}
		text, err := g.runGhostTurn(ctx, conversation, request.Message, true)
		if err != nil {
			return failure("TURN_FAILED", err)
		}
		return map[string]any{"ok": true, "sessionId": conversation.ID, "text": text, "agentKind": "pi", "model": conversation.Model.Model}, nil
	case "find-by-workdir":
		return g.findLegacyWorkspace(input.DirAbs)
	case "create-draft":
		return g.createLegacyDraft(input.DirAbs, input.Title, input.Route, input.PermissionMode)
	case "project-tasks":
		rows := []map[string]any{}
		for _, conversation := range c.store.Snapshot().Conversations {
			if !conversation.Archived && conversation.ProjectID == input.Project {
				rows = append(rows, map[string]any{"id": conversation.ID, "title": conversation.Title, "updatedAt": conversation.UpdatedAt})
			}
		}
		return rows, nil
	case "reveal-session":
		control, row, err := g.targetConversation(input.TaskID)
		if err != nil {
			return nil, err
		}
		target := map[string]string{"sessionId": row.ID}
		if control != g.control {
			context, err := g.control.onebot.contextFor(ctx, map[string]string{"sessionId": row.ID})
			if err != nil {
				return nil, err
			}
			target["botId"] = context.BotID
		}
		g.pushEvent("reveal-session", target)
		return true, nil
	case "open-task":
		g.pushEvent("reveal-session", map[string]string{"sessionId": input.TaskID})
		return map[string]bool{"ok": true}, nil
	case "plugin-task":
		return g.pluginTask(ctx, input.Plugin, input.Request)
	}
	if strings.HasPrefix(input.Op, "routine") {
		if c.onebot == nil {
			return nil, errors.New("Routine service is stopped")
		}
		method := map[string]string{"routine-source": "routines.pluginSource", "routine-publish": "routines.pluginPublish"}[input.Op]
		if method == "" {
			return nil, errors.New("未知插件任务操作：" + input.Op)
		}
		runner, err := g.control.onebot.ensure(ctx)
		if err != nil {
			return nil, err
		}
		return runner.Call(ctx, method, raw)
	}
	if strings.HasPrefix(input.Op, "companion") {
		if c.onebot == nil {
			return nil, errors.New("HOST_NOT_READY")
		}
		return g.companionNative(ctx, input.Op, raw)
	}
	return nil, errors.New("未知插件任务操作：" + input.Op)
}

// pluginTask answers Cindy's cindy.agent.tasks calls from real conversations. A
// plugin sees only what it created; the slot already checked its approval.
func (g *chatGhosts) pluginTask(ctx context.Context, plugin string, raw json.RawMessage) (any, error) {
	var request struct {
		Kind, TaskID, Title, Prompt, Text, After, RequestKey string
		Limit                                                int
		Route                                                json.RawMessage
	}
	if err := json.Unmarshal(raw, &request); err != nil {
		return nil, err
	}
	c := g.control
	owned := func(id string) bool {
		g.mu.Lock()
		defer g.mu.Unlock()
		return g.pluginTasks[id] == plugin
	}
	data := func(value any) (any, error) { return map[string]any{"ok": true, "data": value}, nil }
	failure := func(code, message string) (any, error) {
		return map[string]any{"ok": false, "error": map[string]any{"code": code, "message": message, "retryable": false}}, nil
	}
	view := func(conversation chat.Conversation) map[string]any {
		activity := c.engine.Activity()[conversation.ID]
		return map[string]any{"id": conversation.ID, "title": conversation.Title, "status": map[bool]string{true: "running", false: "idle"}[activity.Phase != "" && activity.Phase != "idle"], "revision": conversation.UpdatedAt, "permissionMode": conversation.Permission, "createdAt": conversation.CreatedAt, "updatedAt": conversation.UpdatedAt, "model": conversation.Model.Model}
	}
	switch request.Kind {
	case "capabilities":
		return data(map[string]any{"maxTasks": 256, "models": true, "write": true})
	case "models":
		config, err := c.aiConfiguration()
		if err != nil {
			return failure("HOST_NOT_READY", err.Error())
		}
		rows := []map[string]any{}
		for _, provider := range config.Providers {
			for _, model := range provider.Models {
				rows = append(rows, map[string]any{"providerId": provider.ID, "agentKind": "pi", "model": model, "name": model})
			}
		}
		return data(rows)
	case "create":
		g.mu.Lock()
		existing := g.pluginRequests[plugin+"\x00"+request.RequestKey]
		g.mu.Unlock()
		if existing != "" {
			if conversation, err := c.store.Conversation(existing); err == nil {
				return data(view(conversation))
			}
		}
		title := request.Title
		if title == "" {
			title = "Plugin task"
		}
		conversation, err := g.ghostConversation("", title)
		if err != nil {
			return failure("HOST_NOT_READY", err.Error())
		}
		g.mu.Lock()
		key:=plugin+"\x00"+request.RequestKey;previous:=g.pluginRequests[key]
		g.pluginTasks[conversation.ID]=plugin;g.pluginRequests[key]=conversation.ID
		saveErr:=g.saveErrandsLocked()
		if saveErr!=nil{delete(g.pluginTasks,conversation.ID);if previous==""{delete(g.pluginRequests,key)}else{g.pluginRequests[key]=previous}}
		g.mu.Unlock()
		if saveErr!=nil{return failure("HOST_NOT_READY",saveErr.Error())}
		return data(view(conversation))
	case "list":
		rows := []map[string]any{}
		for _, conversation := range c.store.Snapshot().Conversations {
			if owned(conversation.ID) && !conversation.Archived {
				rows = append(rows, view(conversation))
			}
		}
		return data(map[string]any{"tasks": rows})
	case "get", "send", "stop", "requestWriteAccess", "setModel":
		if !owned(request.TaskID) {
			return failure("TARGET_UNAVAILABLE", "TARGET_UNAVAILABLE")
		}
		conversation, err := c.store.Conversation(request.TaskID)
		if err != nil {
			return failure("TARGET_UNAVAILABLE", "TARGET_UNAVAILABLE")
		}
		switch request.Kind {
		case "send":
			if _, err = g.runGhostTurn(ctx, conversation, request.Text+request.Prompt, false); err != nil {
				return failure("TASK_BUSY", err.Error())
			}
		case "stop":
			if err = c.engine.StopRequest(ctx, conversation.ID, c.engine.ActiveRequest(conversation.ID)); err != nil {
				return failure("TASK_BUSY", err.Error())
			}
		case "requestWriteAccess", "setModel":
			return failure("UNSUPPORTED_CAPABILITY", "请在任务中由用户调整")
		}
		return data(view(conversation))
	}
	return failure("UNSUPPORTED_CAPABILITY", "UNSUPPORTED_CAPABILITY")
}

// companion answers from OneBot's real bots: the plugin sees what the sidebar shows.
func (g *chatGhosts) companion(ctx context.Context, op string, raw json.RawMessage) (any, error) {
	var input struct {
		CompanionID string `json:"companionId"`
		Limit       int    `json:"limit"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	value, err := g.control.onebot.call(ctx, "bots.list", json.RawMessage("{}"))
	if err != nil {
		return nil, err
	}
	var bots []struct {
		ID                 string `json:"id"`
		Name               string `json:"name"`
		Description        string `json:"description"`
		Status             string `json:"status"`
		CanonicalSessionID string `json:"canonicalSessionId"`
	}
	if err = json.Unmarshal(value, &bots); err != nil {
		var wrapped struct {
			Bots json.RawMessage `json:"bots"`
		}
		if json.Unmarshal(value, &wrapped) != nil || json.Unmarshal(wrapped.Bots, &bots) != nil {
			return nil, err
		}
	}
	unavailable := errors.New("TARGET_UNAVAILABLE")
	switch op {
	case "companions":
		rows := []map[string]any{}
		for _, bot := range bots {
			rows = append(rows, map[string]any{"id": bot.ID, "name": bot.Name, "description": bot.Description, "home": map[string]any{"isSelf": true, "online": true, "deviceName": nil}, "status": bot.Status, "activity": map[string]any{"phase": "idle"}})
		}
		return rows, nil
	}
	for _, bot := range bots {
		if bot.ID != input.CompanionID {
			continue
		}
		switch op {
		case "companion-avatar":
			fallback := bot.Name
			if runes := []rune(fallback); len(runes) > 0 {
				fallback = string(runes[0])
			}
			return map[string]any{"src": nil, "fallback": fallback}, nil
		case "companion-open":
			g.pushEvent("reveal-session", map[string]string{"sessionId": bot.CanonicalSessionID})
			return map[string]bool{"ok": true}, nil
		case "companion-reply", "companion-history":
			params, _ := json.Marshal(map[string]any{"botId": bot.ID, "limit": max(input.Limit, 20)})
			history, callErr := g.control.onebot.call(ctx, "bots.history", params)
			if callErr != nil {
				return nil, callErr
			}
			var read struct {
				Messages []struct {
					Role      string `json:"role"`
					Text      string `json:"text"`
					Content   string `json:"content"`
					CreatedAt int64  `json:"createdAt"`
				} `json:"messages"`
			}
			if json.Unmarshal(history, &read) != nil {
				return nil, errors.New("HOST_NOT_READY")
			}
			rows := []map[string]any{}
			var last map[string]any
			for _, message := range read.Messages {
				text := message.Text
				if text == "" {
					text = message.Content
				}
				row := map[string]any{"role": message.Role, "text": text, "createdAt": message.CreatedAt, "truncated": false}
				rows = append(rows, row)
				if message.Role == "assistant" {
					last = row
				}
			}
			if op == "companion-reply" {
				if last == nil {
					return nil, nil
				}
				return map[string]any{"text": last["text"], "createdAt": last["createdAt"], "truncated": false}, nil
			}
			return map[string]any{"messages": rows, "more": false}, nil
		}
	}
	return nil, unavailable
}

// ghostImage makes one image through the same gateway route as Chat's media tool.
func (g *chatGhosts) ghostImage(ctx context.Context, raw json.RawMessage) (any, error) {
	var input struct {
		Prompt, Model, ProviderID, AspectRatio string
		ImagePaths                             []string
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	if len(input.ImagePaths) > 0 {
		return nil, errors.New("当前图片服务不支持改图")
	}
	size := map[string]string{"16:9": "1536x1024", "9:16": "1024x1536", "1:1": "1024x1024"}[input.AspectRatio]
	body, _ := json.Marshal(map[string]any{"model": input.ProviderID + "/" + input.Model, "prompt": input.Prompt, "n": 1, "size": size, "response_format": "b64_json"})
	request, err := http.NewRequestWithContext(ctx, "POST", "http://127.0.0.1/v1/images/generations", strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	// Chat's in-product route: the same catalog and policy, without opening a port.
	response := &ghostResponse{header: http.Header{}, status: http.StatusOK}
	g.control.ai.serveChat(response, request)
	data := response.body.Bytes()
	if response.status/100 != 2 {
		return nil, errors.New("图片生成失败：HTTP " + strconv.Itoa(response.status))
	}
	var parsed struct {
		Data []struct {
			B64 string `json:"b64_json"`
		} `json:"data"`
	}
	if err = json.Unmarshal(data, &parsed); err != nil || len(parsed.Data) == 0 {
		return nil, errors.New("图片服务没有返回图片")
	}
	if _, err = base64.StdEncoding.DecodeString(parsed.Data[0].B64); err != nil {
		return nil, errors.New("图片服务返回的数据无效")
	}
	return map[string]string{"data": parsed.Data[0].B64, "mimeType": "image/png"}, nil
}

// ghostResponse collects an in-process gateway answer, bounded like any reply.
type ghostResponse struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (r *ghostResponse) Header() http.Header    { return r.header }
func (r *ghostResponse) WriteHeader(status int) { r.status = status }
func (r *ghostResponse) Write(data []byte) (int, error) {
	if r.body.Len()+len(data) > 64<<20 {
		return 0, errors.New("图片服务返回的数据过大")
	}
	return r.body.Write(data)
}

func (g *chatGhosts) loadErrandsLocked() {
	g.errands, g.pluginTasks, g.pluginRequests = map[string]string{}, map[string]string{}, map[string]string{}
	data, err := os.ReadFile(filepath.Join(g.control.directory, "chat", "ghosts", "tasks.json"))
	if err != nil {
		return
	}
	var saved struct{ Errands, Tasks, Requests map[string]string }
	if json.Unmarshal(data, &saved) == nil {
		if saved.Errands != nil {
			g.errands = saved.Errands
		}
		if saved.Tasks != nil {
			g.pluginTasks = saved.Tasks
		}
		if saved.Requests != nil {
			g.pluginRequests = saved.Requests
		}
	}
}

func (g *chatGhosts) saveErrandsLocked() error {
	data, err := json.Marshal(map[string]any{"Errands": g.errands, "Tasks": g.pluginTasks, "Requests": g.pluginRequests})
	if err != nil {
		return err
	}
	file := filepath.Join(g.control.directory, "chat", "ghosts", "tasks.json")
	if err = os.WriteFile(file+".tmp", data, 0600); err == nil {
		err = os.Rename(file+".tmp", file)
	}
	if err != nil {
		_ = recordHostEvent(g.control.directory, "plugin-tasks", err)
	}
	return err
}
