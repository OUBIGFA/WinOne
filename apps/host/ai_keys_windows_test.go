//go:build windows

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"winone/internal/ai"
	"winone/internal/host"
	"winone/internal/settings"
)

func TestProviderKeyRevealIsRepeatableReadOnlyAndScoped(t *testing.T) {
	directory := t.TempDir()
	config, err := settings.Open(filepath.Join(directory, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	service := host.New(config)
	engine := newAIService(context.Background(), directory, filepath.Join(directory, "WinOne.Host.exe"))
	bindAIService(service, engine)
	store, err := ai.Open(engine.storePath)
	if err != nil {
		t.Fatal(err)
	}
	input := store.Configuration()
	input.Providers = []ai.Provider{
		{ID: "one", Name: "Custom", AuthMode: "api-key", Protocol: "chat", BaseURL: "https://example.invalid/v1", Models: []string{"model"}, Keys: []ai.KeySlot{{ID: "first", Enabled: true}, {ID: "second", Enabled: false}, {ID: "empty", Enabled: true}}},
		{ID: "account", Name: "Account", AuthMode: "account", Agent: "codex"},
	}
	first, second := "fixture-reveal-first", "fixture-reveal-second"
	if _, err = store.SaveConfigurationWithKeys(input, nil, ai.CredentialUpdates{"one": {"first": &first, "second": &second}}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(engine.storePath)
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		for _, prefix := range []string{"ai.", "capture.ai."} {
			for id, expected := range map[string]string{"first": first, "second": second} {
				params, _ := json.Marshal(map[string]string{"provider": "one", "id": id})
				response := service.Handle(host.Request{Version: 1, ID: "reveal", Method: prefix + "keys.reveal", Params: params})
				var value struct {
					Key string `json:"key"`
				}
				if response.Error != nil {
					t.Fatal(response.Error)
				}
				if json.Unmarshal(response.Result, &value) != nil || value.Key != expected {
					t.Fatal("explicit reveal returned the wrong key")
				}
			}
			response := service.Handle(host.Request{Version: 1, ID: "public", Method: prefix + "configuration.get"})
			if response.Error != nil || strings.Contains(string(response.Result), first) || strings.Contains(string(response.Result), second) {
				t.Fatal("public configuration leaked a secret", response.Error)
			}
		}
	}
	for _, params := range []string{`{}`, `{"provider":"one"}`, `{"provider":"missing","id":"first"}`, `{"provider":"account","id":"first"}`, `{"provider":"one","id":"missing"}`, `{"provider":"one","id":"empty"}`, `{"provider":"one","id":"first","extra":true}`} {
		response := engine.handle(host.Request{ID: "invalid", Method: "keys.reveal", Params: json.RawMessage(params)})
		if response.Error == nil || strings.Contains(response.Error.Message, first) || len(response.Result) > 0 {
			t.Fatal("invalid key lookup was accepted or leaked a key")
		}
	}
	after, err := os.ReadFile(engine.storePath)
	if err != nil || !bytes.Equal(before, after) || bytes.Contains(after, []byte(first)) {
		t.Fatal("reveal changed persistence or stored plaintext")
	}
	_, plan, err := engine.store.ProviderKeysForModels("one")
	if err != nil {
		t.Fatal(err)
	}
	if err = engine.store.WithKeys(context.Background(), plan, func(_ context.Context, key string) (bool, error) {
		if key != first {
			t.Fatal("viewing keys changed request selection")
		}
		return false, nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestProviderKeyRevealSupportsUnmigratedLegacyCredential(t *testing.T) {
	engine := newAIService(context.Background(), t.TempDir(), "WinOne.Host.exe")
	store, err := ai.Open(engine.storePath)
	if err != nil {
		t.Fatal(err)
	}
	preferences := store.Snapshot()
	preferences.Engine, preferences.Model, preferences.BaseURL = "llm", "fixture-model", "https://example.invalid/v1"
	key := "fixture-legacy-reveal"
	if _, err = store.Save(preferences, &key); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		response := engine.handle(host.Request{ID: "legacy", Method: "keys.reveal", Params: json.RawMessage(`{"provider":"legacy","id":"primary"}`)})
		var value struct {
			Key string `json:"key"`
		}
		if response.Error != nil || json.Unmarshal(response.Result, &value) != nil || value.Key != key {
			t.Fatal("legacy key cannot be viewed", response.Error)
		}
	}
}
