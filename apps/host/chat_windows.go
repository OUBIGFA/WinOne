//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
	"winone/internal/ai"
	"winone/internal/chat"
	"winone/internal/configjson"
	"winone/internal/host"
	"winone/internal/processidentity"
	"winone/internal/recycle"
)

type chatControl struct {
	ctx                context.Context
	directory, desktop string
	language           func() string
	ai                 *aiService
	store              *chat.Store
	engine             *chat.Service
	failure            error
	mu                 sync.Mutex
	owners             map[uint32]*processidentity.Process
	guard              func() (func(), bool)
	automations        *chatAutomations
	previews           *chat.PreviewServer
	terminals          *chat.TerminalManager
	fileSearches       sync.Map
}

func registerChat(service *host.Service, ctx context.Context, directory, executable string, engine *aiService, language func() string) *chatControl {
	c := &chatControl{ctx: ctx, directory: directory, desktop: filepath.Join(filepath.Dir(executable), "WinOne.exe"), language: language, ai: engine, owners: map[uint32]*processidentity.Process{}}
	c.store, c.failure = chat.Open(filepath.Join(directory, "chat"))
	if c.failure == nil {
		c.terminals = chat.NewTerminalManager(ctx)
		c.engine = chat.NewService(ctx, c.store, chat.ServiceOptions{Bun: filepath.Join(filepath.Dir(executable), "shared", "ai", "runtime", "bun.exe"), Worker: filepath.Join(filepath.Dir(executable), "shared", "chat", "worker.js"), Configure: c.configure})
		c.automations = newChatAutomations(c)
		go c.automations.startSaved()
	}
	service.RegisterModule("chat", func() host.Module { return host.Module{ID: "chat", Name: "Chat", State: "ready"} }, c.handle)
	return c
}
func (c *chatControl) aiConfiguration() (ai.Configuration, error) {
	c.ai.mu.Lock()
	defer c.ai.mu.Unlock()
	var err error
	if c.ai.store == nil {
		c.ai.store, err = ai.Open(c.ai.storePath)
	}
	if err != nil {
		return ai.Configuration{}, err
	}
	return c.ai.store.Configuration(), nil
}
func chatModel(config ai.Configuration, selection ai.Selection) (map[string]any, error) {
	for _, provider := range config.Providers {
		if provider.ID == selection.Provider && slices.Contains(provider.Models, selection.Model) {
			metadata := provider.ModelMetadataFor(selection.Model)
			return map[string]any{"id": provider.ID + "/" + selection.Model, "name": provider.ModelName(selection.Model), "provider": provider.ID, "providerName": provider.Name, "modelName": provider.ModelName(selection.Model), "contextWindow": metadata.ContextWindow, "listedContextWindow": metadata.ListedContextWindow, "maxContextWindow": metadata.MaxContextWindow, "maxTokens": metadata.MaxTokens, "reasoning": metadata.Reasoning, "vision": metadata.Vision, "tools": metadata.Tools, "compactionThreshold": metadata.CompactionThreshold, "compactionPercent": metadata.CompactionPercent}, nil
		}
	}
	return nil, errors.New("请在 AI 服务中启用模型，再为对话选择模型")
}
func (c *chatControl) configure(conversation chat.Conversation) (map[string]any, func(), error) {
	config, err := c.aiConfiguration()
	if err != nil {
		return nil, nil, err
	}
	model, err := chatModel(config, conversation.Model)
	if err != nil {
		return nil, nil, err
	}
	bridge, err := chat.NewPrivateBridge(c.ctx, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c.automationPolicy(w, r, conversation.ID) {
			return
		}
		// This identity comes from WinOne's registered conversation, never the
		// public gateway or an untrusted client header.
		cloned := r.Clone(r.Context())
		cloned.Header.Set("X-WinOne-Session", conversation.ID)
		c.ai.serveChat(w, cloned)
	}))
	if err != nil {
		return nil, nil, err
	}
	prefs := c.store.Preferences()
	// Child model overrides use the same private bridge and enabled model list.
	models := []map[string]any{}
	for _, provider := range config.Providers {
		for _, id := range provider.Models {
			entry, modelErr := chatModel(config, ai.Selection{Provider: provider.ID, Model: id})
			if modelErr != nil {
				bridge.Close()
				return nil, nil, modelErr
			}
			models = append(models, entry)
		}
	}
	subagentModel := ""
	if prefs.SubagentModel.Provider != "" {
		selected, modelErr := chatModel(config, prefs.SubagentModel)
		if modelErr != nil {
			bridge.Close()
			return nil, nil, modelErr
		}
		subagentModel = selected["id"].(string)
	}
	runtimeSettings := prefs.RuntimeSettings()
	runtimeSettings["subagentModel"] = subagentModel
	return map[string]any{"baseUrl": bridge.BaseURL, "key": bridge.Key, "model": model, "thinkingLevel": conversation.ThinkingLevel, "fastMode": conversation.FastMode, "runtimeSettings": runtimeSettings, "models": models, "language": c.language(), "agentDir": filepath.Join(c.directory, "chat", "agents", conversation.ID), "mcpAuthDirectory": filepath.Join(c.directory, "chat", "mcp"), "skillDirectories": prefs.SkillDirectories, "pluginDirectories": prefs.PluginDirectories, "mcpServers": prefs.MCPServers, "logPath": filepath.Join(c.directory, "logs", "host.log"), "protectedPaths": []string{filepath.Join(c.directory, "ai-settings.json"), filepath.Join(c.directory, "ai-runtime"), filepath.Join(c.directory, "chat", "agents"), filepath.Join(c.directory, "chat", "index.json"), filepath.Join(c.directory, "chat", "settings.json"), filepath.Join(c.directory, "chat", "mcp"), conversation.SessionDirectory, filepath.Dir(c.desktop)}}, bridge.Close, nil
}

// Verify the actual pipe peer, never a PID supplied by browser content. Keeping
// the process handle also makes UI crashes release idle Pi workers reliably.
func (c *chatControl) attach(pid uint32) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if prior := c.owners[pid]; prior != nil {
		if exited, err := prior.Exited(); err == nil && !exited {
			return nil
		}
		return errors.New("桌面进程已退出，请重新打开 Chat")
	}
	process, err := processidentity.Open(pid, c.desktop)
	if err != nil {
		return err
	}
	c.owners[pid] = process
	go func() {
		_ = process.Wait(c.ctx)
		c.engine.Detach(pid)
		if c.terminals != nil {
			c.terminals.Detach(pid)
		}
		c.mu.Lock()
		if c.owners[pid] == process {
			delete(c.owners, pid)
		}
		c.mu.Unlock()
		process.Close()
	}()
	return nil
}
func (c *chatControl) handle(r host.Request) host.Response {
	fail := func(err error) host.Response {
		return host.Response{ID: r.ID, Error: &host.RPCError{Code: "chat", Message: err.Error()}}
	}
	result := func(value any) host.Response {
		raw, err := json.Marshal(value)
		if err != nil {
			return fail(err)
		}
		return host.Response{ID: r.ID, Result: raw}
	}
	if c.failure != nil {
		return fail(c.failure)
	}
	if err := c.attach(r.PeerPID); err != nil {
		return fail(err)
	}
	switch r.Method {
	case "usage", "project.git", "catalog", "settings.get", "resources", "files", "draft.get", "poll", "state", "stop", "release", "detach", "automations.deleted", "automations.project.read", "automations.connections.get", "automations.templates", "automations.template.apply", "automations.list", "automations.get", "automations.runs", "automations.events", "automations.runtime", "automations.preview":
	default:
		if c.guard != nil {
			release, ok := c.guard()
			if !ok {
				return fail(errors.New("WinOne 正在维护，请稍后重试"))
			}
			defer release()
		}
	}
	// A bounded cold start may take longer than the generic five-second UI RPC.
	if r.Method == "export" {
		return c.exportArchive(r)
	}
	ctx, cancel := context.WithTimeout(c.ctx, 25*time.Second)
	defer cancel()
	if r.Method == "usage" {
		var query chat.UsageQuery
		if err := configjson.DecodeObject(r.Params, &query); err != nil {
			return fail(err)
		}
		var pricing struct {
			SessionCosts map[string]chat.UsagePrice `json:"sessionCosts"`
		}
		var priceError error
		if c.ai != nil {
			runtime, err := c.ai.magpieTranslator()
			var raw json.RawMessage
			if err == nil {
				raw, err = runtime.Usage(ctx, ai.UsageQuery{Period: query.Period, Provider: query.Provider, Model: query.Model})
			}
			priceError = err
			if err == nil {
				priceError = json.Unmarshal(raw, &pricing)
			}
		}
		usage, err := c.store.Usage(query, pricing.SessionCosts)
		if err != nil {
			return fail(err)
		}
		if priceError != nil {
			usage.Warnings = append(usage.Warnings, "会话费用暂不可用，Token 统计仍可查看。")
		}
		return result(usage)
	}
	if strings.HasPrefix(r.Method, "automations.") {
		value, err := c.automations.call(ctx, strings.TrimPrefix(r.Method, "automations."), r.Params)
		if err != nil {
			return fail(err)
		}
		return result(value)
	}
	if r.Method == "link.open" {
		if err := openChatLink(r.Params); err != nil {
			return fail(err)
		}
		return result(nil)
	}
	if strings.HasPrefix(r.Method, "file.") {
		value, err := c.fileAction(r.Method, r.Params)
		if err != nil {
			return fail(err)
		}
		return result(value)
	}
	if strings.HasPrefix(r.Method, "files.") {
		value, err := c.fileSearchAction(ctx, r.Method, r.Params)
		if err != nil {
			return fail(err)
		}
		return result(value)
	}
	if strings.HasPrefix(r.Method, "media.") {
		value, err := c.mediaAction(ctx, r.Method, r.Params)
		if err != nil {
			return fail(err)
		}
		return result(value)
	}
	if strings.HasPrefix(r.Method, "terminal.") {
		value, err := c.terminalAction(r.PeerPID, r.Method, r.Params)
		if err != nil {
			return fail(err)
		}
		return result(value)
	}
	if strings.HasPrefix(r.Method, "review.") {
		value, err := c.reviewAction(ctx, r.Method, r.Params)
		if err != nil {
			return fail(err)
		}
		return result(value)
	}
	var input struct {
		ID              string           `json:"id"`
		Name            string           `json:"name"`
		Path            string           `json:"path"`
		Parent          string           `json:"parent"`
		ProjectID       string           `json:"projectId"`
		Directory       string           `json:"directory"`
		SaveDirectory   string           `json:"saveDirectory"`
		Title           string           `json:"title"`
		Model           ai.Selection     `json:"model"`
		Permission      string           `json:"permission"`
		Archived        *bool            `json:"archived"`
		Pinned          *bool            `json:"pinned"`
		Text            string           `json:"text"`
		RequestID       string           `json:"requestId"`
		DeliveryMode    string           `json:"deliveryMode"`
		RestoreRevision string           `json:"restoreRevision"`
		Draft           chat.Draft       `json:"draft"`
		Cursor          uint64           `json:"cursor"`
		ApprovalID      string           `json:"approvalId"`
		Answers         []string         `json:"answers"`
		CancelQuestion  bool             `json:"cancel"`
		Allow           bool             `json:"allow"`
		Scope           string           `json:"scope"`
		Before          string           `json:"before"`
		Instructions    string           `json:"instructions"`
		Entry           string           `json:"entry"`
		Settings        chat.Preferences `json:"settings"`
		Paths           []string         `json:"paths"`
		Attachments     []string         `json:"attachments"`
		ThinkingLevel   string           `json:"thinkingLevel"`
		FastMode        *bool            `json:"fastMode"`
		Action          string           `json:"action"`
		AgentCursor     map[string]any   `json:"agentCursor"`
	}
	if len(r.Params) > 0 && string(r.Params) != "null" {
		if err := configjson.DecodeObject(r.Params, &input); err != nil {
			return fail(err)
		}
	}
	var value any
	var err error
	switch r.Method {
	case "project.git":
		return c.projectGit(r)
	case "catalog":
		value = c.store.Snapshot()
	case "settings.get":
		value = c.store.Preferences()
	case "settings.save":
		if input.Settings.TitleModel.Provider != "" {
			var config ai.Configuration
			config, err = c.aiConfiguration()
			if err == nil {
				_, err = chatModel(config, input.Settings.TitleModel)
			}
		}
		if err == nil {
			value, err = c.store.SavePreferences(input.Settings)
		}
	case "title.generate":
		value, err = c.generateTitle(ctx, input.ID, input.Text)
	case "title.regenerate":
		value, err = c.generateTitle(ctx, input.ID, input.Text, true)
	case "project.permission":
		value, err = c.configureProjectPermission(ctx, input.ID, input.Permission)
	case "project.update":
		value, err = c.store.RenameProject(input.ID, input.Name)
	case "conversation.delete":
		_, err = c.engine.ConfigureConversation(ctx, input.ID, func(*chat.Conversation) error { return nil })
		if err == nil {
			err = c.store.DeleteConversation(input.ID, func(directory string) error { _, failure := recycle.Directory(ctx, directory); return failure })
		}
		value = c.store.Snapshot()
	case "resources":
		value, err = c.engine.Resources(ctx, input.ID, r.PeerPID)
	case "files":
		value, err = c.store.Files(input.ID, input.Path)
	case "attachments.add":
		var attachments []chat.Attachment
		attachments, err = c.store.AddAttachments(input.ID, input.Paths)
		value = map[string]any{"attachments": attachments}
	case "project.add":
		value, err = c.store.AddProject(input.Path, input.Name, input.Permission)
	case "project.create":
		value, err = c.store.CreateProject(input.Parent, input.Name, input.Permission)
	case "project.remove":
		err = c.store.RemoveProject(input.ID)
		value = c.store.Snapshot()
	case "conversation.new":
		if input.Model.Provider == "" {
			var config ai.Configuration
			config, err = c.aiConfiguration()
			if err == nil {
				input.Model = config.Tasks["chat"]
			}
		}
		if err == nil {
			value, err = c.store.NewConversation(input.ProjectID, input.Directory, input.Title, input.Model, input.SaveDirectory)
		}
		if err == nil && (input.ThinkingLevel != "" || input.FastMode != nil) {
			if input.ThinkingLevel != "" && !chat.ValidThinkingLevel(input.ThinkingLevel) {
				return fail(errors.New("思考强度无效"))
			}
			created := value.(chat.Conversation)
			value, err = c.store.UpdateConversation(created.ID, func(conversation *chat.Conversation) error {
				if input.ThinkingLevel != "" {
					conversation.ThinkingLevel = input.ThinkingLevel
				}
				if input.FastMode != nil {
					conversation.FastMode = *input.FastMode
				}
				return nil
			})
		}
	case "conversation.update":
		// Navigation metadata is independent of Pi's execution and history format.
		value, err = c.store.UpdateConversation(input.ID, func(conversation *chat.Conversation) error {
			if input.Title != "" {
				conversation.Title = input.Title
				conversation.TitleManual = true
			}
			if input.Archived != nil {
				conversation.Archived = *input.Archived
			}
			if input.Pinned != nil {
				conversation.Pinned = *input.Pinned
			}
			return nil
		})
	case "conversation.move":
		value, err = c.engine.MoveConversation(ctx, input.ID, input.ProjectID)
	case "draft.get":
		value, err = c.store.ReadDraft(input.ID)
	case "configure":
		if input.Model.Provider != "" {
			var config ai.Configuration
			config, err = c.aiConfiguration()
			if err == nil {
				_, err = chatModel(config, input.Model)
			}
		}
		if err == nil {
			value, err = c.engine.ConfigureConversation(ctx, input.ID, func(conversation *chat.Conversation) error {
				if input.Model.Provider != "" {
					conversation.Model = input.Model
				}
				if input.Permission != "" {
					conversation.Permission = input.Permission
				}
				return nil
			})
		}
	case "location":
		value, err = c.engine.ConfigureConversation(ctx, input.ID, func(conversation *chat.Conversation) error {
			return chat.RelocateFiles(conversation, input.SaveDirectory)
		})
	case "fork":
		value, err = c.engine.Fork(ctx, input.ID, input.Entry, input.Title)
	case "draft.save":
		err = c.store.SaveDraft(input.ID, input.Draft)
		value = map[string]bool{"saved": err == nil}
	case "open":
		value, err = c.engine.Open(ctx, input.ID, r.PeerPID)
	case "poll":
		value, err = c.engine.Poll(input.ID, input.Cursor)
	case "prompt":
		switch input.DeliveryMode {
		case "steer":
			value, err = c.engine.Steer(ctx, input.ID, input.RequestID, input.Text, input.Attachments)
		case "", "queue":
			value, err = c.engine.Prompt(ctx, input.ID, input.RequestID, input.Text, input.Attachments)
		default:
			err = errors.New("消息发送方式无效")
		}
	case "retry", "edit":
		params := map[string]any{"entry": input.Entry, "requestId": input.RequestID}
		var prepare func() error
		if r.Method == "edit" {
			params["text"] = input.Text
			if input.RestoreRevision != "" {
				prepare = func() error {
					_, err := c.store.ApplyTurnReview(ctx, input.ID, input.Entry, input.RestoreRevision, "undo", func(path string, check func() error) error {
						_, err := recycle.CheckedFile(ctx, path, check)
						return err
					})
					return err
				}
			}
		}
		value, err = c.engine.CommandPrepared(ctx, input.ID, "retry", params, prepare)
	case "state":
		value, err = c.engine.Command(ctx, input.ID, "state", map[string]any{"before": input.Before})
	case "permission":
		value, err = c.engine.Command(ctx, input.ID, "permission", map[string]any{"id": input.ApprovalID, "allow": input.Allow, "scope": input.Scope})
	case "question":
		value, err = c.engine.Command(ctx, input.ID, "question", map[string]any{"id": input.ApprovalID, "answers": input.Answers, "cancel": input.CancelQuestion})
	case "stop":
		value, err = c.engine.Command(ctx, input.ID, "abort", nil)
	case "pause", "resume":
		value, err = c.engine.Command(ctx, input.ID, r.Method, nil)
	case "compact":
		value, err = c.engine.Command(ctx, input.ID, "compact", map[string]string{"instructions": input.Instructions})
	case "thinking":
		if input.ThinkingLevel != "" && !chat.ValidThinkingLevel(input.ThinkingLevel) {
			return fail(errors.New("思考强度无效"))
		}
		value, err = c.engine.Command(ctx, input.ID, "thinking", map[string]string{"value": input.ThinkingLevel})
		if err == nil {
			value, err = c.store.UpdateConversation(input.ID, func(conversation *chat.Conversation) error {
				conversation.ThinkingLevel = input.ThinkingLevel
				return nil
			})
		}
	case "fast":
		if input.FastMode == nil {
			return fail(errors.New("缺少快速模式取值"))
		}
		value, err = c.engine.Command(ctx, input.ID, "fast", map[string]bool{"value": *input.FastMode})
		if err == nil {
			value, err = c.store.UpdateConversation(input.ID, func(conversation *chat.Conversation) error {
				conversation.FastMode = *input.FastMode
				return nil
			})
		}
	case "agents":
		switch input.Action {
		case "tasks":
			value, err = c.engine.Command(ctx, input.ID, "control", map[string]any{"action": "tasks"})
		case "", "list":
			value, err = c.engine.Command(ctx, input.ID, "control", map[string]any{"action": "list"})
		case "stop":
			if input.Name == "" {
				return fail(errors.New("缺少子代理名称"))
			}
			value, err = c.engine.Command(ctx, input.ID, "control", map[string]any{"action": "stop", "params": map[string]string{"name": input.Name}})
		case "read", "follow_up":
			if len(input.Name) == 0 || len(input.Name) > 64 || strings.ContainsAny(input.Name, "\x00\r\n") {
				return fail(errors.New("子代理名称无效"))
			}
			params := map[string]any{"name": input.Name}
			if input.Action == "follow_up" {
				if strings.TrimSpace(input.Text) == "" || len(input.Text) > 100000 {
					return fail(errors.New("后续消息为空或过长"))
				}
				params["message"] = input.Text
			} else if input.AgentCursor != nil {
				cursor, _ := json.Marshal(input.AgentCursor)
				if len(cursor) > 4096 {
					return fail(errors.New("子代理历史位置无效"))
				}
				params["cursor"] = input.AgentCursor
			}
			// Resolve the name in this parent's durable roster. UI input cannot
			// select a different database or unrelated child conversation.
			value, err = c.engine.Command(ctx, input.ID, "control", map[string]any{"action": input.Action, "params": params})
		default:
			return fail(errors.New("不支持的协作操作"))
		}
	case "release":
		c.engine.Release(input.ID, r.PeerPID)
		value = map[string]bool{"released": true}
	case "detach":
		c.engine.Detach(r.PeerPID)
		value = map[string]bool{"released": true}
	default:
		return fail(errors.New("不支持的 Chat 操作"))
	}
	if err != nil {
		return fail(err)
	}
	if r.Method == "open" || r.Method == "state" {
		// Resource paths come from Pi's loader, never the desktop request. Reopen
		// restores read-only resource tabs without requiring a listing click.
		if raw, ok := value.(json.RawMessage); ok {
			var state struct {
				Resources json.RawMessage `json:"resources"`
			}
			if err = json.Unmarshal(raw, &state); err != nil {
				return fail(err)
			}
			if len(state.Resources) > 0 {
				if err = c.store.RememberResources(input.ID, state.Resources); err != nil {
					return fail(err)
				}
			}
		}
	}
	return result(value)
}
func (c *chatControl) active() bool {
	return c.engine != nil && c.engine.Active() || c.automations != nil && c.automations.active.Load()
}
func (c *chatControl) stop() error {
	if c.terminals != nil {
		c.terminals.Close()
	}
	c.mu.Lock()
	if c.previews != nil {
		_ = c.previews.Close()
		c.previews = nil
	}
	c.mu.Unlock()
	if c.engine == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	var schedulerError error
	if c.automations != nil {
		schedulerError = c.automations.close(ctx)
	}
	return errors.Join(schedulerError, c.engine.Stop(ctx))
}
func (c *chatControl) resume() {
	if c.engine != nil {
		c.engine.Resume()
	}
	if c.automations != nil {
		c.automations.resume()
	}
}
