//go:build windows

package ai

import (
	"slices"
	"testing"
	"time"
)

func TestIdleConfigurationEditHandlesDelistingAndProviderRemoval(t *testing.T) {
	for _, remove := range []bool{false, true} {
		t.Run(map[bool]string{false: "select-delisted", true: "remove-provider"}[remove], func(t *testing.T) {
			s, base := idleEditStore(t)
			draft := cloneConfiguration(base)
			if remove {
				draft.Providers = []Provider{}
				draft.Tasks = map[string]Selection{}
			} else {
				draft.Providers[0].Models = []string{"a", "b"}
				draft.Providers[0].ModelMetadata["b"] = ModelMetadata{ReasoningEffort: "high"}
			}
			current := s.Configuration()
			current.Providers[0].RefreshModels([]ModelOption{{ID: "a", Name: "Updated A"}})
			current.Providers[0].ModelRefreshAt = time.Unix(3600, 0).UTC()
			if _, err := s.SaveConfiguration(current, nil); err != nil {
				t.Fatal(err)
			}
			saved, err := s.SaveConfigurationEdit(draft, &base, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if remove {
				if len(saved.Providers) != 0 {
					t.Fatal("provider deletion was undone")
				}
			} else if p := saved.Providers[0]; !slices.Equal(p.Models, []string{"a"}) || !slices.Equal(p.Delisted, []string{"b"}) || p.ModelMetadata["b"].ReasoningEffort != "high" {
				t.Fatalf("delisted pick or override lost: %+v", p)
			}
		})
	}
}

func TestIdleConfigurationEditKeepsExplicitModelFetch(t *testing.T) {
	s, base := idleEditStore(t)
	current := s.Configuration()
	current.Providers[0].RefreshModels([]ModelOption{{ID: "a", Name: "Background A"}, {ID: "old", Name: "Background model"}})
	current.Providers[0].ModelRefreshAt = time.Unix(3600, 0).UTC()
	if _, err := s.SaveConfiguration(current, nil); err != nil {
		t.Fatal(err)
	}
	draft := cloneConfiguration(base)
	draft.Providers[0].RefreshModels([]ModelOption{{ID: "a", Name: "Manual A"}, {ID: "new", Name: "New model"}})
	draft.Providers[0].ModelRefreshAt = time.Unix(7200, 0).UTC()
	draft.Providers[0].Models = []string{"new"}
	draft.Tasks = map[string]Selection{}
	saved, err := s.SaveConfigurationEdit(draft, &base, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if p := saved.Providers[0]; !slices.Equal(p.Models, []string{"new"}) || !slices.Equal(p.CatalogModels, []string{"a", "new"}) || p.ModelNames["a"] != "Manual A" {
		t.Fatalf("manual discovery was overwritten: %+v", p)
	}
}
