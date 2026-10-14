//go:build windows

package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"winone/internal/configjson"
	"winone/internal/settings"
)

type automationMethod struct {
	Server       string            `json:"server"`
	Tool         string            `json:"tool"`
	Arguments    map[string]any    `json:"arguments,omitempty"`
	ParameterMap map[string]string `json:"parameterMap,omitempty"`
}
type automationConnections struct {
	Methods map[string]automationMethod `json:"methods"`
}

var automationMethods = []string{"jira.get", "jira.search_jql", "jira.add_comment", "feishu.recent_chats", "feishu.recent_messages", "notify.feishu", "notify.wecom"}

func (a *chatAutomations) connections() (automationConnections, error) {
	value := automationConnections{Methods: map[string]automationMethod{}}
	raw, err := os.ReadFile(filepath.Join(a.control.directory, "chat", "automation-connections.json"))
	if errors.Is(err, os.ErrNotExist) {
		return value, nil
	}
	if err != nil {
		return value, err
	}
	err = configjson.DecodeObject(raw, &value)
	return value, err
}
func (a *chatAutomations) saveConnections(raw json.RawMessage) (json.RawMessage, error) {
	var input struct {
		Settings automationConnections `json:"settings"`
	}
	if err := configjson.DecodeObject(raw, &input); err != nil {
		return nil, err
	}
	for method, mapping := range input.Settings.Methods {
		if !slices.Contains(automationMethods, method) || strings.TrimSpace(mapping.Server) == "" || strings.TrimSpace(mapping.Tool) == "" {
			return nil, errors.New("请为支持的自动化方法填写 MCP 服务和工具名称")
		}
	}
	value, err := json.Marshal(input.Settings)
	if err != nil {
		return nil, err
	}
	if err = settings.WriteAtomic(filepath.Join(a.control.directory, "chat", "automation-connections.json"), value); err != nil {
		return nil, err
	}
	return value, nil
}

// The script supplies only message/title/target. Model, workspace, permissions
// and binding come from the saved schedule; script input cannot override them.
func (a *chatAutomations) dispatchScript(ctx context.Context, schedule scheduledChat, raw json.RawMessage) (any, error) {
	if schedule.ScriptConfig == nil || !slices.Contains(schedule.ScriptConfig.Capabilities, "sessions.dispatch") {
		return nil, errors.New("sessions.dispatch 未获授权")
	}
	var input struct {
		Message string `json:"message"`
		Title   string `json:"title"`
		Target  string `json:"target_session_id"`
	}
	if err := configjson.DecodeObject(raw, &input); err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.Message) == "" || len(input.Message) > 100000 {
		return nil, errors.New("请输入有效消息")
	}
	if schedule.TargetSessionID != "" && input.Target != "" && schedule.TargetSessionID != input.Target {
		return nil, errors.New("绑定脚本只能发送到绑定会话")
	}
	if input.Target != "" {
		schedule.TargetSessionID = input.Target
	}
	schedule.ExecutionMode = "agent"
	schedule.UseWorktree = false
	schedule.PersistentSession = false
	if input.Title != "" {
		schedule.Name = input.Title
	}
	conversation, busy, err := a.prepare(ctx, schedule)
	if err != nil {
		return nil, err
	}
	for busy {
		cursor, err := a.control.engine.Cursor(conversation.ID)
		if err != nil {
			return nil, err
		}
		page, err := a.control.engine.WaitEvents(ctx, conversation.ID, cursor)
		if err != nil {
			return nil, err
		}
		busy = page.Running
	}
	if _, err = a.control.engine.Open(ctx, conversation.ID, 0); err != nil {
		return nil, err
	}
	requestID := "script-" + rand.Text()
	if _, err = a.control.engine.Prompt(ctx, conversation.ID, requestID, input.Message); err != nil {
		a.control.engine.Release(conversation.ID, 0)
		return nil, err
	}
	go func() {
		defer a.control.engine.Release(conversation.ID, 0)
		cursor, _ := a.control.engine.Cursor(conversation.ID)
		for {
			page, err := a.control.engine.WaitEvents(a.control.ctx, conversation.ID, cursor)
			if err != nil || !page.Running {
				return
			}
			cursor = page.Cursor
		}
	}()
	return map[string]any{"target_session_id": conversation.ID, "agent_kind": "pi", "wake_kind": "accepted", "target_title": conversation.Title}, nil
}
