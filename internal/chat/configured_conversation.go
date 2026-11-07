package chat

import (
	"crypto/rand"
	"errors"
	"strings"

	"winone/internal/ai"
)

// NewConfiguredConversation allocates a normal Chat conversation while applying
// the route captured by a Host adapter in the same atomic catalog write. The
// helper stays beside Store so adapters can reuse newConversation's project,
// workspace and session-directory rules without widening Store's public API.
//
// Host-only legacy `plan` is normalized atomically to Ask plus its read-only
// marker. It must never exist briefly as an unrestricted Ask conversation.
func (s *Store) NewConfiguredConversation(projectID, directory, title string, model ai.Selection, permission, thinkingLevel string, fastMode bool, saveDirectory ...string) (Conversation, error) {
	if strings.TrimSpace(permission) == "" {
		permission = "ask"
	}
	legacyPlan := permission == "plan"
	if legacyPlan {
		permission = "ask"
	}
	if !permissionValid(permission) {
		return Conversation{}, errors.New("Chat 权限模式无效")
	}
	return s.newConversation(rand.Text(), projectID, directory, title, model, func(value *Conversation) {
		value.Permission = permission
		value.LegacyPlan = legacyPlan
		value.ThinkingLevel = thinkingLevel
		value.FastMode = fastMode
	}, saveDirectory...)
}
