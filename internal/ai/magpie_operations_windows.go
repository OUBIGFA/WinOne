//go:build windows

package ai

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"
)

// WithMagpieSettings carries private settings through account/usage operations
// as well as inference. It does not mutate the original Translator.
func (t Translator) WithMagpieSettings(prefs *MagpieSettings) Translator {
	t.RuntimeSettings = prefs.clone()
	return t
}
func (t Translator) withRuntimeSettings(input any) any {
	if t.RuntimeSettings == nil {
		return input
	}
	data, err := json.Marshal(input)
	if err != nil {
		return input
	}
	var out map[string]any
	if json.Unmarshal(data, &out) != nil || out == nil {
		return input
	}
	if current, ok := out["magpieSettings"]; !ok || current == nil {
		out["magpieSettings"] = t.RuntimeSettings
	}
	return out
}

// TestModels retains Magpie's per-protocol result and optional client view.
// It accepts a draft provider already checked by the Host's key/draft path.
func (t Translator) TestModels(ctx context.Context, p Provider, key string, models []string, client string, prefs *MagpieSettings) (json.RawMessage, error) {
	if len(models) > MaxProviderModels || !slices.Contains([]string{"", "codex", "claude-code"}, client) {
		return nil, errors.New("模型测试参数无效")
	}
	for _, id := range models {
		if strings.TrimSpace(id) == "" || len(id) > 256 {
			return nil, errors.New("模型测试标识无效")
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	input := map[string]any{"operation": "models.test", "protocol": p.Protocol, "baseUrl": p.BaseURL, "key": key, "headers": p.Headers, "preset": p.Preset, "presetRegion": p.PresetRegion, "testModels": models, "testClient": client, "limitRestOff": p.LimitRestOff, "additionalTools": p.AdditionalTools, "providerPreferences": p.ProviderPreferences, "modelPreferences": p.ModelMetadata, "magpieSettings": prefs}
	input["keyID"], input["keyOptions"] = credentialRequestOptions(ctx)
	if p.AuthMode == "account" {
		t.AccountBackend = p.AgentBackend
		input["agent"] = p.Agent
		input["agentBackend"] = p.AgentBackend
	}
	raw, err := t.ForProvider(p.ID).invoke(ctx, input)
	if err != nil {
		return nil, errors.New("模型连通性测试未能完成")
	}
	var result struct {
		Results json.RawMessage `json:"results"`
		Error   string          `json:"error"`
	}
	if json.Unmarshal(raw, &result) != nil || len(result.Results) == 0 || result.Error != "" {
		return nil, errors.New("模型连通性测试返回无效响应")
	}
	return raw, nil
}

// KeyBalances reads a saved immutable key snapshot. It deliberately avoids
// WithKeys, whose inference rotation and backoff must not change on a UI read.
func (t Translator) KeyBalances(ctx context.Context, p Provider, plan KeyPlan) (json.RawMessage, error) {
	if p.Off {
		return json.RawMessage(`{"quotas":[]}`), nil
	}
	if p.AuthMode != "api-key" || plan.provider != p.ID {
		return nil, errors.New("余额供应商无效")
	}
	keys := make([]auxiliaryKey, 0, len(plan.keys))
	for _, credential := range plan.keys {
		key, err := unsealKey(credential.sealed)
		if err != nil {
			return nil, errors.New("余额读取失败，请检查已保存的密钥")
		}
		// Balance results retain the slot ID used by the UI while the vendor
		// request uses the same protocol/plan as inference for this credential.
		keys = append(keys, auxiliaryKey{Name: credential.id, Key: key, KeyOptions: credential.options})
	}
	if len(keys) == 0 {
		return json.RawMessage(`{"quotas":[]}`), nil
	}
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	raw, err := t.ForProvider(p.ID).invoke(ctx, map[string]any{"operation": "accounts.key-balance", "providerId": p.ID, "protocol": p.Protocol, "baseUrl": p.BaseURL, "headers": p.Headers, "preset": p.Preset, "presetRegion": p.PresetRegion, "limitRestOff": p.LimitRestOff, "providerPreferences": p.ProviderPreferences, "balanceKeys": keys})
	if err != nil {
		return nil, errors.New("余额读取失败，请刷新重试")
	}
	var result struct {
		Quotas json.RawMessage `json:"quotas"`
		Error  string          `json:"error"`
	}
	if json.Unmarshal(raw, &result) != nil || len(result.Quotas) == 0 || result.Error != "" {
		return nil, errors.New("余额读取失败，服务返回无效结果")
	}
	return raw, nil
}
