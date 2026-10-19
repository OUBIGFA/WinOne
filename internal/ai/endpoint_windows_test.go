//go:build windows

package ai

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestCustomHTTPProviderSupportsDraftSaveReopenAndTasks(t *testing.T) {
	for _, endpoint := range []string{
		"http://relay.example:3000/v1", "http://203.0.113.7:3000/v1",
		"http://192.168.50.10:3000/v1", "http://[2001:db8::7]:3000/v1",
		"http://localhost:3000/v1", "http://127.0.0.1:3000/v1", "http://[::1]:3000/v1",
		"https://relay.example:3443/api/v1",
	} {
		for _, protocol := range []string{"chat", "responses", "anthropic"} {
			t.Run(protocol+"/"+endpoint, func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "ai.json")
				store, err := Open(path)
				if err != nil {
					t.Fatal(err)
				}
				draft := ModelConnectionDraft{Protocol: protocol, BaseURL: endpoint}
				called := false
				err = store.WithModelDraft(context.Background(), "custom", draft, nil, func(_ context.Context, provider Provider, key string) error {
					called = true
					if provider.BaseURL != endpoint || provider.Protocol != protocol || key != "" {
						t.Fatal("draft connection changed")
					}
					return nil
				})
				if err != nil || !called {
					t.Fatal("HTTP draft was rejected", err)
				}
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatal("draft discovery persisted settings")
				}
				configuration := store.Configuration()
				configuration.Providers = []Provider{{ID: "custom", Name: "HTTP service", AuthMode: "api-key", Protocol: protocol, BaseURL: endpoint, Models: []string{"fixture-model"}}}
				configuration.Tasks["default"] = Selection{Provider: "custom", Model: "fixture-model"}
				if _, err = store.SaveConfiguration(configuration, nil); err != nil {
					t.Fatal("HTTP save failed", err)
				}
				reopened, err := Open(path)
				if err != nil {
					t.Fatal("HTTP reopen failed", err)
				}
				provider, _, err := reopened.ProviderForModels("custom")
				if err != nil || provider.BaseURL != endpoint {
					t.Fatal("saved model connection changed", err)
				}
				for _, task := range []string{"ocr", "translate", "chat"} {
					preferences, _, err := reopened.ForTask(task, &Selection{Provider: "custom", Model: "fixture-model"})
					if err != nil || preferences.BaseURL != endpoint || preferences.Protocol != protocol {
						t.Fatal("HTTP task connection changed", task, err)
					}
				}
			})
		}
	}
}

func TestCustomEndpointStillRejectsInvalidOrCredentialBearingURLs(t *testing.T) {
	for _, endpoint := range []string{
		"", "relay.example:3000/v1", "ftp://relay.example/v1", "file:///v1", "https:///v1",
		"http://user:secret@relay.example:3000/v1", "http://relay.example/v1?key=secret",
		"http://relay.example/v1#fragment", "http://relay.example:invalid/v1",
		"http://relay.example:3000/v1/chat/completions", "http://relay.example:3000/v1/responses/", "http://relay.example:3000/v1/messages",
	} {
		t.Run(endpoint, func(t *testing.T) {
			if _, err := (ModelConnectionDraft{Protocol: "chat", BaseURL: endpoint}).Provider("custom"); err == nil {
				t.Fatal("invalid endpoint accepted")
			}
		})
	}
}
