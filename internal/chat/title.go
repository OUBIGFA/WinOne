package chat

import (
	"errors"
	"strings"
)

// 显式 AI 重命名只比较标题状态；消息落库等无关更新不能使慢请求失效。
// 用户后续手动修改标题或另一笔重命名仍优先于当前请求。
func (s *Store) ApplyRequestedTitle(before Conversation, title string) (Conversation, error) {
	return s.UpdateConversation(before.ID, func(c *Conversation) error {
		if c.Title != before.Title || c.TitleManual != before.TitleManual {
			return errors.New("标题已更改，请重新生成标题")
		}
		c.Title = title
		c.TitleManual = true
		return nil
	})
}

// A manual rename wins even when an earlier model request returns later.
func (s *Store) RenameConversation(id, title string) (Conversation, error) {
	return s.UpdateConversation(id, func(c *Conversation) error { c.Title = strings.TrimSpace(title); c.TitleManual = true; return nil })
}
func (s *Store) ApplyGeneratedTitle(id, title string) (Conversation, error) {
	return s.UpdateConversation(id, func(c *Conversation) error {
		if c.TitleManual || c.Title != "Chat" {
			return nil
		}
		c.Title = title
		return nil
	})
}
