package chat

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
)

// DetachHistory moves navigation ownership to the ordinary Chat archive without
// rewriting Pi files or drafts. The destination commits first: a failed source
// save can be retried, while a failed destination never loses the source entry.
// The Host serializes this one-way transfer with OneBot session creation.
func (s *Store) DetachHistory(destination *Store, ids []string) error {
	if destination == nil || destination == s {
		return errors.New("任务历史的目标索引无效")
	}
	return s.change(func(source *Catalog) error {
		if err := destination.change(func(target *Catalog) error {
			for _, id := range ids {
				from := slices.IndexFunc(source.Conversations, func(c Conversation) bool { return c.ID == id })
				to := slices.IndexFunc(target.Conversations, func(c Conversation) bool { return c.ID == id })
				if from < 0 {
					if to < 0 {
						return errors.New("待保留的任务历史不存在")
					}
					continue // Both indexes already committed on an earlier attempt.
				}
				c := source.Conversations[from]
				if to >= 0 {
					if !strings.EqualFold(filepath.Clean(target.Conversations[to].SessionDirectory), filepath.Clean(c.SessionDirectory)) {
						return errors.New("任务历史标识与已有对话冲突")
					}
					continue // Destination committed before a failed source write.
				}
				c.ProjectID, c.Archived, c.Pinned = "", true, false
				// The deleted Home is no longer a working folder. Keep archive
				// viewing independent of it and remove the former Bot's grants.
				c.Directory, c.Permission = filepath.Dir(c.SessionDirectory), "ask"
				target.Conversations = append(target.Conversations, c)
			}
			return nil
		}); err != nil {
			return err
		}
		source.Conversations = slices.DeleteFunc(source.Conversations, func(c Conversation) bool { return slices.Contains(ids, c.ID) })
		return nil
	})
}
