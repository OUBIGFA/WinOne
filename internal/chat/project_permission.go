package chat

import (
	"errors"
	"slices"
)

// Project permissions are one preference. Persist every member in the same
// atomic catalog write so old and newly created conversations agree.
func (s *Store) UpdateProjectPermission(id, permission string) (Project, error) {
	var result Project
	if !permissionValid(permission) {
		return result, errors.New("项目权限无效")
	}
	err := s.change(func(value *Catalog) error {
		index := slices.IndexFunc(value.Projects, func(p Project) bool { return p.ID == id })
		if index < 0 {
			return errors.New("项目不存在")
		}
		value.Projects[index].Permission = permission
		result = value.Projects[index]
		for i := range value.Conversations {
			if value.Conversations[i].ProjectID == id {
				value.Conversations[i].Permission = permission
			}
		}
		return nil
	})
	return result, err
}
