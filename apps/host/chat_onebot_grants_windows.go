//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"time"
	"winone/internal/ai"
	"winone/internal/settings"
)

// Revoke before detaching history. A saved lease must not outlive the Bot even
// when the ordinary archived task retains the original session identifier.
func (b *chatOneBot) revokeSessionGrants(sessionIDs []string) error {
	b.grantMu.Lock()
	defer b.grantMu.Unlock()
	if err := b.loadInputGrantsLocked(); err != nil {
		return err
	}
	next := maps.Clone(b.grants)
	for id, grant := range next {
		if slices.Contains(sessionIDs, grant.SessionID) {
			delete(next, id)
		}
	}
	if len(next) == len(b.grants) {
		return nil
	}
	if err := b.saveInputGrantsLocked(next); err != nil {
		return err
	}
	b.grants = next
	return nil
}

// The input identity is authority, not a permission switch in an editable Bot
// Home. Protect it with the same Windows-user boundary as credentials and write
// it before admitting the input to Pi. No message text or tool arguments live here.
func (b *chatOneBot) loadInputGrantsLocked() error {
	if b.grantsLoaded {
		return nil
	}
	if b.control.directory == "" {
		return errors.New("OneBot 输入回执缺少数据目录")
	}
	file := filepath.Join(b.control.directory, "onebot", "input-grants.bin")
	info, err := os.Lstat(file)
	if errors.Is(err, os.ErrNotExist) {
		b.grantsLoaded = true
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > 2<<20 {
		return errors.New("OneBot 输入回执文件无效")
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	plain, err := ai.UnprotectLocalData(raw)
	if err != nil {
		return fmt.Errorf("OneBot 输入回执无法解密: %w", err)
	}
	defer clear(plain)
	var saved struct {
		Version int                         `json:"version"`
		Grants  map[string]oneBotInputGrant `json:"grants"`
	}
	if err := json.Unmarshal(plain, &saved); err != nil {
		return err
	}
	if saved.Version != 1 || len(saved.Grants) > 2048 {
		return errors.New("OneBot 输入回执版本或数量无效")
	}
	next := map[string]oneBotInputGrant{}
	for id, grant := range saved.Grants {
		if id == "" || len(id) > 128 || grant.SessionID == "" || len(grant.Digest) != 64 {
			return errors.New("OneBot 输入回执内容无效")
		}
		if time.Now().Before(grant.Expires) {
			next[id] = grant
		}
	}
	b.grants = next
	b.grantsLoaded = true
	return nil
}

func (b *chatOneBot) saveInputGrantsLocked(grants map[string]oneBotInputGrant) error {
	raw, err := json.Marshal(struct {
		Version int                         `json:"version"`
		Grants  map[string]oneBotInputGrant `json:"grants"`
	}{1, grants})
	if err != nil {
		return err
	}
	defer clear(raw)
	sealed, err := ai.ProtectLocalData(raw)
	if err != nil {
		return err
	}
	return settings.WriteAtomic(filepath.Join(b.control.directory, "onebot", "input-grants.bin"), sealed)
}
