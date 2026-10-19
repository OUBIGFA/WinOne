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

// A failed AI request still leaves a readable, persisted title. Regeneration
// uses the saved active branch, and a later manual edit always wins.
func (s *Store) ApplyFallbackTitle(before Conversation, text string, regenerate bool) (Conversation, error) {
	if regenerate {
		messages, err := titleMessages(before)
		if err != nil {
			return before, err
		}
		text = ""
		for _, message := range messages {
			if message.role == "user" && strings.TrimSpace(message.text) != "" {
				text = message.text
				break
			}
		}
		if text == "" && len(messages) > 0 {
			text = messages[0].text
		}
	}
	title := titleSlice(strings.Join(strings.Fields(text), " "), titleMaxChars)
	if title == "" {
		return before, errors.New("会话没有可用于标题的文字")
	}
	return s.UpdateConversation(before.ID, func(c *Conversation) error {
		if c.Title != before.Title || c.TitleManual != before.TitleManual {
			return errors.New("标题已更改，保留当前标题")
		}
		c.Title = title
		return nil
	})
}
