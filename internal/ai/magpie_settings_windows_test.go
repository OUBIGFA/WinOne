//go:build windows

package ai

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Removing the global settings persistence or exposing search keys makes this
// fail at the public save/read boundary, without testing a private helper.
func TestMagpieSettingsPersistWithoutPlaintextSearchKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ai.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	c := s.Configuration()
	data, _ := json.Marshal(c)
	var body map[string]any
	_ = json.Unmarshal(data, &body)
	body["magpieSettings"] = map[string]any{"redact": true, "quotaReads": "asked", "vision": "off", "searchAPIs": []any{map[string]any{"vendor": "tavily", "key": "test-secret-search-key"}}}
	data, _ = json.Marshal(body)
	_ = json.Unmarshal(data, &c)
	saved, err := s.SaveConfiguration(c, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, _ = json.Marshal(saved)
	if !strings.Contains(string(data), `"magpieSettings"`) || !strings.Contains(string(data), `"redact":true`) {
		t.Fatal("Magpie settings were lost")
	}
	if strings.Contains(string(data), "test-secret-search-key") {
		t.Fatal("public settings exposed the search key")
	}
	disk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(disk), "test-secret-search-key") {
		t.Fatal("disk stored an unprotected search key")
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	data, _ = json.Marshal(reopened.Configuration())
	if !strings.Contains(string(data), `"hasKey":true`) {
		t.Fatal("search credential disappeared after restart")
	}
}

func TestDeepSeekSearchKeyPersistsPrivatelyAndReachesRuntime(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ai.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	configuration := store.Configuration()
	configuration.MagpieSettings = &MagpieSettings{SearchAPIs: []SearchAPI{{Vendor: "deepseek", Key: "fixture-deepseek-search-key"}}}
	saved, err := store.SaveConfiguration(configuration, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.MagpieSettings.SearchAPIs) != 1 || !saved.MagpieSettings.SearchAPIs[0].HasKey || saved.MagpieSettings.SearchAPIs[0].Key != "" {
		t.Fatal("DeepSeek search credential exposed or lost")
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	settings, err := reopened.MagpieRuntime()
	if err != nil || settings.SearchAPIs[0].Key != "fixture-deepseek-search-key" {
		t.Fatalf("search runtime did not receive the saved credential: %v", err)
	}
	bytes, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(bytes), "fixture-deepseek-search-key") {
		t.Fatal("search key not encrypted on disk")
	}
}

func TestMagpieModelPreferencesKeepMappingAndPrice(t *testing.T) {
	var m ModelMetadata
	if err := json.Unmarshal([]byte(`{"same":"gpt-5","upstreamName":"relay-gpt-5","api":"responses","efforts":["low","high"],"price":{"input":1,"output":2,"cache_read":0.1,"cache_write":0.2}}`), &m); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(m)
	for _, field := range []string{`"same"`, `"upstreamName"`, `"api"`, `"efforts"`, `"price"`} {
		if !strings.Contains(string(data), field) {
			t.Fatalf("model preference lost: %s", field)
		}
	}
}
