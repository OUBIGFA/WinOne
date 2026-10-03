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
	if err := s.RecoveryError(); err != nil {
		return s.store.Snapshot(), errors.Join(ErrRecoveryRequired, err)
	}
	prepared, _, err := s.prepareDocument(next)
	if err != nil {
		return s.store.Snapshot(), err
	}
	return s.store.Commit(prepared, s.applyDocument)
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
	err := s.store.Reapply(s.applyDocument)
	s.recoveryMu.Lock()
	s.startupError = err
	s.recoveryMu.Unlock()
	return err
}
