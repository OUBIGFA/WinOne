// Package configuration owns committed settings and crash-recoverable application.
package configuration

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"sync"
	"winone/internal/catalog"
	"winone/internal/configjson"
	"winone/internal/settings"
)

var ErrConflict = errors.New("配置已变更，请刷新后重试")
var ErrRecoveryRequired = errors.New("配置尚未恢复，请先完成恢复")

// Document holds device settings, including local paths, and is not an export
// archive. Values use stable namespaces such as host, capture and wintray.
type Document struct {
	SchemaVersion   int                        `json:"schemaVersion"`
	Revision        uint64                     `json:"revision"`
	Values          map[string]json.RawMessage `json:"values"`
	ModuleEnabled   map[string]bool            `json:"moduleEnabled,omitempty"`
	DeferredModules map[string]json.RawMessage `json:"deferredModules,omitempty"`
	// RetainedModules contains local settings kept after uninstall. Unlike
	// DeferredModules imported from a profile, these are never portable data.
	RetainedModules map[string]json.RawMessage `json:"retainedModules,omitempty"`
	BuiltinVersion  int                        `json:"builtinVersion,omitempty"`
}
type diskState struct {
	Committed Document  `json:"committed"`
	Pending   *Document `json:"pending,omitempty"`
}

// Store has one Host writer. Runtime application is serialized separately so
// readers can continue observing the last committed document during a long job.
type Store struct {
	operation sync.Mutex
	mu        sync.Mutex
	path      string
	validate  func(Document) error
	state     diskState
}

// Apply must reconcile the complete target idempotently, including rollback.
// It must await asynchronous worker jobs and must not launch configured programs.
type Apply func(Document) error

func Open(path string, defaults Document, validate func(Document) error) (*Store, error) {
	return openStore(path, defaults, validate, nil)
}

func openStore(path string, defaults Document, validate func(Document) error, upgrade func(Document) (Document, bool, error)) (*Store, error) {
	if validate == nil {
		return nil, errors.New("配置校验器缺失")
	}
	state := diskState{Committed: clone(defaults)}
	var original []byte
	file, err := os.Open(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err == nil {
		state = diskState{}
		data, readErr := io.ReadAll(io.LimitReader(file, (8<<20)+1))
		closeErr := file.Close()
		if err = errors.Join(readErr, closeErr); err != nil {
			return nil, err
		}
		if len(data) > 8<<20 {
			return nil, errors.New("配置文件过大")
		}
		if err = configjson.DecodeObject(data, &state); err != nil {
			return nil, err
		}
		original = data
	}
	backupSuffix := ".before-stroke.bak"
	if state.Committed.BuiltinVersion < 2 {
		backupSuffix = ".before-selection.bak"
	} else if state.Committed.BuiltinVersion < 3 {
		backupSuffix = ".before-quick-assistant.bak"
	}
	changed := false
	if upgrade != nil {
		state.Committed, changed, err = upgrade(state.Committed)
		if err != nil {
			return nil, err
		}
		if state.Pending != nil {
			next, pendingChanged, upgradeErr := upgrade(*state.Pending)
			if upgradeErr != nil {
				return nil, upgradeErr
			}
			state.Pending = &next
			changed = changed || pendingChanged
		}
	}
	store := &Store{path: path, validate: validate, state: state}
	if err = store.check(state.Committed); err != nil {
		return nil, err
	}
	if state.Pending != nil {
		if err = store.check(*state.Pending); err != nil {
			return nil, err
		}
		if state.Committed.Revision == math.MaxUint64 || state.Pending.Revision != state.Committed.Revision+1 {
			return nil, errors.New("配置恢复版本不匹配")
		}
	}
	if changed && original != nil {
		if err = settings.PreserveMigrationBackup(path+backupSuffix, original); err != nil {
			return nil, err
		}
		if err = store.persist(state); err != nil {
			return nil, err
		}
	}
	return store, nil
}

func clone(source Document) Document {
	result := source
	result.ModuleEnabled = copyEnabled(source.ModuleEnabled)
	if source.DeferredModules != nil {
		result.DeferredModules = make(map[string]json.RawMessage, len(source.DeferredModules))
		for id, raw := range source.DeferredModules {
			result.DeferredModules[id] = append(json.RawMessage(nil), raw...)
		}
	}
	if source.RetainedModules != nil {
		result.RetainedModules = make(map[string]json.RawMessage, len(source.RetainedModules))
		for id, raw := range source.RetainedModules {
			result.RetainedModules[id] = append(json.RawMessage(nil), raw...)
		}
	}
	if source.Values != nil {
		result.Values = make(map[string]json.RawMessage, len(source.Values))
		for id, data := range source.Values {
			result.Values[id] = append(json.RawMessage(nil), data...)
		}
	}
	return result
}

func (s *Store) check(value Document) error {
	if value.SchemaVersion != 1 || value.Values == nil || value.BuiltinVersion < 0 || value.BuiltinVersion > 4 {
		return errors.New("配置格式无效")
	}
	for id, data := range value.Values {
		trimmed := bytes.TrimSpace(data)
		if !catalog.ValidID(id) || len(data) > 2<<20 || len(trimmed) == 0 || trimmed[0] != '{' || configjson.ValidateObject(data) != nil {
			return errors.New("配置内容无效")
		}
	}
	for id := range value.ModuleEnabled {
		if !catalog.ValidID(id) {
			return errors.New("模块开关标识无效")
		}
	}
	for id, raw := range value.DeferredModules {
		if !catalog.ValidID(id) || len(raw) > 2<<20 || configjson.ValidateObject(raw) != nil {
			return errors.New("待安装模块配置无效")
		}
		if _, exists := value.Values[id]; exists {
			return errors.New("待安装模块配置与当前模块冲突")
		}
	}
	for id, raw := range value.RetainedModules {
		var retained retainedPackageSettings
		if !catalog.ValidOptionalID(id) || len(raw) > 2<<20 || configjson.DecodeObject(raw, &retained) != nil || retained.SchemaVersion < 1 || configjson.ValidateObject(retained.Settings) != nil {
			return errors.New("保留的模块配置无效")
		}
		if _, exists := value.Values[id]; exists {
			return errors.New("保留的模块配置与当前模块冲突")
		}
	}
	return s.validate(clone(value))
}
func (s *Store) Snapshot() Document {
	s.mu.Lock()
	defer s.mu.Unlock()
	return clone(s.state.Committed)
}
func (s *Store) NeedsRecovery() bool { s.mu.Lock(); defer s.mu.Unlock(); return s.state.Pending != nil }

// persist publishes in-memory state only after its complete disk replacement.
func (s *Store) persist(state diskState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if len(data) > 8<<20 {
		return errors.New("配置文件过大")
	}
	if err = settings.WriteAtomic(s.path, data); err != nil {
		return err
	}
	s.mu.Lock()
	s.state = state
	s.mu.Unlock()
	return nil
}

// Commit journals intent before touching runtime state. Even a failed apply may
// have changed resources, so compensation always reconciles the full old target.
func (s *Store) Commit(next Document, apply Apply) (Document, error) {
	s.operation.Lock()
	defer s.operation.Unlock()
	return s.commit(next, apply)
}

// commit also serves catalog changes that already hold operation, so adapter
// registration and its durable preference transaction cannot interleave.
func (s *Store) commit(next Document, apply Apply) (Document, error) {
	previous := s.Snapshot()
	if s.NeedsRecovery() {
		return previous, ErrRecoveryRequired
	}
	if apply == nil {
		return previous, errors.New("配置应用器缺失")
	}
	if next.Revision != previous.Revision {
		return previous, ErrConflict
	}
	if previous.Revision == math.MaxUint64 {
		return previous, errors.New("配置修订号已用尽")
	}
	next = clone(next)
	next.Revision++
	if err := s.check(next); err != nil {
		return previous, err
	}
	if err := s.persist(diskState{Committed: previous, Pending: &next}); err != nil {
		return previous, err
	}
	applyErr := apply(clone(next))
	if applyErr == nil {
		applyErr = s.persist(diskState{Committed: next})
		if applyErr == nil {
			return clone(next), nil
		}
	}
	if rollbackErr := apply(clone(previous)); rollbackErr != nil {
		return previous, errors.Join(applyErr, ErrRecoveryRequired, rollbackErr)
	}
	if err := s.persist(diskState{Committed: previous}); err != nil {
		return previous, errors.Join(applyErr, ErrRecoveryRequired, err)
	}
	return previous, applyErr
}

// Recover restores the last committed target after a crash or failed rollback.
// Pending desired settings are never promoted just because the Host restarted.
func (s *Store) Recover(apply Apply) error {
	return s.recover(apply, false)
}

// Reapply also reconciles a committed target whose startup failed before a
// transaction was requested; all writes remain blocked by the Host during retry.
func (s *Store) Reapply(apply Apply) error { return s.recover(apply, true) }

func (s *Store) recover(apply Apply, force bool) error {
	s.operation.Lock()
	defer s.operation.Unlock()
	pending := s.NeedsRecovery()
	if !pending && !force {
		return nil
	}
	if apply == nil {
		return errors.New("配置应用器缺失")
	}
	previous := s.Snapshot()
	if err := apply(clone(previous)); err != nil {
		return errors.Join(ErrRecoveryRequired, err)
	}
	if !pending {
		return nil
	}
	if err := s.persist(diskState{Committed: previous}); err != nil {
		return errors.Join(ErrRecoveryRequired, err)
	}
	return nil
}
