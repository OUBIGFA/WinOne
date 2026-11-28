package ai

import (
	"encoding/json"
	"testing"
)

func TestUpdatedProviderOptionsSurviveConfiguration(t *testing.T) {
	var p ProviderPreferences
	if err := json.Unmarshal([]byte(`{"addresses":["https://backup.invalid"],"agentAccounts":{"codex":"slot"},"routing":"pace-drain"}`), &p); err != nil {
		t.Fatal(err)
	}
	if err := p.validate(); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(p.clone())
	var got map[string]any
	_ = json.Unmarshal(raw, &got)
	if got["addresses"] == nil || got["agentAccounts"] == nil {
		t.Fatal("new upstream choices were dropped", string(raw))
	}
}
