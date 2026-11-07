package ai

import (
	"encoding/json"
	"testing"
)

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
