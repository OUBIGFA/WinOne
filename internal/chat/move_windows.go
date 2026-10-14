//go:build windows

package chat

import "context"

// Retire an idle runtime before committing the new cwd so a later prompt can
// never keep the previous project's tools, AGENTS.md or loaded resources.
func (s *Service) MoveConversation(ctx context.Context, id, projectID string) (Conversation, error) {
	return s.configureConversation(ctx, id, func() (Conversation, error) { return s.store.MoveConversation(id, projectID) })
}
