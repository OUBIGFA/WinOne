package ai

import (
	"encoding/json"
	"testing"
)

func TestUpdatedRuntimeOptionsAreValidatedAndCopied(t *testing.T) {
	s := &MagpieSettings{TimeZone: "Asia/Shanghai", HideRequestText: true, ClaudeWaysOut: []string{"US", "192.0.2.0/24"}, CodexResetDaysLeast: 1, CodexResetDaysMost: 7}
	if err := s.validate(); err != nil {
		t.Fatal(err)
	}
	c := s.clone()
	if c.TimeZone != s.TimeZone || !c.HideRequestText || c.CodexResetDaysMost != 7 {
		t.Fatal("runtime choice lost")
	}
	c.ClaudeWaysOut[0] = "changed"
	if s.ClaudeWaysOut[0] != "US" {
		t.Fatal("shared mutable egress list")
	}
	for _, bad := range []*MagpieSettings{{TimeZone: "invalid"}, {ClaudeWaysOut: []string{"not-a-region"}}, {CodexResetDaysMost: 366}} {
		if bad.validate() == nil {
			t.Fatal("invalid runtime choice accepted")
		}
	}
}

func TestCodexClientSurvivesRuntimeSnapshot(t *testing.T) {
	var input MagpieSettings
	if err := json.Unmarshal([]byte(`{"codexClient":"desktop"}`), &input); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(input.clone())
	if err != nil {
		t.Fatal(err)
	}
	var saved map[string]any
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	if saved["codexClient"] != "desktop" {
		t.Fatalf("the selected Codex client was lost from runtime settings: %s", raw)
	}
	if saved["noStats"] != true || saved["noUsageStats"] != true {
		t.Fatal("request identity changed the retained recording boundary")
	}
}
