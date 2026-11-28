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
	}
}
