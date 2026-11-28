//go:build windows

package ai

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func multiKeyStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ai.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	c := s.Configuration()
	c.Providers = []Provider{{ID: "one", Name: "One", AuthMode: "api-key", Protocol: "chat", BaseURL: "https://example.invalid/v1", Models: []string{"m"}, Keys: []KeySlot{{ID: "first", Enabled: true}, {ID: "second", Enabled: true}}}}
	c.Tasks["chat"] = Selection{Provider: "one", Model: "m"}
	first, second := "fixture-first-secret", "fixture-second-secret"
	if _, err := s.SaveConfigurationWithKeys(c, nil, CredentialUpdates{"one": {"first": &first, "second": &second}}); err != nil {
		t.Fatal(err)
	}
	return s, path
}

func TestBalanceReadKeepsInferenceKeyRotationAndDisabledKeys(t *testing.T) {
	s, _ := multiKeyStore(t)
	p, plan, err := s.ProviderKeysForModels("one")
	if err != nil {
		t.Fatal(err)
	}
	runtime := Translator{Executable: filepath.Join(t.TempDir(), "missing.exe"), DataDirectory: t.TempDir()}
	if _, err = runtime.KeyBalances(context.Background(), p, plan); err == nil {
		t.Fatal("missing worker reported success")
	}
	if err = s.WithKeys(context.Background(), plan, func(_ context.Context, key string) (bool, error) {
		if key != "fixture-first-secret" {
			t.Fatal("balance read advanced inference rotation")
		}
		return false, nil
	}); err != nil {
		t.Fatal(err)
	}
	c := s.Configuration()
	for i := range c.Providers[0].Keys {
		c.Providers[0].Keys[i].Enabled = false
	}
	if _, err = s.SaveConfiguration(c, nil); err != nil {
		t.Fatal(err)
	}
	p, plan, err = s.ProviderKeysForModels("one")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := runtime.KeyBalances(context.Background(), p, plan)
	if err != nil || string(raw) != `{"quotas":[]}` {
		t.Fatalf("disabled keys caused a read: %s %v", raw, err)
	}
}

func TestDisabledDeletedAndForeignKeysCannotBeUsed(t *testing.T) {
	s, path := multiKeyStore(t)
	c := s.Configuration()
	c.Providers[0].Keys[0].Enabled = false
	if _, err := s.SaveConfiguration(c, nil); err != nil {
		t.Fatal(err)
	}
	_, plan, err := s.ForTaskKeys("chat", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.WithKeys(context.Background(), plan, func(_ context.Context, key string) (bool, error) {
		if key != "fixture-second-secret" {
			t.Fatal("disabled key selected")
		}
		return false, nil
	}); err != nil {
		t.Fatal(err)
	}
	c = s.Configuration()
	c.Providers[0].Keys = c.Providers[0].Keys[:1]
	if _, err = s.SaveConfiguration(c, nil); err != nil {
		t.Fatal(err)
	}
	_, plan, err = s.ForTaskKeys("chat", nil)
	if err != nil {
		t.Fatal(err)
	}
	called := false
	err = s.WithKeys(context.Background(), plan, func(context.Context, string) (bool, error) { called = true; return false, nil })
	if err == nil || called {
		t.Fatal("all-disabled provider must not become an anonymous request")
	}
	before, _ := os.ReadFile(path)
	value := "foreign-secret"
	if _, err = s.SaveConfigurationWithKeys(s.Configuration(), nil, CredentialUpdates{"unknown": {"first": &value}}); err == nil {
		t.Fatal("foreign update accepted")
	}
	after, _ := os.ReadFile(path)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("rejected update modified disk")
	}
}

func TestPinnedDisabledKeyIsPassedAsDisabledWithoutBecomingPrimary(t *testing.T) {
	s, _ := multiKeyStore(t)
	c := s.Configuration()
	c.Providers[0].Keys[0].Enabled = false
	c.Providers[0].AgentAccounts = map[string]string{"codex": "first"}
	if _, err := s.SaveConfiguration(c, nil); err != nil {
		t.Fatal(err)
	}
	_, plan, err := s.ForTaskKeys("chat", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.WithKeys(t.Context(), plan, func(ctx context.Context, key string) (bool, error) {
		keys := credentialRequestKeys(ctx)
		if key != "fixture-second-secret" || len(keys) != 2 || keys[0].Off || !keys[1].Off || keys[1].ID != "first" {
			t.Fatal("disabled pin lost its upstream identity", keys)
		}
		return false, nil
	}); err != nil {
		t.Fatal(err)
	}
	providers, err := s.MagpieProviders()
	if err != nil {
		t.Fatal(err)
	}
	if len(providers) != 1 || len(providers[0].Keys) != 2 || !providers[0].Keys[1].Off {
		t.Fatal("gateway lost disabled pin", providers)
	}
}

// Host storage owns encryption and enabled slots. A complete snapshot, not a
// selected/retried credential, reaches the upstream provider.
func TestEnabledKeySnapshotPreservesOptionsAndDoesNotRetry(t *testing.T) {
	store, path := multiKeyStore(t)
	c := store.Configuration()
	c.Providers[0].Keys[0].KeyOptions = KeyOptions{Name: "First", Protocol: "anthropic", Weight: 3, Plan: "team"}
	c.Providers[0].Keys[1].KeyOptions = KeyOptions{Name: "Second", Protocol: "responses", Weight: 1}
	if _, err := store.SaveConfiguration(c, nil); err != nil {
		t.Fatal(err)
	}
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_, plan, err := store.ForTaskKeys("chat", nil)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	failure := &AttemptFailure{Status: 503}
	err = store.WithKeys(context.Background(), plan, func(ctx context.Context, key string) (bool, error) {
		calls++
		got := credentialRequestKeys(ctx)
		want := []RuntimeKey{{ID: "first", Key: "fixture-first-secret", KeyOptions: KeyOptions{Name: "First", Protocol: "anthropic", Weight: 3, Plan: "team"}}, {ID: "second", Key: "fixture-second-secret", KeyOptions: KeyOptions{Name: "Second", Protocol: "responses", Weight: 1}}}
		if !reflect.DeepEqual(got, want) {
			t.Fatal("upstream credential options changed")
		}
		return false, failure
	})
	if calls != 1 || !errors.Is(err, failure) {
		t.Fatal("Host repeated the upstream operation", calls, err)
	}
}

func TestMultipleKeysStayEncryptedAndSurviveProviderEdits(t *testing.T) {
	store, path := multiKeyStore(t)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	public, _ := json.Marshal(store.Configuration())
	for _, data := range [][]byte{data, public} {
		if strings.Contains(string(data), "fixture-first-secret") || strings.Contains(string(data), "fixture-second-secret") {
			t.Fatal("plaintext credential escaped")
		}
	}
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	c := store.Configuration()
	c.Providers[0].Name = "Renamed"
	if _, err = store.SaveConfiguration(c, nil); err != nil {
		t.Fatal(err)
	}
	_, plan, err := store.ForTaskKeys("chat", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.WithKeys(context.Background(), plan, func(ctx context.Context, _ string) (bool, error) {
		if len(credentialRequestKeys(ctx)) != 2 {
			t.Fatal("provider edit lost a credential")
		}
		return false, nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyKeyCanAddKeysWithoutReenteringTheSecret(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ai.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	prefs := store.Snapshot()
	prefs.Engine = "llm"
	prefs.BaseURL = "https://example.invalid/v1"
	prefs.Model = "m"
	first := "legacy-secret"
	if _, err = store.Save(prefs, &first); err != nil {
		t.Fatal(err)
	}
	sealed := store.protected
	c := store.Configuration()
	c.Providers[0].Keys = append(c.Providers[0].Keys, KeySlot{ID: "second", Enabled: true}, KeySlot{ID: "third", Enabled: true})
	second, third := "second-secret", "third-secret"
	if _, err = store.SaveConfigurationWithKeys(c, nil, CredentialUpdates{"legacy": {"second": &second, "third": &third}}); err != nil {
		t.Fatal(err)
	}
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if store.sealedCredentialLocked("legacy", "primary") != sealed {
		t.Fatal("migration replaced the original credential")
	}
	_, plan, err := store.ForTaskKeys("chat", nil)
	if err != nil {
		t.Fatal(err)
	}
	err = store.WithKeys(context.Background(), plan, func(ctx context.Context, _ string) (bool, error) {
		got := credentialRequestKeys(ctx)
		if len(got) != 3 || got[0].Key != first || got[1].Key != second || got[2].Key != third {
			t.Fatal("legacy runtime snapshot lost credentials")
		}
		return false, nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
