package ai

import "testing"

func TestClearedFeatureDoesNotFallBackToAnotherProvider(t *testing.T) {
	c := Configuration{Tasks: map[string]Selection{"default": {Provider: "personal", Model: "fast"}, "translate": {}}}
	if c.Selected("translate").Provider != "" {
		t.Fatal("explicitly cleared feature silently changed providers")
	}
	if c.Selected("chat").Provider != "personal" {
		t.Fatal("legacy default was not inherited")
	}
}

func TestModelCatalogKeepsDisabledAndManualModelsOutOfActiveChoices(t *testing.T) {
	c := Configuration{Engine: "llm", OCREngine: "local", Target: "auto", Providers: []Provider{{ID: "one", Name: "Service", AuthMode: "api-key", Protocol: "chat", BaseURL: "https://example.invalid/v1", Models: []string{"manual"}, AvailableModels: []string{"remote", "disabled"}}}, Tasks: map[string]Selection{"translate": {Provider: "one", Model: "disabled"}}}
	if err := validateConfiguration(c); err != nil {
		t.Fatal("disabled prior binding must not block provider settings", err)
	}
	got := cloneConfiguration(c)
	if len(got.Providers[0].Models) != 1 || got.Providers[0].Models[0] != "manual" {
		t.Fatal("catalog enabled models without a user selection")
	}
	if len(got.Providers[0].AvailableModels) != 3 {
		t.Fatal("manual model omitted from the catalog")
	}
	if got.ValidateSelection(Selection{Provider: "one", Model: "disabled"}) == nil {
		t.Fatal("disabled model could be invoked")
	}
	if err := got.ValidateSelection(Selection{Provider: "one", Model: "manual"}); err != nil {
		t.Fatal(err)
	}
	got.Providers[0].AvailableModels[0] = "changed"
	if c.Providers[0].AvailableModels[0] != "remote" {
		t.Fatal("catalog snapshot mutated saved data")
	}
}

func TestProviderCanBeSavedBeforeAFeatureChoosesItsModel(t *testing.T) {
	c := Configuration{Engine: "llm", OCREngine: "ai", Target: "auto", Providers: []Provider{{ID: "one", Name: "Service", AuthMode: "api-key", Protocol: "chat", BaseURL: "https://example.invalid/v1"}}}
	if err := validateConfiguration(c); err != nil {
		t.Fatal("provider management required a separate task assignment", err)
	}
	if c.ValidateSelection(c.Selected("translate")) == nil {
		t.Fatal("unconfigured invocation was accepted")
	}
}
