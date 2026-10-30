//go:build windows

package ai

import (
	"encoding/json"
	"math"
	"path/filepath"
	"slices"
	"testing"
	"winone/internal/configjson"
)

func TestModelCreditRatesSurviveTheSavedConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ai.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	config := store.Configuration()
	if err = json.Unmarshal([]byte(`[{"id":"custom","name":"Custom","authMode":"api-key","protocol":"chat","baseUrl":"https://example.invalid/v1","models":["discounted"],"modelMetadata":{"discounted":{"rate":0.5,"rateWas":1}}}]`), &config.Providers); err != nil {
		t.Fatal(err)
	}
	if _, err = store.SaveConfiguration(config, nil); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	provider, _ := reopened.Configuration().Provider("custom")
	raw, err := json.Marshal(provider.ModelMetadataFor("discounted"))
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err = json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	if value["rate"] != 0.5 || value["rateWas"] != float64(1) {
		t.Fatalf("saved credit rates lost: %s", raw)
	}
	for _, raw := range []string{`{"rate":-0.5}`, `{"rateWas":-1}`} {
		var metadata ModelMetadata
		if err = json.Unmarshal([]byte(raw), &metadata); err != nil {
			t.Fatal(err)
		}
		if validateModelMetadata(metadata) == nil {
			t.Fatalf("invalid vendor rate accepted: %s", raw)
		}
	}
}

func TestModelMetadataPersistsAndReachesTaskSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ai.json")
	s, _ := Open(path)
	c := s.Configuration()
	if err := json.Unmarshal([]byte(`[{"id":"custom","name":"Custom","authMode":"api-key","protocol":"chat","baseUrl":"https://example.invalid/v1","models":["model"],"modelNames":{"model":"My model"},"modelMetadata":{"model":{"contextWindow":128000,"maxTokens":8000,"compactionThreshold":100000,"reasoning":true,"reasoningEffort":"high","vision":false,"tools":true,"tier":"priority"}},"maxRPM":20,"maxConcurrency":2,"queueLimit":3,"queueWait":15}]`), &c.Providers); err != nil {
		t.Fatal(err)
	}
	saved, err := s.SaveConfiguration(c, nil)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	p, _, err := reopened.ForTaskKeys("chat", &Selection{Provider: "custom", Model: "model"})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(saved.Providers[0])
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(encoded, &fields)
	if fields["modelMetadata"] == nil || fields["maxRPM"] == nil {
		t.Fatalf("model settings were discarded: %s", encoded)
	}
	if p.ModelName != "My model" {
		t.Fatal("name was not carried to request")
	}
	if p.ModelMetadata.ContextWindow != 128000 || p.ModelMetadata.MaxTokens != 8000 || p.ModelMetadata.CompactionThreshold != 100000 || p.ModelMetadata.Reasoning == nil || !*p.ModelMetadata.Reasoning || p.RequestLimits.MaxRPM != 20 {
		t.Fatalf("request metadata lost: %+v", p)
	}
	*p.ModelMetadata.Reasoning = false
	if !*reopened.Configuration().Providers[0].ModelMetadataFor("model").Reasoning {
		t.Fatal("request mutated saved metadata")
	}
}

func TestPercentageMetadataSurvivesStrictJSONSaveReopenAndTaskSelection(t *testing.T) {
	var metadata ModelMetadata
	if err := configjson.DecodeObject([]byte(`{"contextWindow":256000,"compactionPercent":72.5,"compactionThreshold":100000,"listedContextWindow":128000,"maxContextWindow":1000000}`), &metadata); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "ai.json")
	store, err := Open(file)
	if err != nil {
		t.Fatal(err)
	}
	config := store.Configuration()
	config.Providers = []Provider{{ID: "custom", Name: "Custom", AuthMode: "api-key", Protocol: "chat", BaseURL: "https://example.invalid/v1", Models: []string{"model"}, ModelMetadata: map[string]ModelMetadata{"model": metadata}}}
	if _, err := store.SaveConfiguration(config, nil); err != nil {
		t.Fatal(err)
	}
	store, err = Open(file)
	if err != nil {
		t.Fatal(err)
	}
	prefs, _, err := store.ForTaskKeys("chat", &Selection{Provider: "custom", Model: "model"})
	if err != nil {
		t.Fatal(err)
	}
	got := prefs.ModelMetadata
	if got.CompactionPercent != 72.5 || got.CompactionThreshold != 100000 || got.ListedContextWindow != 128000 || got.MaxContextWindow != 1000000 {
		t.Fatalf("saved metadata did not reach the request unchanged: %+v", got)
	}
}

func TestPercentageMetadataValidationKeepsLegacyTokenSemantics(t *testing.T) {
	for _, percent := range []float64{-1, 100.01, math.NaN(), math.Inf(1)} {
		if err := validateModelMetadata(ModelMetadata{CompactionPercent: percent}); err == nil {
			t.Fatalf("accepted invalid percent %v", percent)
		}
	}
	for _, percent := range []float64{0, 0.1, 72.5, 100} {
		if err := validateModelMetadata(ModelMetadata{CompactionPercent: percent, CompactionThreshold: 100000}); err != nil {
			t.Fatal(percent, err)
		}
	}
	if err := validateModelMetadata(ModelMetadata{MaxContextWindow: -1}); err == nil {
		t.Fatal("accepted negative reported maximum")
	}
}

func TestModelMetadataRejectsInvalidLimits(t *testing.T) {
	// Levels are vendor words, so only text that cannot be a level is refused:
	// padding, Magpie's model:effort separator, control text, an empty entry.
	for _, patch := range []string{`"contextWindow":-1`, `"maxTokens":-1`, `"reasoningEffort":" high"`, `"reasoningEffort":"low:high"`, `"tier":"a\u0000b"`, `"efforts":[""]`, `"levels":["deep\n"]`} {
		s, _ := Open(filepath.Join(t.TempDir(), "ai.json"))
		c := s.Configuration()
		_ = json.Unmarshal([]byte(`[{"id":"custom","name":"Custom","authMode":"api-key","protocol":"chat","baseUrl":"https://example.invalid/v1","models":["model"],"modelMetadata":{"model":{`+patch+`}}}]`), &c.Providers)
		if _, err := s.SaveConfiguration(c, nil); err == nil {
			t.Fatal("accepted invalid metadata", patch)
		}
	}
}

// Regression: Our Free Model's MiMo V2.5 reports light/balanced/deep. The old
// fixed ladder refused the save with "MiMo V2.5：可用思考档位无效 (ai)".
func TestModelMetadataKeepsVendorReasoningLevels(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ai.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	config := store.Configuration()
	if err = json.Unmarshal([]byte(`[{"id":"custom","name":"Custom","authMode":"api-key","protocol":"chat","baseUrl":"https://example.invalid/v1","models":["mimo-v2.5-free"],"modelNames":{"mimo-v2.5-free":"MiMo V2.5"},"modelMetadata":{"mimo-v2.5-free":{"levels":["light","balanced","deep"],"efforts":["balanced","deep"],"reasoningEffort":"deep","tiers":["priority","persistent"]}}}]`), &config.Providers); err != nil {
		t.Fatal(err)
	}
	if _, err = store.SaveConfiguration(config, nil); err != nil {
		t.Fatalf("vendor reasoning levels refused: %v", err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	provider, _ := reopened.Configuration().Provider("custom")
	metadata := provider.ModelMetadataFor("mimo-v2.5-free")
	if !slices.Equal(metadata.Levels, []string{"light", "balanced", "deep"}) || !slices.Equal(metadata.Efforts, []string{"balanced", "deep"}) || metadata.ReasoningEffort != "deep" {
		t.Fatalf("vendor reasoning levels lost: %+v", metadata)
	}
}
