package configuration

import (
	"encoding/json"
	"errors"
	"strings"
	"winone/internal/catalog"
	"winone/internal/configjson"
)

// Copies avoid holding catalog locks while a native projection calls back into
// settings. Catalog mutations also hold store.operation across the transaction.
func (s *HostStore) moduleCatalog() ([]ModuleAdapter, map[string]ModuleAdapter, map[string]bool) {
	s.catalogMu.RLock()
	defer s.catalogMu.RUnlock()
	registered := make(map[string]ModuleAdapter, len(s.registered))
	for id, adapter := range s.registered {
		registered[id] = adapter
	}
	var enabled map[string]bool
	if s.lifecycle != nil {
		enabled = copyEnabled(s.lifecycle.Initial)
	}
	return append([]ModuleAdapter(nil), s.modules...), registered, enabled
}

type retainedPackageSettings struct {
	SchemaVersion int             `json:"schemaVersion"`
	Settings      json.RawMessage `json:"settings"`
}

// PreviewPackageSettings checks retained/default data without registering or
// projecting the module. The same rules are applied again during installation.
func (s *HostStore) PreviewPackageSettings(adapter ModuleAdapter) (json.RawMessage, uint64, error) {
	document := s.store.Snapshot()
	if !catalog.ValidOptionalID(adapter.ID) || adapter.PackageSchemaVersion < 1 || adapter.Load == nil || adapter.Validate == nil {
		return nil, document.Revision, errors.New("模块配置定义无效")
	}
	if _, exists := document.Values[adapter.ID]; exists {
		return nil, document.Revision, errors.New("模块配置已登记，请先恢复或卸载现有模块")
	}
	raw, err := moduleInitialSettings(document, adapter)
	if err == nil {
		err = adapter.Validate(raw)
	}
	return raw, document.Revision, err
}

func moduleInitialSettings(document Document, adapter ModuleAdapter) (json.RawMessage, error) {
	if _, exists := document.DeferredModules[adapter.ID]; exists && adapter.PackageSchemaVersion > 0 {
		// Portable settings can contain path references and have a different
		// schema. They require the module's preview/mapping adapter first.
		return nil, errors.New("模块存在待应用的导入配置，请先完成配置预览与转换")
	}
	if raw, exists := document.RetainedModules[adapter.ID]; exists && adapter.PackageSchemaVersion > 0 {
		var retained retainedPackageSettings
		if err := configjson.DecodeObject(raw, &retained); err != nil {
			return nil, err
		}
		if retained.SchemaVersion != adapter.PackageSchemaVersion {
			if adapter.Migrate == nil {
				return nil, errors.New("保留的模块配置版本不兼容，请先完成配置迁移")
			}
			return adapter.Migrate(retained.SchemaVersion, append(json.RawMessage(nil), retained.Settings...))
		}
		return append(json.RawMessage(nil), retained.Settings...), nil
	}
	return adapter.Load()
}

func (s *HostStore) RetainedPackageVersion(id string) (int, error) {
	raw, exists := s.store.Snapshot().RetainedModules[id]
	if !exists {
		return 0, nil
	}
	var retained retainedPackageSettings
	if err := configjson.DecodeObject(raw, &retained); err != nil {
		return 0, err
	}
	return retained.SchemaVersion, nil
}

func (s *HostStore) putPackageAdapter(adapter ModuleAdapter) {
	s.catalogMu.Lock()
	defer s.catalogMu.Unlock()
	for index, current := range s.modules {
		if current.ID == adapter.ID {
			s.modules[index] = adapter
			s.registered[adapter.ID] = adapter
			return
		}
	}
	s.modules = append(s.modules, adapter)
	s.registered[adapter.ID] = adapter
	s.lifecycle.Initial[adapter.ID] = false
}

func (s *HostStore) dropPackageAdapter(id string) {
	s.catalogMu.Lock()
	defer s.catalogMu.Unlock()
	for index, adapter := range s.modules {
		if adapter.ID == id {
			s.modules = append(s.modules[:index], s.modules[index+1:]...)
			break
		}
	}
	delete(s.registered, id)
	delete(s.lifecycle.Initial, id)
	delete(s.moduleErrors, id)
}

// RegisterPackageModule is called by the verified package coordinator, never
// directly with UI-supplied callbacks. It defaults to disabled and restores only
// settings with the exact schema accepted by the selected package adapter.
// The coordinator must also hold the Host's configuration admission lease while
// binding/unbinding the corresponding runtime gate; this lock covers storage.
func (s *HostStore) RegisterPackageModule(adapter ModuleAdapter, revision uint64) error {
	if !catalog.ValidOptionalID(adapter.ID) || adapter.PackageSchemaVersion < 1 || adapter.Unavailable != nil || adapter.Load == nil || adapter.Validate == nil || adapter.Apply == nil || s.lifecycle == nil {
		return errors.New("可选模块配置适配器无效")
	}
	s.store.operation.Lock()
	defer s.store.operation.Unlock()
	if err := s.RecoveryError(); err != nil {
		return errors.Join(ErrRecoveryRequired, err)
	}
	previous := s.store.Snapshot()
	if previous.Revision != revision {
		return ErrConflict
	}
	_, registered, _ := s.moduleCatalog()
	if current, exists := registered[adapter.ID]; exists && current.PackageSchemaVersion == 0 && current.Unavailable == nil {
		return errors.New("模块配置已由固定适配器登记")
	}
	for id := range registered {
		if id != adapter.ID && (strings.HasPrefix(id, adapter.ID+".") || strings.HasPrefix(adapter.ID, id+".")) {
			return errors.New("模块配置命名空间冲突")
		}
	}
	if _, exists := previous.Values[adapter.ID]; exists {
		return errors.New("模块配置已登记")
	}
	if enabled, exists := previous.ModuleEnabled[adapter.ID]; exists && (enabled || registered[adapter.ID].PackageSchemaVersion == 0 && registered[adapter.ID].Unavailable == nil) {
		return errors.New("模块开关已登记")
	}
	raw, err := moduleInitialSettings(previous, adapter)
	if err != nil {
		return err
	}
	if err = adapter.Validate(raw); err != nil {
		return err
	}
	s.putPackageAdapter(adapter)
	next := clone(previous)
	next.Values[adapter.ID] = append(json.RawMessage(nil), raw...)
	next.ModuleEnabled[adapter.ID] = false
	delete(next.RetainedModules, adapter.ID)
	prepared, _, err := s.prepareDocument(next)
	if err == nil {
		_, err = s.store.commit(prepared, s.applyDocument)
	}
	if err != nil && !s.store.NeedsRecovery() {
		s.dropPackageAdapter(adapter.ID)
	}
	if err == nil {
		s.catalogMu.Lock()
		delete(s.moduleErrors, adapter.ID)
		s.catalogMu.Unlock()
	}
	return err
}

// UnregisterPackageModule first lets the shared lifecycle restore the module's
// resources. Only a successful commit removes its adapter; settings remain in
// the canonical document for a compatible reinstall. Export needs a separate
// portable schema/mapping adapter and cannot copy these local values.
func (s *HostStore) UnregisterPackageModule(id string, revision uint64) error {
	s.store.operation.Lock()
	defer s.store.operation.Unlock()
	if err := s.RecoveryError(); err != nil {
		return errors.Join(ErrRecoveryRequired, err)
	}
	previous := s.store.Snapshot()
	if previous.Revision != revision {
		return ErrConflict
	}
	_, registered, _ := s.moduleCatalog()
	adapter, exists := registered[id]
	if exists && adapter.Unavailable != nil {
		return adapter.Unavailable
	}
	if !exists || adapter.PackageSchemaVersion < 1 || !catalog.ValidOptionalID(id) || s.lifecycle == nil {
		return errors.New("可选模块配置未登记")
	}
	raw, exists := previous.Values[id]
	if !exists {
		return errors.New("模块配置尚未恢复")
	}
	retained, err := json.Marshal(retainedPackageSettings{adapter.PackageSchemaVersion, raw})
	if err != nil {
		return err
	}
	next := clone(previous)
	delete(next.Values, id)
	delete(next.ModuleEnabled, id)
	if next.RetainedModules == nil {
		next.RetainedModules = map[string]json.RawMessage{}
	}
	next.RetainedModules[id] = retained
	if _, err = s.store.commit(next, s.applyDocument); err != nil {
		return err
	}
	s.dropPackageAdapter(id)
	return nil
}
