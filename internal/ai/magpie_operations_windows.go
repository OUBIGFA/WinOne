//go:build windows

package ai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"
)

// MagpieGateway handles APIs whose provider is selected by the upstream global
// settings, such as search MCP and image generation with no explicit model.
func (t Translator) MagpieGateway(ctx context.Context, prefs *MagpieSettings, providers []RuntimeProvider, path string, body json.RawMessage, headers http.Header, w http.ResponseWriter) (bool, error) {
	return t.GatewayRequest(ctx, Preferences{GlobalGateway: true, MagpieSettings: prefs, Auxiliaries: providers}, "", path, body, headers, w)
}

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
	input := map[string]any{"operation": "models.test", "protocol": p.Protocol, "baseUrl": p.BaseURL, "key": key, "headers": p.Headers, "preset": p.Preset, "presetRegion": p.PresetRegion, "testModels": models, "testClient": client, "additionalTools": p.AdditionalTools, "modelPreferences": p.ModelMetadata, "magpieSettings": prefs}
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
	if p.AuthMode != "api-key" || plan.provider != p.ID {
		return nil, errors.New("余额供应商无效")
	}
	keys := make([]auxiliaryKey, 0, len(plan.keys))
	for _, credential := range plan.keys {
		key, err := unsealKey(credential.sealed)
		if err != nil {
			return nil, errors.New("余额读取失败，请检查已保存的密钥")
		}
		keys = append(keys, auxiliaryKey{Name: credential.id, Key: key})
	}
	if len(keys) == 0 {
		return json.RawMessage(`{"quotas":[]}`), nil
	}
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	raw, err := t.ForProvider(p.ID).invoke(ctx, map[string]any{"operation": "accounts.key-balance", "providerId": p.ID, "protocol": p.Protocol, "baseUrl": p.BaseURL, "headers": p.Headers, "preset": p.Preset, "presetRegion": p.PresetRegion, "balanceKeys": keys})
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

// MaintenanceNeeded leaves the default installation with no quota worker.
// Enabling a schedule/alert or allowance tray selection opts into the reads.
func (s *MagpieSettings) MaintenanceNeeded() bool {
	if s == nil || s.NoAccountDiscovery {
		return false
	}
	if s.UsageAlert > 0 || s.BalanceAlert > 0 || s.ResetReminder > 0 || len(s.TrayUsages) > 0 || s.CodexWarmup != "" || s.ClaudeWarmup != "" || s.CodexWarmAt != "" || s.ClaudeWarmAt != "" || len(s.CodexWarmAts) > 0 || len(s.ClaudeWarmAts) > 0 || len(s.CodexWarmAtOf) > 0 || len(s.CodexAutoReset) > 0 || s.WorkBuddyCheckin || s.TraeCheckin || s.MiniMaxCheckin || s.QoderCheckin {
		return true
	}
	for _, on := range s.PluginCheckins {
		if on {
			return true
		}
	}
	return false
}

// Maintain is one bounded tick, scheduled by the Host. Vendor quota readers
// obey quotaReads; explicit user refresh remains Asked(ctx) in accounts.quota.
func (t Translator) Maintain(ctx context.Context, p Provider, prefs *MagpieSettings) (json.RawMessage, error) {
	t.AccountBackend = p.AgentBackend
	if p.AuthMode != "account" || !prefs.MaintenanceNeeded() {
		return json.RawMessage(`{"quotas":[],"alerts":[],"checkins":[],"errors":[]}`), nil
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	raw, err := t.ForProvider(p.ID).invoke(ctx, map[string]any{"operation": "magpie.maintain", "providerId": p.ID, "agent": p.Agent, "agentBackend": p.AgentBackend, "magpieSettings": prefs})
	if err != nil {
		return nil, errors.New("账号额度维护失败")
	}
	return raw, nil
}
