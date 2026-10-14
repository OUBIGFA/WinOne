//go:build windows

package ai

import (
	"fmt"
	"path/filepath"
	"testing"
)

func TestProviderHeadersPersistAndBelongToTheRequestSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ai.json")
	store, _ := Open(path)
	configuration := store.Configuration()
	configuration.Providers = []Provider{{ID: "custom", Name: "Custom", AuthMode: "api-key", Protocol: "chat", BaseURL: "https://example.invalid/v1", Models: []string{"model"}, Headers: map[string]string{"X-Organization": "fixture", "Authorization": "Fixture token"}}}
	saved, err := store.SaveConfiguration(configuration, nil)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	prefs, _, err := reopened.ForTaskKeys("chat", &Selection{Provider: "custom", Model: "model"})
	if err != nil {
		t.Fatal(err)
	}
	if prefs.Headers["X-Organization"] != "fixture" {
		t.Fatal("custom header lost")
	}
	prefs.Headers["X-Organization"] = "mutated"
	if reopened.Configuration().Providers[0].Headers["X-Organization"] != "fixture" {
		t.Fatal("request mutated saved headers")
	}
	saved.Providers[0].Headers = nil
	saved.Providers[0].Name = "Renamed"
	retained, err := store.SaveConfiguration(saved, nil)
	if err != nil || retained.Providers[0].Headers["Authorization"] != "Fixture token" {
		t.Fatal("older caller cleared headers", err)
	}
	retained.Providers[0].Headers = map[string]string{}
	cleared, err := store.SaveConfiguration(retained, nil)
	if err != nil || len(cleared.Providers[0].Headers) != 0 {
		t.Fatal("explicit clear failed", err)
	}
}
func TestSelectAllCanSaveMoreThan128DiscoveredModels(t *testing.T) {
	store, _ := Open(filepath.Join(t.TempDir(), "ai.json"))
	configuration := store.Configuration()
	models := []string{}
	for i := 0; i < 180; i++ {
		models = append(models, fmt.Sprintf("model-%d", i))
	}
	configuration.Providers = []Provider{{ID: "all", Name: "All", AuthMode: "api-key", Protocol: "chat", BaseURL: "https://example.invalid/v1", Models: models, AvailableModels: models}}
	saved, err := store.SaveConfiguration(configuration, nil)
	if err != nil {
		t.Fatal("bulk selection could not be saved", err)
	}
	if len(saved.Providers[0].Models) != 180 {
		t.Fatal("bulk selection was truncated")
	}
}
