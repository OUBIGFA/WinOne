//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"winone/internal/chat"
	"winone/internal/recycle"
)

// Replay only intents committed by the profile-deletion transaction. The
// worker also rejects cleanup while a profile still owns the same identifier.
func (b *chatOneBot) recoverDeletedBots(ctx context.Context, runner *chat.Runner) error {
	b.sessionMu.Lock()
	defer b.sessionMu.Unlock()
	raw, err := runner.Call(ctx, "lifecycle.pendingCleanup", map[string]any{})
	if err != nil {
		return err
	}
	var pending []struct {
		BotID string `json:"botId"`
	}
	if err := json.Unmarshal(raw, &pending); err != nil {
		return err
	}
	var failures []error
	for _, item := range pending {
		b.refreshMu.Lock()
		b.deletionContext = &oneBotDeletionContext{BotID: item.BotID}
		b.refreshMu.Unlock()
		raw, err := runner.Call(ctx, "lifecycle.retryCleanup", map[string]string{"botId": item.BotID})
		b.refreshMu.Lock()
		b.deletionContext = nil
		b.refreshMu.Unlock()
		if err != nil {
			failures = append(failures, err)
			continue
		}
		var result struct {
			Warnings []string `json:"warnings"`
		}
		if err := json.Unmarshal(raw, &result); err != nil {
			failures = append(failures, err)
		} else if len(result.Warnings) > 0 {
			failures = append(failures, fmt.Errorf("OneBot %s 清理待重试：%v", item.BotID, result.Warnings))
		}
	}
	return errors.Join(failures...)
}

func (b *chatOneBot) recycleBotHome(ctx context.Context, botID string) error {
	b.refreshMu.Lock()
	prepared := b.deletionContext
	b.refreshMu.Unlock()
	if prepared == nil || prepared.BotID != botID || botID == "." || botID == ".." || filepath.Base(botID) != botID {
		return errors.New("OneBot Home 清理缺少已核对的档案")
	}
	root := filepath.Join(b.control.directory, "onebot", "bots")
	directory := filepath.Join(root, botID)
	if _, err := os.Lstat(directory); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	_, err := recycle.CheckedDirectory(ctx, directory, func() error {
		if !oneBotInside(root, directory) {
			return errors.New("OneBot Home 已移出登记目录，未清理")
		}
		return nil
	})
	return err
}

// Host admission captures the exact Bot-owned sessions before the worker locks
// its profile. A private callback cannot substitute another Bot or history mode.
type oneBotDeletionContext struct {
	BotID           string   `json:"botId"`
	SessionIDs      []string `json:"sessionIds"`
	KeepTaskHistory bool     `json:"keepTaskHistory"`
}

func (b *chatOneBot) deleteBotSessions(ctx context.Context, input oneBotDeletionContext) error {
	b.refreshMu.Lock()
	prepared := b.deletionContext
	b.refreshMu.Unlock()
	if prepared == nil || prepared.BotID != input.BotID || prepared.KeepTaskHistory != input.KeepTaskHistory || len(prepared.SessionIDs) != len(input.SessionIDs) {
		return errors.New("OneBot 删除缺少已核对的会话清单")
	}
	ids := slices.Clone(input.SessionIDs)
	slices.Sort(ids)
	if len(slices.Compact(ids)) != len(ids) {
		return errors.New("OneBot 删除会话清单重复")
	}
	for _, id := range input.SessionIDs {
		if !slices.Contains(prepared.SessionIDs, id) {
			return errors.New("会话不属于正在删除的 OneBot")
		}
		// Close is idempotent and must finish before history leaves its owner.
		if err := b.sessions.engine.CloseSession(ctx, id); err != nil {
			return err
		}
	}
	if err := b.revokeSessionGrants(input.SessionIDs); err != nil {
		return err
	}
	if input.KeepTaskHistory {
		return b.sessions.store.DetachHistory(b.control.store, input.SessionIDs)
	}
	for _, id := range input.SessionIDs {
		// A previous attempt may have recycled an earlier session before a
		// later one failed. The profile's captured ownership list still applies.
		if _, err := b.sessions.store.Conversation(id); err != nil {
			continue
		}
		if err := b.sessions.store.DeleteConversation(id, func(directory string) error {
			_, err := recycle.Directory(ctx, directory)
			return err
		}); err != nil {
			return err
		}
	}
	return nil
}
