//go:build windows

package ai

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// Test an inline proxy through the original transport. No credential snapshot
// or provider save is required, and unsuccessful checks remain failed checks.
func (t Translator) ProbeProxy(ctx context.Context, choice string) (json.RawMessage, error) {
	preferences := ProviderPreferences{Proxy: choice}
	if err := preferences.validate(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	raw, err := t.invoke(ctx, map[string]any{"operation": "proxies.test", "providerPreferences": preferences})
	var result struct {
		OK *bool `json:"ok"`
	}
	if err != nil || json.Unmarshal(raw, &result) != nil || result.OK == nil {
		return nil, errors.New("代理检查未能完成，请检查地址和代理服务")
	}
	return raw, nil
}
