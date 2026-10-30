package chat

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// AdmitTaskConversation publishes a durable coordinator-created Session into
// ordinary Chat without resetting a retried task's model, draft or archive state.
// Only the private Host adapter supplies prepared ownership and identity.
func (s *Store) AdmitTaskConversation(prepared Conversation) (Conversation, error) {
	if !identifier.MatchString(prepared.ID) || !identifier.MatchString(prepared.OneBotParentID) ||
		prepared.OneBotTaskID == "" || len(prepared.OneBotTaskID) > 128 || prepared.OneBotOwnerID == "" || len(prepared.OneBotOwnerID) > 128 {
		return Conversation{}, errors.New("OneBot 任务归属无效")
	}
	var result Conversation
	err := s.change(func(catalog *Catalog) error {
		index := slices.IndexFunc(catalog.Conversations, func(c Conversation) bool { return c.ID == prepared.ID })
		if index >= 0 {
			current := catalog.Conversations[index]
			if current.OneBotTaskID != prepared.OneBotTaskID || current.OneBotOwnerID != prepared.OneBotOwnerID || current.OneBotParentID != prepared.OneBotParentID {
				return errors.New("此 Chat 已属于另一个任务，未覆盖已有内容")
			}
			result = current
			return nil
		}
		directory, err := existingDirectory(prepared.Directory)
		if err != nil {
			return err
		}
		title := strings.TrimSpace(prepared.Title)
		if title == "" {
			title = "Chat"
		}
		created := prepared.CreatedAt
		if created == "" {
			created = time.Now().UTC().Format(time.RFC3339Nano)
		}
		result = Conversation{ID: prepared.ID, Title: title, Directory: directory,
			SessionDirectory: filepath.Join(s.root, "sessions", prepared.ID), Model: prepared.Model,
			Permission: prepared.Permission, ThinkingLevel: prepared.ThinkingLevel, FastMode: prepared.FastMode,
			OneBotTaskID: prepared.OneBotTaskID, OneBotOwnerID: prepared.OneBotOwnerID, OneBotParentID: prepared.OneBotParentID,
			CreatedAt: created, UpdatedAt: created}
		catalog.Conversations = append(catalog.Conversations, result)
		if err = validateCatalog(*catalog); err != nil {
			return err
		}
		return os.MkdirAll(result.SessionDirectory, 0700)
	})
	return result, err
}
