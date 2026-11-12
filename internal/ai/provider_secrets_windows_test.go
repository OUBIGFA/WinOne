//go:build windows

package ai

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSupplementalProviderSecretsAreWriteOnlyAndReachExecution(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ai.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	c := s.Configuration()
	p := Provider{ID: "cloud", Name: "Cloud", AuthMode: "api-key", Protocol: "gemini", BaseURL: "https://example.invalid/v1beta", Models: []string{"model"}}
	p.AccessKeyID, p.SecretAccessKey, p.BalanceToken = new("fixture-ak-id"), new("fixture-cloud-secret"), new("fixture-balance-token")
	c.Providers, c.Tasks = []Provider{p}, map[string]Selection{"default": {Provider: "cloud", Model: "model"}}
	saved, err := s.SaveConfiguration(c, nil)
	if err != nil {
		t.Fatal(err)
	}
	checkPublic := func(c Configuration) {
		t.Helper()
		encoded, _ := json.Marshal(c)
		if strings.Contains(string(encoded), "fixture-") {
			t.Fatal("public snapshot leaked provider credentials")
		}
		p := c.Providers[0]
		if !p.HasAccessKeyID || !p.HasSecretAccessKey || !p.HasBalanceToken {
			t.Fatal("saved status lost")
		}
	}
	checkPublic(saved)
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "fixture-") {
		t.Fatal("credentials written unencrypted")
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	checkPublic(s.Configuration())
	pr, _, err := s.ProviderKeysForModels("cloud")
	if err != nil || pr.BalanceToken == nil || *pr.BalanceToken != "fixture-balance-token" {
		t.Fatal("saved secret did not reach discovery", err)
	}
	runtime, _, err := s.ForTaskKeys("default", nil)
	if err != nil || runtime.SecretAccessKey == nil || *runtime.SecretAccessKey != "fixture-cloud-secret" {
		t.Fatal("saved secret did not reach inference", err)
	}
	// Omitting credentials retains them. An explicit empty edit clears one.
	c = s.Configuration()
	if _, err = s.SaveConfiguration(c, nil); err != nil {
		t.Fatal(err)
	}
	c = s.Configuration()
	c.Providers[0].BalanceToken = new("")
	if _, err = s.SaveConfiguration(c, nil); err != nil {
		t.Fatal(err)
	}
	if s.Configuration().Providers[0].HasBalanceToken || !s.Configuration().Providers[0].HasSecretAccessKey {
		t.Fatal("clearing one secret changed another")
	}
}
