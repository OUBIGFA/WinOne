//go:build windows

package ai

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"maps"
	"path/filepath"
	"winone/internal/settings"
)

func (s *Store) configurationLocked() Configuration {
	var c Configuration
	if s.configuration != nil {
		c = cloneConfiguration(*s.configuration)
	} else {
		p := s.value
		p.HasKey = s.protected != ""
		c = legacyConfiguration(p)
	}
	for i := range c.Providers {
		p := &c.Providers[i]
		if p.AuthMode != "api-key" {
			p.Keys = nil
			p.KeyPolicy = nil
			p.HasKey = false
			continue
		}
		if p.Keys == nil && s.sealedCredentialLocked(p.ID, "primary") != "" {
			p.Keys = []KeySlot{{ID: "primary", Enabled: true}}
		}
		p.HasKey = false
		for j := range p.Keys {
			p.Keys[j].HasKey = s.sealedCredentialLocked(p.ID, p.Keys[j].ID) != ""
			p.HasKey = p.HasKey || p.Keys[j].HasKey
		}
	}
	for i := range c.MagpieSettings.SearchAPIs {
		api := &c.MagpieSettings.SearchAPIs[i]
		api.Key = ""
		api.HasKey = s.searchKeys[api.Vendor] != ""
	}
	return c
}
func (s *Store) Configuration() Configuration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.configurationLocked()
}

// ProviderForModels binds discovery to a saved provider without requiring an
// enabled model yet. Credentials never enter the public provider response.
func (s *Store) ProviderForModels(id string) (Provider, string, error) {
	p, plan, err := s.ProviderKeysForModels(id)
	if err != nil {
		return p, "", err
	}
	key, err := plan.firstKey()
	return p, key, err
}
func (s *Store) ProviderKeysForModels(id string) (Provider, KeyPlan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.configurationLocked()
	p, ok := c.Provider(id)
	if !ok {
		return Provider{}, KeyPlan{}, errors.New("请先保存供应商")
	}
	if p.AuthMode == "account" {
		return p, KeyPlan{}, nil
	}
	return p, s.keyPlanLocked(p), nil
}
func (s *Store) SaveConfiguration(c Configuration, updates map[string]*string) (Configuration, error) {
	return s.SaveConfigurationWithKeys(c, updates, nil)
}
func (s *Store) SaveConfigurationWithKeys(c Configuration, updates map[string]*string, edits CredentialUpdates) (Configuration, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	old := s.configurationLocked()
	fail := func(err error) (Configuration, error) { return old, err }
	if c.Revision != old.Revision {
		return fail(settings.ErrConflict)
	}
	// Older callers cannot express this display preference; preserve the saved choice.
	if c.AutoRetranslate == nil {
		c.AutoRetranslate = old.AutoRetranslate
	}
	if c.ShowReasoning == nil {
		c.ShowReasoning = old.ShowReasoning
	}
	if c.MagpieSettings == nil {
		c.MagpieSettings = old.MagpieSettings
	}
	searchKeys, err := s.prepareSearchKeys(&c)
	if err != nil {
		return fail(err)
	}
	for i := range c.Providers {
		p := &c.Providers[i]
		if previous, ok := old.Provider(p.ID); ok {
			if p.ModelMetadata == nil {
				p.ModelMetadata = previous.ModelMetadata
			}
			if p.Avatar == nil {
				p.Avatar = previous.Avatar
			}
			if p.Headers == nil && p.AuthMode == previous.AuthMode {
				p.Headers = maps.Clone(previous.Headers)
			}
			if p.ModelFree == nil && p.AuthMode == previous.AuthMode && p.Agent == previous.Agent && p.AgentBackend == previous.AgentBackend && p.BaseURL == previous.BaseURL && p.Protocol == previous.Protocol {
				p.ModelFree = previous.ModelFree
			}
			if p.Keys == nil {
				p.Keys = previous.Keys
			}
			if p.KeyPolicy == nil {
				p.KeyPolicy = previous.KeyPolicy
			}
		}
		if key := updates[p.ID]; p.AuthMode == "api-key" && len(p.Keys) == 0 && key != nil && *key != "" {
			p.Keys = []KeySlot{{ID: "primary", Enabled: true}}
		}
		if c.Providers[i].AvailableModels == nil {
			if previous, ok := old.Provider(c.Providers[i].ID); ok {
				c.Providers[i].AvailableModels = previous.AllModels()
			}
		}
	}
	if err := validateConfiguration(c); err != nil {
		return fail(err)
	}
	c = cloneConfiguration(c)
	if c.Tasks == nil {
		c.Tasks = map[string]Selection{}
	}
	sets, keys, err := s.updatedCredentialsLocked(&c, updates, edits)
	if err != nil {
		return fail(err)
	}
	c.Revision++
	// Preserve a readable legacy projection for rollback, while new readers
	// always use the versioned collection and its per-provider credentials.
	projected := projectConfiguration(c)
	selected := Selection{Provider: projected.ProviderID}
	encoded, err := json.Marshal(stored{SchemaVersion: 2, Preferences: projected, ProtectedKey: keys[selected.Provider], Configuration: &c, ProtectedKeys: keys, ProtectedCredentialKeys: sets, ProtectedSearchKeys: searchKeys})
	if err != nil {
		return fail(err)
	}
	if len(encoded) > maximumSettingsBytes {
		return fail(errors.New("AI 配置过大，请减少服务或模型数量后重试"))
	}
	if err = settings.WriteAtomic(s.path, encoded); err != nil {
		return fail(err)
	}
	s.configuration = &c
	s.keys = keys
	s.credentialKeys = sets
	s.searchKeys = searchKeys
	s.pruneKeyRoundsLocked(sets)
	s.value = projected
	s.protected = keys[selected.Provider]
	return cloneConfiguration(c), nil
}
func projectConfiguration(c Configuration) Preferences {
	p := DefaultPreferences()
	p.Revision = c.Revision
	p.Engine = c.Engine
	p.OCREngine = c.OCREngine
	p.Target = c.Target
	p.Prompt = c.Prompt
	selected := c.Selected("default")
	// Old versions have one service slot. When no general default exists,
	// project the active translation/OCR task with its matching credential.
	if selected.Provider == "" && c.Engine == "llm" {
		selected = c.Selected("translate")
	}
	if selected.Provider == "" && c.OCREngine == "ai" {
		selected = c.Selected("ocr")
	}
	if selected.Provider == "" {
		selected = c.Selected("chat")
	}
	provider, ok := c.Provider(selected.Provider)
	if ok {
		p.ProviderID = provider.ID
		p.AuthMode = provider.AuthMode
		p.Agent = provider.Agent
		p.AgentBackend = provider.AgentBackend
		p.Preset, p.PresetRegion = provider.Preset, provider.PresetRegion
		p.Protocol = provider.Protocol
		p.BaseURL = provider.BaseURL
		p.Model = selected.Model
		p.HasKey = provider.HasKey
	}
	if p.Protocol == "" {
		p.Protocol = "chat"
	}
	return p
}

// ForTask takes one immutable routing/credential snapshot. A window override
// is validated against the same saved model list and never changes defaults.
func (s *Store) ForTask(task string, override *Selection) (Preferences, string, error) {
	p, plan, err := s.ForTaskKeys(task, override)
	if err != nil {
		return p, "", err
	}
	key, err := plan.firstKey()
	return p, key, err
}
func (s *Store) ForTaskKeys(task string, override *Selection) (Preferences, KeyPlan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.configurationLocked()
	p := projectConfiguration(c)
	if override == nil && (task == "ocr" && c.OCREngine == "local" || task == "translate" && c.Engine == "google") {
		return p, KeyPlan{}, nil
	}
	selection := c.Selected(task)
	if override != nil {
		selection = *override
	}
	if err := c.ValidateSelection(selection); err != nil {
		return p, KeyPlan{}, err
	}
	provider, _ := c.Provider(selection.Provider)
	p.Engine = "llm"
	p.OCREngine = "ai"
	p.AuthMode = provider.AuthMode
	p.Protocol = provider.Protocol
	p.BaseURL = provider.BaseURL
	p.Model = selection.Model
	p.Agent = provider.Agent
	p.AgentBackend = provider.AgentBackend
	p.Preset, p.PresetRegion = provider.Preset, provider.PresetRegion
	p.ProviderID = provider.ID
	p.ProviderName = provider.Name
	p.ModelName = provider.ModelName(selection.Model)
	p.ProviderIcon = provider.DisplayIcon()
	p.AdditionalTools = provider.AdditionalTools
	p.Headers = maps.Clone(provider.Headers)
	p.ModelMetadata = provider.ModelMetadataFor(selection.Model)
	p.RequestLimits = provider.RequestLimits
	var err error
	p.MagpieSettings, err = s.runtimeMagpieSettingsLocked(c)
	if err != nil {
		return p, KeyPlan{}, err
	}
	p.Auxiliaries, err = s.runtimeProvidersLocked(c)
	if err != nil {
		return p, KeyPlan{}, err
	}
	if p.Protocol == "" {
		p.Protocol = "chat"
	}
	if p.AuthMode == "account" {
		return p, KeyPlan{}, nil
	}
	return p, s.keyPlanLocked(provider), nil
}
func unsealKey(sealed string) (string, error) {
	if sealed == "" {
		return "", nil
	}
	data, err := base64.StdEncoding.DecodeString(sealed)
	if err != nil {
		return "", errors.New("AI 密钥无法读取，请重新填写")
	}
	plain, err := protect(data, true)
	if err != nil {
		return "", errors.New("AI 密钥无法解密，请重新填写")
	}
	defer clear(plain)
	return string(plain), nil
}

// The migrated account keeps its existing private home; each newly configured
// account receives a separate home even when it uses the same agent type.
func (t Translator) ForProvider(id string) Translator {
	if t.RootDataDirectory == "" {
		t.RootDataDirectory = t.DataDirectory
	}
	if t.UsagePath == "" {
		t.UsagePath = filepath.Join(t.DataDirectory, "config", "magpie", "usage.jsonl")
	}
	if id != "" && id != "legacy" {
		t.DataDirectory = filepath.Join(t.DataDirectory, "profiles", "provider-"+id)
	}
	return t
}
