//go:build windows

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"winone/internal/chat"
)

// This grant is created by the private IM adapter, never by the model or a
// renderer. A channel tool supplies its immutable input receipt to look it up.
type oneBotInputGrant struct {
	SessionID    string
	Channel      string
	Group        bool
	ConnectionID string
	Owner        bool
	ReplyTaskID  string
	Digest       string
	// Principal is who the turn speaks for (Cindy teammateTurnPrincipal):
	// "entrusted" for an owner-created routine, "autonomous" for a Host
	// wake-up such as the follow-up. Empty derives it from Owner.
	Principal string
	Expires   time.Time
}

// principal: the owner typed this turn or was verified in an IM lane; Host
// wake-ups, teammate messages and other speakers act on the teammate's own.
func (g oneBotInputGrant) principal() string {
	if g.Principal != "" {
		return g.Principal
	}
	if g.Owner {
		return "owner"
	}
	return "autonomous"
}

func (b *chatOneBot) grantInput(id string, grant oneBotInputGrant) error {
	if id == "" || len(id) > 128 {
		return errors.New("OneBot 输入回执无效")
	}
	b.grantMu.Lock()
	defer b.grantMu.Unlock()
	if err := b.loadInputGrantsLocked(); err != nil {
		return err
	}
	now := time.Now()
	next := maps.Clone(b.grants)
	for key, value := range next {
		if now.After(value.Expires) {
			delete(next, key)
		}
	}
	if previous, ok := next[id]; ok {
		if previous.SessionID != grant.SessionID || previous.Owner != grant.Owner || previous.ReplyTaskID != grant.ReplyTaskID || previous.Digest != grant.Digest || previous.ConnectionID != grant.ConnectionID || previous.Channel != grant.Channel || previous.Group != grant.Group || previous.Principal != grant.Principal {
			return errors.New("OneBot 输入回执的归属或内容已变化")
		}
		// Delivery retries do not extend authority or rewrite a valid receipt.
		return nil
	}
	if len(next) >= 2048 {
		return errors.New("OneBot 输入租约已满，请等待当前任务结束")
	}
	grant.Expires = now.Add(24 * time.Hour)
	next[id] = grant
	if err := b.saveInputGrantsLocked(next); err != nil {
		return err
	}
	b.grants = next
	return nil
}
func (b *chatOneBot) inputGrant(sessionID, requestID string) (oneBotInputGrant, error) {
	if requestID == "" {
		return oneBotInputGrant{}, errors.New("OneBot 输入回执缺失")
	}
	b.grantMu.Lock()
	defer b.grantMu.Unlock()
	if err := b.loadInputGrantsLocked(); err != nil {
		return oneBotInputGrant{}, err
	}
	value, ok := b.grants[requestID]
	if !ok || value.SessionID != sessionID || time.Now().After(value.Expires) {
		return value, errors.New("OneBot 当前输入没有有效的渠道租约")
	}
	return value, nil
}
func oneBotInside(root, target string) bool {
	if !filepath.IsAbs(root) || !filepath.IsAbs(target) {
		return false
	}
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false
	}
	target, err = filepath.EvalSymlinks(target)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(root, target)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func (b *chatOneBot) serveIM(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST required", 405)
		return
	}
	route := strings.TrimPrefix(r.URL.Path, "/v1/")
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	fail := func(err error) { http.Error(w, err.Error(), 400) }
	if route == "im/protected" {
		b.serveProtectedIM(w, r, raw)
		return
	}
	if route == "tasks" {
		b.serveTask(w, r, raw)
		return
	}
	if strings.HasPrefix(route, "onebot/") {
		b.serveOneBotBridge(w, r, strings.TrimPrefix(route, "onebot/"), raw)
		return
	}
	if route == "lifecycle" {
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		var input struct {
			oneBotDeletionContext
			Action    string `json:"action"`
			SessionID string `json:"sessionId"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			fail(err)
			return
		}
		if input.Action != "close" && input.Action != "restart" && input.Action != "delete-sessions" && input.Action != "recycle-home" {
			fail(errors.New("不支持的 OneBot 会话控制"))
			return
		}
		if b.sessions == nil {
			fail(errors.New("OneBot 会话尚未就绪"))
			return
		}
		if input.Action == "delete-sessions" || input.Action == "recycle-home" {
			var cleanupErr error
			if input.Action == "recycle-home" {
				cleanupErr = b.recycleBotHome(ctx, input.BotID)
			} else {
				cleanupErr = b.deleteBotSessions(ctx, input.oneBotDeletionContext)
			}
			if cleanupErr != nil {
				fail(cleanupErr)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]bool{"ok": true})
			return
		}
		if _, err := b.sessions.store.Conversation(input.SessionID); err != nil {
			fail(err)
			return
		}
		if input.Action == "restart" {
			b.refreshMu.Lock()
			_, prepared := b.restartContexts[input.SessionID]
			b.refreshMu.Unlock()
			if !prepared {
				fail(errors.New("OneBot 重启缺少已核对的档案"))
				return
			}
		}
		if err := b.sessions.engine.CloseSession(ctx, input.SessionID); err != nil {
			fail(err)
			return
		}
		if input.Action == "restart" {
			if _, err := b.sessions.engine.Open(ctx, input.SessionID, 0); err != nil {
				fail(err)
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
		return
	}
	if route == "im/route" || route == "im/resolve" {
		var input struct {
			BotID        string   `json:"botId"`
			Channel      string   `json:"channel"`
			Group        bool     `json:"group"`
			ConnectionID string   `json:"connectionId"`
			RequestID    string   `json:"requestId"`
			RouteKey     string   `json:"routeKey"`
			Title        string   `json:"title"`
			Prompt       string   `json:"prompt"`
			Owner        bool     `json:"ownerAuthorized"`
			Protected    bool     `json:"protectedContent"`
			Attachments  []string `json:"attachments"`
		}
		if err = json.Unmarshal(raw, &input); err != nil {
			fail(err)
			return
		}
		if input.Protected {
			fail(errors.New("受保护消息不能写入持久会话"))
			return
		}
		if route != "im/resolve" && (len(input.RequestID) < 16 || len(input.RequestID) > 128) || len(input.ConnectionID) != 12 || len(input.RouteKey) > 512 || len(input.Title) > 200 || len(input.Prompt) > 100000 {
			fail(errors.New("OneBot IM 请求无效"))
			return
		}
		role := "channel"
		if input.RouteKey == "canonical" {
			if !input.Owner {
				fail(errors.New("非主人不能使用 OneBot 主会话"))
				return
			}
			role = "canonical"
		}
		if role == "channel" && !strings.HasPrefix(input.RouteKey, "im:") {
			fail(errors.New("OneBot 渠道专线无效"))
			return
		}
		conversation, err := b.openRoute(r.Context(), input.BotID, input.RouteKey, role, input.Title)
		if err != nil {
			fail(err)
			return
		}
		if route == "im/resolve" {
			json.NewEncoder(w).Encode(map[string]any{"sessionId": conversation.ID})
			return
		}
		digest := sha256.Sum256([]byte(input.Prompt))
		grant := oneBotInputGrant{SessionID: conversation.ID, Channel: input.Channel, Group: input.Group, ConnectionID: input.ConnectionID, Owner: input.Owner, Digest: hex.EncodeToString(digest[:])}
		if err = b.grantInput(input.RequestID, grant); err != nil {
			fail(err)
			return
		}
		runner, stampErr := b.ensure(r.Context())
		if stampErr == nil {
			_, stampErr = runner.Call(r.Context(), "imSource.stamp", map[string]any{"sessionId": conversation.ID, "inputId": input.RequestID, "channel": input.Channel, "owner": input.Owner, "group": input.Group, "text": input.Prompt})
		}
		if stampErr != nil {
			fail(stampErr)
			return
		}
		// Transport-signed staging paths only. The model cannot nominate arbitrary
		// local files for upload by forging an IM attachment record.
		mediaRoot := filepath.Join(b.control.directory, "onebot", "im-media")
		for _, file := range input.Attachments {
			if !oneBotInside(mediaRoot, file) && !(input.Channel == "wechat" && (oneBotInside(filepath.Join(b.control.directory, "onebot", "im-attachments", "wechat"), file) || oneBotInside(filepath.Join(b.control.directory, "onebot", "media", "blobs"), file))) {
				fail(errors.New("IM 附件不属于入站暂存目录"))
				return
			}
		}
		attachments := []string{}
		// Text-only IM messages are valid. Store.AddAttachments deliberately
		// rejects an empty picker selection, so do not call it without files.
		if len(input.Attachments) > 0 {
			added, addErr := b.sessions.store.AddAttachments(conversation.ID, input.Attachments)
			if addErr != nil {
				fail(addErr)
				return
			}
			for _, file := range added {
				attachments = append(attachments, file.Path)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"sessionId": conversation.ID, "attachments": attachments, "allowedFileRoots": []string{conversation.Directory}})
		return
	}
	if route == "im/interaction" {
		var input struct {
			SessionID string          `json:"sessionId"`
			RequestID string          `json:"requestId"`
			ID        string          `json:"id"`
			Kind      string          `json:"kind"`
			Allow     bool            `json:"allow"`
			Answers   json.RawMessage `json:"answers"`
			Cancel    bool            `json:"cancel"`
		}
		if err = json.Unmarshal(raw, &input); err != nil {
			fail(err)
			return
		}
		grant, err := b.inputGrant(input.SessionID, input.RequestID)
		if err != nil {
			fail(err)
			return
		}
		method := "permission"
		var params any = map[string]any{"id": input.ID, "requestId": input.RequestID, "allow": input.Allow, "scope": "once"}
		if input.Kind == "question" {
			method = "question"
			params = map[string]any{"id": input.ID, "requestId": input.RequestID, "answers": input.Answers, "cancel": input.Cancel}
		} else if !grant.Owner {
			fail(errors.New("只有主人可以批准操作"))
			return
		}
		var value json.RawMessage
		if valueRunner, exists := b.protectedTurns.Load(input.SessionID + ":" + input.RequestID); exists {
			value, err = valueRunner.(*chat.Runner).Call(r.Context(), method, params)
		} else {
			value, err = b.sessions.engine.Command(r.Context(), input.SessionID, method, params)
		}
		if err != nil {
			fail(err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(value)
		return
	}
	if route != "prepare" && route != "execute" && route != "wait" && route != "stop" && route != "command" && route != "notify" {
		http.NotFound(w, r)
		return
	}
	var input struct {
		ID       string `json:"id"`
		RunID    string `json:"runId"`
		Prompt   string `json:"prompt"`
		Schedule struct {
			Target string `json:"targetSessionId"`
		} `json:"schedule"`
		Run struct {
			ID string `json:"id"`
		} `json:"run"`
	}
	if err = json.Unmarshal(raw, &input); err != nil {
		fail(err)
		return
	}
	id := input.ID
	if route == "prepare" || route == "command" || route == "notify" {
		id = input.Schedule.Target
	}
	if route == "notify" {
		input.RunID = input.Run.ID
	}
	if _, err = b.sessions.store.Conversation(id); err != nil {
		fail(err)
		return
	}
	{
		grant, err := b.inputGrant(id, input.RunID)
		if err != nil {
			fail(err)
			return
		}
		if (route == "command" || route == "notify") && grant.Channel != "routine" {
			fail(errors.New("此操作需要已受理的例行任务"))
			return
		}
		if route == "execute" {
			digest := sha256.Sum256([]byte(input.Prompt))
			if grant.Digest != hex.EncodeToString(digest[:]) {
				fail(errors.New("IM 内容与已受理回执不一致"))
				return
			}
		}
	}
	r.Body = io.NopCloser(bytes.NewReader(raw))
	b.imDriver.serve(w, r)
}

// A persisted record wakes the IM host after restart. Merely having OneBot
// profile files never starts channel polling or any additional model process.
// startSaved starts the Worker after launch when saved IM connections or an
// active teammate's routines need it; routines also run teammate follow-up.
func (b *chatOneBot) startSaved() {
	routines, err := b.savedRoutinesActive()
	if err != nil {
		_ = recordHostEvent(b.control.directory, "onebot:startup", fmt.Errorf("routines: %w", err))
	}
	if routines || b.savedConnections() {
		ctx, cancel := context.WithTimeout(b.control.ctx, 30*time.Second)
		defer cancel()
		if _, err = b.ensure(ctx); err != nil {
			_ = recordHostEvent(b.control.directory, "onebot:startup", fmt.Errorf("worker: %w", err))
		}
	}
}

func (b *chatOneBot) savedConnections() bool {
	file := filepath.Join(b.control.directory, "onebot", "bot-im-connections.json")
	info, err := os.Stat(file)
	if errors.Is(err, os.ErrNotExist) {
		return false
	}
	if err != nil || info.Size() > 256*1024 {
		if err == nil {
			err = errors.New("OneBot IM 连接记录过大")
		}
		_ = recordHostEvent(b.control.directory, "onebot:startup", err)
		return false
	}
	raw, err := os.ReadFile(file)
	var value struct {
		Connections map[string]json.RawMessage `json:"connections"`
	}
	if err == nil {
		err = json.Unmarshal(raw, &value)
	}
	if err != nil {
		_ = recordHostEvent(b.control.directory, "onebot:startup", err)
		return false
	}
	return len(value.Connections) > 0
}
