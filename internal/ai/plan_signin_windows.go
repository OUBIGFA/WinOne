//go:build windows

package ai

import (
	"context"
	"encoding/json"
	"errors"
)

func planRequest(operation string, p Provider) map[string]any {
	return map[string]any{"operation": operation, "providerId": p.ID, "protocol": p.Protocol, "baseUrl": p.BaseURL, "preset": p.Preset, "presetRegion": p.PresetRegion, "providerPreferences": p.ProviderPreferences}
}

// PlanSession reads or removes only the selected provider's platform login.
func (t Translator) PlanSession(ctx context.Context, operation string, p Provider) (json.RawMessage, error) {
	if p.AuthMode != "api-key" || (operation != "plans.get" && operation != "plans.signout") {
		return nil, errors.New("套餐登录操作无效")
	}
	raw, err := t.ForProvider(p.ID).invoke(ctx, planRequest(operation, p))
	var response struct {
		Error string `json:"error"`
	}
	if err != nil || json.Unmarshal(raw, &response) != nil {
		return nil, errors.New("套餐登录状态读取失败")
	}
	if response.Error != "" {
		return nil, errors.New(response.Error)
	}
	return raw, nil
}

// SignInPlan uses the same lifetime owner as account sign-in. Vendor
// credentials remain in the selected WinOne private profile.
func (t Translator) SignInPlan(ctx context.Context, p Provider, controls <-chan string, progress func(json.RawMessage)) error {
	if p.AuthMode != "api-key" {
		return errors.New("套餐供应商无效")
	}
	return t.ForProvider(p.ID).signInInput(ctx, planRequest("plans.signin", p), controls, progress)
}
