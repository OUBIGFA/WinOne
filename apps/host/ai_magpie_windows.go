//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"winone/internal/ai"
	"winone/internal/configjson"
	"winone/internal/host"
)

func (s *aiService) magpieStore() (*ai.Store, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.store == nil {
		var err error
		s.store, err = ai.Open(s.storePath)
		if err != nil {
			return nil, errors.New("AI 设置无法读取，请恢复配置文件后重试：" + err.Error())
		}
	}
	return s.store, nil
}

func (s *aiService) magpieTranslator() (ai.Translator, error) {
	store, err := s.magpieStore()
	if err != nil {
		return ai.Translator{}, err
	}
	prefs, err := store.MagpieRuntime()
	if err != nil {
		return ai.Translator{}, err
	}
	return s.translator.WithMagpieSettings(prefs), nil
}

// Preferences and credentials remain in the shared WinOne store. Catalog reads
// receive a private runtime snapshot; only explicit Show/Copy reveals a key.
func (s *aiService) handleMagpie(r host.Request) host.Response {
	fail := func(err error) host.Response {
		return host.Response{ID: r.ID, Error: &host.RPCError{Code: "ai", Message: err.Error()}}
	}
	store, err := s.magpieStore()
	if err != nil {
		return fail(err)
	}
	switch r.Method {
	case "magpie.catalog":
		var params struct{}
		if err = configjson.DecodeObject(r.Params, &params); err != nil {
			return fail(err)
		}
		prefs, err := store.MagpieRuntime()
		if err != nil {
			return fail(err)
		}
		providers, err := store.MagpieProviders()
		if err != nil {
			return fail(err)
		}
		ctx, cancel := context.WithTimeout(s.ctx, 25*time.Second)
		defer cancel()
		raw, err := s.translator.MagpieCatalog(ctx, prefs, providers...)
		if err != nil {
			return fail(err)
		}
		// MCP URLs use the Host listener, never the short worker's private port.
		if s.gateway != nil {
			var catalog map[string]json.RawMessage
			if err := json.Unmarshal(raw, &catalog); err != nil {
				return fail(err)
			}
			catalog["gateway"], _ = json.Marshal(strings.TrimSuffix(s.gateway.Status().BaseURL, "/v1"))
			raw, err = json.Marshal(catalog)
			if err != nil {
				return fail(err)
			}
		}
		return host.Response{ID: r.ID, Result: raw}
	case "magpie.usage-aliases":
		var params struct {
			Aliases map[string]string  `json:"aliases"`
			Edit    *ai.UsageAliasEdit `json:"edit,omitempty"`
		}
		if err = configjson.DecodeObject(r.Params, &params); err != nil {
			return fail(err)
		}
		aliases, err := s.translator.UsageAliases(s.ctx, params.Aliases, params.Edit)
		if err != nil {
			return fail(err)
		}
		raw, err := json.Marshal(map[string]any{"usageAliases": aliases})
		if err != nil {
			return fail(err)
		}
		return host.Response{ID: r.ID, Result: raw}
	case "magpie.search-key":
		var params struct {
			Vendor string `json:"vendor"`
		}
		if err = configjson.DecodeObject(r.Params, &params); err != nil {
			return fail(err)
		}
		key, err := store.RevealSearchKey(params.Vendor)
		if err != nil {
			return fail(err)
		}
		raw, err := json.Marshal(map[string]string{"key": key})
		if err != nil {
			return fail(err)
		}
		return host.Response{ID: r.ID, Result: raw}
	default:
		return fail(errors.New("不支持的 Magpie 设置操作"))
	}
}
