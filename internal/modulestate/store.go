// Package modulestate persists module intent separately from volatile process state.
package modulestate

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
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
	defaults := Snapshot{SchemaVersion: 5, Modules: map[string]Record{}}
	for _, m := range catalog.Builtins() {
		defaults.Modules[m.ID] = Record{Enabled: true}
	}
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	migrated := false
	backupSuffix := ".before-stroke.bak"
	if err == nil {
		var saved Snapshot
		if err = json.Unmarshal(data, &saved); err != nil {
			return nil, err
		}
		if saved.SchemaVersion == 1 {
			backupSuffix = ".before-selection.bak"
			for _, id := range []string{"capture", "always-on-top", "wintray"} {
				if _, ok := saved.Modules[id]; !ok {
					return nil, errors.New("旧模块状态缺少基础模块，请检查备份")
				}
			}
			if _, exists := saved.Modules["selection"]; !exists {
				saved.Modules["selection"] = Record{Enabled: true}
			}
			if saved.Revision == math.MaxUint64 {
				return nil, errors.New("模块状态修订已耗尽")
			}
			saved.SchemaVersion = 2
			saved.Revision++
			migrated = true
		}
		if saved.SchemaVersion == 2 {
			if !migrated {
				backupSuffix = ".before-quick-assistant.bak"
			}
			if _, exists := saved.Modules["selection"]; !exists {
				return nil, errors.New("模块状态缺少划词助手")
			}
			if saved.Revision == math.MaxUint64 {
				return nil, errors.New("模块状态修订已耗尽")
			}
			if _, exists := saved.Modules["quick-assistant"]; !exists {
				saved.Modules["quick-assistant"] = Record{Enabled: true}
			}
			saved.SchemaVersion = 3
			saved.Revision++
			migrated = true
		}
		if saved.SchemaVersion == 3 {
			// Add the new bundled engine once. Never enable an existing disabled
			// scope or alter any prior module's enable/pending-stop intent.
			if _, exists := saved.Modules["quick-assistant"]; !exists {
				return nil, errors.New("模块状态缺少快捷助手")
			}
			if saved.Revision == math.MaxUint64 {
				return nil, errors.New("模块状态修订已耗尽")
			}
			if _, exists := saved.Modules["stroke"]; !exists {
				saved.Modules["stroke"] = Record{Enabled: true}
			}
			saved.SchemaVersion = 4
			saved.Revision++
			migrated = true
		}
		if saved.SchemaVersion == 4 {
			if !migrated {
				backupSuffix = ".before-timed-power.bak"
			}
			if _, exists := saved.Modules["stroke"]; !exists {
				return nil, errors.New("模块状态缺少鼠标手势")
			}
			if saved.Revision == math.MaxUint64 {
				return nil, errors.New("模块状态修订已耗尽")
			}
			if _, exists := saved.Modules["timed-power"]; !exists {
				saved.Modules["timed-power"] = Record{Enabled: true}
			}
			saved.SchemaVersion = 5
			saved.Revision++
			migrated = true
		}
		for id := range defaults.Modules {
			if _, ok := saved.Modules[id]; !ok {
				return nil, errors.New("模块状态缺少基础模块，请检查备份")
			}
		}
		defaults = saved
	}
	if defaults.SchemaVersion != 5 || defaults.Modules == nil {
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
	if migrated {
		if err = settings.PreserveMigrationBackup(path+backupSuffix, data); err != nil {
			return nil, err
		}
		next, marshalErr := json.Marshal(defaults)
		if marshalErr != nil {
			return nil, marshalErr
		}
		if err = settings.WriteAtomic(path, next); err != nil {
			return nil, err
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

// ReplacePackageBinding is the compare-and-replace boundary for a journaled
// package change. It never enables code or clears unfinished native recovery.
// An empty target unbinds a removed package while retaining its preferences.
func (s *Store) ReplacePackageBinding(id, expected, target string) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !catalog.ValidOptionalID(id) || (expected != "" && !validPackageDigest(expected)) || (target != "" && !validPackageDigest(target)) {
		return clone(s.value), errors.New("模块包标识或摘要无效")
	}
	record := s.value.Modules[id]
	if record.Enabled || record.PendingStop {
		return clone(s.value), errors.New("请先关闭模块并完成资源恢复")
	}
	if record.PackageSHA256 == target {
		return clone(s.value), nil
	}
	if record.PackageSHA256 != expected {
		return clone(s.value), settings.ErrConflict
	}
	next := clone(s.value)
	record.PackageSHA256 = target
	next.Modules[id] = record
	return s.commit(next)
}
