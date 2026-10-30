//go:build windows

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"winone/internal/chat"
	"winone/internal/recycle"
	"winone/internal/settings"
)

// Cindy's local group lane keys, not Chat Server execution grants. Only the
// private Worker supplies these keys after the original group service admits a member.
var oneBotGroupRoute = regexp.MustCompile(`^group:([^\\/:\s]+)(?::plan:([^\\/:\s]+))?$`)

func (b *chatOneBot) groupLane(ctx context.Context, raw json.RawMessage) (any, error) {
	var input struct {
		BotID, RouteKey, Title, WorkingDir, SessionID string
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	parts := oneBotGroupRoute.FindStringSubmatch(input.RouteKey)
	if parts == nil || len(input.RouteKey) > 512 || (parts[2] == "") != (input.WorkingDir == "") || input.SessionID != "" && parts[2] == "" {
		return nil, errors.New("群专线或分工目录无效")
	}
	if input.WorkingDir != "" {
		if !filepath.IsAbs(input.WorkingDir) {
			return nil, errors.New("分工目录必须是绝对路径")
		}
		info, err := os.Stat(input.WorkingDir)
		if err != nil || !info.IsDir() {
			return nil, errors.New("分工目录不存在")
		}
	}
	// Preserve the service's distinction between a missing member and a model
	// that is not available, so one failed member does not fail the whole round.
	profile, err := b.contextFor(ctx, map[string]string{"botId": input.BotID, "role": "group", "routeKey": input.RouteKey})
	if err != nil {
		return map[string]any{"ok": false, "errorCode": "MEMBER_UNAVAILABLE", "message": "伙伴当前不可用"}, nil
	}
	configuration, err := b.control.aiConfiguration()
	if err != nil {
		return nil, err
	}
	profile.DefaultModelChain = b.defaultModelChain(configuration)
	if _, err = resolveOneBotModel(profile, configuration, chat.Conversation{}); err != nil {
		return map[string]any{"ok": false, "errorCode": "NO_MODEL", "message": "伙伴还没有可用模型"}, nil
	}
	row, err := b.openRegisteredRoute(ctx, input.BotID, input.RouteKey, "group", input.Title, input.WorkingDir, input.SessionID)
	if err != nil {
		return nil, err
	}
	// A changed profile is applied at the Pi boundary, never by restarting a
	// running turn. This is the same refresh path used by private conversations.
	if err = b.flushRuntimeRefresh(ctx); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "sessionId": row.ID}, nil
}

// Member removal and group deletion archive their lanes in Cindy's transaction
// first. Mirror that state in Chat and close only those registered group lanes.
func (b *chatOneBot) closeGroupLanes(ctx context.Context, raw json.RawMessage) (any, error) {
	var input struct {
		SessionIDs []string `json:"sessionIds"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	runner, err := b.ensure(ctx)
	if err != nil {
		return nil, err
	}
	for _, id := range input.SessionIDs {
		linked, err := runner.Call(ctx, "sessions.lookup", map[string]string{"sessionId": id})
		if err != nil {
			return nil, err
		}
		var link struct {
			Role       string
			ArchivedAt *int64
		}
		if err = json.Unmarshal(linked, &link); err != nil {
			return nil, err
		}
		if link.Role != "group" || link.ArchivedAt == nil {
			return nil, errors.New("只能关闭已归档的群专线")
		}
	}
	for _, id := range input.SessionIDs {
		if err = b.sessions.engine.CloseSession(ctx, id); err != nil {
			return nil, err
		}
		if _, err = b.sessions.store.UpdateConversation(id, func(c *chat.Conversation) error { c.Archived = true; return nil }); err != nil {
			return nil, err
		}
	}
	if err = b.revokeSessionGrants(input.SessionIDs); err != nil {
		return nil, err
	}
	return map[string]any{"closed": len(input.SessionIDs)}, nil
}

// Project validation follows Cindy's physical-directory check. Managed dialogue
// workspaces cannot be reused as projects through a junction or a lexical alias.
func (b *chatOneBot) groupProjectDirectory(raw json.RawMessage) (any, error) {
	var input struct {
		Directory string `json:"directory"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	fail := func(message string) (any, error) { return map[string]any{"ok": false, "message": message}, nil }
	if !filepath.IsAbs(input.Directory) {
		return fail("项目文件夹必须是绝对路径")
	}
	directory, err := filepath.EvalSymlinks(input.Directory)
	if err != nil {
		return fail("项目文件夹不存在")
	}
	info, err := os.Stat(directory)
	if err != nil || !info.IsDir() {
		return fail("项目文件夹不存在")
	}
	for _, root := range []string{filepath.Join(b.control.directory, "onebot"), filepath.Join(b.control.directory, "chat")} {
		if oneBotInside(root, directory) {
			return fail("托管对话目录不能用作项目文件夹")
		}
	}
	return map[string]any{"ok": true, "dir": filepath.Clean(directory)}, nil
}

func (b *chatOneBot) recycleGroupFolder(ctx context.Context, raw json.RawMessage) (any, error) {
	var input struct {
		Directory string `json:"directory"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	root := filepath.Join(b.control.directory, "onebot", "bot-groups")
	directory := filepath.Clean(input.Directory)
	if !filepath.IsAbs(directory) || !strings.EqualFold(filepath.Dir(directory), root) || filepath.Base(directory) == "." {
		return nil, errors.New("只能回收本机群聊自己的文件夹")
	}
	if _, err := os.Lstat(directory); errors.Is(err, os.ErrNotExist) {
		return map[string]bool{"ok": true}, nil
	} else if err != nil {
		return nil, err
	}
	_, err := recycle.CheckedDirectory(ctx, directory, func() error {
		if !oneBotInside(root, directory) {
			return errors.New("群聊文件夹已移出托管目录，未清理")
		}
		return nil
	})
	return map[string]bool{"ok": err == nil}, err
}

type oneBotGroupAttachment struct {
	Name string `json:"name"`
	Path string `json:"path"`
	URL  string `json:"url"`
}

func (b *chatOneBot) groupFileAction(ctx context.Context, method string, raw json.RawMessage) (any, error) {
	runner, err := b.ensure(ctx)
	if err != nil {
		return nil, err
	}
	value, err := runner.Call(ctx, "groups.files.describe", raw)
	if err != nil {
		return nil, err
	}
	var file struct {
		Path         string
		DownloadOnly bool
	}
	if err = json.Unmarshal(value, &file); err != nil {
		return nil, err
	}
	if file.DownloadOnly {
		return nil, errors.New("此类附件只能另存为，不能直接打开或定位")
	}
	err = openVerifiedChatPath(file.Path, method == "groups.files.reveal")
	return map[string]bool{"success": err == nil}, err
}

func (b *chatOneBot) recycleStagedGroupFolder(ctx context.Context, raw json.RawMessage) (any, error) {
	var input struct{ Directory string }
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	root := filepath.Join(b.control.directory, "onebot", "bot-groups")
	relative, err := filepath.Rel(root, input.Directory)
	if err != nil {
		return nil, err
	}
	parts := strings.Split(relative, string(filepath.Separator))
	if len(parts) != 4 || parts[0] == ".." || parts[1] != "attachments" || parts[2] != "staged" || !regexp.MustCompile("^[0-9a-f-]{36}$").MatchString(parts[3]) || !oneBotInside(root, input.Directory) {
		return nil, errors.New("只能回收本机群聊的暂存附件")
	}
	if _, err = os.Stat(input.Directory); errors.Is(err, os.ErrNotExist) {
		return map[string]bool{"ok": true}, nil
	}
	if err != nil {
		return nil, err
	}
	_, err = recycle.CheckedDirectory(ctx, input.Directory, func() error {
		if !oneBotInside(root, input.Directory) {
			return errors.New("暂存附件目录已变化")
		}
		return nil
	})
	return map[string]bool{"ok": err == nil}, err
}

// The original group service has already checked the chosen files. Import an
// immutable per-input copy into the existing Chat attachment store; never pass
// managed media URLs to Pi as if they were filesystem paths.
func (b *chatOneBot) groupAttachments(sessionID, requestID string, inputs []oneBotGroupAttachment) ([]string, error) {
	if len(inputs) > 16 {
		return nil, errors.New("一次最多发送 16 个附件")
	}
	// A durable input hashes its attachment paths. Persist the mapping before
	// dispatch, so a lost acknowledgement/reopened Host retains those exact
	// paths and bytes even if the picked source file has since been changed.
	b.sessionMu.Lock()
	defer b.sessionMu.Unlock()
	row, err := b.sessions.store.Conversation(sessionID)
	if err != nil {
		return nil, err
	}
	requestHash := sha256.Sum256([]byte(requestID))
	manifest := filepath.Join(row.SessionDirectory, "native-attachments", hex.EncodeToString(requestHash[:])+".json")
	inputBytes, err := json.Marshal(inputs)
	if err != nil {
		return nil, err
	}
	inputHash := sha256.Sum256(inputBytes)
	fingerprint := hex.EncodeToString(inputHash[:])
	var saved struct {
		Fingerprint string   `json:"fingerprint"`
		Paths       []string `json:"paths"`
	}
	if raw, err := os.ReadFile(manifest); err == nil {
		if err = json.Unmarshal(raw, &saved); err != nil {
			return nil, err
		}
		if saved.Fingerprint != fingerprint || len(saved.Paths) != len(inputs) {
			return nil, errors.New("同一群聊输入的附件已变化")
		}
		if err = b.sessions.store.ValidateAttachments(sessionID, saved.Paths); err != nil {
			return nil, err
		}
		return saved.Paths, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	paths := make([]string, 0, len(inputs))
	for _, input := range inputs {
		source := input.Path
		if input.URL != "" {
			match := oneBotManagedMedia.FindStringSubmatch(input.URL)
			if match == nil || source != "" {
				return nil, errors.New("群聊媒体地址无效")
			}
			root := filepath.Join(b.control.directory, "onebot", "media", "blobs")
			source = filepath.Join(root, match[1]+"."+match[2])
			if !oneBotInside(root, source) {
				return nil, errors.New("群聊媒体不属于本机存储")
			}
		}
		if !filepath.IsAbs(source) {
			return nil, errors.New("附件位置必须是绝对路径")
		}
		reader, err := os.Open(source)
		if err != nil {
			return nil, err
		}
		info, err := reader.Stat()
		if err != nil || !info.Mode().IsRegular() {
			reader.Close()
			return nil, errors.New("群聊附件不是普通文件")
		}
		data, err := io.ReadAll(io.LimitReader(reader, chat.MediaImageLimit+1))
		reader.Close()
		if err != nil {
			return nil, err
		}
		name := input.Name
		if name == "" {
			name = filepath.Base(source)
		}
		added, err := b.sessions.store.ImportAttachment(sessionID, name, data)
		if err != nil {
			return nil, fmt.Errorf("无法添加群聊附件：%w", err)
		}
		paths = append(paths, added.Path)
	}
	saved.Fingerprint, saved.Paths = fingerprint, paths
	raw, err := json.Marshal(saved)
	if err != nil {
		return nil, err
	}
	if err = settings.WriteAtomic(manifest, raw); err != nil {
		return nil, err
	}
	return paths, nil
}
