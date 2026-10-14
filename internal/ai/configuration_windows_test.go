//go:build windows

package ai

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEmptyAIOverrideDoesNotFallBackToGoogle(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "ai.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = store.ForTaskKeys("translate", &Selection{}); err == nil {
		t.Fatal("missing AI model unexpectedly used the built-in translator")
	}
	preferences, _, err := store.ForTaskKeys("translate", nil)
	if err != nil || preferences.Engine != "google" {
		t.Fatal("normal screenshot translation lost its built-in engine", err)
	}
}

func TestPluginAccountBackendSurvivesSaveAndTaskSelection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ai.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	var provider Provider
	if err := json.Unmarshal([]byte(`{"id":"cline-service","name":"Cline","authMode":"account","agent":"cline","agentBackend":"plugin","models":["test-model"]}`), &provider); err != nil {
		t.Fatal(err)
	}
	configuration := store.Configuration()
	configuration.Engine = "llm"
	configuration.Providers = []Provider{provider}
	configuration.Tasks["default"] = Selection{Provider: "cline-service", Model: "test-model"}
	if _, err := store.SaveConfiguration(configuration, nil); err != nil {
		t.Fatal(err)
	}
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range []string{"translate", "ocr", "chat"} {
		selection := Selection{Provider: "cline-service", Model: "test-model"}
		preferences, key, err := store.ForTask(task, &selection)
		if err != nil || key != "" {
			t.Fatalf("account route: %v", err)
		}
		encoded, _ := json.Marshal(preferences)
		var fields map[string]any
		if json.Unmarshal(encoded, &fields) != nil || fields["agentBackend"] != "plugin" {
			t.Fatalf("%s lost its plugin connection: %s", task, encoded)
		}
	}
}

func TestTaskOnlyConfigurationRetainsAValidLegacyProjection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ai.json")
	s, _ := Open(path)
	c := s.Configuration()
	c.Engine = "llm"
	c.Providers = []Provider{{ID: "one", Name: "One", AuthMode: "api-key", Protocol: "chat", BaseURL: "https://one.example/v1", Models: []string{"m"}}}
	c.Tasks["translate"] = Selection{Provider: "one", Model: "m"}
	key := "fixture-key"
	if _, err := s.SaveConfiguration(c, map[string]*string{"one": &key}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	var legacy stored
	if err := json.Unmarshal(data, &legacy); err != nil {
		t.Fatal(err)
	}
	legacy.SchemaVersion = 0
	legacy.Configuration = nil
	legacy.ProtectedKeys = nil
	data, _ = json.Marshal(legacy)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal("legacy version cannot read projected settings", err)
	}
	p, secret, err := reopened.ForRequest()
	if err != nil || p.Model != "m" || secret != key {
		t.Fatal("legacy projection lost its model/credential", err)
	}
}

func TestOversizedConfigurationCannotCreateAnUnreadableFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ai.json")
	s, _ := Open(path)
	c := s.Configuration()
	for provider := 0; provider < 32; provider++ {
		models := []string{}
		for model := 0; model < 128; model++ {
			models = append(models, fmt.Sprintf("%03d-", model)+strings.Repeat("x", 252))
		}
		c.Providers = append(c.Providers, Provider{ID: fmt.Sprintf("p%d", provider), Name: fmt.Sprintf("Provider %d", provider), AuthMode: "api-key", Protocol: "chat", BaseURL: "https://example.invalid/v1", Models: models})
	}
	if _, err := s.SaveConfiguration(c, nil); err == nil {
		t.Fatal("oversized configuration was saved")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("failed save changed the settings file")
	}
}

func TestConfigurationMigratesLegacyAndRoutesTasksWithoutSharingKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ai.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	legacy := s.Snapshot()
	legacy.Engine = "llm"
	legacy.Model = "old-model"
	legacy.BaseURL = "https://old.example/v1"
	key := "legacy-secret"
	if _, err = s.Save(legacy, &key); err != nil {
		t.Fatal(err)
	}
	config := s.Configuration()
	if len(config.Providers) != 1 || config.Providers[0].Models[0] != "old-model" || !config.Providers[0].HasKey {
		t.Fatal("legacy service was not migrated")
	}
	config.Providers = append(config.Providers, Provider{ID: "second", Name: "Second", AuthMode: "api-key", Protocol: "responses", BaseURL: "https://second.example/v1", Models: []string{"fast", "vision"}})
	config.OCREngine = "ai"
	config.Tasks["ocr"] = Selection{Provider: "second", Model: "vision"}
	config.Tasks["chat"] = Selection{Provider: "second", Model: "fast"}
	newer := "separate-secret"
	saved, err := s.SaveConfiguration(config, map[string]*string{"second": &newer})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveConfiguration(config, nil); err == nil {
		t.Fatal("stale configuration was accepted")
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range []struct{ task, provider, model, key string }{{"translate", "legacy", "old-model", key}, {"ocr", "second", "vision", newer}, {"chat", "second", "fast", newer}} {
		got, secret, e := s.ForTask(entry.task, nil)
		if e != nil || got.ProviderID != entry.provider || got.Model != entry.model || secret != entry.key {
			t.Fatalf("wrong %s route: %+v %v", entry.task, got, e)
		}
	}
	selected, secret, err := s.ForTask("chat", &Selection{Provider: "legacy", Model: "old-model"})
	if err != nil || selected.Model != "old-model" || secret != key {
		t.Fatal("window override failed", err)
	}
	if s.Configuration().Tasks["chat"].Model != "fast" {
		t.Fatal("window override changed the global task")
	}
	saved.Providers[0].Models[0] = "modified outside store"
	if s.Configuration().Providers[0].Models[0] != "old-model" {
		t.Fatal("snapshot exposed shared storage")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), key) || strings.Contains(string(data), newer) {
		t.Fatal("configuration persisted plaintext credentials")
	}
}

func TestConfigurationRejectsMissingRoutesAndPreservesCredentials(t *testing.T) {
	s, _ := Open(filepath.Join(t.TempDir(), "ai.json"))
	config := s.Configuration()
	config.Providers = []Provider{{ID: "one", Name: "One", AuthMode: "api-key", Protocol: "chat", BaseURL: "https://one.example/v1", Models: []string{"m"}}}
	config.Tasks["default"] = Selection{Provider: "one", Model: "m"}
	key := "saved-secret"
	config, err := s.SaveConfiguration(config, map[string]*string{"one": &key})
	if err != nil {
		t.Fatal(err)
	}
	invalid := s.Configuration()
	invalid.Providers = nil
	if _, err = s.SaveConfiguration(invalid, nil); err == nil {
		t.Fatal("referenced provider deletion accepted")
	}
	if _, _, err = s.ForTask("chat", &Selection{Provider: "one", Model: "unknown"}); err == nil {
		t.Fatal("unconfigured model accepted")
	}
	config.Providers[0].Name = "Renamed"
	if _, err = s.SaveConfiguration(config, nil); err != nil {
		t.Fatal(err)
	}
	_, got, err := s.ForTask("chat", nil)
	if err != nil || got != key {
		t.Fatal("rename lost credentials")
	}
}

func TestAccountProfilesHaveIndependentScopesAndNoAPIKey(t *testing.T) {
	s, _ := Open(filepath.Join(t.TempDir(), "ai.json"))
	config := s.Configuration()
	config.Providers = []Provider{{ID: "account-a", Name: "Personal", AuthMode: "account", Agent: "codex", Models: []string{"m"}}, {ID: "account-b", Name: "Work", AuthMode: "account", Agent: "codex", Models: []string{"m"}}}
	config.Tasks["default"] = Selection{Provider: "account-a", Model: "m"}
	config.Tasks["chat"] = Selection{Provider: "account-b", Model: "m"}
	if _, err := s.SaveConfiguration(config, nil); err != nil {
		t.Fatal(err)
	}
	a, key, err := s.ForTask("default", nil)
	if err != nil || key != "" {
		t.Fatal(err)
	}
	b, key, err := s.ForTask("chat", nil)
	if err != nil || key != "" {
		t.Fatal(err)
	}
	runtime := Translator{DataDirectory: t.TempDir()}
	if runtime.ForProvider(a.ProviderID).DataDirectory == runtime.ForProvider(b.ProviderID).DataDirectory {
		t.Fatal("account profiles share their login directory")
	}
}
