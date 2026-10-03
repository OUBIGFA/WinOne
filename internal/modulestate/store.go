// Package modulestate persists module intent separately from volatile process state.
package modulestate

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"winone/internal/catalog"
	"winone/internal/settings"
)

type Record struct {
	Enabled       bool   `json:"enabled"`
	PendingStop   bool   `json:"pendingStop"`
	PackageSHA256 string `json:"packageSha256,omitempty"`
}
type Snapshot struct {
	SchemaVersion int               `json:"schemaVersion"`
	Revision      uint64            `json:"revision"`
	Modules       map[string]Record `json:"modules"`
}
type Store struct {
	mu    sync.Mutex
	path  string
	value Snapshot
}

func Open(path string) (*Store, error) {
	defaults := Snapshot{SchemaVersion: 1, Modules: map[string]Record{}}
	for _, m := range catalog.Builtins() {
		defaults.Modules[m.ID] = Record{Enabled: true}
	}
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err == nil {
		var saved Snapshot
		if err = json.Unmarshal(data, &saved); err != nil {
			return nil, err
		}
		for id := range defaults.Modules {
			if _, ok := saved.Modules[id]; !ok {
				return nil, errors.New("模块状态缺少基础模块，请检查备份")
			}
		}
		defaults = saved
	}
	if defaults.SchemaVersion != 1 || defaults.Modules == nil {
		return nil, errors.New("不支持的模块状态格式")
	}
	for id, record := range defaults.Modules {
		if record.Enabled && record.PendingStop {
			return nil, errors.New("模块状态存在冲突")
		}
		if record.PackageSHA256 != "" && (!catalog.ValidOptionalID(id) || !validPackageDigest(record.PackageSHA256)) {
			return nil, errors.New("模块包绑定无效")
		}
	}
	return &Store{path: path, value: defaults}, nil
}
func clone(value Snapshot) Snapshot {
	copied := value
	copied.Modules = make(map[string]Record, len(value.Modules))
	for id, record := range value.Modules {
		copied.Modules[id] = record
	}
	return copied
}
func (s *Store) Snapshot() Snapshot { s.mu.Lock(); defer s.mu.Unlock(); return clone(s.value) }

// SetIntent marks a disable as pending before native effects are changed.
func (s *Store) SetIntent(expected uint64, id string, enabled bool) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if expected != s.value.Revision {
		return clone(s.value), settings.ErrConflict
	}
	record, ok := s.value.Modules[id]
	if !ok {
		return clone(s.value), errors.New("模块未登记")
	}
	if enabled && record.PendingStop {
		return clone(s.value), errors.New("请先恢复未完成的模块关闭")
	}
	next := clone(s.value)
	record.Enabled = enabled
	record.PendingStop = !enabled
	next.Modules[id] = record
	return s.commit(next)
}

// FinishStop clears the recovery marker only after every native resource is returned.
func (s *Store) FinishStop(id string) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.value.Modules[id]
	if !ok || record.Enabled || !record.PendingStop {
		return clone(s.value), errors.New("模块没有待完成的关闭事务")
	}
	next := clone(s.value)
	record.PendingStop = false
	next.Modules[id] = record
	return s.commit(next)
}
func (s *Store) commit(next Snapshot) (Snapshot, error) {
	if next.Revision == ^uint64(0) {
		return clone(s.value), errors.New("模块状态修订已耗尽")
	}
	next.Revision++
	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return clone(s.value), err
	}
	if err = settings.WriteAtomic(s.path, data); err != nil {
		return clone(s.value), err
	}
	s.value = next
	return clone(next), nil
}

func validPackageDigest(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32 && value == strings.ToLower(value)
}

// BindPackage finishes installation with a disabled intent. An exact replay
// preserves later user choices; rebinding active or recovering code is forbidden.
func (s *Store) BindPackage(id, digest string) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !catalog.ValidOptionalID(id) || !validPackageDigest(digest) {
		return clone(s.value), errors.New("模块包标识或摘要无效")
	}
	record := s.value.Modules[id]
	if record.PendingStop {
		return clone(s.value), errors.New("请先恢复未完成的模块关闭")
	}
	if record.PackageSHA256 == digest {
		return clone(s.value), nil
	}
	if record.Enabled {
		return clone(s.value), errors.New("请先关闭模块再更换包")
	}
	next := clone(s.value)
	next.Modules[id] = Record{PackageSHA256: digest}
	return s.commit(next)
}
