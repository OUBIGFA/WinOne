package chat

import (
	"errors"
	"slices"
	"time"
)

// MoveConversation changes navigation and the next tool cwd. Pi history, draft,
// attachment copies and the selected session storage stay at their old paths.
func (s *Store) MoveConversation(id, projectID string) (Conversation, error) {
	var result Conversation
	err := s.change(func(catalog *Catalog) error {
		index := slices.IndexFunc(catalog.Conversations, func(c Conversation) bool { return c.ID == id })
		if index < 0 {
			return errors.New("对话不存在")
		}
		directory := catalog.Conversations[index].Directory
		if projectID != "" {
			project := slices.IndexFunc(catalog.Projects, func(p Project) bool { return p.ID == projectID })
			if project < 0 {
				return errors.New("目标项目不存在")
			}
			var err error
			directory, err = existingDirectory(catalog.Projects[project].Path)
			if err != nil {
				return err
			}
		}
		catalog.Conversations[index].ProjectID = projectID
		catalog.Conversations[index].Directory = directory
		catalog.Conversations[index].UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		result = catalog.Conversations[index]
		return nil
	})
	return result, err
}
