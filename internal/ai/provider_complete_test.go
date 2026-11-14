package ai

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestCompleteProviderOptionsSurviveSaveDocument(t *testing.T) {
	// These are public upstream options, not secrets or transient observations.
	raw := []byte(`{"endpoints":{"chat":"https://chat.invalid/v1","gemini":"https://gemini.invalid/v1beta","decide":"https://router.invalid"},"fallback":["backup/model"],"quotaRest":60,"quotaResetAt":"00:00+08:00","restCap":-1,"nativeCompaction":true,"pinUpstream":true,"modelsURL":"https://models.invalid/list","listOff":true,"refreshEvery":"6h","autoInclude":["new-*"],"unlisted":true,"hiddenDrawers":["image"],"hiddenVideomakers":["video"],"balanceURL":"https://balance.invalid","balancePath":"data.amount","balanceMethod":"POST","zhipuTeam":{"org":"org","project":"project"},"vertex":{"project":"cloud-project","location":"global","credentials":"fixture.json","impersonate":"service@example.test"},"family":"custom","catalog":"openai","keyOwnBalances":{"primary":{"cny":128,"at":"2026-11-08T00:00:00Z","refill":{"every":"week","day":1}}}}`)
	// The new connection and account controls must survive a save/reopen,
	// rather than being accepted and silently dropped as unknown JSON fields.
	raw = append(raw[:len(raw)-1], []byte(`,"noStandIn":true,"upstreamProviders":true,"socket":"auto","clientCert":"C:/fixture/client.pem","clientKey":"C:/fixture/client-key.pem","serverCA":"C:/fixture/ca.pem","serverPin":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","resolve":{"relay.invalid":["127.0.0.1","dns:gateway.invalid"]},"accountWindowAmounts":{"fixture@example.test":{"5h":800.5}},"keyPriceRates":{"primary":0.5},"noBalanceProbe":true}`)...)
	var prefs ProviderPreferences
	if err := json.Unmarshal(raw, &prefs); err != nil {
		t.Fatal(err)
	}
	if err := prefs.validate(); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(prefs.clone())
	if err != nil {
		t.Fatal(err)
	}
	var want, got map[string]any
	json.Unmarshal(raw, &want)
	json.Unmarshal(encoded, &got)
	for key, value := range want {
		if !reflect.DeepEqual(value, got[key]) {
			t.Errorf("option %s lost or changed: %v", key, got[key])
		}
	}
}

func TestOwnBalanceRejectsInvalidCurrencyAndRefill(t *testing.T) {
	for _, raw := range []string{
		`{"cny":-1,"at":"2026-11-08T00:00:00Z"}`,
		`{"usd":10,"at":"2026-11-08T00:00:00Z","refill":{"every":"week","day":8}}`,
		`{"usd":10,"at":"2026-11-08T00:00:00Z","refill":{"every":"month","day":0}}`,
	} {
		var balance OwnBalance
		json.Unmarshal([]byte(raw), &balance)
		if (ProviderPreferences{KeyOwnBalances: map[string]OwnBalance{"primary": balance}}).validate() == nil {
			t.Errorf("accepted %s", raw)
		}
	}
}
