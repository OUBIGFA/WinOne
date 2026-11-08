package ai

import (
	"reflect"
	"testing"
	"time"
)

func TestAutoIncludeDoesNotReenableUncheckedModels(t *testing.T) {
	got := IncludeDiscovered([]string{"manual"}, []string{"gpt-old"}, []string{"gpt-old", "GPT-new", "claude"}, []string{"gpt-*"})
	if !reflect.DeepEqual(got, []string{"manual", "GPT-new"}) {
		t.Fatal(got)
	}
	got = IncludeDiscovered(got, nil, []string{"gpt-old", "gpt-new"}, []string{"re:gpt-old"})
	if !reflect.DeepEqual(got, []string{"manual", "GPT-new", "gpt-old"}) {
		t.Fatal(got)
	}
}

func TestModelDiscoveryScheduleAndValidation(t *testing.T) {
	p := Provider{AuthMode: "api-key", Preset: "remote-magpie"}
	if p.ModelRefreshInterval() != 10*time.Minute {
		t.Fatal("remote default missing")
	}
	p.ListOff = true
	if p.ModelRefreshInterval() != 0 {
		t.Fatal("disabled discovery still scheduled")
	}
	for _, p := range []ProviderPreferences{{RefreshEvery: "1s"}, {AutoInclude: []string{"exact-id"}}, {AutoInclude: []string{"re:["}}} {
		if p.validate() == nil {
			t.Fatal("invalid discovery option accepted", p)
		}
	}
	if err := (ProviderPreferences{RefreshEvery: "1d", AutoInclude: []string{"re:(?-i)GPT-.*", "gpt-*"}}).validate(); err != nil {
		t.Fatal(err)
	}
}

func TestCatalogRefreshKeepsManualModelsAndOnlyUserOverrides(t *testing.T) {
	p := Provider{Models: []string{"a", "manual", "gone"}, AvailableModels: []string{"a", "manual", "gone"}, CatalogModels: []string{"a", "gone"}, ManualModels: []string{"manual"}, ModelMetadata: map[string]ModelMetadata{"a": {ContextWindow: 200, Rate: 1}, "manual": {ContextWindow: 99}}, CatalogMetadata: map[string]ModelMetadata{"a": {ContextWindow: 100, Rate: 1}}}
	p.RefreshModels([]ModelOption{{ID: "a", Metadata: ModelMetadata{ContextWindow: 150, Rate: 2}}, {ID: "new"}})
	if !reflect.DeepEqual(p.Models, []string{"a", "manual"}) || !reflect.DeepEqual(p.Delisted, []string{"gone"}) {
		t.Fatalf("picks=%v delisted=%v", p.Models, p.Delisted)
	}
	if p.ModelMetadata["a"].ContextWindow != 200 || p.ModelMetadata["a"].Rate != 2 || p.ModelMetadata["manual"].ContextWindow != 99 {
		t.Fatal(p.ModelMetadata)
	}
	before := cloneConfiguration(Configuration{Providers: []Provider{p}, MagpieSettings: &MagpieSettings{}}).Providers[0]
	p.RefreshModels(nil)
	if !reflect.DeepEqual(before.ModelMetadata, p.ModelMetadata) || !reflect.DeepEqual(before.Models, p.Models) {
		t.Fatal("empty reply destroyed catalog")
	}
	p.RefreshModels([]ModelOption{{ID: "a"}, {ID: "gone"}})
	if !reflect.DeepEqual(p.Models, []string{"a", "manual", "gone"}) || len(p.Delisted) != 0 {
		t.Fatal("returned model not restored", p.Models, p.Delisted)
	}
}

func TestLegacyCatalogRetainsUnknownManualOrigins(t *testing.T) {
	p := Provider{Models: []string{"listed", "custom"}, AvailableModels: []string{"listed", "custom"}}
	p.RefreshModels([]ModelOption{{ID: "listed"}})
	if !reflect.DeepEqual(p.ManualModels, []string{"custom"}) || len(p.Models) != 2 {
		t.Fatal(p)
	}
}

func TestCatalogRefreshPreservesRenamedModels(t *testing.T) {
	p := Provider{Models: []string{"a", "b"}, ModelNames: map[string]string{"a": "My model", "b": "Vendor B"}, CatalogNames: map[string]string{"a": "Vendor A", "b": "Vendor B"}}
	p.RefreshModels([]ModelOption{{ID: "a", Name: "New A"}, {ID: "b", Name: "New B"}})
	if p.ModelNames["a"] != "My model" || p.ModelNames["b"] != "New B" {
		t.Fatal(p.ModelNames)
	}
}
