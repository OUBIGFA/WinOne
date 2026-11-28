package ai

import (
	"math"
	"reflect"
	"testing"
)

func TestConfigurationSnapshotOwnsLegacyKeyPolicy(t *testing.T) {
	original := Configuration{Providers: []Provider{{KeyPolicy: &KeyPolicy{Strategy: "ordered", MaxAttempts: 2}}}}
	copy := cloneConfiguration(original)
	copy.Providers[0].KeyPolicy.MaxAttempts = 99
	if original.Providers[0].KeyPolicy.MaxAttempts != 2 {
		t.Fatal("editing a snapshot changed the original key policy")
	}
}

func TestProviderCopyPreservesValuesForValidation(t *testing.T) {
	p := ProviderPreferences{Routing: "rotate", PriceRate: math.NaN()}
	copy := p.clone()
	if copy.Routing != p.Routing || !math.IsNaN(copy.PriceRate) || copy.validate() == nil {
		t.Fatal("copy discarded an invalid value before validation could reject it")
	}
}

func TestProviderCopyOwnsNestedOptions(t *testing.T) {
	p := ProviderPreferences{
		Addresses:            []string{"https://relay.invalid"},
		AgentAccounts:        map[string]string{"magpie": "primary"},
		KeyBalanceAuth:       map[string]KeyBalanceAuth{"primary": {Token: new("fixture"), User: "42"}},
		AccountModels:        map[string][]string{"user": {"model"}},
		AccountWindowCaps:    map[string]map[string]int{"user": {"week": 50}},
		AccountWindowAmounts: map[string]map[string]float64{"user": {"day": 10}},
		KeyOwnBalances:       map[string]OwnBalance{"primary": {CNY: new(20.0), Refill: &BalanceRefill{Every: "week", Day: 1}}},
		RateWindows:          []RateWindow{{Days: []int{1}, Start: "00:00", End: "24:00", Rate: 1}},
		Resolve:              map[string][]string{"relay.invalid": {"127.0.0.1"}},
		ExtraBody:            map[string]any{"options": []any{map[string]any{"enabled": true}}},
	}
	copy := p.clone()
	if !reflect.DeepEqual(p, copy) {
		t.Fatal("copy changed the provider options")
	}
	copy.Addresses[0] = "edited"
	copy.AgentAccounts["magpie"] = "edited"
	*copy.KeyBalanceAuth["primary"].Token = "edited"
	copy.AccountModels["user"][0] = "edited"
	copy.AccountWindowCaps["user"]["week"] = 1
	copy.AccountWindowAmounts["user"]["day"] = 1
	*copy.KeyOwnBalances["primary"].CNY = 1
	copy.KeyOwnBalances["primary"].Refill.Day = 2
	copy.RateWindows[0].Days[0] = 2
	copy.Resolve["relay.invalid"][0] = "edited"
	copy.ExtraBody["options"].([]any)[0].(map[string]any)["enabled"] = false
	if p.Addresses[0] != "https://relay.invalid" || p.AgentAccounts["magpie"] != "primary" || *p.KeyBalanceAuth["primary"].Token != "fixture" ||
		p.AccountModels["user"][0] != "model" || p.AccountWindowCaps["user"]["week"] != 50 || p.AccountWindowAmounts["user"]["day"] != 10 ||
		*p.KeyOwnBalances["primary"].CNY != 20 || p.KeyOwnBalances["primary"].Refill.Day != 1 || p.RateWindows[0].Days[0] != 1 ||
		p.Resolve["relay.invalid"][0] != "127.0.0.1" || p.ExtraBody["options"].([]any)[0].(map[string]any)["enabled"] != true {
		t.Fatal("editing the copy changed a nested provider option")
	}
}
