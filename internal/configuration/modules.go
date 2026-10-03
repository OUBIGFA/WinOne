package configuration

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"winone/internal/settings"
)

// ModuleAdapter supplies a one-time legacy reader and a read-only worker
// projection. Apply must await the runtime and must not activate an idle module.
type ModuleAdapter struct {
	ID       string
	Load     func() (json.RawMessage, error)
	Validate func(json.RawMessage) error
	Apply    func(json.RawMessage, uint64) error
	Prepare  func(json.RawMessage, settings.Settings) (json.RawMessage, error)
}

// prepareDocument converges compatibility fields onto global preferences before
// validation and persistence; workers never choose a second product theme.
func (s *HostStore) prepareDocument(document Document) (Document, bool, error) {
	next := clone(document)
	changed := false
	global := hostValue(next)
	if global.Startup == nil {
		var legacy struct {
			RunAtLogon     bool `json:"runAtLogon"`
			StartMinimized bool `json:"startMinimizedToTray"`
		}
		if raw, exists := next.Values["wintray"]; exists {
			if err := json.Unmarshal(raw, &legacy); err != nil {
				return Document{}, false, err
			}
		}
		global.Startup = &settings.StartupPreferences{RunAtLogon: legacy.RunAtLogon, StartMinimized: legacy.StartMinimized}
		global.Revision = 0
		raw, err := json.Marshal(global)
		if err != nil {
			return Document{}, false, err
		}
		next.Values["host"] = raw
		changed = true
	}
	for _, adapter := range s.modules {
		raw, exists := next.Values[adapter.ID]
		if !exists || adapter.Prepare == nil {
			continue
		}
		prepared, err := adapter.Prepare(append(json.RawMessage(nil), raw...), global)
		if err != nil {
			return Document{}, false, err
		}
		if !bytes.Equal(raw, prepared) {
			changed = true
		}
		next.Values[adapter.ID] = prepared
	}
	return next, changed, nil
}

func (s *HostStore) applyDocument(document Document) error {
	if document.ModuleEnabled != nil {
		if s.lifecycle == nil {
			return errors.New("模块生命周期接口未就绪")
		}
		if err := s.lifecycle.Before(copyEnabled(document.ModuleEnabled)); err != nil {
			return err
		}
	}
	for _, adapter := range s.modules {
		if raw, exists := document.Values[adapter.ID]; exists {
			if err := adapter.Apply(append(json.RawMessage(nil), raw...), document.Revision); err != nil {
				return fmt.Errorf("%s: %w", adapter.ID, err)
			}
		}
	}
	if document.ModuleEnabled != nil {
		if err := s.lifecycle.After(copyEnabled(document.ModuleEnabled)); err != nil {
			return err
		}
	}
	// Lifecycle transitions may release/register shortcuts. Apply the target
	// global preferences last so an enable callback cannot restore old shortcuts.
	if s.apply != nil {
		if err := s.apply(hostValue(document)); err != nil {
			return err
		}
	}
	return nil
}

// Module returns committed preferences even if the feature is disabled.
func (s *HostStore) Module(id string) (json.RawMessage, uint64, error) {
	document := s.store.Snapshot()
	raw, exists := document.Values[id]
	if !exists || id == "host" {
		if err := s.moduleErrors[id]; err != nil {
			return nil, document.Revision, err
		}
		return nil, document.Revision, errors.New("模块配置未登记")
	}
	return raw, document.Revision, nil
}

func (s *HostStore) SaveModule(id string, revision uint64, raw json.RawMessage) (json.RawMessage, uint64, error) {
	document := s.store.Snapshot()
	if err := s.RecoveryError(); err != nil {
		return nil, document.Revision, errors.Join(ErrRecoveryRequired, err)
	}
	if _, exists := document.Values[id]; !exists || id == "host" {
		return nil, document.Revision, errors.New("模块配置未登记")
	}
	if revision != document.Revision {
		return nil, document.Revision, ErrConflict
	}
	document.Values[id] = append(json.RawMessage(nil), raw...)
	saved, err := s.CommitDocument(document)
	return saved.Values[id], saved.Revision, err
}
