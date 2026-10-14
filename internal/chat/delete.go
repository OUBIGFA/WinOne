package chat

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"winone/internal/settings"
)

// DeleteConversation removes the registered history, never the working folder.
// The caller first closes the runtime and supplies the native recycle operation.
func (s *Store) DeleteConversation(id string, recycleDirectory func(string) error) error {
	return s.change(func(value *Catalog) error {
		index := slices.IndexFunc(value.Conversations, func(c Conversation) bool { return c.ID == id })
		if index < 0 {
			return errors.New("对话不存在")
		}
		c := value.Conversations[index]
		if filepath.Base(c.SessionDirectory) != c.ID || strings.EqualFold(filepath.Clean(c.Directory), filepath.Clean(c.SessionDirectory)) {
			return errors.New("会话保存目录归属无法确认，未删除")
		}
		if info, err := os.Lstat(c.SessionDirectory); err == nil {
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return errors.New("会话保存位置已变成链接，未删除")
			}
			resolved, err := filepath.EvalSymlinks(c.SessionDirectory)
			if err != nil {
				return err
			}
			if !strings.EqualFold(filepath.Clean(resolved), filepath.Clean(c.SessionDirectory)) {
				return errors.New("会话保存位置经过链接，未删除")
			}
			// Include the navigation record in the recycled folder so the original
			// title, model and paths remain recoverable even if the index write fails.
			raw, err := json.MarshalIndent(c, "", "  ")
			if err != nil {
				return err
			}
			if err = settings.WriteAtomic(filepath.Join(c.SessionDirectory, "winone-conversation.json"), raw); err != nil {
				return err
			}
			if recycleDirectory == nil {
				return errors.New("回收站不可用，未删除")
			}
			if err = recycleDirectory(c.SessionDirectory); err != nil {
				return fmt.Errorf("对话未删除：%w", err)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		value.Conversations = slices.Delete(value.Conversations, index, index+1)
		return nil
	})
}
