//go:build windows

package ai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// prepareSearchKeys makes a new credential set before saving anything. The
// submitted public object is copied so neither a failed save nor encryption
// mutates a caller's draft. Omitted keys preserve existing credentials.
func (s *Store) prepareSearchKeys(c *Configuration) (map[string]string, error) {
	c.MagpieSettings = c.MagpieSettings.clone()
	next := map[string]string{}
	for i := range c.MagpieSettings.SearchAPIs {
		api := &c.MagpieSettings.SearchAPIs[i]
		if api.KeyList != nil {
			api.Key, api.HasKey = "", false
			seen := map[string]bool{}
			kept := []SearchKey{}
			for j := range *api.KeyList {
				slot := &(*api.KeyList)[j]
				ref := api.Vendor + "/" + slot.ID
				sealed := s.searchKeys[ref]
				plain := slot.Key
				if plain == "" && sealed != "" {
					var err error
					plain, err = unsealKey(sealed)
					if err != nil {
						return nil, err
					}
				}
				// Upstream AddSearchKeys keeps an existing key's position and
				// switch. Compare at this encrypted-storage boundary so the UI
				// does not have to reveal saved keys just to add another one.
				if plain != "" && seen[plain] {
					continue
				}
				if slot.Key != "" {
					data, err := protect([]byte(slot.Key), false)
					if err != nil {
						return nil, err
					}
					sealed = base64.StdEncoding.EncodeToString(data)
				}
				if sealed == "" {
					return nil, errors.New("搜索 API 密钥已不存在，请重新输入")
				}
				next[ref] = sealed
				slot.Key, slot.HasKey = "", true
				seen[plain] = true
				kept = append(kept, *slot)
				api.HasKey = true
			}
			api.KeyList = &kept
			continue
		}
		sealed := s.searchKeys[api.Vendor]
		if api.Key != "" {
			if len(api.Key) > 16384 {
				return nil, errors.New("搜索 API 密钥过长")
			}
			data, err := protect([]byte(api.Key), false)
			if err != nil {
				return nil, err
			}
			sealed = base64.StdEncoding.EncodeToString(data)
		}
		api.Key = ""
		api.HasKey = sealed != ""
		if sealed != "" {
			next[api.Vendor] = sealed
		}
	}
	return next, nil
}

func (s *Store) runtimeMagpieSettingsLocked(c Configuration) (*MagpieSettings, error) {
	out := c.MagpieSettings.clone()
	out.ModelPrices = map[string]ModelPrice{}
	out.ModelSameAs = map[string]string{}
	for _, p := range c.Providers {
		for id, m := range p.ModelMetadata {
			if m.Price != nil {
				out.ModelPrices[p.ID+"/"+id] = *m.Price
			}
			if m.Same != "" {
				out.ModelSameAs[p.ID+"/"+id] = m.Same
			}
		}
	}
	for i := range out.SearchAPIs {
		if out.SearchAPIs[i].KeyList != nil {
			var enabled []string
			for j := range *out.SearchAPIs[i].KeyList {
				slot := &(*out.SearchAPIs[i].KeyList)[j]
				key, err := unsealKey(s.searchKeys[out.SearchAPIs[i].Vendor+"/"+slot.ID])
				if err != nil {
					return nil, err
				}
				slot.Key = key
				if !slot.Off {
					enabled = append(enabled, key)
				}
			}
			out.SearchAPIs[i].Key = strings.Join(enabled, ",")
			continue
		}
		key, err := unsealKey(s.searchKeys[out.SearchAPIs[i].Vendor])
		if err != nil {
			return nil, err
		}
		out.SearchAPIs[i].Key = key
	}
	return out, nil
}

// MagpieRuntime returns the settings for a private worker request. It must
// never be used as an IPC response because it contains unsealed search keys.
func (s *Store) MagpieRuntime() (*MagpieSettings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runtimeMagpieSettingsLocked(s.configurationLocked())
}

// RevealSearchKey is only called by the settings window's explicit Show/Copy.
func (s *Store) RevealSearchKey(vendor string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.searchKeys[vendor] == "" {
		return "", errors.New("该搜索 API 未设置密钥")
	}
	return unsealKey(s.searchKeys[vendor])
}

// MagpieCatalog reads upstream catalog metadata in an isolated worker. Its
// response has no secrets; the settings it receives are request-local only.
func (t Translator) MagpieCatalog(ctx context.Context, prefs *MagpieSettings, providers ...RuntimeProvider) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	auxiliary, stop, err := t.auxiliaryBridge(ctx, providers, prefs)
	if err != nil {
		return nil, err
	}
	defer stop()
	raw, err := t.invoke(ctx, map[string]any{"operation": "magpie.catalog", "magpieSettings": prefs, "auxiliaryProviders": auxiliary})
	if err != nil {
		return nil, errors.New("Magpie 设置目录读取失败")
	}
	return raw, nil
}
