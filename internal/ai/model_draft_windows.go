//go:build windows

package ai

import (
	"context"
	"errors"
)

// ModelConnectionDraft contains only the fields needed for discovery. A name,
// enabled model or global task assignment is not required to query a service.
type ModelConnectionDraft struct {
	AdditionalTools bool           `json:"additionalTools,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	Protocol     string            `json:"protocol"`
	BaseURL      string            `json:"baseUrl"`
	Preset       string            `json:"preset,omitempty"`
	PresetRegion string            `json:"presetRegion,omitempty"`
	Keys         []KeySlot         `json:"keys"`
	KeyPolicy    *KeyPolicy        `json:"keyPolicy,omitempty"`
}

func (draft ModelConnectionDraft) Provider(id string) (Provider, error) {
	p := Provider{ID: id, Name: "模型连接", AuthMode: "api-key", Protocol: draft.Protocol, BaseURL: draft.BaseURL, Preset: draft.Preset, PresetRegion: draft.PresetRegion, Keys: draft.Keys, KeyPolicy: draft.KeyPolicy, Models: []string{}}
	p.AdditionalTools = draft.AdditionalTools
	p.Headers = draft.Headers
	err := validateConfiguration(Configuration{Engine: "llm", OCREngine: "local", Target: "auto", Providers: []Provider{p}, Tasks: map[string]Selection{}})
	return p, err
}

// WithModelDraft uses the form's connection and key edits without saving them.
// A private credential snapshot reuses validation and retry policy while keeping
// draft failures/cursors separate from requests using the saved configuration.
func (s *Store) WithModelDraft(ctx context.Context, id string, draft ModelConnectionDraft, edits map[string]*string, run func(context.Context, Provider, string) error) error {
	s.mu.Lock()
	p, keys, err := s.modelDraftLocked(id, draft, edits)
	s.mu.Unlock()
	if err != nil {
		return err
	}
	snapshot := &Store{credentialKeys: keys}
	return snapshot.WithKeys(ctx, snapshot.keyPlanLocked(p), func(ctx context.Context, key string) (bool, error) {
		return false, run(ctx, p, key)
	})
}

func (s *Store) modelDraftLocked(id string, draft ModelConnectionDraft, edits map[string]*string) (Provider, map[string]map[string]string, error) {
	previous, exists := s.configurationLocked().Provider(id)
	if exists && previous.AuthMode != "api-key" || id == "legacy" && !exists {
		return Provider{}, nil, errors.New("所选服务不是自定义供应商")
	}
	p, err := draft.Provider(id)
	if err != nil {
		return Provider{}, nil, err
	}
	if p.Keys == nil && exists {
		p.Keys = previous.Keys
	}
	if p.Headers == nil && exists {
		p.Headers = previous.Headers
	}
	if p.KeyPolicy == nil && exists {
		p.KeyPolicy = previous.KeyPolicy
	}
	c := Configuration{Engine: "llm", OCREngine: "local", Target: "auto", Providers: []Provider{p}, Tasks: map[string]Selection{}}
	if err := validateConfiguration(c); err != nil {
		return Provider{}, nil, err
	}
	c = cloneConfiguration(c)
	keys, _, err := s.updatedCredentialsLocked(&c, nil, CredentialUpdates{id: edits})
	return c.Providers[0], keys, err
}
