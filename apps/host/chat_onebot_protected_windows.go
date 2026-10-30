//go:build windows

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"time"

	"winone/internal/chat"
)

// A protected Telegram turn uses the same Pi tools/model/permission policy in
// an in-memory SessionManager. The channel route may exist, but no source text,
// response or attachment is added to its durable transcript or learning index.
func (b *chatOneBot) serveProtectedIM(w http.ResponseWriter, r *http.Request, raw []byte) {
	var input struct {
		Connection struct{ ID, BotID, Channel string }
		Message    struct {
			Text             string
			ProtectedContent bool
			Attachments      []struct{ AbsPath, OriginalName, MimeType string }
		}
		RequestID, RouteKey, Title string
		OwnerAuthorized, Group     bool
	}
	fail := func(err error) { http.Error(w, err.Error(), 400) }
	if err := json.Unmarshal(raw, &input); err != nil {
		fail(err)
		return
	}
	if !input.Message.ProtectedContent || input.Connection.Channel != "telegram" || len(input.Connection.ID) != 12 || len(input.RequestID) > 128 || len(input.RequestID) < 16 || len(input.Message.Text) > 100000 {
		fail(errors.New("受保护消息请求无效"))
		return
	}
	role := "channel"
	if input.RouteKey == "canonical" {
		if !input.OwnerAuthorized {
			fail(errors.New("只有主人可以使用主对话"))
			return
		}
		role = "canonical"
	}
	row, err := b.openRoute(r.Context(), input.Connection.BotID, input.RouteKey, role, input.Title)
	if err != nil {
		fail(err)
		return
	}
	digest := sha256.Sum256([]byte(input.Message.Text))
	if err = b.grantInput(input.RequestID, oneBotInputGrant{SessionID: row.ID, Channel: "telegram", ConnectionID: input.Connection.ID, Owner: input.OwnerAuthorized, Group: input.Group, Digest: hex.EncodeToString(digest[:])}); err != nil {
		fail(err)
		return
	}
	files := []map[string]string{}
	for _, file := range input.Message.Attachments {
		if !oneBotInside(filepath.Join(b.control.directory, "onebot", "im-media"), file.AbsPath) {
			fail(errors.New("受保护附件路径无效"))
			return
		}
		files = append(files, map[string]string{"path": file.AbsPath, "name": file.OriginalName, "mimeType": file.MimeType})
	}
	config, release, err := b.configureSession(row)
	if err != nil {
		fail(err)
		return
	}
	defer release()
	config["cwd"], config["sessionDirectory"], config["sessionFile"], config["permission"] = row.Directory, row.SessionDirectory, row.SessionFile, row.Permission
	notifications := make(chan json.RawMessage, 32)
	base := filepath.Dir(b.control.desktop)
	runner, err := chat.StartRunner(r.Context(), filepath.Join(base, "shared", "ai", "runtime", "bun.exe"), filepath.Join(base, "shared", "chat", "worker.js"), row.Directory, func(event json.RawMessage) {
		select {
		case notifications <- event:
		case <-r.Context().Done():
		}
	})
	if err != nil {
		fail(err)
		return
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = runner.Close(closeCtx)
	}()
	key := row.ID + ":" + input.RequestID
	b.protectedTurns.Store(key, runner)
	defer b.protectedTurns.Delete(key)
	type outcome struct {
		value json.RawMessage
		err   error
	}
	done := make(chan outcome, 1)
	go func() {
		value, err := runner.Call(r.Context(), "protected.run", map[string]any{"config": config, "inputId": input.RequestID, "text": input.Message.Text, "files": files})
		done <- outcome{value, err}
	}()
	w.Header().Set("Content-Type", "application/x-ndjson")
	encoder := json.NewEncoder(w)
	send := func(value any) bool {
		if encoder.Encode(value) != nil {
			return false
		}
		if flush, ok := w.(http.Flusher); ok {
			flush.Flush()
		}
		return true
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case value := <-done:
			if value.err != nil {
				send(map[string]string{"error": "受保护消息处理未完成"})
			} else {
				send(map[string]any{"result": value.value})
			}
			return
		case event := <-notifications:
			var message struct {
				Type    string
				Request map[string]any
			}
			if json.Unmarshal(event, &message) == nil && message.Type == "approval" {
				message.Request["requestId"] = input.RequestID
				message.Request["sessionId"] = row.ID
				if !send(map[string]any{"approval": message.Request, "sessionId": row.ID}) {
					return
				}
			}
		}
	}
}
