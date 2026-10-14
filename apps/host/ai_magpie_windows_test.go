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

func TestModelConnectivityRejectsInvalidDraftsWithoutChangingSettings(t *testing.T) {
	engine := newAIService(context.Background(), t.TempDir(), "WinOne.Host.exe")
	store, err := ai.Open(engine.storePath)
	if err != nil {
		t.Fatal(err)
	}
	engine.store = store
	initial := store.Configuration()
	initial.Providers = []ai.Provider{{ID: "saved", Name: "Saved", AuthMode: "api-key", Protocol: "chat", BaseURL: "https://example.invalid/v1", Models: []string{"model"}}}
	if _, err = store.SaveConfiguration(initial, nil); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(engine.storePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, params := range []string{
		`{"provider":"saved","models":["model"],"as":"unsupported"}`,
		`{"provider":"saved","models":["model"],"draft":{"agent":"codex"},"connection":{"protocol":"chat","baseUrl":"https://example.invalid/v1"}}`,
		`{"provider":"saved","models":["model"],"keyUpdates":{"primary":"draft-key"}}`,
		`{"provider":"saved","models":["model"],"metadata":{"model":{"maxTokens":-1}}}`,
	} {
		response := engine.handle(host.Request{ID: "test", Method: "models.test", Params: json.RawMessage(params)})
		if response.Error == nil || strings.Contains(response.Error.Message, "不支持的 AI 操作") {
			t.Fatalf("invalid test request was not validated: %+v", response)
		}
	}
	after, err := os.ReadFile(engine.storePath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("model test changed the saved settings")
	}
}

func TestMagpieSearchKeyIsRevealedOnlyByExplicitMethod(t *testing.T) {
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
	input.MagpieSettings = &ai.MagpieSettings{NoStats: true, NoUsageStats: true, SearchAPIs: []ai.SearchAPI{{Vendor: "tavily", Key: "private-search-fixture"}}}
	if _, err = store.SaveConfiguration(input, nil); err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"ai.configuration.get", "capture.ai.configuration.get"} {
		response := service.Handle(host.Request{Version: 1, ID: "public", Method: method})
		if response.Error != nil || strings.Contains(string(response.Result), "private-search-fixture") {
			t.Fatalf("public preferences leaked a search key: %v", response.Error)
		}
	}
	for _, method := range []string{"ai.magpie.search-key", "capture.ai.magpie.search-key"} {
		response := service.Handle(host.Request{Version: 1, ID: "reveal", Method: method, Params: json.RawMessage(`{"vendor":"tavily"}`)})
		var value struct {
			Key string `json:"key"`
		}
		if response.Error != nil {
			t.Fatal(response.Error)
		}
		if err = json.Unmarshal(response.Result, &value); err != nil || value.Key != "private-search-fixture" {
			t.Fatal("explicit reveal did not return the stored key")
		}
	}
	for _, params := range []string{`{"vendor":"unknown"}`, `{"vendor":"tavily","extra":true}`, `{}`} {
		response := engine.handle(host.Request{ID: "invalid", Method: "magpie.search-key", Params: json.RawMessage(params)})
		if response.Error == nil {
			t.Fatalf("invalid search key request accepted: %s", params)
		}
	}
}

func TestMagpieCatalogFailureIsExplicitAndDoesNotRewriteSettings(t *testing.T) {
	directory := t.TempDir()
	engine := newAIService(context.Background(), directory, filepath.Join(directory, "WinOne.Host.exe"))
	if err := os.WriteFile(engine.storePath, []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	response := engine.handle(host.Request{ID: "catalog", Method: "magpie.catalog", Params: json.RawMessage(`{}`)})
	if response.Error == nil || !strings.Contains(response.Error.Message, "设置") {
		t.Fatalf("broken settings were masked: %+v", response)
	}
	data, err := os.ReadFile(engine.storePath)
	if err != nil || string(data) != "invalid" {
		t.Fatal("catalog rewrote invalid settings")
	}
}
