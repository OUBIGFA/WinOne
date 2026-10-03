package configuration

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"sync"
	"winone/internal/catalog"
	"winone/internal/configjson"
	"winone/internal/settings"
)

// HostStore adapts the existing UI settings API to the canonical configuration.
// Legacy settings.json is read once and thereafter retained without modification.
type HostStore struct {
	store        *Store
	apply        func(settings.Settings) error
	modules      []ModuleAdapter
	moduleErrors map[string]error
	lifecycle    *ModuleLifecycle
	recoveryMu   sync.RWMutex
	startupError error
}

func OpenHost(directory string, adapters ...ModuleAdapter) (*HostStore, error) {
	return openHost(directory, nil, adapters...)
}

func OpenHostWithLifecycle(directory string, lifecycle ModuleLifecycle, adapters ...ModuleAdapter) (*HostStore, error) {
	if lifecycle.Initial == nil || lifecycle.Before == nil || lifecycle.After == nil {
		return nil, errors.New("模块生命周期接口不完整")
	}
	lifecycle.Initial = copyEnabled(lifecycle.Initial)
	return openHost(directory, &lifecycle, adapters...)
}

func openHost(directory string, lifecycle *ModuleLifecycle, adapters ...ModuleAdapter) (*HostStore, error) {
	registered := map[string]ModuleAdapter{}
	for _, adapter := range adapters {
		if !catalog.ValidID(adapter.ID) || adapter.ID == "host" || adapter.Load == nil || adapter.Validate == nil || adapter.Apply == nil {
			return nil, errors.New("模块配置适配器无效")
		}
		if _, exists := registered[adapter.ID]; exists {
			return nil, errors.New("模块配置适配器重复")
		}
		registered[adapter.ID] = adapter
	}
	path := filepath.Join(directory, "configuration.json")
	_, err := os.Stat(path)
	migrate := errors.Is(err, os.ErrNotExist)
	if err != nil && !migrate {
		return nil, err
	}
	initial := Document{}
	if migrate {
		legacy, err := settings.Open(filepath.Join(directory, "settings.json"))
		if err != nil {
			return nil, err
		}
		value := legacy.Snapshot()
		initial = Document{SchemaVersion: 1, Revision: value.Revision, Values: map[string]json.RawMessage{}}
		value.Revision = 0 // The document revision is the single concurrency token.
		initial.Values["host"], err = json.Marshal(value)
		if err != nil {
			return nil, err
		}
	}
	validate := func(document Document) error {
		var enabled map[string]bool
		if lifecycle != nil {
			enabled = lifecycle.Initial
		}
		return validateRegisteredDocument(document, registered, enabled)
	}
	store, err := Open(path, initial, validate)
	if err != nil {
		return nil, err
	}
	if migrate {
		if err = store.persist(diskState{Committed: initial}); err != nil {
			return nil, err
		}
	}
	result := &HostStore{store: store, modules: append([]ModuleAdapter(nil), adapters...), moduleErrors: map[string]error{}, lifecycle: lifecycle}
	if err = store.Recover(result.applyDocument); err != nil {
		result.startupError = err
		return result, err
	}
	document := store.Snapshot()
	changed := false
	if lifecycle != nil && document.ModuleEnabled == nil {
		document.ModuleEnabled = copyEnabled(lifecycle.Initial)
		changed = true
	}
	for _, adapter := range adapters {
		if _, exists := document.Values[adapter.ID]; exists {
			continue
		}
		raw, loadErr := adapter.Load()
		if loadErr == nil {
			loadErr = adapter.Validate(raw)
		}
		if loadErr != nil {
			result.moduleErrors[adapter.ID] = loadErr
			continue
		}
		document.Values[adapter.ID] = append(json.RawMessage(nil), raw...)
		changed = true
	}
	prepared, normalized, prepareErr := result.prepareDocument(document)
	if prepareErr != nil {
		return nil, prepareErr
	}
	document = prepared
	changed = changed || normalized
	if changed {
		if !migrate {
			if document.Revision == math.MaxUint64 {
				return nil, errors.New("配置修订号已用尽")
			}
			document.Revision++
		}
		if err = store.check(document); err != nil {
			return nil, err
		}
		if err = store.persist(diskState{Committed: document}); err != nil {
			return nil, err
		}
	}
	// Startup projects the committed document before any module process exists.
	if err = result.applyDocument(document); err != nil {
		result.startupError = err
		return result, err
	}
	return result, nil
}

func validateHostDocument(document Document) error {
	var value settings.Settings
	if err := configjson.DecodeObject(document.Values["host"], &value); err != nil {
		return err
	}
	if value.Revision != 0 {
		return errors.New("全局配置修订号应由配置中心管理")
	}
	return settings.Validate(value)
}

func hostValue(document Document) settings.Settings {
	var value settings.Settings
	// Open and Commit validate this namespace before publication; documents are
	// deep copies, so decoding cannot fail for a published snapshot.
	_ = json.Unmarshal(document.Values["host"], &value)
	value.Revision = document.Revision
	return value
}

func (s *HostStore) Snapshot() settings.Settings { return hostValue(s.store.Snapshot()) }

// CommittedDocument is a deep copy across all preference namespaces. Export must
// use this single snapshot rather than querying workers at different moments.
func (s *HostStore) CommittedDocument() Document { return s.store.Snapshot() }

// SetApply is installed during startup, before serving requests. The callback
// reconciles desired preferences both for a change and for its compensation.
func (s *HostStore) SetApply(apply func(settings.Settings) error) { s.apply = apply }

func (s *HostStore) Save(next settings.Settings) (settings.Settings, error) {
	document := s.store.Snapshot()
	if next.Language == "" {
		next.Language = hostValue(document).Language
	}
	if err := s.RecoveryError(); err != nil {
		return hostValue(document), errors.Join(ErrRecoveryRequired, err)
	}
	if next.Revision != document.Revision {
		return hostValue(document), settings.ErrConflict
	}
	if err := settings.Validate(next); err != nil {
		return hostValue(document), err
	}
	next.Revision = 0
	data, err := json.Marshal(next)
	if err != nil {
		return hostValue(document), err
	}
	document.Values["host"] = data
	saved, err := s.CommitDocument(document)
	if errors.Is(err, ErrConflict) {
		err = settings.ErrConflict
	}
	return hostValue(saved), err
}
