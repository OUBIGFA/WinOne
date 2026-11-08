package main

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/settings"
	"github.com/yetone/magpie/internal/usage"
)

func TestSavedOwnBalanceUsesActualKeyAndSharedLedger(t *testing.T) {
	t.Setenv("WINONE_AI_USAGE_PATH", filepath.Join(t.TempDir(), "usage.jsonl"))
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	provider.ForgetBalances()
	t.Cleanup(func() {
		usage.WinOnePath = ""
		usage.WinOneRecord = nil
		provider.WinOneProviders = nil
		settings.WinOneSettings.Store(nil)
		provider.ForgetBalances()
	})
	settings.WinOneUse(settings.Settings{NoStats: true, NoUsageStats: true, ModelPrices: map[string]settings.ModelPrice{
		"*/own-balance-fixture": {Input: new(2.0), Output: new(0.0), CacheRead: new(0.0), CacheWrite: new(0.0)},
	}})
	at := time.Now().Add(-time.Minute)
	if err := configureUsage(request{ProviderID: "saved-provider"}); err != nil {
		t.Fatal(err)
	}
	usage.Append(usage.Record{Time: time.Now(), ProviderKeyID: provider.KeyID("fixture-secret"), ProviderKeyName: "primary", Model: "own-balance-fixture", Input: 1000000, Status: 200})
	usage.Append(usage.Record{Time: time.Now(), ProviderKeyID: provider.KeyID("other-secret"), Model: "own-balance-fixture", Input: 1000000, Status: 200})
	var out bytes.Buffer
	err := readKeyBalances(context.Background(), request{ProviderID: "saved-provider", Protocol: "chat", BaseURL: "https://api.openai.com/v1",
		BalanceKeys:         []provider.KeyAccount{{Name: "primary", Key: "fixture-secret"}},
		ProviderPreferences: &ProviderPreferences{KeyOwnBalances: map[string]OwnBalance{"primary": {USD: 20, At: at}}},
	}, &out)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Quotas []provider.SubscriptionQuota `json:"quotas"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Quotas) != 1 || result.Quotas[0].Estimate == nil || result.Quotas[0].Estimate.Spent != 2 || result.Quotas[0].Balance != "≈$18.00" {
		t.Fatalf("own balance did not subtract this key's usage: %s", out.String())
	}
	if strings.Contains(out.String(), "fixture-secret") {
		t.Fatal("key exposed")
	}
}
