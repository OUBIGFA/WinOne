//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"
	"winone/internal/ai"
	"winone/internal/configjson"
	"winone/internal/host"
)

// Connectivity checks use the same draft connection and credential snapshot as
// model discovery. They never commit settings, and return Magpie's individual
// protocol results rather than turning an HTTP failure into a successful test.
func (s *aiService) handleModelTest(r host.Request) host.Response {
	fail := func(err error) host.Response {
		return host.Response{ID: r.ID, Error: &host.RPCError{Code: "ai", Message: err.Error()}}
	}
	var params struct {
		Provider   string                      `json:"provider"`
		Draft      *ai.AccountDraft            `json:"draft,omitempty"`
		Connection *ai.ModelConnectionDraft    `json:"connection,omitempty"`
		KeyUpdates map[string]*string          `json:"keyUpdates,omitempty"`
		Models     []string                    `json:"models"`
		As         string                      `json:"as,omitempty"`
		Metadata   map[string]ai.ModelMetadata `json:"metadata,omitempty"`
	}
	if err := configjson.DecodeObject(r.Params, &params); err != nil {
		return fail(err)
	}
	if params.Connection != nil && params.Draft != nil || params.Connection == nil && len(params.KeyUpdates) > 0 || !slices.Contains([]string{"", "codex", "claude-code"}, params.As) || len(params.Models) > ai.MaxProviderModels || len(params.Metadata) > ai.MaxProviderModels {
		return fail(errors.New("模型测试参数无效"))
	}
	validID := func(id string) bool {
		return strings.TrimSpace(id) != "" && len(id) <= 256 && !strings.ContainsRune(id, '\x00')
	}
	for _, id := range params.Models {
		if !validID(id) {
			return fail(errors.New("模型测试标识无效"))
		}
	}
	for id, m := range params.Metadata {
		if !validID(id) {
			return fail(errors.New("模型设置标识无效"))
		}
		if err := ai.ValidateModelMetadata(m); err != nil {
			return fail(err)
		}
	}
	store, err := s.magpieStore()
	if err != nil {
		return fail(err)
	}
	var provider ai.Provider
	var keys ai.KeyPlan
	if params.Connection != nil {
		provider, err = params.Connection.Provider(params.Provider)
	} else if params.Draft != nil {
		provider, err = s.accountProvider(params.Provider, params.Draft.Agent, params.Draft)
	} else {
		provider, keys, err = store.ProviderKeysForModels(params.Provider)
	}
	if err != nil {
		return fail(err)
	}
	prefs, err := store.MagpieRuntime()
	if err != nil {
		return fail(err)
	}
	ctx, cancel := context.WithTimeout(s.ctx, 95*time.Second)
	defer cancel()
	gate := s.accountGateFor(provider.ID)
	select {
	case gate <- struct{}{}:
		defer func() { <-gate }()
	case <-ctx.Done():
		return fail(errors.New("等待供应商操作完成超时，请稍后重试"))
	}
	var raw json.RawMessage
	run := func(ctx context.Context, p ai.Provider, key string) error {
		// Metadata is the complete unsaved map from the model settings form.
		if params.Metadata != nil {
			p.ModelMetadata = params.Metadata
		}
		var err error
		raw, err = s.translator.TestModels(ctx, p, key, params.Models, params.As, prefs)
		return err
	}
	if params.Connection != nil {
		err = store.WithModelDraft(ctx, params.Provider, *params.Connection, params.KeyUpdates, run)
	} else if params.Draft != nil {
		err = run(ctx, provider, "")
	} else {
		err = store.WithKeys(ctx, keys, func(ctx context.Context, key string) (bool, error) { return false, run(ctx, provider, key) })
	}
	if err != nil {
		return fail(err)
	}
	return host.Response{ID: r.ID, Result: raw}
}
