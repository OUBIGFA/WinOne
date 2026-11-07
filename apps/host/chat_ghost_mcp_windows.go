//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"time"
	"winone/internal/chat"
)

type ghostToolScope struct {
	control  *chatControl
	taskID   string
	revision string
	timer    *time.Timer
}

// Page calls borrow the same native session/permission implementation as Pi.
// A scope holds only a normal execution lease, not an approval or a model turn.
func (g *chatGhosts) mcpTask(ctx context.Context, op string, raw json.RawMessage) (any, error) {
	var input struct {
		SessionID, ScopeID, ClientID, Text, AuthoredText string
		ExpectedEpoch                                    *uint64
		Request                                          json.RawMessage
		PresentationSource                               json.RawMessage
		Files                                            []struct{ URL, Path, Name string }
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	if op == "tool-release" {
		g.releaseToolScope(input.ScopeID)
		return true, nil
	}
	control, row, err := g.targetConversation(input.SessionID)
	if err != nil || control.engine == nil {
		return nil, ghostTaskFail("HOST_NOT_READY", "会话不可用")
	}
	if row.Archived {
		return nil, ghostTaskFail("TASK_NOT_FOUND", "任务已归档")
	}
	if op == "tool-scope" {
		if _, err = control.engine.Open(ctx, row.ID, 0); err != nil {
			return nil, err
		}
		state, err := control.engine.Command(ctx, row.ID, "execution.inspect", nil)
		if err != nil {
			control.engine.Release(row.ID, 0)
			return nil, err
		}
		var result map[string]any
		if json.Unmarshal(state, &result) != nil {
			control.engine.Release(row.ID, 0)
			return nil, errors.New("会话权限快照无效")
		}
		id := ghostRequestID()
		scope := &ghostToolScope{control: control, taskID: row.ID, revision: ghostAuthority(row)}
		g.mu.Lock()
		if g.toolScopes == nil {
			g.toolScopes = map[string]*ghostToolScope{}
		}
		g.toolScopes[id] = scope
		// The release timer is published under the same mutex as its scope so
		// an early cancellation cannot race the timer's installation.
		scope.timer = time.AfterFunc(15*time.Minute, func() { g.releaseToolScope(id) })
		g.mu.Unlock()
		result["scopeId"], result["revision"], result["workingDir"] = id, ghostAuthority(row), row.Directory
		return result, nil
	}
	if op == "tool-authorize" {
		g.mu.Lock()
		scope := g.toolScopes[input.ScopeID]
		g.mu.Unlock()
		if scope == nil || scope.control != control || scope.taskID != row.ID || scope.revision != ghostAuthority(row) {
			return nil, ghostTaskFail("STALE_REVISION", "会话权限在操作前发生变化")
		}
		return control.engine.Command(ctx, row.ID, "tool.authorize", map[string]any{"request": input.Request})
	}
	if op != "mcp-message" {
		return nil, errors.New("未知插件页操作")
	}
	if !strings.HasPrefix(input.ClientID, "mcp-app-message:") || input.ExpectedEpoch == nil {
		return nil, ghostTaskFail("PERMISSION_DENIED", "插件页输入缺少本机凭据")
	}
	// Only the original View's already-checked image resource links reach this
	// route. They are immutable copies in this task's normal attachment boundary.
	paths := []string{}
	for _, file := range input.Files {
		uri := file.URL
		if uri == "" {
			uri = file.Path
		}
		if !strings.HasPrefix(uri, "cindy-media://blobs/") {
			return nil, ghostTaskFail("INVALID_ATTACHMENTS", "插件页图片地址无效")
		}
		args, _ := json.Marshal(map[string]string{"url": uri})
		value, err := g.call(ctx, "media-path", args)
		if err != nil {
			return nil, err
		}
		var target struct{ Path string }
		if json.Unmarshal(value, &target) != nil {
			return nil, errors.New("图片位置无效")
		}
		files, err := control.store.AddAttachments(row.ID, []string{filepath.Clean(target.Path)})
		if err != nil {
			return nil, err
		}
		for _, attachment := range files {
			paths = append(paths, attachment.Path)
		}
	}
	if _, err = control.engine.Open(ctx, row.ID, 0); err != nil {
		return nil, err
	}
	defer control.engine.Release(row.ID, 0)
	return control.engine.DispatchPresentedInput(ctx, row.ID, input.ClientID, input.Text, "queue", chat.NativeInputPresentation{Text: &input.AuthoredText, ReviewRequest: json.RawMessage(`{"kind":"delegated-continuation"}`), SourcePlugin: input.PresentationSource, ExpectedMcpAppEpoch: input.ExpectedEpoch}, paths)
}

func (g *chatGhosts) connectionResume(ctx context.Context, params json.RawMessage) (any, error) {
	var input struct {
		RequestID, TargetSessionID, Message, PersistedContent, ClientID string
		ExpectedEpoch                                                   *uint64
	}
	if err := json.Unmarshal(params, &input); err != nil {
		return nil, err
	}
	if input.RequestID == "" || input.ClientID != "bot-authorization-resume:"+input.RequestID || input.ExpectedEpoch == nil {
		return nil, errors.New("账号续接缺少当前本机请求凭据")
	}
	control, row, err := g.targetConversation(input.TargetSessionID)
	if err != nil || control == g.control || control.engine == nil || row.OneBotTaskID != "" || row.Archived {
		return nil, errors.New("账号续接的本机伙伴会话不可用")
	}
	if _, err = control.engine.Open(ctx, row.ID, 0); err != nil {
		return nil, err
	}
	defer control.engine.Release(row.ID, 0)
	_, err = control.engine.DispatchPresentedInput(ctx, row.ID, input.ClientID, input.Message, "queue", chat.NativeInputPresentation{Text: &input.PersistedContent, ReviewRequest: json.RawMessage(`{"kind":"delegated-continuation"}`), ExpectedMcpAppEpoch: input.ExpectedEpoch})
	if err != nil {
		return nil, err
	}
	return map[string]bool{"ok": true}, nil
}

func (g *chatGhosts) mailAttachmentSnapshot(params json.RawMessage) (any, error) {
	var input struct{ SessionID, Path string }
	if err := json.Unmarshal(params, &input); err != nil {
		return nil, err
	}
	source, err := filepath.EvalSymlinks(input.Path)
	if err != nil {
		return nil, err
	}
	root, err := filepath.EvalSymlinks(filepath.Join(g.control.directory, "chat", "ghosts", "mail-staging"))
	if err != nil {
		return nil, err
	}
	relative, err := filepath.Rel(root, source)
	if err != nil || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil, errors.New("邮件附件快照不属于当前插件宿主")
	}
	control, row, err := g.targetConversation(input.SessionID)
	if err != nil || row.Archived {
		return nil, errors.New("附件会话已不可用")
	}
	files, err := control.store.AddAttachments(row.ID, []string{source})
	if err != nil {
		return nil, err
	}
	return map[string]string{"path": files[0].Path}, nil
}

// Handoff uses the same native batch permission decision as a tool call; it
// holds a normal session lease only until that decision settles.
func (g *chatGhosts) authorizeHandoff(ctx context.Context, params json.RawMessage) (any, error) {
	var input struct{ SessionID string }
	if err := json.Unmarshal(params, &input); err != nil {
		return nil, err
	}
	control, row, err := g.targetConversation(input.SessionID)
	if err != nil || control.engine == nil {
		return nil, errors.New("交接会话不可用")
	}
	if _, err = control.engine.Open(ctx, row.ID, 0); err != nil {
		return nil, err
	}
	defer control.engine.Release(row.ID, 0)
	return control.engine.Command(ctx, row.ID, "files.handoff.authorize", params)
}

// Authority changes, not title/history autosaves, invalidate page/file loans.
func ghostAuthority(row chat.Conversation) string {
	data, _ := json.Marshal([]any{row.Directory, row.Permission, row.LegacyPlan, row.Archived, row.McpAppEpoch})
	return string(data)
}

func (g *chatGhosts) releaseToolScope(id string) {
	g.mu.Lock()
	scope := g.toolScopes[id]
	delete(g.toolScopes, id)
	g.mu.Unlock()
	if scope != nil {
		if scope.timer != nil {
			scope.timer.Stop()
		}
		scope.control.engine.Release(scope.taskID, 0)
	}
}

// Input consumption, not just queue acceptance, must still have a live View
// guard in this Main process. A restarted process cannot reconstruct it.
func (g *chatGhosts) appMessageAdmit(w http.ResponseWriter, r *http.Request, row chat.Conversation) bool {
	if r.URL.Path != "/v1/mcp-app-message-admit" {
		return false
	}
	var input struct{ RequestID string }
	if r.Method != "POST" || json.NewDecoder(http.MaxBytesReader(w, r.Body, 65536)).Decode(&input) != nil || !strings.HasPrefix(input.RequestID, "mcp-app-message:") {
		http.Error(w, "Invalid App input", 400)
		return true
	}
	runner, err := g.ensure(r.Context())
	if err == nil {
		_, err = runner.Call(r.Context(), "mcp-app-message-admit", map[string]string{"sessionId": row.ID, "requestId": input.RequestID})
	}
	if err != nil {
		http.Error(w, "App view authorization is no longer live", 403)
		return true
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"accepted":true}`))
	return true
}
