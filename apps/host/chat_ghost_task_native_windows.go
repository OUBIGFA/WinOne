//go:build windows

package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"winone/internal/ai"
	"winone/internal/chat"
)

// Only the authenticated Ghost worker enters this namespace. Public tasks
// requests first pass Cindy's receipt service; the Host independently binds
// every mutation to the plugin's persisted conversation identity.
type ghostTaskFailure struct{ Code, Message string }

func (e *ghostTaskFailure) Error() string      { return e.Message }
func ghostTaskFail(code, message string) error { return &ghostTaskFailure{code, message} }

type ghostTaskRoute struct {
	AgentKind  string          `json:"agentKind"`
	ProviderID string          `json:"providerId"`
	Model      string          `json:"model"`
	Effort     string          `json:"effort"`
	FastMode   json.RawMessage `json:"fastMode"`
}

type ghostTaskImage struct{ Name, MimeType, Hash, Data string }

func saveGhostInputImages(row chat.Conversation, images []ghostTaskImage) ([]string, error) {
	files := []string{}
	for _, image := range images {
		data, err := base64.StdEncoding.DecodeString(image.Data)
		if err != nil {
			return nil, ghostTaskFail("INVALID_ATTACHMENTS", "Invalid image")
		}
		hash := sha256.Sum256(data)
		if hex.EncodeToString(hash[:]) != image.Hash {
			return nil, ghostTaskFail("INVALID_ATTACHMENTS", "Image digest differs")
		}
		ext := map[string]string{"image/png": ".png", "image/jpeg": ".jpg", "image/webp": ".webp"}[image.MimeType]
		if ext == "" {
			return nil, ghostTaskFail("INVALID_ATTACHMENTS", "Unsupported image")
		}
		dir := filepath.Join(row.SessionDirectory, "plugin-inputs")
		if err = os.MkdirAll(dir, 0700); err != nil {
			return nil, err
		}
		file := filepath.Join(dir, image.Hash+ext)
		if err = os.WriteFile(file, data, 0600); err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	return files, nil
}

func ghostConversationView(row chat.Conversation) map[string]any {
	status := "active"
	if row.Archived {
		status = "archived"
	}
	return map[string]any{"taskId": row.ID, "title": row.Title, "status": status, "revision": row.Revision, "workingDir": row.Directory, "permissionMode": row.Permission, "planModeEnabled": row.LegacyPlan, "hidden": row.Hidden,
		"resolvedConfig": map[string]any{"agentKind": "pi", "providerId": row.Model.Provider, "model": row.Model.Model, "effort": row.ThinkingLevel, "fastMode": row.FastMode}}
}
func (g *chatGhosts) resolveTaskRoute(route *ghostTaskRoute, source string) (ghostTaskRoute, error) {
	configuration, err := g.control.aiConfiguration()
	if err != nil {
		return ghostTaskRoute{}, err
	}
	if route == nil {
		selected := configuration.Tasks["chat"]
		effort := ""
		fast := false
		if row, e := g.control.store.Conversation(source); e == nil {
			selected = row.Model
			effort = row.ThinkingLevel
			fast = row.FastMode
		}
		fastJSON, _ := json.Marshal(fast)
		route = &ghostTaskRoute{AgentKind: "pi", ProviderID: selected.Provider, Model: selected.Model, Effort: effort, FastMode: fastJSON}
	}
	if route.AgentKind != "pi" {
		return ghostTaskRoute{}, ghostTaskFail("ROUTE_UNAVAILABLE", "WinOne plugin tasks use Pi")
	}
	selected, fast, err := oneBotRouteSelection(oneBotModelRoute{Harness: "pi", ProviderID: route.ProviderID, Model: route.Model, Effort: route.Effort, FastMode: route.FastMode}, configuration)
	if err != nil {
		return ghostTaskRoute{}, ghostTaskFail("ROUTE_UNAVAILABLE", err.Error())
	}
	out := *route
	out.ProviderID, out.Model = selected.Provider, selected.Model
	out.FastMode, _ = json.Marshal(fast)
	return out, nil
}
func (g *chatGhosts) pluginNative(ctx context.Context, raw json.RawMessage) (any, error) {
	var input struct {
		Method, PluginID, TaskID, Title, WorkingDir, SourceSessionID, ClientID, Text, Context, RequestID, After, Mode, PermissionMode string
		Route                                                                                                                         *ghostTaskRoute
		Hidden, IsolatedWorkspace, Project, AutoTitle                                                                                 bool
		ExpectedRevision                                                                                                              uint64
		Cursor                                                                                                                        uint64
		Limit                                                                                                                         int
		InputClientIDs                                                                                                                []string
		Execution                                                                                                                     json.RawMessage
		Images                                                                                                                        []ghostTaskImage
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	c := g.control
	// Model validation is a read-only settings operation, not task authority.
	if input.Method == "resolve-route" {
		return g.resolveTaskRoute(input.Route, input.SourceSessionID)
	}
	if input.PluginID == "" {
		return nil, ghostTaskFail("PERMISSION_DENIED", "Missing plugin identity")
	}
	switch input.Method {
	case "validate-directory":
		if !filepath.IsAbs(input.WorkingDir) {
			return nil, ghostTaskFail("INVALID_REQUEST", "请选择绝对路径的工作目录")
		}
		info, err := os.Stat(input.WorkingDir)
		if err != nil || !info.IsDir() {
			return nil, ghostTaskFail("INVALID_REQUEST", "工作目录不存在或无法读取")
		}
		return true, nil
	case "models":
		models := []map[string]any{}
		for _, row := range g.modelList() {
			models = append(models, map[string]any{"id": row["providerId"].(string) + "/" + row["id"].(string), "route": map[string]any{"agentKind": "pi", "providerId": row["providerId"], "model": row["id"], "effort": "", "fastMode": false}, "efforts": row["efforts"]})
		}
		return map[string]any{"models": models}, nil
	case "create":
		route, err := g.resolveTaskRoute(input.Route, input.SourceSessionID)
		if err != nil {
			return nil, err
		}
		if old, e := c.store.Conversation(input.TaskID); e == nil {
			if old.PluginID != input.PluginID {
				return nil, ghostTaskFail("TASK_NOT_FOUND", "Task not found")
			}
			return ghostConversationView(old), nil
		}
		directory := input.WorkingDir
		if directory == "" && input.SourceSessionID != "" && !input.IsolatedWorkspace {
			if source, e := c.store.Conversation(input.SourceSessionID); e == nil {
				directory = source.Directory
			}
		}
		if input.IsolatedWorkspace {
			directory = ""
		}
		projectID := ""
		if input.Project {
			for _, project := range c.store.Snapshot().Projects {
				if strings.EqualFold(filepath.Clean(project.Path), filepath.Clean(directory)) {
					projectID = project.ID
					break
				}
			}
			if projectID == "" {
				return nil, ghostTaskFail("TARGET_UNAVAILABLE", "Project is no longer available")
			}
		}
		var fast bool
		_ = json.Unmarshal(route.FastMode, &fast)
		title := input.Title
		// Cindy keeps its New Maker sentinel in the receipt; WinOne's ordinary
		// title path uses Chat. Adapt only opted-in auto naming, not user text.
		if input.AutoTitle && title == "New Maker" {
			title = "Chat"
		}
		row, err := c.store.NewPluginConversation(chat.Conversation{ID: input.TaskID, ProjectID: projectID, PluginID: input.PluginID, Hidden: input.Hidden, Title: title, Directory: directory, Model: ai.Selection{Provider: route.ProviderID, Model: route.Model}, Permission: input.PermissionMode, ThinkingLevel: route.Effort, FastMode: fast})
		if err != nil {
			return nil, err
		}
		return ghostConversationView(row), nil
	case "background-list":
		rows := []map[string]any{}
		for _, row := range c.store.Snapshot().Conversations {
			if row.PluginID == input.PluginID && row.Hidden {
				rows = append(rows, ghostConversationView(row))
			}
		}
		return rows, nil
	case "reveal-all":
		for _, row := range c.store.Snapshot().Conversations {
			if row.PluginID == input.PluginID && row.Hidden {
				if _, err := c.store.UpdateConversation(row.ID, func(value *chat.Conversation) error { value.Hidden = false; return nil }); err != nil {
					return nil, err
				}
			}
		}
		return true, nil
	}
	row, err := c.store.Conversation(input.TaskID)
	if err != nil || row.PluginID != input.PluginID {
		return nil, ghostTaskFail("TASK_NOT_FOUND", "Task not found")
	}
	if input.Method == "reveal" {
		updated, err := c.store.UpdateConversation(row.ID, func(value *chat.Conversation) error { value.Hidden = false; return nil })
		if err != nil {
			return nil, err
		}
		return ghostConversationView(updated), nil
	}
	if input.Method == "get" {
		return ghostConversationView(row), nil
	}
	if input.Method == "hidden" {
		updated, err := c.store.UpdateConversation(row.ID, func(value *chat.Conversation) error { value.Hidden = input.Hidden; return nil })
		if err != nil {
			return nil, err
		}
		return ghostConversationView(updated), nil
	}
	if input.Method == "set-model" || input.Method == "permission" || input.Method == "permission-restore" || input.Method == "archive" {
		var selected ghostTaskRoute
		if input.Method == "set-model" {
			selected, err = g.resolveTaskRoute(input.Route, "")
			if err != nil {
				return nil, err
			}
		}
		if input.Method == "set-model" && c.engine.Activity()[row.ID].Phase == "running" {
			configuration, err := c.aiConfiguration()
			if err != nil {
				return nil, err
			}
			model, err := chatModel(configuration, ai.Selection{Provider: selected.ProviderID, Model: selected.Model})
			if err != nil {
				return nil, err
			}
			models := []map[string]any{}
			for _, provider := range configuration.Providers {
				for _, name := range provider.Models {
					value, e := chatModel(configuration, ai.Selection{Provider: provider.ID, Model: name})
					if e != nil {
						return nil, e
					}
					models = append(models, value)
				}
			}
			var fast bool
			_ = json.Unmarshal(selected.FastMode, &fast)
			updated, err := c.engine.DeferModelConfiguration(ctx, row.ID, map[string]any{"model": model, "models": models, "thinkingLevel": selected.Effort, "fastMode": fast}, func(value *chat.Conversation) error {
				if value.PluginID != input.PluginID || value.Revision != input.ExpectedRevision {
					return ghostTaskFail("STALE_REVISION", "Task configuration changed")
				}
				value.Model, value.ThinkingLevel, value.FastMode = ai.Selection{Provider: selected.ProviderID, Model: selected.Model}, selected.Effort, fast
				return nil
			})
			if err == nil {
				return map[string]any{"status": "deferred", "task": ghostConversationView(updated)}, nil
			}
			if !errors.Is(err, chat.ErrBusy) {
				return nil, err
			}
		}
		// Keep the freshly opened native process owned through the state fence.
		// A prior read-only inspect may already have released its idle Worker.
		if _, err = c.engine.Open(ctx, row.ID, 0); err != nil {
			return nil, err
		}
		defer c.engine.Release(row.ID, 0)
		updated, err := c.engine.ConfigureConversationWithCheck(ctx, row.ID, func(snapshot json.RawMessage) error {
			var state struct {
				Running                       bool
				Pending, Approvals, Questions []json.RawMessage
			}
			if json.Unmarshal(snapshot, &state) != nil || state.Running || len(state.Pending)+len(state.Approvals)+len(state.Questions) > 0 {
				return ghostTaskFail("TASK_BUSY", "Wait for the task to become idle")
			}
			return nil
		}, func(value *chat.Conversation) error {
			if value.PluginID != input.PluginID {
				return ghostTaskFail("TASK_NOT_FOUND", "Task not found")
			}
			if input.Method != "archive" && value.Revision != input.ExpectedRevision {
				return ghostTaskFail("STALE_REVISION", "Task configuration changed")
			}
			if value.Archived && input.Method != "archive" {
				return ghostTaskFail("TASK_BUSY", "Task is archived")
			}
			switch input.Method {
			case "set-model":
				value.ModelDeferred = false
				value.Model = ai.Selection{Provider: selected.ProviderID, Model: selected.Model}
				value.ThinkingLevel = selected.Effort
				_ = json.Unmarshal(selected.FastMode, &value.FastMode)
			case "permission", "permission-restore":
				allowed := input.Mode == "acceptEdits" || input.Mode == "auto"
				if input.Method == "permission-restore" {
					// Host-only compensation after a preference save failed. The
					// public slot still accepts only acceptEdits or Auto requests.
					allowed = allowed || input.Mode == "ask" || input.Mode == "bypassPermissions"
				}
				if !allowed {
					return ghostTaskFail("INVALID_REQUEST", "Invalid write permission")
				}
				if input.Method == "permission" && input.Mode == "acceptEdits" && value.HasMessages {
					return ghostTaskFail("TASK_BUSY", "Only an unstarted task can request edit access")
				}
				value.Permission = input.Mode
			case "archive":
				value.Archived = true
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		if input.Method == "set-model" {
			return map[string]any{"status": "updated", "task": ghostConversationView(updated)}, nil
		}
		return ghostConversationView(updated), nil
	}
	if _, err = c.engine.Open(ctx, row.ID, 0); err != nil {
		return nil, err
	}
	defer c.engine.Release(row.ID, 0)
	switch input.Method {
	case "inspect":
		value, err := c.engine.Command(ctx, row.ID, "execution.inspect", nil)
		return json.RawMessage(value), err
	case "assert-unstarted", "assert-idle":
		if input.Method == "assert-unstarted" && row.HasMessages {
			return nil, ghostTaskFail("TASK_BUSY", "Task has already started")
		}
		value, err := c.engine.Command(ctx, row.ID, "execution.inspect", nil)
		if err != nil {
			return nil, err
		}
		var state struct {
			Running                       bool
			Pending, Approvals, Questions []json.RawMessage
		}
		if err = json.Unmarshal(value, &state); err != nil {
			return nil, err
		}
		if state.Running || len(state.Pending)+len(state.Approvals)+len(state.Questions) > 0 {
			return nil, ghostTaskFail("TASK_BUSY", "Wait for the task to become idle")
		}
		return true, nil
	case "send":
		if row.Archived {
			return nil, ghostTaskFail("TASK_BUSY", "Task is archived")
		}
		if !strings.HasPrefix(input.ClientID, "plugin-task:") {
			return nil, ghostTaskFail("INVALID_REQUEST", "Invalid plugin input identity")
		}
		files, err := saveGhostInputImages(row, input.Images)
		if err != nil {
			return nil, err
		}
		text := input.Text
		if input.Context != "" {
			text += "\n\n<reference_material>\n" + input.Context + "\n</reference_material>"
		}
		result, err := c.engine.DispatchPresentedInput(ctx, row.ID, input.ClientID, text, "queue", chat.NativeInputPresentation{Text: &input.Text, ReviewRequest: json.RawMessage(`{"kind":"agent-request"}`)}, files)
		if err != nil {
			return nil, err
		}
		var receipt map[string]any
		if err = json.Unmarshal(result, &receipt); err != nil {
			return nil, err
		}
		receipt["ok"] = receipt["accepted"] == true
		return receipt, nil
	case "wait":
		return c.waitInputResult(ctx, row.ID, input.RequestID, input.Cursor)
	case "cancel":
		data, err := c.engine.Command(ctx, row.ID, "execution.inspect", nil)
		if err != nil {
			return nil, err
		}
		var state struct {
			RequestID string
			Running   bool
			Execution json.RawMessage
			Pending   []struct{ ID string }
		}
		if err = json.Unmarshal(data, &state); err != nil {
			return nil, err
		}
		for _, pending := range state.Pending {
			if slices.Contains(input.InputClientIDs, pending.ID) {
				if _, err = c.engine.Command(ctx, row.ID, "queue", map[string]string{"id": pending.ID, "action": "cancel"}); err != nil {
					return nil, err
				}
			}
		}
		if state.Running && slices.Contains(input.InputClientIDs, state.RequestID) {
			if _, err = c.engine.Command(ctx, row.ID, "abort.input", map[string]any{"id": state.RequestID, "execution": input.Execution}); err != nil {
				return nil, err
			}
			return "stopping", nil
		}
		return "cancelled", nil
	case "messages":
		return g.pluginMessages(ctx, row.ID, input.After, input.Limit)
	case "auto-title":
		// Use the regular asynchronous title path; plugin-supplied context is not
		// presented as owner instruction or allowed to change the model route.
		return c.generateTitle(ctx, row.ID, input.Text)
	}
	return nil, ghostTaskFail("INVALID_REQUEST", "Unknown native plugin operation")
}

// Shared with OneBot's existing native receipt adapter. A terminal belongs to
// the requested input; a later assistant reply must never settle another run.
func (c *chatControl) waitInputResult(ctx context.Context, id, request string, cursor uint64) (any, error) {
	result, err := c.engine.Command(ctx, id, "input.result", map[string]string{"id": request})
	if err != nil {
		return nil, err
	}
	var terminal struct{ Status string }
	if err = json.Unmarshal(result, &terminal); err != nil {
		return nil, err
	}
	snapshot, err := c.engine.Command(ctx, id, "execution.inspect", nil)
	if err != nil {
		return nil, err
	}
	if terminal.Status != "" {
		return map[string]any{"terminal": json.RawMessage(result), "cursor": cursor, "state": json.RawMessage(snapshot)}, nil
	}
	wait, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	page, err := c.engine.WaitEvents(wait, id, cursor)
	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		return nil, err
	}
	return map[string]any{"cursor": page.Cursor, "state": json.RawMessage(snapshot)}, nil
}

func (g *chatGhosts) pluginMessages(ctx context.Context, id, after string, limit int) (any, error) {
	if limit == 0 {
		limit = 50
	}
	if limit < 1 || limit > 100 {
		return nil, ghostTaskFail("INVALID_REQUEST", "Invalid page size")
	}
	raw, err := g.control.engine.Command(ctx, id, "state", nil)
	if err != nil {
		return nil, err
	}
	var state struct{ Messages []json.RawMessage }
	if err = json.Unmarshal(raw, &state); err != nil {
		return nil, err
	}
	items := []map[string]any{}
	found := after == ""
	next := ""
	for _, entry := range state.Messages {
		var message struct {
			ID, RequestID, Role string
			Content             json.RawMessage
			Timestamp           int64
		}
		if json.Unmarshal(entry, &message) != nil {
			continue
		}
		if !found {
			if message.ID == after {
				found = true
			}
			continue
		}
		if len(items) == limit {
			break
		}
		items = append(items, map[string]any{"id": message.ID, "role": message.Role, "content": message.Content, "createdAt": message.Timestamp})
		next = message.ID
	}
	if !found {
		return nil, ghostTaskFail("INVALID_REQUEST", "Invalid message cursor")
	}
	var cursor any
	if len(items) == limit {
		cursor = next
	}
	return map[string]any{"items": items, "nextCursor": cursor}, nil
}
