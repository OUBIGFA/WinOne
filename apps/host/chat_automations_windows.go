//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"winone/internal/ai"
	"winone/internal/chat"
)

type automationEvent struct {
	Sequence uint64          `json:"sequence"`
	Data     json.RawMessage `json:"data"`
}
type automationPage struct {
	Cursor uint64            `json:"cursor"`
	Events []json.RawMessage `json:"events"`
}
type chatAutomations struct {
	control       *chatControl
	startMu       sync.Mutex
	runner        *chat.Runner
	bridge        *chat.PrivateBridge
	stopped       bool
	mu            sync.Mutex
	sequence      uint64
	events        []automationEvent
	changed       chan struct{}
	active        atomic.Bool
	desktopNotify func(string, string) error
}

func newChatAutomations(c *chatControl) *chatAutomations {
	return &chatAutomations{control: c, changed: make(chan struct{})}
}
func (a *chatAutomations) event(raw json.RawMessage) {
	var message struct {
		Type   string          `json:"type"`
		Active bool            `json:"active"`
		Event  json.RawMessage `json:"event"`
	}
	if json.Unmarshal(raw, &message) != nil {
		return
	}
	if message.Type == "scheduler_activation" {
		a.active.Store(message.Active)
		return
	}
	if message.Type != "scheduler_event" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sequence++
	a.events = append(a.events, automationEvent{a.sequence, message.Event})
	if len(a.events) > 256 {
		a.events = a.events[len(a.events)-256:]
	}
	close(a.changed)
	a.changed = make(chan struct{})
}
func (a *chatAutomations) ensure(ctx context.Context) (*chat.Runner, error) {
	a.startMu.Lock()
	defer a.startMu.Unlock()
	if a.stopped {
		return nil, errors.New("自动化正在退出")
	}
	if a.runner != nil {
		select {
		case <-a.runner.Done():
			a.runner = nil
			if a.bridge != nil {
				a.bridge.Close()
				a.bridge = nil
			}
		default:
			return a.runner, nil
		}
	}
	directory := filepath.Join(a.control.directory, "chat")
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	bridge, err := chat.NewPrivateBridge(a.control.ctx, http.HandlerFunc(a.serve))
	if err != nil {
		return nil, err
	}
	base := filepath.Dir(a.control.desktop)
	runner, err := chat.StartRunner(a.control.ctx, filepath.Join(base, "shared", "ai", "runtime", "bun.exe"), filepath.Join(base, "shared", "chat", "scheduler.js"), directory, a.event)
	if err != nil {
		bridge.Close()
		return nil, err
	}
	projectDirectories := []string{}
	for _, project := range a.control.store.Snapshot().Projects {
		projectDirectories = append(projectDirectories, project.Path)
	}
	_, err = runner.Call(ctx, "init", map[string]any{"directory": directory, "projectDirectories": projectDirectories, "baseUrl": bridge.BaseURL, "key": bridge.Key})
	if err != nil {
		closing, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = runner.Close(closing)
		bridge.Close()
		return nil, err
	}
	a.runner, a.bridge = runner, bridge
	return runner, nil
}
func (a *chatAutomations) startSaved() {
	raw, err := os.ReadFile(filepath.Join(a.control.directory, "chat", "automations-active.json"))
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	var state struct {
		Active bool `json:"active"`
	}
	if err == nil {
		err = json.Unmarshal(raw, &state)
	}
	if err != nil {
		a.eventError(err)
		return
	}
	if !state.Active {
		return
	}
	a.active.Store(true)
	ctx, cancel := context.WithTimeout(a.control.ctx, 25*time.Second)
	defer cancel()
	if _, err = a.ensure(ctx); err != nil {
		a.eventError(err)
	}
}
func (a *chatAutomations) eventError(err error) {
	raw, _ := json.Marshal(map[string]any{"type": "scheduler_event", "event": map[string]any{"type": "diagnostic", "message": err.Error()}})
	a.event(raw)
}
func (a *chatAutomations) call(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, error) {
	if method == "connections.get" {
		value, err := a.connections()
		if err != nil {
			return nil, err
		}
		servers := []map[string]any{}
		for _, server := range a.control.store.Preferences().MCPServers {
			servers = append(servers, map[string]any{"name": server.Name, "enabled": server.Enabled})
		}
		return json.Marshal(map[string]any{"settings": value, "servers": servers})
	}
	if method == "connections.save" {
		return a.saveConnections(params)
	}
	switch method {
	case "hook.test":
		return a.hookTest(ctx, params)
	case "open":
		return a.openPath(ctx, params)
	case "detectCwd":
		return a.detectCwd(ctx, params)
	case "channels":
		return a.channels()
	case "capabilityStatus":
		return a.capabilityStatus()
	}
	switch method {
	case "list", "get", "create", "update", "pause", "resume", "delete", "deleted", "restore", "run", "runs", "deleteRun", "markRead", "runtime", "preview", "silence", "notify", "events", "templates", "template.save", "template.delete", "template.apply", "project.read", "project.write", "project.reconcile", "project.promote", "project.clone", "project.demote",
		"hook.generate", "sidebarIndexRuns", "inflightCount", "markAllRead", "project.upsertSchedule", "project.removeSchedule":
	default:
		return nil, errors.New("不支持的自动化操作")
	}
	runner, err := a.ensure(ctx)
	if err != nil {
		return nil, err
	}
	if method == "hook.generate" {
		// A bound task's check lives in that task's folder and uses its model.
		var input map[string]any
		if err = json.Unmarshal(params, &input); err != nil {
			return nil, err
		}
		if id, _ := input["targetSessionId"].(string); id != "" {
			if conversation, lookup := a.control.store.Conversation(id); lookup == nil {
				if dir, _ := input["workingDir"].(string); dir == "" {
					input["workingDir"] = conversation.Directory
				}
				if provider, _ := input["providerId"].(string); provider == "" {
					input["providerId"], input["model"] = conversation.Model.Provider, conversation.Model.Model
				}
			}
		}
		if params, err = json.Marshal(input); err != nil {
			return nil, err
		}
	}
	if method == "events" {
		var input struct {
			Cursor uint64 `json:"cursor"`
		}
		if len(params) > 0 && string(params) != "null" {
			if err = json.Unmarshal(params, &input); err != nil {
				return nil, err
			}
		}
		for {
			a.mu.Lock()
			page := automationPage{Cursor: a.sequence, Events: []json.RawMessage{}}
			for _, event := range a.events {
				if event.Sequence > input.Cursor {
					page.Events = append(page.Events, event.Data)
				}
			}
			changed := a.changed
			a.mu.Unlock()
			if len(page.Events) > 0 || page.Cursor < input.Cursor {
				return json.Marshal(page)
			}
			select {
			case <-ctx.Done():
				return json.Marshal(page)
			case <-changed:
			case <-runner.Done():
				return nil, errors.New("自动化调度进程已退出")
			}
		}
	}
	return runner.Call(ctx, method, params)
}
func (a *chatAutomations) close(ctx context.Context) error {
	a.startMu.Lock()
	defer a.startMu.Unlock()
	a.stopped = true
	if a.runner == nil {
		return nil
	}
	err := a.runner.Close(ctx)
	a.runner = nil
	if a.bridge != nil {
		a.bridge.Close()
		a.bridge = nil
	}
	return err
}
func (a *chatAutomations) resume() {
	a.startMu.Lock()
	a.stopped = false
	a.startMu.Unlock()
	go a.startSaved()
}

type scheduledChat struct {
	Effort   string `json:"effort,omitempty"`
	FastMode *bool  `json:"fastMode,omitempty"`
	Notify   struct {
		Desktop bool `json:"desktop"`
		Feishu  bool `json:"feishu"`
		Wecom   bool `json:"wecomGroup"`
	} `json:"notify"`
	NotificationChannel string            `json:"notificationChannel,omitempty"`
	NotificationText    string            `json:"notificationText,omitempty"`
	ID                  string            `json:"id"`
	Name                string            `json:"name"`
	Prompt              string            `json:"prompt"`
	ProviderID          string            `json:"providerId"`
	Model               string            `json:"model"`
	PermissionMode      string            `json:"permissionMode"`
	WorkspaceKind       string            `json:"workspaceKind"`
	WorkingDir          string            `json:"workingDir"`
	TargetSessionID     string            `json:"targetSessionId"`
	PersistentSession   bool              `json:"persistentSession"`
	UseWorktree         bool              `json:"useWorktree"`
	ExecutionMode       string            `json:"executionMode"`
	ScriptConfig        *scheduledCommand `json:"scriptConfig,omitempty"`
	PreRunHook          *scheduledCommand `json:"preRunHook,omitempty"`
	LastFinishedAt      int64             `json:"lastFinishedAt,omitempty"`
}

type scheduledCommand struct {
	Command      string   `json:"command"`
	TimeoutMs    int64    `json:"timeoutMs,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
}

var errAutomationTargetUnavailable = errors.New("绑定的对话已删除或归档")

func (a *chatAutomations) prepare(ctx context.Context, schedule scheduledChat) (chat.Conversation, bool, error) {
	if schedule.ExecutionMode == "script" {
		return chat.Conversation{}, false, errors.New("脚本必须通过独立执行入口运行")
	}
	if schedule.TargetSessionID != "" {
		conversation, err := a.control.store.Conversation(schedule.TargetSessionID)
		if err == nil && !conversation.Archived {
			if state, pollErr := a.control.engine.Poll(conversation.ID, 0); pollErr == nil && state.Running {
				return conversation, true, nil
			}
			selection := conversation.Model
			if schedule.ProviderID != "" && schedule.Model != "" {
				selection = ai.Selection{Provider: schedule.ProviderID, Model: schedule.Model}
			}
			permission := conversation.Permission
			if schedule.PermissionMode != "" {
				permission = schedule.PermissionMode
			}
			thinking, fast := conversation.ThinkingLevel, conversation.FastMode
			if schedule.Effort != "" {
				thinking = schedule.Effort
			}
			if schedule.FastMode != nil {
				fast = *schedule.FastMode
			}
			if selection != conversation.Model || permission != conversation.Permission || thinking != conversation.ThinkingLevel || fast != conversation.FastMode {
				config, err := a.control.aiConfiguration()
				if err != nil {
					return chat.Conversation{}, false, err
				}
				if _, err = chatModel(config, selection); err != nil {
					return chat.Conversation{}, false, err
				}
				conversation, err = a.control.engine.ConfigureConversation(ctx, conversation.ID, func(value *chat.Conversation) error {
					value.Model = selection
					value.Permission = permission
					value.ThinkingLevel = thinking
					value.FastMode = fast
					return nil
				})
				if err != nil {
					return chat.Conversation{}, false, err
				}
			}
			return conversation, false, nil
		}
		if !schedule.PersistentSession {
			return chat.Conversation{}, false, errAutomationTargetUnavailable
		}
	}
	selection := ai.Selection{Provider: schedule.ProviderID, Model: schedule.Model}
	config, err := a.control.aiConfiguration()
	if err != nil {
		return chat.Conversation{}, false, err
	}
	if selection.Provider == "" {
		selection = config.Tasks["chat"]
	}
	if _, err = chatModel(config, selection); err != nil {
		return chat.Conversation{}, false, err
	}
	directory := schedule.WorkingDir
	if schedule.WorkspaceKind == "dialogue" {
		directory = ""
	}
	if directory != "" {
		if !filepath.IsAbs(directory) {
			return chat.Conversation{}, false, errors.New("自动化工作目录必须是绝对路径")
		}
		info, err := os.Stat(directory)
		if err != nil {
			return chat.Conversation{}, false, err
		}
		if !info.IsDir() {
			return chat.Conversation{}, false, errors.New("自动化工作位置不是文件夹")
		}
	}
	if schedule.UseWorktree {
		if directory == "" {
			return chat.Conversation{}, false, errors.New("请选择 Git 项目后使用工作树")
		}
		parent := filepath.Join(a.control.directory, "chat", "worktrees")
		if err := os.MkdirAll(parent, 0700); err != nil {
			return chat.Conversation{}, false, err
		}
		target := filepath.Join(parent, fmt.Sprintf("run-%d", time.Now().UnixNano()))
		if err := chat.CreateDetachedWorktree(ctx, directory, target); err != nil {
			return chat.Conversation{}, false, err
		}
		directory = target
	}
	conversation, err := a.control.store.NewConversation("", directory, schedule.Name, selection)
	if err != nil {
		return chat.Conversation{}, false, err
	}
	// Cindy runs unattended automations with full access. Schedules saved by
	// earlier WinOne versions keep the permission stored on them.
	permission := schedule.PermissionMode
	if permission == "" {
		permission = "bypassPermissions"
	}
	thinking, fast := schedule.Effort, false
	if schedule.FastMode != nil {
		fast = *schedule.FastMode
	}
	conversation, err = a.control.store.UpdateConversation(conversation.ID, func(value *chat.Conversation) error {
		value.Permission = permission
		value.ThinkingLevel = thinking
		value.FastMode = fast
		return nil
	})
	return conversation, false, err
}
func (a *chatAutomations) serve(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST required", 405)
		return
	}
	var input struct {
		Schedule    scheduledChat       `json:"schedule"`
		Run         automationRunNotice `json:"run"`
		ID          string              `json:"id"`
		RunID       string              `json:"runId"`
		Prompt      string              `json:"prompt"`
		Attachments []string            `json:"attachments"`
		Mode        string              `json:"mode"`
		FiredAt     int64               `json:"firedAt"`
		Call        json.RawMessage     `json:"call"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&input); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	fail := func(err error) {
		if errors.Is(err, errAutomationTargetUnavailable) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]string{"code": "target_unavailable", "error": err.Error()})
			return
		}
		http.Error(w, err.Error(), 500)
	}
	switch strings.TrimPrefix(r.URL.Path, "/v1/") {
	case "text":
		var request struct {
			Prompt     string `json:"prompt"`
			ProviderID string `json:"providerId"`
			Model      string `json:"model"`
			TimeoutMs  int64  `json:"timeoutMs"`
		}
		if err := json.Unmarshal(input.Call, &request); err != nil || strings.TrimSpace(request.Prompt) == "" || len(request.Prompt) > 256<<10 {
			http.Error(w, "Invalid text request", 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(a.utilityText(r.Context(), request.Prompt, request.ProviderID, request.Model, time.Duration(request.TimeoutMs)*time.Millisecond))
	case "notify":
		if err := a.notifyRun(r.Context(), input.Schedule, input.Run); err != nil {
			fail(err)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]bool{"delivered": true})
	case "command":
		if input.Mode != "script" && input.Mode != "hook" {
			http.Error(w, "Invalid command mode", 400)
			return
		}
		if input.Mode == "hook" && input.Schedule.PreRunHook == nil || input.Mode == "script" && (input.Schedule.ExecutionMode != "script" || input.Schedule.ScriptConfig == nil) {
			http.Error(w, "Command configuration missing", 400)
			return
		}
		value, err := a.command(r.Context(), input.Schedule, input.Mode, input.RunID, input.FiredAt)
		if err != nil {
			fail(err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(value)
	case "conversation":
		value, err := a.control.store.Conversation(input.ID)
		if err != nil {
			_ = json.NewEncoder(w).Encode(nil)
			return
		}
		status := "active"
		if value.Archived {
			status = "archived"
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"id": value.ID, "status": status})
	case "dispatch":
		value, err := a.dispatchScript(r.Context(), input.Schedule, input.Call)
		if err != nil {
			fail(err)
			return
		}
		_ = json.NewEncoder(w).Encode(value)
	case "retire":
		runner, err := a.ensure(r.Context())
		if err != nil {
			fail(err)
			return
		}
		value, err := runner.Call(r.Context(), "retire", map[string]any{"id": input.ID, "runId": input.RunID})
		if err != nil {
			fail(err)
			return
		}
		_, _ = w.Write(value)
	case "prepare":
		conversation, busy, err := a.prepare(r.Context(), input.Schedule)
		if err != nil {
			fail(err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"sessionId": conversation.ID, "deferred": busy, "deferRetryMs": 10000})
	case "stop":
		if err := a.control.engine.StopRequest(r.Context(), input.ID, input.RunID); err != nil {
			fail(err)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]bool{"stopped": true})
	case "wait":
		cursor, err := a.control.engine.Cursor(input.ID)
		if err != nil {
			_ = json.NewEncoder(w).Encode(map[string]bool{"idle": true})
			return
		}
		for {
			page, err := a.control.engine.WaitEvents(r.Context(), input.ID, cursor)
			if err != nil {
				if _, getErr := a.control.store.Conversation(input.ID); getErr != nil {
					fail(getErr)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]bool{"idle": true})
				return
			}
			if !page.Running {
				_ = json.NewEncoder(w).Encode(map[string]bool{"idle": true})
				return
			}
			cursor = page.Cursor
		}
	case "execute":
		if _, err := a.control.engine.Open(r.Context(), input.ID, 0); err != nil {
			fail(err)
			return
		}
		completed := false
		defer func() {
			if !completed {
				// The HTTP request can disappear with its scheduler worker. Keep
				// cleanup independent of that cancelled context and identity-bound.
				cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				if err := a.control.engine.StopRequest(cleanup, input.ID, input.RunID); err != nil {
					a.eventError(err)
				}
				cancel()
			}
			a.control.engine.Release(input.ID, 0)
		}()
		cursor, err := a.control.engine.Cursor(input.ID)
		if err != nil {
			fail(err)
			return
		}
		// Scheduled and channel inputs use the native inbox. They never consume
		// an owner's unsent text or attachments in the conversation's composer.
		if _, err = a.control.engine.DispatchInput(r.Context(), input.ID, input.RunID, input.Prompt, input.Attachments); err != nil {
			fail(err)
			return
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		flush, ok := w.(http.Flusher)
		if !ok {
			fail(errors.New("响应不支持流式输出"))
			return
		}
		encoder := json.NewEncoder(w)
		if err = encoder.Encode(map[string]bool{"started": true}); err != nil {
			return
		}
		flush.Flush()
		for {
			wait, cancel := context.WithTimeout(r.Context(), 30*time.Second)
			page, err := a.control.engine.WaitEvents(wait, input.ID, cursor)
			cancel()
			if errors.Is(err, context.DeadlineExceeded) {
				if err = encoder.Encode(map[string]bool{"progress": false}); err != nil {
					return
				}
				flush.Flush()
				continue
			}
			if err != nil {
				_ = encoder.Encode(map[string]any{"terminal": map[string]any{"status": "failed", "error": err.Error()}})
				flush.Flush()
				return
			}
			for index, entry := range page.Events {
				var terminal struct {
					Type      string `json:"type"`
					RequestID string `json:"requestId"`
					Status    string `json:"status"`
					Error     string `json:"error"`
				}
				if json.Unmarshal(entry.Data, &terminal) == nil && (terminal.Type == "settled" || terminal.Type == "input_settled") && terminal.RequestID == input.RunID {
					page.Events = page.Events[:index+1]
					page.Cursor = entry.Sequence
					page.Running = false
					page.Status = terminal.Status
					page.Error = terminal.Error
					completed = true
					break
				}
			}
			if err = encoder.Encode(page); err != nil {
				return
			}
			flush.Flush()
			cursor = page.Cursor
			if !page.Running {
				return
			}
		}
	default:
		http.Error(w, "Unknown scheduler operation", 404)
	}
}
