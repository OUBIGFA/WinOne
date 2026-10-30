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

	"golang.org/x/sys/windows"
	"winone/internal/ai"
	"winone/internal/chat"
	"winone/internal/configjson"
)

// Host methods behind Cindy's automation page (b7e46bc). Cindy's Electron
// main process owns these; WinOne serves them from Host so the vendored page
// keeps every control: pre-run check test, folder/config opening, worktree
// detection and notification channel state.

// hookTest runs one pre-run check with the production command path (Windows
// Job, bundled Bun for xdt-node/winone-node) and returns Cindy's result shape.
func (a *chatAutomations) hookTest(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	var input struct {
		Command         string `json:"command"`
		TimeoutMs       int64  `json:"timeoutMs"`
		WorkingDir      string `json:"workingDir"`
		TargetSessionID string `json:"targetSessionId"`
		ScheduleName    string `json:"scheduleName"`
	}
	if err := configjson.DecodeObject(params, &input); err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.Command) == "" || len(input.Command) > 8192 || input.TimeoutMs < 0 || input.TimeoutMs > 24*60*60*1000 {
		return nil, errors.New("前置检查命令无效")
	}
	if _, err := a.ensure(ctx); err != nil {
		return nil, err
	}
	schedule := scheduledChat{ID: "self-test", Name: input.ScheduleName, WorkingDir: input.WorkingDir, TargetSessionID: input.TargetSessionID, PreRunHook: &scheduledCommand{Command: input.Command, TimeoutMs: input.TimeoutMs}}
	if schedule.Name == "" {
		schedule.Name = "self-test"
	}
	if schedule.WorkingDir == "" && schedule.TargetSessionID != "" {
		if conversation, err := a.control.store.Conversation(schedule.TargetSessionID); err == nil {
			schedule.WorkingDir = conversation.Directory
		}
	}
	raw, err := a.command(ctx, schedule, "hook", "self-test", time.Now().UnixMilli())
	if err != nil {
		return nil, err
	}
	var result struct {
		Value      json.RawMessage `json:"value"`
		Error      string          `json:"error"`
		PreRunHook json.RawMessage `json:"preRunHook"`
	}
	if err = json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	if len(result.Value) > 0 && string(result.Value) != "null" {
		return result.Value, nil
	}
	if len(result.PreRunHook) > 0 && string(result.PreRunHook) != "null" {
		return result.PreRunHook, nil
	}
	if result.Error != "" {
		return nil, errors.New(result.Error)
	}
	return nil, errors.New("前置检查没有返回结果")
}

// openPath opens an automation folder in Explorer, or a project's
// .cindy/automations/schedules.json with its default editor. Only folders that
// belong to automations or Chat projects qualify; nothing is executed.
func (a *chatAutomations) openPath(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	var input struct {
		Path string `json:"path"`
	}
	if err := configjson.DecodeObject(params, &input); err != nil {
		return nil, err
	}
	target := filepath.Clean(input.Path)
	if !filepath.IsAbs(target) || strings.ContainsRune(target, 0) {
		return nil, errors.New("路径无效")
	}
	allowed := map[string]bool{}
	for _, project := range a.control.store.Snapshot().Projects {
		allowed[strings.ToLower(filepath.Clean(project.Path))] = true
	}
	if runner, err := a.ensure(ctx); err == nil {
		if raw, err := runner.Call(ctx, "list", map[string]any{}); err == nil {
			var schedules []struct {
				WorkingDir string `json:"workingDir"`
			}
			if json.Unmarshal(raw, &schedules) == nil {
				for _, schedule := range schedules {
					if schedule.WorkingDir != "" {
						allowed[strings.ToLower(filepath.Clean(schedule.WorkingDir))] = true
					}
				}
			}
		}
	}
	owner, file := target, false
	if strings.EqualFold(filepath.Base(target), "schedules.json") {
		automations := filepath.Dir(target)
		if !strings.EqualFold(filepath.Base(automations), "automations") || !strings.EqualFold(filepath.Base(filepath.Dir(automations)), ".cindy") {
			return nil, errors.New("只能打开自动化的项目配置")
		}
		owner, file = filepath.Dir(filepath.Dir(automations)), true
	}
	if !allowed[strings.ToLower(owner)] {
		return nil, errors.New("只能打开自动化或项目使用的文件夹")
	}
	info, err := os.Stat(target)
	if err != nil {
		return nil, err
	}
	if file == info.IsDir() {
		return nil, errors.New("路径类型不符")
	}
	verb, _ := windows.UTF16PtrFromString("open")
	path, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return nil, err
	}
	if file {
		err = windows.ShellExecute(0, verb, path, nil, nil, windows.SW_SHOWNORMAL)
	} else {
		explorer, _ := windows.UTF16PtrFromString("explorer.exe")
		quoted, _ := windows.UTF16PtrFromString("\"" + strings.ReplaceAll(target, "\"", "") + "\"")
		err = windows.ShellExecute(0, verb, explorer, quoted, nil, windows.SW_SHOWNORMAL)
	}
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]bool{"opened": true})
}

func (a *chatAutomations) detectCwd(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	var input struct {
		WorkingDir string `json:"workingDir"`
	}
	if err := configjson.DecodeObject(params, &input); err != nil {
		return nil, err
	}
	state, err := chat.ReadWorktreeState(ctx, input.WorkingDir)
	if err != nil {
		return nil, err
	}
	return json.Marshal(state)
}

// channels reports whether automation completion channels can deliver. Both
// channels are delivered by the connector mappings in Chat settings until
// OneBot message platforms take them over.
func (a *chatAutomations) channels() (json.RawMessage, error) {
	connections, err := a.connections()
	if err != nil {
		return nil, err
	}
	feishu := "disconnected"
	if _, ok := connections.Methods["notify.feishu"]; ok {
		feishu = "connected"
	}
	_, wecom := connections.Methods["notify.wecom"]
	return json.Marshal(map[string]any{
		"feishu":     map[string]string{"status": feishu},
		"wecomGroup": map[string]bool{"configured": wecom, "enabled": wecom},
	})
}

// capabilityStatus maps script capabilities to configured connector methods.
func (a *chatAutomations) capabilityStatus() (json.RawMessage, error) {
	connections, err := a.connections()
	if err != nil {
		return nil, err
	}
	required := map[string][]string{"jira.read": {"jira.get", "jira.search_jql"}, "jira.comment": {"jira.add_comment"}, "feishu.read": {"feishu.recent_chats", "feishu.recent_messages"}}
	names := map[string]string{"jira.read": "Jira", "jira.comment": "Jira", "feishu.read": "飞书"}
	statuses := []map[string]string{{"capability": "sessions.dispatch", "state": "ok"}}
	for _, capability := range []string{"jira.read", "jira.comment", "feishu.read"} {
		state := "ok"
		for _, method := range required[capability] {
			if _, ok := connections.Methods[method]; !ok {
				state = "ghost-missing"
			}
		}
		entry := map[string]string{"capability": capability, "state": state}
		if state != "ok" {
			entry["ghostName"] = names[capability]
		}
		statuses = append(statuses, entry)
	}
	return json.Marshal(map[string]any{"statuses": statuses})
}

// utilityText serves one-shot model text for vendored Cindy helpers (pre-run
// check generation). It uses the requested AI service model or Chat's default
// with the same key/account admission as conversation titles, and reports
// failures in Cindy's UtilityTextFailure shape.
func (a *chatAutomations) utilityText(ctx context.Context, prompt, providerID, model string, timeout time.Duration) map[string]any {
	failure := func(reason, attempt string, selection ai.Selection) map[string]any {
		attempts := []map[string]any{}
		if attempt != "" {
			attempts = append(attempts, map[string]any{"providerId": selection.Provider, "model": selection.Model, "transport": "litellm-chat-completions", "status": "failed", "reason": attempt})
		}
		return map[string]any{"ok": false, "reason": reason, "attempts": attempts}
	}
	config, err := a.control.aiConfiguration()
	if err != nil {
		return failure("no_candidate", "", ai.Selection{})
	}
	selection := ai.Selection{Provider: providerID, Model: model}
	if selection.Provider == "" || selection.Model == "" {
		selection = config.Tasks["chat"]
	}
	if _, err = chatModel(config, selection); err != nil {
		return failure("no_candidate", "", selection)
	}
	c := a.control
	c.ai.mu.Lock()
	preferences, plan, err := c.ai.store.ForTaskKeys("chat", &selection)
	store, runtime := c.ai.store, c.ai.translator
	c.ai.mu.Unlock()
	if err != nil {
		return failure("all_candidates_failed", "api_key_missing", selection)
	}
	if timeout <= 0 || timeout > 2*time.Minute {
		timeout = 90 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var text string
	run := func(callCtx context.Context, key string) (bool, error) {
		var output strings.Builder
		_, err := runtime.ForProvider(preferences.ProviderID).GenerateStream(callCtx, preferences, key, ai.Generation{Text: prompt}, func(delta ai.Delta) error {
			output.WriteString(delta.Text)
			return nil
		})
		text = output.String()
		return text != "", err
	}
	if preferences.AuthMode == "account" {
		gate := c.ai.accountGateFor(preferences.ProviderID)
		select {
		case gate <- struct{}{}:
			_, err = run(ctx, "")
			<-gate
		case <-ctx.Done():
			err = ctx.Err()
		}
	} else {
		err = store.WithKeys(ctx, plan, run)
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return failure("timeout", "timeout", selection)
	case err != nil:
		return failure("all_candidates_failed", "request_failed", selection)
	case strings.TrimSpace(text) == "":
		return failure("empty_response", "empty_response", selection)
	}
	return map[string]any{"ok": true, "text": text, "providerId": selection.Provider, "model": selection.Model}
}
