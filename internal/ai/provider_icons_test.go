package ai

import "testing"

func TestProviderIconIdentifiesServiceWithoutConfusingModelMaker(t *testing.T) {
	for _, entry := range []struct {
		provider Provider
		want     string
	}{
		{Provider{BaseURL: "https://api.openai.com/v1"}, "openai"},
		{Provider{BaseURL: "https://openrouter.ai/api/v1", Models: []string{"gpt-5"}}, "openrouter-color"},
		{Provider{BaseURL: "https://api.openai.com.example.invalid/v1"}, "service"},
		{Provider{AuthMode: "account", Agent: "claude"}, "claude-color"},
		{Provider{BaseURL: "http://localhost:1234/v1", Name: "My provider", Models: []string{"gpt-5"}}, "service"},
	} {
		if got := providerIcon(entry.provider); got != entry.want {
			t.Fatalf("provider icon %q, want %q", got, entry.want)
		}
	}
}

func TestEverySupportedAccountHasBrandArtwork(t *testing.T) {
	for _, agent := range []string{"chatgpt-api", "codex", "claude", "copilot", "gemini", "antigravity", "cursor", "grok", "devin", "kiro", "zed", "factory", "workbuddy", "workbuddy-ai", "commandcode-plan", "qoder", "qoder-cn", "mimo-app", "zcode"} {
		if got := providerIcon(Provider{AuthMode: "account", Agent: agent}); got == "service" || got == "" {
			t.Errorf("missing account artwork: %s", agent)
		}
	}
}
