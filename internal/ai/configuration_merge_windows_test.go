//go:build windows

package ai

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"
	"winone/internal/settings"
)

func idleEditStore(t *testing.T) (*Store, Configuration) {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "ai.json"))
	if err != nil {
		t.Fatal(err)
	}
	c := s.Configuration()
	p := Provider{ID: "one", Name: "Original", AuthMode: "api-key", Protocol: "chat", BaseURL: "https://example.invalid/v1", Models: []string{"a"}}
	p.RefreshModels([]ModelOption{{ID: "a", Name: "A", Metadata: ModelMetadata{ContextWindow: 1000}}, {ID: "b", Name: "B"}})
	c.Providers = []Provider{p}
	c.Tasks["default"] = Selection{Provider: "one", Model: "a"}
	key := "fixture-old-key"
	if _, err = s.SaveConfiguration(c, map[string]*string{"one": &key}); err != nil {
		t.Fatal(err)
	}
	return s, s.Configuration()
}

func TestConfigurationEditSurvivesRepeatedBackgroundRefresh(t *testing.T) {
	s, base := idleEditStore(t)
	draft := cloneConfiguration(base)
	draft.Providers[0].Name = "My edited provider"
	draft.Providers[0].ModelNames["a"] = "My model name"
	draft.Providers[0].Models = []string{"a", "b"}
	draft.Providers[0].ModelMetadata["a"] = ModelMetadata{ReasoningEffort: "high"}
	// Advance the real persisted store repeatedly, using the exact refresh
	// and save path used by the scheduler; no clock sleeps or network needed.
	for i := 1; i <= 24; i++ {
		current := s.Configuration()
		current.Providers[0].RefreshModels([]ModelOption{{ID: "a", Name: "Updated A", Metadata: ModelMetadata{ContextWindow: 1000 + i}}, {ID: "b", Name: "B"}, {ID: "new", Name: "New"}})
		current.Providers[0].ModelRefreshAt = time.Unix(int64(i)*3600, 0).UTC()
		current.Providers[0].Models = append(current.Providers[0].Models, "new")
		current.Providers[0].Models = slices.Compact(current.Providers[0].Models)
		if _, err := s.SaveConfiguration(current, nil); err != nil {
			t.Fatal(err)
		}
	}
	// This is the former page request: a stale revision alone is insufficient.
	if _, err := s.SaveConfiguration(draft, nil); !errors.Is(err, settings.ErrConflict) {
		t.Fatal(err)
	}
	key := "fixture-new-key"
	saved, err := s.SaveConfigurationEdit(draft, &base, nil, CredentialUpdates{"one": {"primary": &key}})
	if err != nil {
		t.Fatal("idle editor could not save", err)
	}
	p := saved.Providers[0]
	if p.Name != draft.Providers[0].Name || !slices.Equal(p.Models, []string{"a", "b", "new"}) || p.CatalogMetadata["a"].ContextWindow != 1024 || p.ModelNames["a"] != "My model name" || p.ModelRefreshAt != time.Unix(24*3600, 0).UTC() {
		t.Fatalf("draft or refreshed catalog lost: %+v", p)
	}
	if !reflect.DeepEqual(p.ModelMetadataFor("a"), ModelMetadata{ReasoningEffort: "high"}) {
		t.Fatal("defaults became overrides")
	}
	reopened, err := Open(s.path)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := reopened.RevealCredential("one", "primary"); err != nil || got != key {
		t.Fatal("key edit lost", err)
	}
	if !reflect.DeepEqual(reopened.Configuration(), s.Configuration()) {
		t.Fatal("merged configuration did not survive reopening")
	}
}

func TestConfigurationEditKeepsRefreshErrorAndIndependentUserChanges(t *testing.T) {
	s, base := idleEditStore(t)
	draft := cloneConfiguration(base)
	draft.ShowReasoning = new(false)
	current := s.Configuration()
	current.Providers[0].ModelRefreshError = "模型自动刷新失败，请检查连接后重试"
	current.Providers[0].Name = "Other window"
	if _, err := s.SaveConfiguration(current, nil); err != nil {
		t.Fatal(err)
	}
	saved, err := s.SaveConfigurationEdit(draft, &base, nil, nil)
	if err != nil || saved.ReasoningVisible() || saved.Providers[0].Name != "Other window" || saved.Providers[0].ModelRefreshError == "" {
		t.Fatal("independent changes were not merged", err)
	}
}

func TestConfigurationEditConflictKeepsDiskAndCredential(t *testing.T) {
	s, base := idleEditStore(t)
	draft := cloneConfiguration(base)
	draft.Providers[0].Name = "Draft name"
	current := s.Configuration()
	current.Providers[0].Name = "Other name"
	if _, err := s.SaveConfiguration(current, nil); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(s.path)
	key := "fixture-rejected-key"
	if _, err := s.SaveConfigurationEdit(draft, &base, nil, CredentialUpdates{"one": {"primary": &key}}); err == nil {
		t.Fatal("overlapping change accepted")
	}
	after, _ := os.ReadFile(s.path)
	if string(before) != string(after) {
		t.Fatal("failed merge wrote to disk")
	}
	if got, _ := s.RevealCredential("one", "primary"); got != "fixture-old-key" {
		t.Fatal("failed merge changed credential")
	}
	if draft.Providers[0].Name != "Draft name" || base.Providers[0].Name != "Original" {
		t.Fatal("merge mutated caller snapshots")
	}
}
