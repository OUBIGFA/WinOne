//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"time"
	"unicode/utf8"

	"winone/internal/chat"
)

// Conversation facts for Cindy's teammate tools (workbench, task digest), from
// the Host that owns both Chat stores. The Worker applies the original access
// rules first; these reads return records and runtime facts, never permissions.

// workbenchMessageMaxChars matches Cindy WORKBENCH_TOOL_MAX_MESSAGE_CHARS.
const workbenchMessageMaxChars = 4000

// findConversation locates a conversation in ordinary Chat or in the
// teammates' own store (Cindy keeps both in one session table).
func (b *chatOneBot) findConversation(id string) (*chatControl, chat.Conversation, string, bool) {
	if b.control.store != nil {
		if row, err := b.control.store.Conversation(id); err == nil {
			return b.control, row, "chat", true
		}
	}
	if b.sessions != nil {
		if row, err := b.sessions.store.Conversation(id); err == nil {
			return b.sessions, row, "onebot", true
		}
	}
	return nil, chat.Conversation{}, "", false
}

func oneBotConversationRoute(row chat.Conversation) map[string]any {
	if row.Model.Model == "" {
		return nil
	}
	return map[string]any{"harness": "pi", "providerId": row.Model.Provider, "model": row.Model.Model, "effort": row.ThinkingLevel, "fastMode": row.FastMode}
}

// taskConversationFacts: the record a workbench read or digest needs, including
// the transcript location the Worker reads with Pi's own session reader.
func (b *chatOneBot) taskConversationFacts(raw json.RawMessage) (map[string]any, error) {
	var input struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	_, row, store, ok := b.findConversation(input.SessionID)
	if !ok {
		return map[string]any{"found": false}, nil
	}
	return map[string]any{"found": true, "id": row.ID, "title": row.Title, "directory": row.Directory, "sessionDirectory": row.SessionDirectory, "sessionFile": row.SessionFile,
		"archived": row.Archived, "store": store, "oneBotTask": row.OneBotTaskID != "", "route": oneBotConversationRoute(row)}, nil
}

// taskRuntimeFacts: the live activity of one conversation and, for a teammate
// caller, its effective model chain with verified context windows (Cindy
// taskDigestHost readActivity / suggestRoute inputs).
func (b *chatOneBot) taskRuntimeFacts(ctx context.Context, raw json.RawMessage) (map[string]any, error) {
	var input struct {
		SessionID       string `json:"sessionId"`
		CallerSessionID string `json:"callerSessionId"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	control, row, _, ok := b.findConversation(input.SessionID)
	if !ok {
		return map[string]any{"found": false}, nil
	}
	var activity any
	if control.engine != nil {
		if snapshots := control.engine.ActivitySnapshots(row.ID); len(snapshots) == 1 {
			activity = snapshots[0]
		}
	}
	result := map[string]any{"found": true, "activity": activity, "chain": []oneBotModelRoute{}, "windows": map[string]int{}}
	configuration, err := b.control.aiConfiguration()
	if err != nil {
		return result, nil
	}
	windows := map[string]int{}
	window := func(provider, model string) {
		for _, entry := range configuration.Providers {
			if entry.ID == provider {
				if size := entry.ModelMetadataFor(model).ContextWindow; size > 0 {
					windows[provider+"/"+model] = size
				}
			}
		}
	}
	window(row.Model.Provider, row.Model.Model)
	if input.CallerSessionID != "" {
		if value, err := b.contextFor(ctx, map[string]string{"sessionId": input.CallerSessionID}); err == nil {
			value.DefaultModelChain = b.defaultModelChain(configuration)
			if plan, err := resolveOneBotModel(value, configuration, chat.Conversation{}); err == nil {
				chain := oneBotResolvedModelChain(plan, configuration)
				for _, route := range chain {
					window(route.ProviderID, route.Model)
				}
				result["chain"] = chain
			}
		}
	}
	result["windows"] = windows
	return result, nil
}

// routineHookTest runs a teammate's new pre-run check once in its chat folder
// through the same Job-contained command path as a scheduled run (Cindy's
// installHookScript self-test), before the teammate attaches it to a routine.
func (b *chatOneBot) routineHookTest(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
	var input struct {
		SessionID    string `json:"sessionId"`
		Command      string `json:"command"`
		ScheduleName string `json:"scheduleName"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	if input.Command == "" || len(input.Command) > 32000 || len(input.ScheduleName) > 200 {
		return nil, errors.New("前置检查命令无效")
	}
	value, err := b.contextFor(ctx, map[string]string{"sessionId": input.SessionID})
	if err != nil {
		return nil, err
	}
	if value.Role != "canonical" && value.Role != "thread" {
		return nil, errors.New("只有伙伴主任务可以安装前置检查")
	}
	if b.imDriver == nil {
		return nil, errors.New("OneBot 例行任务执行器尚未就绪")
	}
	name := input.ScheduleName
	if name == "" {
		name = "self-test"
	}
	return b.imDriver.command(ctx, scheduledChat{ID: "self-test", Name: name, TargetSessionID: input.SessionID, PreRunHook: &scheduledCommand{Command: input.Command, TimeoutMs: 30000}}, "hook", "self-test", time.Now().UnixMilli())
}

// workbenchSend delivers a teammate's continue message into an owner task in
// ordinary Chat, queued behind its current turn and marked with the teammate's
// chat as its origin (Cindy send_to_session from a companion).
func (b *chatOneBot) workbenchSend(ctx context.Context, raw json.RawMessage) (map[string]any, error) {
	var input struct {
		SessionID           string `json:"sessionId"`
		Message             string `json:"message"`
		DispatcherSessionID string `json:"dispatcherSessionId"`
		ClientID            string `json:"clientId"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	failure := func(code, message string) (map[string]any, error) {
		return map[string]any{"ok": false, "errorCode": code, "message": message}, nil
	}
	if input.Message == "" || utf8.RuneCountInString(input.Message) > workbenchMessageMaxChars || input.ClientID == "" || len(input.ClientID) > 128 {
		return failure("INVALID_ARGS", "消息不能为空，且不超过 4000 字")
	}
	if b.sessions == nil {
		return nil, errors.New("OneBot 会话尚未就绪")
	}
	if _, err := b.sessions.store.Conversation(input.DispatcherSessionID); err != nil {
		return failure("NOT_A_BOT_SESSION", "当前任务不属于任何伙伴")
	}
	row, err := b.control.store.Conversation(input.SessionID)
	if err != nil {
		return failure("TASK_NOT_FOUND", "找不到这件任务，它可能已被删除")
	}
	if row.Archived {
		return failure("TASK_ARCHIVED", "这件任务已归档，请让主人先恢复它")
	}
	if b.control.engine == nil {
		return nil, errors.New("Chat 执行器尚未就绪")
	}
	if _, err = b.control.engine.Open(ctx, row.ID, 0); err != nil {
		return nil, err
	}
	defer b.control.engine.Release(row.ID, 0)
	result, err := b.control.engine.DispatchPresentedInput(ctx, row.ID, input.ClientID, input.Message, "queue", chat.NativeInputPresentation{OriginSessionID: input.DispatcherSessionID})
	if err != nil {
		// The input may already be saved: the teammate rereads the digest first.
		return failure("DELIVERY_UNKNOWN", err.Error())
	}
	var accepted struct {
		Queued bool `json:"queued"`
	}
	_ = json.Unmarshal(result, &accepted)
	if accepted.Queued {
		return map[string]any{"ok": true, "wakeKind": "queued", "queuedMessageId": input.ClientID}, nil
	}
	return map[string]any{"ok": true, "wakeKind": "started"}, nil
}
