package configuration

import "errors"

// ModuleLifecycle stops disabled modules before preference application, then
// enables modules only after their new settings are available. Both phases must
// be idempotent and usable for rollback; neither may launch configured programs.
type ModuleLifecycle struct {
	Initial map[string]bool
	Before  func(map[string]bool) error
	After   func(map[string]bool) error
}

func copyEnabled(source map[string]bool) map[string]bool {
	if source == nil {
		return nil
	}
	result := make(map[string]bool, len(source))
	for id, enabled := range source {
		result[id] = enabled
	}
	return result
}

// CommitDocument is the whole-profile boundary; callers must retain its current
// revision and validate any source-file digest before entering this transaction.
func (s *HostStore) CommitDocument(next Document) (Document, error) {
	defer s.notifySettled()
	previous := s.store.Snapshot()
	if err := s.RecoveryError(); err != nil {
		return previous, errors.Join(ErrRecoveryRequired, err)
	}
	if err := s.checkUnavailableUnchanged(previous, next); err != nil {
		return previous, err
	}
	prepared, _, err := s.prepareDocument(next)
	if err != nil {
		return previous, err
	}
	forward := true
	return s.store.Commit(prepared, func(document Document) error {
		if !forward {
			return s.applyDocument(document)
		}
		forward = false
		return s.applyProjection(document, &previous)
	})
}

func (s *HostStore) RecoveryError() error {
	s.recoveryMu.RLock()
	err := s.startupError
	s.recoveryMu.RUnlock()
	if err != nil {
		return err
	}
	if s.store.NeedsRecovery() {
		return ErrRecoveryRequired
	}
	return nil
}

// Recover is an explicit retry that keeps the Host readable when native recovery
// cannot finish during startup. Successful recovery does not advance revisions.
func (s *HostStore) Recover() error {
	defer s.notifySettled()
	if err := s.unavailableRecovery(); err != nil {
		return err
	}
	err := s.store.Reapply(s.applyDocument)
	s.recoveryMu.Lock()
	s.startupError = err
	s.recoveryMu.Unlock()
	return err
}
