package ai

import (
	"strings"
	"testing"
)

func TestDisabledModelsHaveUpstreamGlobSemanticsAtHostBoundary(t *testing.T) {
	s := MagpieClientSettings{DisabledModels: []string{"My-Account/Claude-?", "*hidden*"}}
	if !s.ModelDisabled("my-account", "claude-a", "") || s.ModelDisabled("other", "claude-a", "") || !s.ModelDisabled("other", "model", "my-hidden-model") {
		t.Fatal("disabled-model pattern changed")
	}
	c := Configuration{Providers: []Provider{{ID: "my-account", Models: []string{"claude-a"}}}, MagpieSettings: &MagpieSettings{MagpieClientSettings: s}}
	if c.ValidateSelection(Selection{Provider: "my-account", Model: "claude-a"}) == nil {
		t.Fatal("private account ID bypassed disabled list")
	}
}

func TestClientSettingsAndCodexBackgroundConfig(t *testing.T) {
	s := MagpieClientSettings{CheckinOff: []string{"one"}}
	if s.UsesClientRules() {
		t.Fatal("a check-in setting starts routing")
	}
	s.PickedModels = map[string][]string{"codex": {}}
	if !s.UsesClientRules() {
		t.Fatal("an empty picked-only list must hide all models")
	}
	s.CodexAsyncCompact = true
	if snippet := s.CodexAsyncSnippet(); !strings.Contains(snippet, "model_async_compact_threshold_percent = 65") || !strings.Contains(snippet, "remote_compaction = \"v2\"") {
		t.Fatal(snippet)
	}
	s.OTel.Enabled = true
	if s.validate() == nil {
		t.Fatal("export without a destination accepted")
	}
	s.OTel.Endpoint = "https://collector.example/api/public/otel"
	if err := s.validate(); err != nil {
		t.Fatal(err)
	}
	s.OTel.Endpoint = "https://user:secret@collector.example"
	if s.validate() == nil {
		t.Fatal("credential-bearing endpoint accepted")
	}
}
