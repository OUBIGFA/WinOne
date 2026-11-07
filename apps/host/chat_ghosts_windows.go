//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"winone/internal/ai"
	"winone/internal/chat"
)

// chatGhosts is shared by ordinary Chat and OneBot. No plugin process, profile
// or account is opened merely by starting WinOne or reading an empty catalog.
type chatGhosts struct {
	control                                                *chatControl
	mu                                                     sync.Mutex
	runner                                                 *chat.Runner
	bridge                                                 *chat.PrivateBridge
	stopped                                                bool
	focusedSession                                         string
	libraryReadSession, libraryReadPlugin, libraryReadRoot string
	libraryReadEverGranted                                 bool
	// Prompts and pushes wait here until the native window reads them. A prompt
	// holds its plugin's request open; its answer or the plugin's deadline ends it.
	prompts        map[string]*ghostPrompt
	events         []ghostEvent
	eventSequence  uint64
	errands        map[string]string
	pluginTasks    map[string]string
	pluginRequests map[string]string
	toolScopes     map[string]*ghostToolScope
}

type ghostPrompt struct {
	ID      string          `json:"id"`
	Request json.RawMessage `json:"request"`
	answer  chan json.RawMessage
}

type ghostEvent struct {
	Sequence uint64 `json:"sequence"`
	Channel  string `json:"channel"`
	Payload  any    `json:"payload"`
}

const ghostEventLimit = 256

func newChatGhosts(c *chatControl) *chatGhosts {
	g := &chatGhosts{control: c, prompts: map[string]*ghostPrompt{}}
	g.loadErrandsLocked()
	return g
}

// Invalidation never starts a plugin worker. Cindy's watch hub coalesces these
// signals and retains its heartbeat if a best-effort notification is lost.
func (g *chatGhosts) notifyTaskChange() {
	g.mu.Lock()
	runner, stopped := g.runner, g.stopped
	g.mu.Unlock()
	if stopped || runner == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(g.control.ctx, 4*time.Second)
		defer cancel()
		_, _ = runner.Call(ctx, "tasks-event", map[string]string{"type": "changed"})
	}()
}

func (g *chatGhosts) pushEvent(channel string, payload any) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.eventSequence++
	g.events = append(g.events, ghostEvent{g.eventSequence, channel, payload})
	if len(g.events) > ghostEventLimit {
		g.events = g.events[len(g.events)-ghostEventLimit:]
	}
}

// pending is what the native window reads: new pushes and every open prompt.
func (g *chatGhosts) pending(after uint64) map[string]any {
	g.mu.Lock()
	defer g.mu.Unlock()
	events := []ghostEvent{}
	for _, event := range g.events {
		if event.Sequence > after {
			events = append(events, event)
		}
	}
	prompts := []*ghostPrompt{}
	for _, prompt := range g.prompts {
		prompts = append(prompts, prompt)
	}
	sort.Slice(prompts, func(i, j int) bool { return prompts[i].ID < prompts[j].ID })
	return map[string]any{"cursor": g.eventSequence, "events": events, "prompts": prompts}
}

func (g *chatGhosts) ask(ctx context.Context, request json.RawMessage) (json.RawMessage, error) {
	prompt := &ghostPrompt{ID: fmt.Sprintf("%020d-%s", time.Now().UnixNano(), ghostRequestID()), Request: request, answer: make(chan json.RawMessage, 1)}
	g.mu.Lock()
	g.prompts[prompt.ID] = prompt
	g.mu.Unlock()
	defer func() {
		g.mu.Lock()
		delete(g.prompts, prompt.ID)
		g.mu.Unlock()
	}()
	select {
	case answer := <-prompt.answer:
		return answer, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (g *chatGhosts) event(raw json.RawMessage) {
	var message struct {
		Type  string `json:"type"`
		Event struct {
			Type, Message, Channel string
			Payload                json.RawMessage
		} `json:"event"`
	}
	if json.Unmarshal(raw, &message) != nil || message.Type != "ghost_event" {
		return
	}
	switch message.Event.Type {
	case "diagnostic":
		_ = recordHostEvent(g.control.directory, "plugin-check", errors.New(message.Event.Message))
	case "push":
		g.pushEvent(message.Event.Channel, message.Event.Payload)
	case "changed":
		g.pushEvent("changed", nil)
	}
}

func (g *chatGhosts) ensure(ctx context.Context) (*chat.Runner, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.stopped {
		return nil, errors.New("插件检查服务正在退出")
	}
	if g.runner != nil {
		select {
		case <-g.runner.Done():
			g.runner = nil
			if g.bridge != nil {
				g.bridge.Close()
				g.bridge = nil
			}
		default:
			return g.runner, nil
		}
	}
	directory := filepath.Join(g.control.directory, "chat", "ghosts")
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	key, err := oneBotMasterKey(directory) // Existing DPAPI + encrypted worker KV.
	if err != nil {
		return nil, err
	}
	bridge, err := chat.NewPrivateBridge(g.control.ctx, http.HandlerFunc(g.serve))
	if err != nil {
		return nil, err
	}
	base := filepath.Dir(g.control.desktop)
	assets := filepath.Join(base, "shared", "chat")
	runner, err := chat.StartRunner(g.control.ctx, filepath.Join(base, "shared", "ai", "runtime", "bun.exe"), filepath.Join(assets, "ghost.js"), directory, g.event)
	if err != nil {
		bridge.Close()
		return nil, err
	}
	_, err = runner.Call(ctx, "init", map[string]any{"directory": directory, "executable": filepath.Join(base, "WinOne.Plugin.exe"), "sdkFile": filepath.Join(assets, "ghost-sdk.js"), "mailSDKFile": filepath.Join(assets, "ghost-mail-sdk.js"), "snapshotWorker": filepath.Join(assets, "ghost-snapshot.js"), "masterKey": key, "baseUrl": bridge.BaseURL, "key": bridge.Key, "runtimeExecutable": filepath.Join(base, "shared", "ai", "runtime", "bun.exe"), "nodeWorker": filepath.Join(assets, "nodeRuntimeWorkerProcess.js")})
	if err != nil {
		closing, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = runner.Close(closing)
		bridge.Close()
		return nil, err
	}
	if g.focusedSession != "" {
		_, _ = runner.Call(ctx, "session-focus", map[string]string{"sessionId": g.focusedSession})
	}
	g.runner, g.bridge = runner, bridge
	return runner, nil
}

func (g *chatGhosts) call(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, error) {
	if method == "write-access-retry" {
		// This is an explicit native owner action. A plugin cannot clear the
		// process-local refusal memory or switch the mode to renew its prompt.
		var input struct {
			TaskID string `json:"taskId"`
		}
		if err := json.Unmarshal(params, &input); err != nil {
			return nil, err
		}
		row, err := g.control.store.Conversation(input.TaskID)
		if err != nil || row.PluginID == "" {
			return nil, errors.New("当前任务不是插件创建的任务")
		}
		runner, err := g.ensure(ctx)
		if err != nil {
			return nil, err
		}
		return runner.Call(ctx, "tasks-write-retry", map[string]string{"pluginId": row.PluginID, "taskId": row.ID})
	}
	if method == "workdir-preferences" || method == "set-workdir-preference" {
		var input struct {
			TaskID, PluginID string
			Disabled         bool
		}
		if err := json.Unmarshal(params, &input); err != nil {
			return nil, err
		}
		_, row, err := g.targetConversation(input.TaskID)
		if err != nil {
			return nil, err
		}
		runner, err := g.ensure(ctx)
		if err != nil {
			return nil, err
		}
		return runner.Call(ctx, method, map[string]any{"workingDir": row.Directory, "pluginId": input.PluginID, "disabled": input.Disabled})
	}
	if method == "mail-ui" {
		return g.mailUi(ctx, params)
	}
	if method == "media-preview" {
		runner, err := g.ensure(ctx)
		if err != nil {
			return nil, err
		}
		value, err := runner.Call(ctx, "media-path", params)
		if err != nil {
			return nil, err
		}
		var target struct {
			Path string `json:"path"`
		}
		if err = json.Unmarshal(value, &target); err != nil {
			return nil, err
		}
		server, err := g.control.previewServer()
		if err != nil {
			return nil, err
		}
		preview, err := server.OpenVerified(target.Path)
		if err != nil {
			return nil, err
		}
		return json.Marshal(preview)
	}
	if method == "session-focus" {
		var input struct {
			SessionID string `json:"sessionId"`
		}
		if err := json.Unmarshal(params, &input); err != nil {
			return nil, err
		}
		g.mu.Lock()
		g.focusedSession = input.SessionID
		if g.libraryReadRoot != "" {
			g.libraryReadSession = input.SessionID
		}
		runner := g.runner
		g.mu.Unlock()
		if runner == nil {
			return json.RawMessage("true"), nil
		}
		return runner.Call(ctx, method, params)
	}
	switch method {
	case "list", "candidates", "reply-overrides", "pages", "badges":
		// Before the first explicit install this is a read-only empty catalog,
		// not a second always-running plugin or browser host.
		if _, err := os.Stat(filepath.Join(g.control.directory, "chat", "ghosts", "plugins")); errors.Is(err, os.ErrNotExist) {
			return json.RawMessage("[]"), nil
		}
	case "resolve", "screen", "inspect", "install", "enabled", "remove", "configuration", "configure",
		"setup", "account-action", "settings-session", "library-action", "background-tasks", "reveal-background-task", "card-get", "card-action", "card-link", "media-path", "resolve-panel-media", "panel-media-action", "mcp-app-native", "mcp-app-list", "mcp-app-release-owner", "mcp-app-message-admit", "mail-ui", "mail-preview", "agent-connection-action", "agent-connection-pending", "ui-approval", "handoff-action", "models", "user-gesture", "session-focus", "page-session", "page-attach", "page-detach", "page-navigation", "page-gesture", "worktree-readiness", "badge-clear", "commands", "recommendations":
	case "pending":
		var input struct {
			After uint64 `json:"after"`
		}
		if len(params) > 0 && json.Unmarshal(params, &input) != nil {
			return nil, errors.New("插件请求格式无效")
		}
		return json.Marshal(g.pending(input.After))
	case "answer":
		var input struct {
			ID     string          `json:"id"`
			Answer json.RawMessage `json:"answer"`
		}
		if json.Unmarshal(params, &input) != nil {
			return nil, errors.New("插件请求格式无效")
		}
		g.mu.Lock()
		prompt := g.prompts[input.ID]
		g.mu.Unlock()
		if prompt != nil {
			select {
			case prompt.answer <- input.Answer:
			default: // A repeated click never blocks the UI RPC or replaces its first answer.
			}
		}
		return json.RawMessage("true"), nil
	default:
		return nil, errors.New("未知插件操作")
	}
	runner, err := g.ensure(ctx)
	if err != nil {
		return nil, err
	}
	return runner.Call(ctx, method, params)
}

// installed says a plugin exists, without starting anything for those who have none.
func (g *chatGhosts) installed() bool {
	entries, err := os.ReadDir(filepath.Join(g.control.directory, "chat", "ghosts", "plugins"))
	return err == nil && len(entries) > 0
}

// screenInput runs a plugin's $command and will-user-message hooks on what the
// user sent. A plugin that fails or times out never stops the message (the slot's
// own rule); only an explicit verdict changes it.
func (g *chatGhosts) screenInput(ctx context.Context, conversation, text string) (string, error) {
	if !g.installed() {
		return text, nil
	}
	runner, err := g.ensure(ctx)
	if err != nil {
		return text, nil
	}
	if strings.HasPrefix(text, "$") || strings.HasPrefix(text, "￥") {
		workdir := ""
		if _, row, err := g.targetConversation(conversation); err == nil {
			workdir = row.Directory
		}
		params, _ := json.Marshal(map[string]string{"text": text, "workingDir": workdir})
		if value, callErr := runner.Call(ctx, "expand-command", json.RawMessage(params)); callErr == nil {
			var expanded struct{ Text string }
			if json.Unmarshal(value, &expanded) == nil && expanded.Text != "" {
				text = expanded.Text
			}
		}
	}
	params, _ := json.Marshal(map[string]string{"sessionId": conversation, "text": text})
	value, err := runner.Call(ctx, "screen-user", json.RawMessage(params))
	if err != nil {
		return text, nil
	}
	var verdict struct {
		Action, Text, Reason, GhostName string
	}
	if json.Unmarshal(value, &verdict) != nil {
		return text, nil
	}
	switch verdict.Action {
	case "block":
		return "", fmt.Errorf("插件“%s”拦下了这条消息：%s", verdict.GhostName, verdict.Reason)
	case "rewrite":
		if verdict.Text != "" {
			return verdict.Text, nil
		}
	}
	return text, nil
}

// skillDirectories are the Agent Skills enabled plugins bundle.
func (g *chatGhosts) skillDirectories(ctx context.Context) []string {
	if !g.installed() {
		return nil
	}
	runner, err := g.ensure(ctx)
	if err != nil {
		return nil
	}
	value, err := runner.Call(ctx, "agent-skills", json.RawMessage("{}"))
	if err != nil {
		return nil
	}
	var directories []string
	_ = json.Unmarshal(value, &directories)
	return directories
}

// turnSettled tells subscribers a turn ended, then lets will-assistant-message
// hooks replace how the answer reads. The model's history keeps the original.
func (g *chatGhosts) turnSettled(conversation string) {
	if !g.installed() {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(g.control.ctx, 6*time.Minute)
		defer cancel()
		runner, err := g.ensure(ctx)
		if err != nil {
			return
		}
		model := ""
		if record, lookup := g.control.store.Conversation(conversation); lookup == nil {
			model = record.Model.Model
		}
		params, _ := json.Marshal(map[string]any{"name": "did-turn-end", "data": map[string]any{"sessionId": conversation, "agent": "pi", "model": model, "durationMs": 0, "endReason": "completed"}})
		_, _ = runner.Call(ctx, "turn-event", json.RawMessage(params))
		text := g.lastAnswer(ctx, conversation)
		if text == "" {
			return
		}
		params, _ = json.Marshal(map[string]string{"sessionId": conversation, "text": text})
		value, err := runner.Call(ctx, "screen-assistant", json.RawMessage(params))
		if err != nil {
			return
		}
		var verdict struct {
			Action, Text, HTML string
		}
		if json.Unmarshal(value, &verdict) == nil && verdict.Action == "rewrite" && verdict.Text != "" {
			g.pushEvent("assistant-rewritten", map[string]string{"sessionId": conversation, "text": verdict.Text})
		}
	}()
}

// lastAnswer reads the turn's answer for hooks, only when a plugin subscribes.
func (g *chatGhosts) lastAnswer(ctx context.Context, conversation string) string {
	runner, err := g.ensure(ctx)
	if err != nil {
		return ""
	}
	value, err := runner.Call(ctx, "has-assistant-hook", json.RawMessage("{}"))
	if err != nil || string(value) != "true" {
		return ""
	}
	raw, err := g.control.engine.Command(ctx, conversation, "state", nil)
	if err != nil {
		return ""
	}
	var state struct {
		Messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	if json.Unmarshal(raw, &state) != nil {
		return ""
	}
	for index := len(state.Messages) - 1; index >= 0; index-- {
		message := state.Messages[index]
		if message.Role != "assistant" {
			continue
		}
		var parts []struct {
			Type, Text string
		}
		if json.Unmarshal(message.Content, &parts) != nil {
			var plain string
			if json.Unmarshal(message.Content, &plain) == nil {
				return plain
			}
			return ""
		}
		out := ""
		for _, part := range parts {
			if part.Type == "text" {
				out += part.Text
			}
		}
		return out
	}
	return ""
}

// toolCall is a Pi worker's switchboard request. The conversation comes from the
// worker's private bridge, so a plugin or model cannot name another task.
func (g *chatGhosts) toolCall(w http.ResponseWriter, r *http.Request, conversation chat.Conversation) bool {
	path := strings.TrimPrefix(r.URL.Path, "/v1/")
	if path == "ghost-input-admit" || path == "ghost-review" {
		g.taskAdmissionHandler(conversation)(w, r)
		return true
	}
	if g.appMessageAdmit(w, r, conversation) {
		return true
	}
	if path == "ghost-library-context" {
		g.mu.Lock()
		session, root, known := g.libraryReadSession, g.libraryReadRoot, g.libraryReadEverGranted
		g.mu.Unlock()
		value := map[string]any{}
		if known {
			value["libraryRoot"] = nil
			if r.Method == "POST" && session == conversation.ID && root != "" {
				if resolved, err := filepath.EvalSymlinks(root); err == nil && strings.EqualFold(resolved, root) {
					value["libraryRoot"] = root
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(value)
		return true
	}
	if path == "ghost-read-library" {
		var input struct{ Path string }
		allowed := false
		if r.Method == "POST" && json.NewDecoder(http.MaxBytesReader(w, r.Body, 65536)).Decode(&input) == nil {
			g.mu.Lock()
			session, root := g.libraryReadSession, g.libraryReadRoot
			g.mu.Unlock()
			if session == conversation.ID && root != "" {
				resolvedRoot, rootErr := filepath.EvalSymlinks(root)
				resolvedPath, pathErr := filepath.EvalSymlinks(input.Path)
				if rootErr == nil && pathErr == nil && strings.EqualFold(resolvedRoot, root) {
					relative, err := filepath.Rel(root, resolvedPath)
					allowed = err == nil && (relative == "." || relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative))
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"allowed": allowed})
		return true
	}
	if path == "ghost-team" {
		var request struct {
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if r.Method != "POST" || json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&request) != nil {
			http.Error(w, "Invalid Orca request", http.StatusBadRequest)
			return true
		}
		runner, err := g.ensure(r.Context())
		if err == nil {
			value, callErr := runner.Call(r.Context(), "teams-operation", map[string]any{"sessionId": conversation.ID, "method": request.Method, "params": request.Params})
			err = callErr
			if err == nil {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(value)
				return true
			}
		}
		http.Error(w, err.Error(), http.StatusBadGateway)
		return true
	}
	if strings.TrimPrefix(r.URL.Path, "/v1/") != "ghost-tool" {
		return false
	}
	var request struct {
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&request); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return true
	}
	switch request.Method {
	case "agent-roster", "agent-list", "agent-native-event":
		if _, err := os.Stat(filepath.Join(g.control.directory, "chat", "ghosts", "plugins")); errors.Is(err, os.ErrNotExist) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("[]"))
			return true
		}
	case "agent-info", "agent-manual", "agent-call", "agent-connect-account", "agent-cancel", "forge-guide", "forge-scaffold", "forge-pack", "forge-install":
	default:
		http.Error(w, "Unknown plugin request", http.StatusBadRequest)
		return true
	}
	fields := map[string]any{}
	if len(request.Params) > 0 && json.Unmarshal(request.Params, &fields) != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return true
	}
	approved, _ := fields["approved"].(bool)
	delete(fields, "approved")
	if request.Method == "agent-native-event" {
		fields["sessionId"] = conversation.ID
	}
	fields["session"] = map[string]any{"id": conversation.ID, "workingDir": conversation.Directory, "readOnly": conversation.LegacyPlan, "approved": approved || conversation.Permission == "bypassPermissions"}
	if request.Method == "agent-call" {
		if control, _, lookup := g.targetConversation(conversation.ID); lookup == nil && control.engine != nil {
			if raw, inspectErr := control.engine.Command(r.Context(), conversation.ID, "execution.inspect", nil); inspectErr == nil {
				var state struct {
					InstanceID      string
					PlanModeEnabled bool
				}
				if json.Unmarshal(raw, &state) == nil {
					fields["session"].(map[string]any)["instanceId"] = state.InstanceID
					fields["session"].(map[string]any)["readOnly"] = state.PlanModeEnabled
				}
			}
		}
	}
	runner, err := g.ensure(r.Context())
	if err == nil {
		var value json.RawMessage
		// Runner marshals its argument. Passing json.Marshal's []byte here
		// would encode base64 instead of the plugin's JSON object.
		if value, err = runner.Call(r.Context(), request.Method, fields); err == nil {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(value)
			return true
		}
	}
	http.Error(w, err.Error(), http.StatusBadGateway)
	return true
}

func (g *chatGhosts) close(ctx context.Context) error {
	g.mu.Lock()
	g.stopped = true
	scopeIDs := make([]string, 0, len(g.toolScopes))
	for id := range g.toolScopes {
		scopeIDs = append(scopeIDs, id)
	}
	for _, prompt := range g.prompts {
		select {
		case prompt.answer <- nil:
		default:
		}
	}
	runner, bridge := g.runner, g.bridge
	g.runner, g.bridge = nil, nil
	g.mu.Unlock()
	for _, id := range scopeIDs {
		g.releaseToolScope(id)
	}
	// The worker may emit final prompt/state events while draining. Never hold
	// the event mutex while waiting for its stdout loop or process to close.
	if bridge != nil {
		bridge.Close()
	}
	if runner == nil {
		return nil
	}
	return runner.Close(ctx)
}

func (g *chatGhosts) resume() {
	g.mu.Lock()
	g.stopped = false
	g.mu.Unlock()
}

// serve is reachable only through the private authenticated bridge. A plugin
// cannot call these command/credential endpoints directly from its browser.
func (g *chatGhosts) serve(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" || strings.TrimPrefix(r.URL.Path, "/v1/") != "ghost-request" {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	var request struct {
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<20)).Decode(&request); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	var value any
	var err error
	switch request.Method {
	case "gh-token":
		// Same read-only source as Cindy. The token is returned only to this
		// sealed worker and injected by its pinned network slot, never to UI.
		if executable, lookup := exec.LookPath("gh"); lookup == nil {
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()
			bytes, tokenErr := exec.CommandContext(ctx, executable, "auth", "token").Output()
			if tokenErr == nil {
				value = strings.TrimSpace(string(bytes))
			}
			clear(bytes)
		}
	case "library-read-root":
		var input struct {
			GhostID string
			Root    *string
		}
		if err = json.Unmarshal(request.Params, &input); err == nil {
			if input.Root != nil {
				resolved, resolveErr := filepath.EvalSymlinks(*input.Root)
				if resolveErr != nil {
					err = resolveErr
					break
				}
				input.Root = &resolved
			}
			g.mu.Lock()
			if input.Root == nil {
				if g.libraryReadPlugin == input.GhostID {
					g.libraryReadSession, g.libraryReadPlugin, g.libraryReadRoot = "", "", ""
				}
				value = "not-granted"
			} else if g.focusedSession == "" {
				value = "not-granted"
			} else {
				g.libraryReadSession, g.libraryReadPlugin, g.libraryReadRoot = g.focusedSession, input.GhostID, *input.Root
				g.libraryReadEverGranted = true
				value = "granted"
			}
			g.mu.Unlock()
		}
	case "ui":
		value, err = g.ask(r.Context(), request.Params)
	case "task":
		value, err = g.ghostTask(r.Context(), request.Params)
	case "open-external":
		err = openChatLink(request.Params)
	case "show-item":
		var input struct {
			Path string `json:"path"`
		}
		if err = json.Unmarshal(request.Params, &input); err == nil {
			g.pushEvent("show-item", input)
		}
	case "clipboard-png", "clipboard-file":
		value, err = g.clipboard(r.Context(), request.Method, request.Params)
	case "session-snapshot":
		var input struct {
			SessionID         string `json:"sessionId"`
			SessionInstanceID string `json:"sessionInstanceId"`
			Live              bool   `json:"live"`
		}
		if err = json.Unmarshal(request.Params, &input); err == nil {
			control, conversation, lookup := g.targetConversation(input.SessionID)
			if lookup == nil {
				value = map[string]any{"workingDir": conversation.Directory, "permissionMode": conversation.Permission, "planModeEnabled": conversation.LegacyPlan, "archived": conversation.Archived, "mcpAppEpoch": conversation.McpAppEpoch, "remoteHostId": nil, "model": conversation.Model.Model, "isCompanion": control != g.control && conversation.OneBotTaskID == "", "permissionGeneration": ghostAuthority(conversation), "sessionInstanceId": nil}
				if input.Live {
					value = nil
					if control.engine != nil {
						if raw, inspectErr := control.engine.Command(r.Context(), conversation.ID, "execution.inspect", nil); inspectErr == nil {
							var state struct {
								InstanceID, RequestID, Permission string
								PlanModeEnabled, Running          bool
								Execution                         json.RawMessage
							}
							if json.Unmarshal(raw, &state) == nil && (input.SessionInstanceID == "" || state.InstanceID == input.SessionInstanceID) {
								value = map[string]any{"workingDir": conversation.Directory, "permissionMode": state.Permission, "planModeEnabled": state.PlanModeEnabled, "remoteHostId": nil, "instanceId": state.InstanceID, "requestId": state.RequestID, "execution": state.Execution}
							}
						}
					}
				}
			}
		}
	case "connection-resume":
		value, err = g.connectionResume(r.Context(), request.Params)
	case "mail-attachment-snapshot":
		value, err = g.mailAttachmentSnapshot(request.Params)
	case "handoff-authorize":
		value, err = g.authorizeHandoff(r.Context(), request.Params)
	case "oneshot-text":
		var input struct {
			Prompt    string
			TimeoutMs int64
			MaxTokens int
			Route     struct{ ProviderID, Model string }
		}
		if err = json.Unmarshal(request.Params, &input); err == nil {
			selection := g.control.store.Preferences().TitleModel
			if input.Route.Model != "" {
				selection = ai.Selection{Provider: input.Route.ProviderID, Model: input.Route.Model}
			}
			value = g.control.automations.utilityText(r.Context(), input.Prompt, selection.Provider, selection.Model, time.Duration(input.TimeoutMs)*time.Millisecond, input.MaxTokens)
		}
	case "image-generate", "video-generate", "embed-text":
		value, err = g.pluginMedia(r.Context(), request.Method, request.Params)
	case "media-config":
		value, err = g.mediaCatalog(r.Context())
	case "models":
		value = g.modelList()
	default:
		err = errors.New("插件请求的宿主能力不可用")
	}
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		var taskError *ghostTaskFailure
		if errors.As(err, &taskError) {
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "name": "PluginTaskError", "code": taskError.Code, "message": taskError.Message, "retryable": false})
		} else {
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "message": err.Error()})
		}
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "value": value})
}

func (g *chatGhosts) modelList() []map[string]any {
	rows := []map[string]any{}
	config, err := g.control.aiConfiguration()
	if err != nil {
		return rows
	}
	for _, provider := range config.Providers {
		for _, model := range provider.Models {
			metadata := provider.ModelMetadata[model]
			efforts := append(append([]string{}, metadata.Efforts...), metadata.Levels...)
			rows = append(rows, map[string]any{"id": model, "providerId": provider.ID, "providerName": provider.Name, "name": model, "efforts": efforts})
		}
	}
	return rows
}

// mediaConfig offers the enabled models as Cindy's image catalog does; the plugin
// asks for one by id and the gateway applies the user's policy to it.
func (g *chatGhosts) mediaConfig() map[string]any {
	models := []map[string]any{}
	for _, row := range g.modelList() {
		models = append(models, map[string]any{"id": row["id"], "name": row["name"], "providerId": row["providerId"]})
	}
	var defaults any
	if len(models) > 0 {
		defaults = map[string]any{"default": models[0]["id"]}
	}
	return map[string]any{"models": models, "defaults": defaults}
}

// servePluginControl is the same Ghost owner for scheduler and routine bridges.
func (a *chatAutomations) servePluginControl(w http.ResponseWriter, r *http.Request) bool {
	method := ""
	switch strings.TrimPrefix(r.URL.Path, "/v1/") {
	case "plugin-candidates":
		method = "candidates"
	case "plugin-resolve":
		method = "resolve"
	case "plugin-screen":
		method = "screen"
	default:
		return false
	}
	if a.control.ghosts == nil {
		http.Error(w, "Plugin host unavailable", http.StatusServiceUnavailable)
		return true
	}
	var params json.RawMessage
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&params); err != nil {
		http.Error(w, "Invalid plugin request", http.StatusBadRequest)
		return true
	}
	value, err := a.control.ghosts.call(r.Context(), method, params)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return true
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(value)
	return true
}
