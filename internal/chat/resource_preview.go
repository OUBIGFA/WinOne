package chat

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// RememberResources accepts only the trusted resource-loader response. The
// renderer cannot register paths. Grants are exact, read-only and per session;
// replacing the resource list also revokes resources that are no longer loaded.
func (s *Store) RememberResources(id string, raw json.RawMessage) error {
	c, err := s.Conversation(id)
	if err != nil {
		return err
	}
	var resources struct {
		Agents []struct{ Path string } `json:"agents"`
		Skills []struct{ Path string } `json:"skills"`
	}
	if err = json.Unmarshal(raw, &resources); err != nil {
		return err
	}
	if len(resources.Agents)+len(resources.Skills) > 10000 {
		return errors.New("已加载资源过多")
	}
	grants := map[string]string{}
	for _, resource := range append(resources.Agents, resources.Skills...) {
		path := resource.Path
		if path == "" {
			continue
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(c.Directory, path)
		}
		resolved, err := resolvePreviewPath(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return err
		}
		info, err := os.Stat(resolved)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			continue
		}
		grants[strings.ToLower(filepath.Clean(path))] = resolved
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.resources == nil {
		s.resources = map[string]map[string]string{}
	}
	s.resources[id] = grants
	return nil
}

func (s *Store) resourcePreviewAllowed(id, path, target string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	recorded := s.resources[id][strings.ToLower(filepath.Clean(path))]
	return recorded != "" && strings.EqualFold(recorded, target)
}

func (s *Store) reviewPreviewAllowed(id, path, target string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	recorded := s.reviewFiles[id][strings.ToLower(filepath.Clean(path))]
	return recorded != "" && strings.EqualFold(recorded, target)
}
