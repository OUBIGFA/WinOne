package configuration

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"winone/internal/catalog"
	"winone/internal/configjson"
)

func (s *HostStore) packageUnavailable(id string) error {
	s.catalogMu.RLock()
	defer s.catalogMu.RUnlock()
	if err := s.registered[id].Unavailable; err != nil {
		return fmt.Errorf("模块 %s 不可用：%w", id, err)
	}
	return nil
}

func samePackageNamespace(before, after Document, id string) bool {
	for _, pair := range [][2]map[string]json.RawMessage{{before.Values, after.Values}, {before.DeferredModules, after.DeferredModules}, {before.RetainedModules, after.RetainedModules}} {
		left, lok := pair[0][id]
		right, rok := pair[1][id]
		if lok != rok || !bytes.Equal(left, right) {
			return false
		}
	}
	left, lok := before.ModuleEnabled[id]
	right, rok := after.ModuleEnabled[id]
	return lok == rok && left == right
}
func (s *HostStore) checkUnavailableUnchanged(before, after Document) error {
	_, registered, _ := s.moduleCatalog()
	for id, adapter := range registered {
		if adapter.Unavailable != nil && !samePackageNamespace(before, after, id) {
			return fmt.Errorf("模块 %s 不可用，保留其配置并先恢复模块：%w", id, adapter.Unavailable)
		}
	}
	return nil
}

// A transaction that touched quarantined data may have changed native state.
// Keep its pending record until a verified adapter can restore the old target.
func (s *HostStore) unavailableRecovery() error {
	s.store.mu.Lock()
	previous := clone(s.store.state.Committed)
	var pending *Document
	if s.store.state.Pending != nil {
		copy := clone(*s.store.state.Pending)
		pending = &copy
	}
	s.store.mu.Unlock()
	if pending != nil {
		if err := s.checkUnavailableUnchanged(previous, *pending); err != nil {
			return errors.Join(ErrRecoveryRequired, err)
		}
	}
	return nil
}

// RestorePackageAdapter validates and projects the last committed values with
// a newly verified adapter. It does not clear pending transactions or change
// intent. The package coordinator holds admission and finishes recovery next.
func (s *HostStore) RestorePackageAdapter(adapter ModuleAdapter, revision uint64) error {
	if !catalog.ValidOptionalID(adapter.ID) || adapter.Unavailable != nil || adapter.PackageSchemaVersion < 1 || adapter.Load == nil || adapter.Validate == nil || adapter.Apply == nil || s.lifecycle == nil {
		return errors.New("可选模块恢复适配器无效")
	}
	s.store.operation.Lock()
	defer s.store.operation.Unlock()
	previous := s.store.Snapshot()
	if previous.Revision != revision {
		return ErrConflict
	}
	_, registered, _ := s.moduleCatalog()
	if registered[adapter.ID].Unavailable == nil {
		return errors.New("模块没有待恢复的隔离配置")
	}
	raw, exists := previous.Values[adapter.ID]
	if !exists {
		if previous.ModuleEnabled[adapter.ID] {
			return errors.New("启用的模块缺少已提交配置，请恢复配置备份")
		}
		var err error
		raw, err = moduleInitialSettings(previous, adapter)
		if err != nil {
			return err
		}
	}
	if err := adapter.Validate(raw); err != nil {
		return err
	}
	if err := adapter.Apply(raw, previous.Revision); err != nil {
		return err
	}
	s.putPackageAdapter(adapter)
	s.catalogMu.Lock()
	delete(s.moduleErrors, adapter.ID)
	s.catalogMu.Unlock()
	return nil
}

func (s *HostStore) QuarantinePackage(id string, reason error) error {
	if !catalog.ValidOptionalID(id) || reason == nil {
		return errors.New("模块隔离信息无效")
	}
	s.store.operation.Lock()
	defer s.store.operation.Unlock()
	s.catalogMu.Lock()
	defer s.catalogMu.Unlock()
	if _, exists := s.registered[id]; !exists {
		if s.lifecycle == nil {
			return errors.New("模块生命周期未登记")
		}
		s.lifecycle.Initial[id] = s.store.Snapshot().ModuleEnabled[id]
		adapter := ModuleAdapter{ID: id, Unavailable: reason}
		s.modules = append(s.modules, adapter)
		s.registered[id] = adapter
		s.moduleErrors[id] = reason
		return nil
	}
	adapter := ModuleAdapter{ID: id, Unavailable: reason}
	s.registered[id] = adapter
	s.moduleErrors[id] = reason
	for index, current := range s.modules {
		if current.ID == id {
			s.modules[index] = adapter
			break
		}
	}
	return nil
}

func (s *HostStore) PackageHints() map[string]PackageHint {
	s.store.mu.Lock()
	defer s.store.mu.Unlock()
	return packageHints(s.store.state)
}

// ReadPackageHints only identifies local namespaces so a missing/corrupt package
// can be shown without running it. Normal OpenHost still validates the document;
// this read grants neither trust nor permission to project opaque settings.
type PackageHint struct {
	Enabled       bool
	Values        json.RawMessage
	PendingValues json.RawMessage
}

func ReadPackageHints(directory string) (map[string]PackageHint, error) {
	result := map[string]PackageHint{}
	file, err := os.Open(filepath.Join(directory, "configuration.json"))
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, (8<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > 8<<20 {
		return nil, errors.New("配置文件过大")
	}
	return PackageHintsFromData(raw)
}

// PackageHintsFromData shares normal discovery's interpretation with a
// read-only maintenance probe. It neither applies nor promotes pending data.
func PackageHintsFromData(raw []byte) (map[string]PackageHint, error) {
	if len(raw) > 8<<20 {
		return nil, errors.New("配置文件过大")
	}
	var state diskState
	if err := configjson.DecodeObject(raw, &state); err != nil {
		return nil, err
	}
	for _, document := range []*Document{&state.Committed, state.Pending} {
		if document != nil && document.SchemaVersion != 1 {
			return nil, errors.New("配置版本不受支持")
		}
	}
	return packageHints(state), nil
}

func packageHints(state diskState) map[string]PackageHint {
	result := map[string]PackageHint{}
	for _, document := range []*Document{&state.Committed, state.Pending} {
		if document == nil {
			continue
		}
		for id := range document.Values {
			if catalog.ValidOptionalID(id) {
				hint := result[id]
				hint.Enabled = state.Committed.ModuleEnabled[id]
				hint.Values = append(json.RawMessage(nil), state.Committed.Values[id]...)
				if state.Pending != nil {
					hint.PendingValues = append(json.RawMessage(nil), state.Pending.Values[id]...)
				}
				result[id] = hint
			}
		}
		for id := range document.ModuleEnabled {
			if catalog.ValidOptionalID(id) {
				hint := result[id]
				hint.Enabled = state.Committed.ModuleEnabled[id]
				result[id] = hint
			}
		}
	}
	return result
}
