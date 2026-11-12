package main

import (
	"encoding/json"
	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/settings"
	"testing"
	"time"
)

// Old full-version switches are data only and cannot revive removed services.
func TestRuntimeSettingsIgnoreRemovedServices(t *testing.T) {
	old := settings.WinOneSettings.Load()
	defer settings.WinOneSettings.Store(old)
	in := request{MagpieSettings: json.RawMessage(`{"claudePrintMode":true,"redact":true,"redactModel":true,"builtinPreset":"all","builtinPlugins":{"routing":{"enabled":true}},"requestArchiveLocal":true,"otel":{"enabled":true},"noStats":false}`)}
	if err := applyRuntimeSettings(in); err != nil {
		t.Fatal(err)
	}
	s := settings.Load()
	if !s.ClaudePrintMode || !s.NoWSLAgents || !s.NoStats || !s.NoUsageStats || !s.RedactModelOff || s.Redact || s.RedactModel || s.RequestArchiveLocal || s.OTel.Enabled {
		t.Fatal("removed runtime settings became active")
	}
}

func TestWorkerImageDeadlinePreservesBothUpstreamPhases(t *testing.T) {
	old := settings.WinOneSettings.Load()
	defer settings.WinOneSettings.Store(old)
	for _, minutes := range []int{0, 1, 30, 60} {
		settings.WinOneUse(settings.Settings{ImageTimeout: minutes})
		want := 2*settings.Load().ImageWait() + time.Minute
		for _, operation := range []string{"gateway"} {
			for _, path := range []string{"/v1/images/generations", "/v1/images/edits"} {
				if got := workerRequestWait(request{Operation: operation, GatewayPath: path}); got != want {
					t.Fatalf("%s %s: %v, want %v", operation, path, got, want)
				}
			}
			if workerRequestWait(request{Operation: operation, GatewayPath: "/v1/chat/completions"}) != 10*time.Minute {
				t.Fatal("image timeout changed a normal protocol deadline")
			}
		}
	}
	if workerRequestWait(request{Operation: "generate"}) != 90*time.Second {
		t.Fatal("image timeout changed the short result task deadline")
	}
}

func TestAdditionalToolsAndNativeSearchModelMetadata(t *testing.T) {
	old := settings.WinOneSettings.Load()
	defer settings.WinOneSettings.Store(old)
	defer func() { provider.WinOneProviders = nil; provider.WinOneResetSearchModels() }()
	p, err := configuredProvider(request{Protocol: "responses", BaseURL: "https://fixture.invalid/v1", AdditionalTools: true})
	if err != nil || !p.AdditionalTools {
		t.Fatal("provider additional_tools not preserved", err)
	}
	if err := installAuxiliaryProviders(request{AuxiliaryProviders: []auxiliaryProvider{{Provider: provider.Provider{ID: "fixture", Name: "Fixture", Chat: "http://127.0.0.1:1/v1", Models: []string{"search-model", "plain-model"}}, Metadata: map[string]modelMetadata{"search-model": {Searches: true}}}}}); err != nil {
		t.Fatal(err)
	}
	models := provider.WinOneProviders[0].Available()
	found := false
	for _, m := range models {
		if m.ID == "search-model" {
			found = m.WebSearch == "native"
		}
		if m.ID == "plain-model" && m.WebSearch == "native" {
			t.Fatal("native search leaked to another model")
		}
	}
	if !found {
		t.Fatal("auxiliary model lost native search capability")
	}
}

// Validate the upstream lookup, not merely the JSON field direction.
func TestModelAliasKeepsWireAndCompaction(t *testing.T) {
	old := settings.WinOneSettings.Load()
	defer settings.WinOneSettings.Store(old)
	settings.WinOneUse(settings.Settings{})
	p := provider.Provider{ID: "fixture", Models: []string{"vendor-model"}}
	if err := installModelPreferences(p.ID, "vendor-model", modelMetadata{Alias: "my-model", ContextWindow: 128000, CompactionPercent: 72.5}); err != nil {
		t.Fatal(err)
	}
	if got := p.OwnModelID("my-model"); got != "vendor-model" {
		t.Fatalf("wire model: %s", got)
	}
	if got := settings.Load().ModelCompacts["fixture/my-model"]; got != 92800 {
		t.Fatalf("compact tokens: %d", got)
	}
	if err := installModelPreferences(p.ID, "vendor-model", modelMetadata{}); err != nil {
		t.Fatal(err)
	}
	if got := p.OwnModelID("my-model"); got != "my-model" {
		t.Fatalf("cleared alias still active: %s", got)
	}
	if _, exists := settings.Load().ModelCompacts["fixture/my-model"]; exists {
		t.Fatal("cleared alias retained its model overrides")
	}
}

func TestAuxiliaryModelNamesAndAliasContext(t *testing.T) {
	old, providers := settings.WinOneSettings.Load(), provider.WinOneProviders
	defer func() { settings.WinOneSettings.Store(old); provider.WinOneProviders = providers }()
	settings.WinOneUse(settings.Settings{})
	provider.WinOneProviders = nil
	metadata := map[string]modelMetadata{"original": {Alias: "friendly", ContextWindow: 64000}}
	if err := installAuxiliaryProviders(request{AuxiliaryProviders: []auxiliaryProvider{{Provider: provider.Provider{ID: "fixture", Models: []string{"original"}}, Metadata: metadata, ModelNames: map[string]string{"original": "My model"}}}}); err != nil {
		t.Fatal(err)
	}
	if got := provider.WinOneProviders[0].ContextOf("friendly"); got != 64000 {
		t.Fatalf("alias context = %d", got)
	}
	if got := settings.Load().ModelNames["fixture/friendly"]; got != "My model" {
		t.Fatalf("alias name = %q", got)
	}
	if err := installModelNames("fixture", nil, metadata); err != nil {
		t.Fatal(err)
	}
	if len(settings.Load().ModelNames) != 0 {
		t.Fatal("cleared names remained active")
	}
}
