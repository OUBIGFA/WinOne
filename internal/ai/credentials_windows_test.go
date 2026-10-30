//go:build windows

package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
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

func TestMultipleKeysPersistEncryptedAndRotateAcrossTasks(t *testing.T) {
	s, path := multiKeyStore(t)
	stored, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	public, _ := json.Marshal(s.Configuration())
	for _, data := range [][]byte{stored, public} {
		if strings.Contains(string(data), "fixture-first-secret") || strings.Contains(string(data), "fixture-second-secret") {
			t.Fatal("plaintext credential escaped")
		}
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	var seen []string
	for range 4 {
		_, plan, err := s.ForTaskKeys("chat", nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.WithKeys(context.Background(), plan, func(_ context.Context, key string) (bool, error) { seen = append(seen, key); return false, nil }); err != nil {
			t.Fatal(err)
		}
	}
	if want := []string{"fixture-first-secret", "fixture-second-secret", "fixture-first-secret", "fixture-second-secret"}; !reflect.DeepEqual(seen, want) {
		t.Fatalf("rotation=%v", seen)
	}
	c := s.Configuration()
	c.Providers[0].Name = "Renamed"
	if _, err = s.SaveConfiguration(c, nil); err != nil {
		t.Fatal(err)
	}
	if len(s.Configuration().Providers[0].Keys) != 2 {
		t.Fatal("editing provider removed credentials")
	}
}

func TestLegacyKeyCanAddTwoKeysWithoutReentryAndRotateThreeWays(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ai.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	p := s.Snapshot()
	p.Engine = "llm"
	p.BaseURL = "https://example.invalid/v1"
	p.Model = "m"
	first := "legacy-secret"
	if _, err = s.Save(p, &first); err != nil {
		t.Fatal(err)
	}
	sealed := s.protected
	c := s.Configuration()
	if len(c.Providers[0].Keys) != 1 || c.Providers[0].Keys[0].ID != "primary" || !c.Providers[0].Keys[0].HasKey {
		t.Fatal("legacy key was not exposed as a saved slot")
	}
	c.Providers[0].Keys = append(c.Providers[0].Keys, KeySlot{ID: "second", Enabled: true}, KeySlot{ID: "third", Enabled: true})
	second, third := "second-secret", "third-secret"
	if _, err = s.SaveConfigurationWithKeys(c, nil, CredentialUpdates{"legacy": {"second": &second, "third": &third}}); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.sealedCredentialLocked("legacy", "primary") != sealed {
		t.Fatal("migration replaced the existing credential")
	}
	var got []string
	for range 4 {
		_, plan, err := s.ForTaskKeys("chat", nil)
		if err != nil {
			t.Fatal(err)
		}
		err = s.WithKeys(context.Background(), plan, func(_ context.Context, key string) (bool, error) { got = append(got, key); return false, nil })
		if err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(got, []string{first, second, third, first}) {
		t.Fatalf("three-key rotation=%v", got)
	}
}

func TestKeyAttemptLimitAndFailoverSwitch(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprint(enabled), func(t *testing.T) {
			s, _ := multiKeyStore(t)
			c := s.Configuration()
			c.Providers[0].Keys = append(c.Providers[0].Keys, KeySlot{ID: "third", Enabled: true})
			c.Providers[0].KeyPolicy = &KeyPolicy{Strategy: "ordered", Failover: enabled, MaxAttempts: 2, CooldownSeconds: 60}
			third := "third-secret"
			if _, err := s.SaveConfigurationWithKeys(c, nil, CredentialUpdates{"one": {"third": &third}}); err != nil {
				t.Fatal(err)
			}
			_, plan, err := s.ForTaskKeys("chat", nil)
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			err = s.WithKeys(context.Background(), plan, func(context.Context, string) (bool, error) { calls++; return false, &AttemptFailure{Status: 503} })
			want := 1
			if enabled {
				want = 2
			}
			if err == nil || calls != want {
				t.Fatalf("attempts=%d want=%d error=%v", calls, want, err)
			}
		})
	}
}

func TestFailoverConsumesTheBackupKeyRoundRobinTurn(t *testing.T) {
	s, _ := multiKeyStore(t)
	c := s.Configuration()
	c.Providers[0].Keys = append(c.Providers[0].Keys, KeySlot{ID: "third", Enabled: true})
	third := "third-secret"
	if _, err := s.SaveConfigurationWithKeys(c, nil, CredentialUpdates{"one": {"third": &third}}); err != nil {
		t.Fatal(err)
	}
	var keys []string
	for range 2 {
		_, plan, err := s.ForTaskKeys("chat", nil)
		if err != nil {
			t.Fatal(err)
		}
		err = s.WithKeys(context.Background(), plan, func(_ context.Context, key string) (bool, error) {
			keys = append(keys, key)
			if key == "fixture-first-secret" {
				return false, &AttemptFailure{Status: 401}
			}
			return false, nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if want := []string{"fixture-first-secret", "fixture-second-secret", "third-secret"}; !reflect.DeepEqual(keys, want) {
		t.Fatalf("backup usage did not advance the round: %v", keys)
	}
}

func TestConcurrentKeyRotationDoesNotChooseTheSameFirstKey(t *testing.T) {
	s, _ := multiKeyStore(t)
	var wg sync.WaitGroup
	var mu sync.Mutex
	counts := map[string]int{}
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, plan, err := s.ForTaskKeys("chat", nil)
			if err != nil {
				t.Error(err)
				return
			}
			err = s.WithKeys(context.Background(), plan, func(_ context.Context, key string) (bool, error) {
				mu.Lock()
				counts[key]++
				mu.Unlock()
				return false, nil
			})
			if err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if counts["fixture-first-secret"] != 10 || counts["fixture-second-secret"] != 10 {
		t.Fatalf("concurrent rotation=%v", counts)
	}
}

func TestSingleKeyRetainsImmediateManualRetry(t *testing.T) {
	s, _ := multiKeyStore(t)
	c := s.Configuration()
	c.Providers[0].Keys = c.Providers[0].Keys[:1]
	if _, err := s.SaveConfiguration(c, nil); err != nil {
		t.Fatal(err)
	}
	calls := 0
	for index := range 2 {
		_, plan, err := s.ForTaskKeys("chat", nil)
		if err != nil {
			t.Fatal(err)
		}
		err = s.WithKeys(context.Background(), plan, func(context.Context, string) (bool, error) {
			calls++
			if index == 0 {
				return false, &AttemptFailure{Status: 503}
			}
			return false, nil
		})
		if index == 1 && err != nil {
			t.Fatal("single-key retry blocked", err)
		}
	}
	if calls != 2 {
		t.Fatal("manual retry did not reach the provider")
	}
}

func TestKeyCooldownAndEditingAKeyDoNotShareFailureState(t *testing.T) {
	s, _ := multiKeyStore(t)
	now := time.Unix(1000, 0)
	s.now = func() time.Time { return now }
	c := s.Configuration()
	c.Providers[0].KeyPolicy = &KeyPolicy{Strategy: "ordered", Failover: true, MaxAttempts: 3, CooldownSeconds: 60}
	if _, err := s.SaveConfiguration(c, nil); err != nil {
		t.Fatal(err)
	}
	var seen []string
	call := func(failFirst bool) {
		t.Helper()
		_, plan, err := s.ForTaskKeys("chat", nil)
		if err != nil {
			t.Fatal(err)
		}
		err = s.WithKeys(context.Background(), plan, func(_ context.Context, key string) (bool, error) {
			seen = append(seen, key)
			if failFirst && key == "fixture-first-secret" {
				return false, &AttemptFailure{Status: 429, RetryAfter: 120 * time.Second}
			}
			return false, nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	call(true)
	call(false)
	now = now.Add(61 * time.Second)
	call(false)
	now = now.Add(60 * time.Second)
	call(false)
	if want := []string{"fixture-first-secret", "fixture-second-secret", "fixture-second-secret", "fixture-second-secret", "fixture-first-secret"}; !reflect.DeepEqual(seen, want) {
		t.Fatalf("cooldown=%v", seen)
	}
	call(true)
	c = s.Configuration()
	replacement := "replacement-secret"
	if _, err := s.SaveConfigurationWithKeys(c, nil, CredentialUpdates{"one": {"first": &replacement}}); err != nil {
		t.Fatal(err)
	}
	call(false)
	if seen[len(seen)-1] != replacement {
		t.Fatal("replacement inherited the old key's cooldown")
	}
}

func TestProviderLimitRestOffKeepsLimitsRetryableButPreservesOtherCooldowns(t *testing.T) {
	for _, off := range []bool{false, true} {
		for _, failure := range []*AttemptFailure{{Status: 429}, {Status: 403, Limited: true}, {Status: 503}} {
			t.Run(fmt.Sprintf("off=%t/status=%d", off, failure.Status), func(t *testing.T) {
				s, _ := multiKeyStore(t)
				c := s.Configuration()
				c.Providers[0].LimitRestOff = off
				c.Providers[0].KeyPolicy = &KeyPolicy{Strategy: "ordered", Failover: true, MaxAttempts: 3, CooldownSeconds: 60}
				if _, err := s.SaveConfiguration(c, nil); err != nil {
					t.Fatal(err)
				}
				for round := 0; round < 2; round++ {
					_, plan, err := s.ForTaskKeys("chat", nil)
					if err != nil {
						t.Fatal(err)
					}
					var seen []string
					err = s.WithKeys(context.Background(), plan, func(_ context.Context, key string) (bool, error) {
						seen = append(seen, key)
						if round == 0 && key == "fixture-first-secret" {
							return false, failure
						}
						return false, nil
					})
					if err != nil {
						t.Fatal(err)
					}
					if round == 1 {
						want := "fixture-second-secret"
						if off && (failure.Status == 429 || failure.Limited) {
							want = "fixture-first-secret"
						}
						if len(seen) != 1 || seen[0] != want {
							t.Fatalf("manual retry=%v, want %s", seen, want)
						}
					}
				}
			})
		}
	}
}

func TestKeyFailoverStopsAfterOutputBadRequestOrCancellation(t *testing.T) {
	for _, test := range []struct {
		name    string
		partial bool
		err     error
	}{{"output", true, &AttemptFailure{Status: 503}}, {"bad-request", false, &AttemptFailure{Status: 400}}, {"cancelled", false, context.Canceled}} {
		t.Run(test.name, func(t *testing.T) {
			s, _ := multiKeyStore(t)
			_, plan, err := s.ForTaskKeys("chat", nil)
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			err = s.WithKeys(context.Background(), plan, func(context.Context, string) (bool, error) { calls++; return test.partial, test.err })
			if err == nil || calls != 1 {
				t.Fatalf("error=%v calls=%d", err, calls)
			}
			if test.name == "cancelled" && !errors.Is(err, context.Canceled) {
				t.Fatal("cancellation identity lost")
			}
		})
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
