//go:build windows

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"winone/internal/chat"
	"winone/internal/host"
)

type oneBotContext struct {
	ProfileVersion      int                `json:"profileVersion"`
	DefaultModelChain   []oneBotModelRoute `json:"defaultModelChain"`
	BotID               string             `json:"botId"`
	Name                string             `json:"name"`
	HomeDir             string             `json:"homeDir"`
	Workspace           string             `json:"workspace"`
	CanonicalSessionID  string             `json:"canonicalSessionId"`
	SystemPrompt        string             `json:"systemPrompt"`
	SkillDirectories    []string           `json:"skillDirectories"`
	MemoryEnabled       bool               `json:"memoryEnabled"`
	ImportedEnvironment string             `json:"importedEnvironment"`
	Role                string             `json:"role"`
	Capabilities        struct {
		Model              string             `json:"model"`
		ProviderID         string             `json:"providerId"`
		Effort             string             `json:"effort"`
		FastMode           json.RawMessage    `json:"fastMode"`
		Permissions        string             `json:"permissions"`
		MCPMode            string             `json:"mcpMode"`
		MCPServers         []string           `json:"mcpServers"`
		SkillMode          string             `json:"skillMode"`
		Skills             []string           `json:"skills"`
		ToolsetMode        string             `json:"toolsetMode"`
		Toolsets           []string           `json:"toolsets"`
		ModelChain         []oneBotModelRoute `json:"modelChain"`
		ModelChainOverride json.RawMessage    `json:"modelChainOverride"`
		ModelOverride      json.RawMessage    `json:"modelOverride"`
		TaskModelOverride  *oneBotModelRoute  `json:"taskModelOverride"`
	} `json:"capabilities"`
}

func (b *chatOneBot) contextFor(ctx context.Context, params any) (oneBotContext, error) {
	runner, err := b.ensure(ctx)
	if err != nil {
		return oneBotContext{}, err
	}
	raw, err := runner.Call(ctx, "runtime.context", params)
	if err != nil {
		return oneBotContext{}, err
	}
	var value oneBotContext
	err = json.Unmarshal(raw, &value)
	return value, err
}

// Canonical session creation is serialized and durably bound before returning.
// OneBot conversations live in their own existing Chat store, never the ordinary
// Chat sidebar index; both still use the identical Pi service and renderer.
func (b *chatOneBot) openSession(ctx context.Context, botID string) (chat.Conversation, error) {
	return b.openRoute(ctx, botID, "canonical", "canonical", "")
}
func (b *chatOneBot) openRoute(ctx context.Context, botID, routeKey, role, title string) (chat.Conversation, error) {
	return b.openRouteInDirectory(ctx, botID, routeKey, role, title, "")
}

// Group plans use their recorded work directory; ordinary lanes retain the
// companion Home workspace. Route creation still shares the same admission lock.
func (b *chatOneBot) openRouteInDirectory(ctx context.Context, botID, routeKey, role, title, directory string) (chat.Conversation, error) {
	return b.openRegisteredRoute(ctx, botID, routeKey, role, title, directory, "")
}

// Only group worktree preparation supplies an identity before lane creation.
func (b *chatOneBot) openRegisteredRoute(ctx context.Context, botID, routeKey, role, title, directory, registeredID string) (chat.Conversation, error) {
	b.sessionMu.Lock()
	defer b.sessionMu.Unlock()
	runner, err := b.ensure(ctx)
	if err != nil {
		return chat.Conversation{}, err
	}
	found, err := runner.Call(ctx, "sessions.find", map[string]any{"botId": botID, "routeKey": routeKey})
	if err != nil {
		return chat.Conversation{}, err
	}
	var existing struct {
		SessionID string `json:"sessionId"`
	}
	if err = json.Unmarshal(found, &existing); err != nil {
		return chat.Conversation{}, err
	}
	if existing.SessionID != "" {
		if registeredID != "" && registeredID != existing.SessionID {
			return chat.Conversation{}, errors.New("分工会话与工作树登记不一致")
		}
		row, err := b.sessions.store.Conversation(existing.SessionID)
		if err == nil && (row.Archived || directory != "" && !strings.EqualFold(filepath.Clean(row.Directory), filepath.Clean(directory))) {
			return row, errors.New("专线已归档或工作目录与原安排不一致")
		}
		return row, err
	}
	value, err := b.contextFor(ctx, map[string]any{"botId": botID, "routeKey": routeKey, "role": role})
	if err != nil {
		return chat.Conversation{}, err
	}
	if role == "canonical" && value.CanonicalSessionID != "" {
		return b.sessions.store.Conversation(value.CanonicalSessionID)
	}
	configuration, err := b.control.aiConfiguration()
	if err != nil {
		return chat.Conversation{}, err
	}
	value.DefaultModelChain = b.defaultModelChain(configuration)
	plan, err := resolveOneBotModel(value, configuration, chat.Conversation{})
	if err != nil {
		return chat.Conversation{}, err
	}
	history := filepath.Join(b.control.directory, "onebot", "history")
	if err = os.MkdirAll(history, 0700); err != nil {
		return chat.Conversation{}, err
	}
	if title == "" {
		title = value.Name
	}
	if directory == "" {
		directory = value.Workspace
	}
	var conversation chat.Conversation
	if registeredID == "" {
		conversation, err = b.sessions.store.NewConversation("", directory, title, plan.Model, history)
	} else {
		conversation, err = b.sessions.store.NewRegisteredConversation(registeredID, directory, title, plan.Model, history)
	}
	if err != nil {
		return conversation, err
	}
	permission := oneBotPermission(value.Capabilities.Permissions)
	conversation, err = b.sessions.store.UpdateConversation(conversation.ID, func(c *chat.Conversation) error {
		c.Permission = permission
		c.ThinkingLevel = plan.ThinkingLevel
		c.FastMode = plan.FastMode
		c.OneBotModelKey = plan.Key
		return nil
	})
	if err != nil {
		return conversation, err
	}
	_, err = runner.Call(ctx, "sessions.bind", map[string]any{"botId": botID, "sessionId": conversation.ID, "routeKey": routeKey, "role": role})
	return conversation, err
}

func (b *chatOneBot) configureSession(conversation chat.Conversation) (map[string]any, func(), error) {
	ctx, cancel := context.WithTimeout(b.control.ctx, 25*time.Second)
	defer cancel()
	// Restart is called back while the lifecycle worker holds its profile lock.
	// Use its admission snapshot rather than asking the same locked worker.
	b.refreshMu.Lock()
	value, prepared := b.restartContexts[conversation.ID]
	b.refreshMu.Unlock()
	var err error
	if !prepared {
		value, err = b.contextFor(ctx, map[string]any{"sessionId": conversation.ID})
	}
	if err != nil {
		return nil, nil, err
	}
	conversation, err = b.prepareModelConversation(conversation, value)
	if err != nil {
		return nil, nil, err
	}
	configuration, err := b.control.aiConfiguration()
	if err != nil {
		return nil, nil, err
	}
	value.DefaultModelChain = b.defaultModelChain(configuration)
	plan, err := resolveOneBotModel(value, configuration, conversation)
	if err != nil {
		return nil, nil, err
	}
	config, release, err := b.control.configure(conversation)
	if err != nil {
		return nil, nil, err
	}
	bridge, err := chat.NewPrivateBridge(b.control.ctx, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		route := strings.TrimPrefix(r.URL.Path, "/v1")
		if r.Method != "POST" || (route != "/tool" && route != "/policy" && route != "/runtime" && route != "/runtime.applied" && route != "/runtime.model" && route != "/imported-tools" && route != "/imported-tool") {
			http.NotFound(w, r)
			return
		}
		var input struct {
			Method          string          `json:"method"`
			Args            json.RawMessage `json:"args"`
			RequestID       string          `json:"requestId"`
			Tool            string          `json:"tool"`
			Path            string          `json:"path"`
			External        bool            `json:"external"`
			Revision        string          `json:"revision"`
			Resources       json.RawMessage `json:"resources"`
			ModelID         string          `json:"modelId"`
			PreviousModelID string          `json:"previousModelId"`
			ModelKey        string          `json:"modelKey"`
		}
		limit := int64(256 * 1024)
		if route == "/runtime.applied" {
			limit = 1 << 20
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			http.Error(w, "OneBot 工具参数无效", http.StatusBadRequest)
			return
		}
		if route == "/runtime" || route == "/runtime.applied" || route == "/runtime.model" {
			var result any
			var err error
			if route == "/runtime" {
				result, err = b.runtimeConfiguration(r.Context(), conversation.ID)
			} else if route == "/runtime.model" {
				err = b.runtimeModelApplied(conversation.ID, input.PreviousModelID, input.ModelID, input.ModelKey, b.activeRuntimeContext(conversation.ID, value))
				result = map[string]bool{"applied": err == nil}
			} else {
				err = b.runtimeApplied(conversation.ID, input.Revision, input.Resources)
				result = map[string]bool{"applied": err == nil}
			}
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(result)
			return
		}
		if route == "/imported-tools" {
			result, err := b.importedTools(r.Context(), conversation.ID, "list", nil)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(result)
			return
		}
		grant, err := b.inputGrant(conversation.ID, input.RequestID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		ownerAuthorized := grant.Owner
		if route == "/imported-tool" {
			if !ownerAuthorized {
				http.Error(w, "导入连接只能执行主人授权的请求", http.StatusForbidden)
				return
			}
			result, err := b.importedTools(r.Context(), conversation.ID, "call", input.Args)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(result)
			return
		}
		if route == "/policy" {
			if !ownerAuthorized {
				readOnly := input.Tool == "read" || input.Tool == "ls" || input.Tool == "grep" || input.Tool == "find"
				active := b.activeRuntimeContext(conversation.ID, value)
				roots := append([]string{conversation.Directory}, active.SkillDirectories...)
				inside := false
				for _, root := range roots {
					if oneBotInside(root, input.Path) {
						inside = true
						break
					}
				}
				if input.External || !readOnly || !inside {
					taskReply := grant.ReplyTaskID != "" && (input.Tool == "message_session_task" || input.Tool == "check_session_task")
					// Cindy lets a teammate answer a teammate message on its own
					// (taskActionHost: send_to_agent); the store still requires its main chat.
					teammateMessage := input.Tool == "send_to_agent" || input.Tool == "list_agents"
					// Host wake-ups in the teammate's own chat (follow-up, routines,
					// task results, teammate messages) act on its own behalf: typed
					// teammate tools pass here and the Worker gates each action
					// (Cindy NEEDS_OWNER_REQUEST). Raw files and commands stay limited.
					ownChat := input.External && value.Role != "channel" && grant.principal() == "autonomous"
					if !taskReply && !teammateMessage && !ownChat {
						http.Error(w, "非主人触发的渠道轮次只能读取本专线工作区和已挂载技能", http.StatusForbidden)
						return
					}
				}
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"allowed": true, "requestId": input.RequestID, "channel": grant.Channel, "group": grant.Group})
			return
		}
		runner, err := b.ensure(r.Context())
		var raw json.RawMessage
		if err == nil {
			principal := grant.principal()
			params := map[string]any{"sessionId": conversation.ID, "method": input.Method, "args": input.Args, "ownerAuthorized": ownerAuthorized, "principal": principal, "requestId": input.RequestID}
			if strings.HasPrefix(input.Method, "teammate.") {
				// Cindy's teammate tools (routines, workbench, todos, digest,
				// creation). The caller is this conversation, never a tool argument.
				raw, err = runner.Call(r.Context(), input.Method, map[string]any{"callerSessionId": conversation.ID, "args": input.Args, "requestId": input.RequestID, "ownerAuthorized": ownerAuthorized, "principal": principal})
			} else if input.Method == "tasks.start" || input.Method == "tasks.get" || input.Method == "tasks.list" || input.Method == "tasks.message" || input.Method == "tasks.stop" || input.Method == "tasks.agents.list" || input.Method == "tasks.agents.send" {
				var taskParams map[string]any
				err = json.Unmarshal(input.Args, &taskParams)
				if err == nil {
					taskParams["callerSessionId"] = conversation.ID
					taskParams["requestId"] = input.RequestID
					taskParams["ownerAuthorized"] = ownerAuthorized
					taskParams["principal"] = principal
					raw, err = runner.Call(r.Context(), input.Method, taskParams)
				}
			} else {
				if input.Method == "projects.list" || input.Method == "projects.set" {
					params["catalog"], err = b.projectCatalog()
				}
				if err == nil {
					raw, err = runner.Call(r.Context(), "session.tool", params)
				}
			}
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(raw)
	}))
	if err != nil {
		release()
		return nil, nil, err
	}
	config["onebot"] = map[string]any{"botId": value.BotID, "homeDir": value.HomeDir, "systemPrompt": value.SystemPrompt, "memoryEnabled": value.MemoryEnabled, "channelScoped": value.Role == "channel", "role": value.Role, "receiptScoped": true, "baseUrl": bridge.BaseURL, "key": bridge.Key, "profileVersion": value.ProfileVersion, "modelChain": oneBotResolvedModelChain(plan, configuration), "modelChainKey": plan.Key}
	applyOneBotResources(config, value)
	return config, func() { bridge.Close(); release() }, nil
}

// Keep the same selection policy for live execution and model-free browsing.
func applyOneBotResources(config map[string]any, value oneBotContext) {
	target, ok := config["onebot"].(map[string]any)
	if !ok {
		target = map[string]any{}
		config["onebot"] = target
	}
	target["capabilities"], target["skillDirectories"], target["importedEnvironment"] = value.Capabilities, value.SkillDirectories, value.ImportedEnvironment
	own := append([]string{}, value.SkillDirectories...)
	if inherited, ok := config["skillDirectories"].([]string); ok {
		own = append(own, inherited...)
	}
	config["skillDirectories"] = own
}

func (b *chatOneBot) inspectSessionResources(ctx context.Context, conversation chat.Conversation) (map[string]any, error) {
	value, err := b.contextFor(ctx, map[string]string{"sessionId": conversation.ID})
	if err != nil {
		return nil, err
	}
	prefs := b.control.store.Preferences()
	config := map[string]any{"cwd": conversation.Directory, "agentDir": filepath.Join(b.control.directory, "chat", "agents", conversation.ID), "skillDirectories": prefs.SkillDirectories, "pluginDirectories": prefs.PluginDirectories, "mcpServers": prefs.MCPServers}
	applyOneBotResources(config, value)
	return config, nil
}

func (b *chatOneBot) handleChat(request host.Request) host.Response {
	fail := func(err error) host.Response {
		return host.Response{ID: request.ID, Error: &host.RPCError{Code: "onebot", Message: err.Error()}}
	}
	ctx, cancel := context.WithTimeout(b.control.ctx, 25*time.Second)
	defer cancel()
	if _, err := b.ensure(ctx); err != nil {
		return fail(err)
	}
	request.Method = strings.TrimPrefix(request.Method, "onebot.chat.")
	if err := b.sessions.attach(request.PeerPID); err != nil {
		return fail(err)
	}
	if request.Method == "prompt" || request.Method == "retry" || request.Method == "edit" {
		var input struct {
			ID        string `json:"id"`
			RequestID string `json:"requestId"`
			Text      string `json:"text"`
			Entry     string `json:"entry"`
		}
		if err := json.Unmarshal(request.Params, &input); err != nil {
			return fail(err)
		}
		if _, err := b.sessions.store.Conversation(input.ID); err != nil {
			return fail(err)
		}
		if len(input.Text) > 100000 || len(input.Entry) > 128 {
			return fail(errors.New("OneBot 输入无效或过长"))
		}
		content := input.Text
		if request.Method != "prompt" {
			// A local retry/edit is a fresh owner input, not an IM turn borrowing
			// the historical sender's authority. Bind its selected history entry.
			raw, _ := json.Marshal([]string{request.Method, input.Entry, input.Text})
			content = string(raw)
		}
		digest := sha256.Sum256([]byte(content))
		if err := b.grantInput(input.RequestID, oneBotInputGrant{SessionID: input.ID, ConnectionID: "desktop", Owner: true, Digest: hex.EncodeToString(digest[:])}); err != nil {
			return fail(err)
		}
	}
	// The shared conversation UI cannot create a detached task in the OneBot
	// store. Creation belongs to profiles/session.open; all normal task operations
	// keep their existing behavior and permission checks.
	if request.Method == "conversation.new" || request.Method == "automation.run" || strings.HasPrefix(request.Method, "onebot.") || strings.HasPrefix(request.Method, "project.") || strings.HasPrefix(request.Method, "automations.") {
		return fail(errors.New("请在 OneBot 中管理伙伴和关联项目"))
	}
	if request.Method == "prompt" || request.Method == "open" || request.Method == "resume" {
		var input struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(request.Params, &input); err != nil {
			return fail(err)
		}
		if err := b.flushRuntimeRefresh(ctx); err != nil {
			return fail(err)
		}
		b.refreshMu.Lock()
		revision := b.refresh[input.ID]
		b.refreshMu.Unlock()
		if revision > 0 {
			state, pollErr := b.sessions.engine.Poll(input.ID, 0)
			if pollErr == nil && !state.Running {
				if _, err := b.sessions.engine.ConfigureConversation(ctx, input.ID, func(c *chat.Conversation) error { return nil }); err != nil {
					return fail(err)
				}
				if _, err := b.sessions.engine.Open(ctx, input.ID, request.PeerPID); err != nil {
					return fail(err)
				}
				b.refreshMu.Lock()
				if b.refresh[input.ID] == revision {
					delete(b.refresh, input.ID)
				}
				b.refreshMu.Unlock()
			}
		}
	}
	return b.sessions.handle(request)
}
