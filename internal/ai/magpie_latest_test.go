package ai

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRoutingDefaultsAndOwnBalancesSurvivePublicRoundTrip(t *testing.T) {
	var settings MagpieSettings
	json.Unmarshal([]byte(`{"groupSink":true,"groupFirstToken":12,"groupPatience":-1}`), &settings)
	data, _ := json.Marshal(settings)
	for _, field := range []string{`"groupSink":true`, `"groupFirstToken":12`, `"groupPatience":-1`} {
		if !strings.Contains(string(data), field) {
			t.Errorf("lost %s", field)
		}
	}
	var prefs ProviderPreferences
	json.Unmarshal([]byte(`{"magpieUserAgent":true,"keyOwnBalances":{"primary":{"usd":25,"at":"2026-11-08T00:00:00Z"}}}`), &prefs)
	data, _ = json.Marshal(prefs)
	for _, field := range []string{`"magpieUserAgent":true`, `"keyOwnBalances"`, `"usd":25`} {
		if !strings.Contains(string(data), field) {
			t.Errorf("lost %s", field)
		}
	}
}

func TestLatestMagpieOptionsRetainExactSemantics(t *testing.T) {
	var s MagpieSettings
	if err := json.Unmarshal([]byte(`{"searchCount":20,"retryAfterCap":60,"codexResetLead":3,"codexSubagentModel":"lead","chinaMirror":true,"searchAPIs":[{"vendor":"tavily","key":"fixture","mix":true}]}`), &s); err != nil {
		t.Fatal(err)
	}
	if err := s.validate(); err != nil {
		t.Fatal(err)
	}
	if s.SearchCount != 20 || *s.RetryAfterCap != 60 || s.CodexResetLead != 3 || s.CodexSubagentModel != "lead" || !s.ChinaMirror || !s.SearchAPIs[0].Mix {
		t.Fatalf("lost new upstream options: %+v", s)
	}
	if len(s.AgentCompacts) != 0 {
		t.Fatal("unexpected compaction overrides")
	}
	s.AgentCompacts = map[string]AgentCompact{"codex": {At: 250000}, "claude": {Full: true}}
	s.RouterAllTools = true
	if err := s.validate(); err != nil {
		t.Fatal(err)
	}
	s.SearchCount = 21
	if s.validate() == nil {
		t.Fatal("out of range search count accepted")
	}
	s.SearchCount = 0
	s.CodexResetLead = 2
	if s.validate() == nil {
		t.Fatal("invalid early reset accepted")
	}
}
