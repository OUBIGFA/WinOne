package ai

import (
	"fmt"
	"reflect"
	"testing"
)

func TestCatalogMergePreservesOrderAndOwnership(t *testing.T) {
	p := Provider{AvailableModels: []string{"b", "a", "b"}, Models: []string{"a", "c", "c", "d"}}
	got := p.AllModels()
	if !reflect.DeepEqual(got, []string{"b", "a", "b", "c", "d"}) {
		t.Fatal(got)
	}
	got[0] = "edited"
	if p.AvailableModels[0] != "b" {
		t.Fatal("caller changed the stored catalog")
	}
	got = IncludeDiscovered([]string{"selected"}, []string{"unchecked"}, []string{"unchecked", "new", "new", "selected", "later"}, []string{"*"})
	if !reflect.DeepEqual(got, []string{"selected", "new", "later"}) {
		t.Fatal(got)
	}
}

// Use the real catalog merge and configuration copy with a large relay list.
// The benchmark has no vendor connection or account data.
func BenchmarkCatalogReads(b *testing.B) {
	for _, n := range []int{100, 1000, 5000} {
		ids := make([]string, n)
		for i := range ids {
			ids[i] = fmt.Sprintf("model-%05d", i)
		}
		p := Provider{ID: "fixture", Name: "Fixture", AvailableModels: ids, Models: ids, ModelDefaultsSeparated: true}
		b.Run(fmt.Sprint(n)+"/snapshot", func(b *testing.B) {
			c := Configuration{Providers: []Provider{p}}
			b.ReportAllocs()
			for b.Loop() {
				_ = cloneConfiguration(c)
			}
		})
		b.Run(fmt.Sprint(n)+"/discover", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_ = IncludeDiscovered(nil, ids, ids, []string{"*"})
			}
		})
		b.Run(fmt.Sprint(n)+"/new-model-rules", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_ = IncludeDiscovered(nil, nil, ids, []string{"re:model-[0-9]+", "other-*"})
			}
		})
		b.Run(fmt.Sprint(n)+"/refresh", func(b *testing.B) {
			models := make([]ModelOption, len(ids))
			for i, id := range ids {
				models[i] = ModelOption{ID: id, Name: id}
			}
			b.ReportAllocs()
			for b.Loop() {
				current := p
				current.RefreshModels(models)
			}
		})
	}
}

func BenchmarkProviderSnapshot(b *testing.B) {
	p := ProviderPreferences{Contexts: map[string]int{"*": 128000}, MaxImages: map[string]int{"*": 8},
		Addresses: []string{"https://relay.invalid/v1"}, AgentAccounts: map[string]string{"magpie": "primary"},
		AccountModels:  map[string][]string{"fixture": {"model-a", "model-b"}},
		KeyBalanceAuth: map[string]KeyBalanceAuth{"primary": {User: "42", HasToken: true}},
		RateWindows:    []RateWindow{{Days: []int{1, 2, 3, 4, 5}, Start: "08:00", End: "18:00", Rate: 0.5}}}
	b.ReportAllocs()
	for b.Loop() {
		_ = p.clone()
	}
}
