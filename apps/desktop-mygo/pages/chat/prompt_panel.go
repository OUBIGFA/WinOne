package chat

import (
	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
)

// PromptPanel answers a task's waiting permission request or question from another
// page, as a group chat does for a teammate that waits. It follows the task with a
// full conversation model, so the card and its receipts are the task's own.
type PromptPanel struct{ chat *Chat }

// NewPromptPanel opens a task's prompts without showing the conversation.
func NewPromptPanel(env *core.Env, sessionID string, bot BotBinding) *PromptPanel {
	return &PromptPanel{chat: NewOneBotConversation(env, sessionID, bot)}
}

// View draws the prompt that waits, if any, and reports whether one did.
func (p *PromptPanel) View(c *ui.Context) bool {
	chat := p.chat
	chat.ui.services = c.Services()
	chat.takeIntents() // The embedded prompt has no full Chat.View to open its session.
	if len(chat.view.Approvals) == 0 {
		return false
	}
	if chat.view.Approvals[0].Kind == "question" {
		chat.questionCard(c, chat.view.Approvals[0])
	} else {
		chat.approvalCard(c, chat.view.Approvals[0])
	}
	return true
}

// Pending is this controller's actual waiting request, not a guessed group
// execution status. It lets a group label only the speaker who needs input.
func (p *PromptPanel) Pending() bool { return len(p.chat.view.Approvals) > 0 }

// Close releases the task.
func (p *PromptPanel) Close() { p.chat.Close() }
