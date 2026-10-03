package configuration

import (
	"encoding/json"
	"errors"
	"winone/internal/catalog"
	"winone/internal/configjson"
)

// validateRegisteredDocument is shared by normal loading and read-only upgrade
// checks. Only validation callbacks run; no loading, migration or recovery.
func validateRegisteredDocument(document Document, registered map[string]ModuleAdapter, enabled map[string]bool) error {
	if document.ModuleEnabled != nil {
		if enabled == nil {
			return errors.New("配置需要模块生命周期恢复支持")
		}
		if len(document.ModuleEnabled) != len(enabled) {
			return errors.New("模块开关列表不完整")
		}
		for id := range enabled {
			if _, exists := document.ModuleEnabled[id]; !exists {
				return errors.New("模块开关缺失")
			}
		}
	}
	for id, raw := range document.Values {
		if id == "host" {
			continue
		}
		adapter, exists := registered[id]
		if !exists {
			return errors.New("配置包含尚未支持的模块，请使用匹配版本")
		}
		if err := adapter.Validate(append(json.RawMessage(nil), raw...)); err != nil {
			return err
		}
	}
	return validateHostDocument(document)
}

// CheckUpgrade validates a complete canonical configuration without touching
// disk or runtime state. Incomplete/legacy profiles require a separate migration
// preview; a pending transaction must be recovered by the old version first.
func CheckUpgrade(data []byte, adapters ...ModuleAdapter) error {
	if len(data) > 8<<20 {
		return errors.New("配置文件过大")
	}
	registered := map[string]ModuleAdapter{}
	enabled := map[string]bool{}
	for _, adapter := range adapters {
		if !catalog.ValidID(adapter.ID) || adapter.ID == "host" || adapter.Validate == nil {
			return errors.New("模块配置校验器无效")
		}
		if _, exists := registered[adapter.ID]; exists {
			return errors.New("模块配置校验器重复")
		}
		registered[adapter.ID], enabled[adapter.ID] = adapter, true
	}
	var state diskState
	if err := configjson.DecodeObject(data, &state); err != nil {
		return err
	}
	if state.Pending != nil {
		return ErrRecoveryRequired
	}
	validator := &Store{validate: func(document Document) error { return validateRegisteredDocument(document, registered, enabled) }}
	if err := validator.check(state.Committed); err != nil {
		return err
	}
	if state.Committed.ModuleEnabled == nil {
		return errors.New("请先完成模块配置迁移")
	}
	for id := range registered {
		if _, exists := state.Committed.Values[id]; !exists {
			return errors.New("请先完成模块配置迁移")
		}
	}
	return nil
}
