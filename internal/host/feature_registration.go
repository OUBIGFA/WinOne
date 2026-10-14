package host

import (
	"errors"
	"slices"
)

// FeatureRegistration owns one binding, not all future uses of its module ID.
// All fields, including removal state, are protected by the service mutex.
type FeatureRegistration struct {
	service *Service
	id      string
	removed bool
}

// Remove only unpublishes a stopped optional feature. It does not wait for or
// terminate handlers: the lifecycle gate must drain and retire captured calls.
// Replaying a successful removal cannot affect a later installation of the ID.
func (r *FeatureRegistration) Remove() error {
	if r == nil || r.service == nil {
		return errors.New("feature registration is missing")
	}
	s := r.service
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.removed {
		return nil
	}
	binding, exists := s.modules[r.id]
	if !exists || binding.registration != r {
		return errors.New("feature binding changed")
	}
	for index, manifest := range s.features {
		if manifest.ID != r.id {
			continue
		}
		if manifest.Distribution != "optional" {
			return errors.New("bundled features cannot be removed")
		}
		s.features = slices.Delete(s.features, index, index+1)
		delete(s.modules, r.id)
		r.removed = true
		return nil
	}
	return errors.New("feature catalog binding is missing")
}
