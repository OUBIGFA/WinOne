//go:build windows

package ai

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
)

// CredentialUpdates is write-only: provider ID -> key ID -> replacement.
// Omission/nil preserves the cipher; an empty replacement clears that key.
type CredentialUpdates map[string]map[string]*string

func (s *Store) sealedCredentialLocked(provider, id string) string {
	if keys, ok := s.credentialKeys[provider]; ok {
		return keys[id]
	}
	if id != "primary" {
		return ""
	}
	if s.configuration == nil && provider == "legacy" {
		return s.protected
	}
	return s.keys[provider]
}

// RevealCredential reads one saved key only for an explicit user action. It
// neither rotates request keys nor consumes the stored value; disabled keys
// remain inspectable. Configuration snapshots and exports still omit secrets.
func (s *Store) RevealCredential(provider, id string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.configurationLocked().Provider(provider)
	if !ok || p.AuthMode != "api-key" {
		return "", errors.New("请选择已保存的自定义供应商")
	}
	for _, slot := range p.Keys {
		if slot.ID != id {
			continue
		}
		sealed := s.sealedCredentialLocked(provider, id)
		if sealed == "" {
			return "", errors.New("此密钥尚未保存，请先填写密钥")
		}
		return unsealKey(sealed)
	}
	return "", errors.New("密钥输入项不存在或不属于该供应商")
}

func sealCredential(value string) (string, error) {
	if len(value) > 16384 {
		return "", errors.New("密钥过长")
	}
	if value == "" {
		return "", nil
	}
	plain := []byte(value)
	defer clear(plain)
	sealed, err := protect(plain, false)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(sealed), nil
}

func (s *Store) updatedCredentialsLocked(c *Configuration, legacy map[string]*string, edits CredentialUpdates) (map[string]map[string]string, map[string]string, error) {
	sets := map[string]map[string]string{}
	for _, provider := range c.Providers {
		if provider.AuthMode != "api-key" {
			continue
		}
		keys := map[string]string{}
		for _, slot := range provider.Keys {
			keys[slot.ID] = s.sealedCredentialLocked(provider.ID, slot.ID)
		}
		sets[provider.ID] = keys
	}
	for providerID, value := range legacy {
		provider, ok := c.Provider(providerID)
		if !ok || provider.AuthMode != "api-key" {
			return nil, nil, errors.New("密钥必须属于已配置的自定义供应商")
		}
		if value == nil {
			continue
		}
		if len(edits[providerID]) > 0 || len(provider.Keys) > 1 {
			return nil, nil, errors.New("多个密钥请逐项编辑")
		}
		if len(provider.Keys) == 0 {
			if *value != "" {
				return nil, nil, errors.New("密钥输入项不存在")
			}
			continue
		}
		sealed, err := sealCredential(*value)
		if err != nil {
			return nil, nil, err
		}
		sets[providerID][provider.Keys[0].ID] = sealed
	}
	for providerID, changes := range edits {
		provider, ok := c.Provider(providerID)
		if !ok || provider.AuthMode != "api-key" {
			return nil, nil, errors.New("密钥必须属于已配置的自定义供应商")
		}
		for id, value := range changes {
			if _, ok := sets[providerID][id]; !ok {
				return nil, nil, errors.New("密钥输入项不存在或不属于该供应商")
			}
			if value == nil {
				continue
			}
			sealed, err := sealCredential(*value)
			if err != nil {
				return nil, nil, err
			}
			sets[providerID][id] = sealed
		}
	}
	projection := map[string]string{}
	for i := range c.Providers {
		p := &c.Providers[i]
		p.HasKey = false
		seen := map[[32]byte]bool{}
		for j := range p.Keys {
			slot := &p.Keys[j]
			sealed := sets[p.ID][slot.ID]
			slot.HasKey = sealed != ""
			p.HasKey = p.HasKey || slot.HasKey
			if sealed == "" {
				continue
			}
			plain, err := unsealKey(sealed)
			if err != nil {
				return nil, nil, err
			}
			hash := sha256.Sum256([]byte(plain))
			if seen[hash] {
				return nil, nil, errors.New("同一供应商中存在重复密钥")
			}
			seen[hash] = true
			// An old reader gets the first enabled credential. Disabled/deleted
			// keys must never return through the legacy projection.
			if slot.Enabled && projection[p.ID] == "" {
				projection[p.ID] = sealed
			}
		}
	}
	return sets, projection, nil
}

func (s *Store) pruneKeyRoundsLocked(sets map[string]map[string]string) {
	for provider, round := range s.keyRounds {
		keys, exists := sets[provider]
		if !exists {
			delete(s.keyRounds, provider)
			continue
		}
		for id, rest := range round.rest {
			if keys[id] != rest.sealed {
				delete(round.rest, id)
			}
		}
	}
}
