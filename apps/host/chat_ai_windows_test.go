//go:build windows

package main

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"winone/internal/ai"
)

func TestChatUsesEnabledModelsIndependentlyOfPublicGateway(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := newAIService(ctx, t.TempDir(), filepath.Join(t.TempDir(), "WinOne.Host.exe"))
	s.initGateway()
	s.store, _ = ai.Open(s.storePath)
	config := s.store.Configuration()
	config.Providers = []ai.Provider{{ID: "custom", Name: "Custom", AuthMode: "api-key", Protocol: "chat", BaseURL: "https://example.com/v1", Models: []string{"enabled"}}}
	key := "fixture-key"
	if _, err := s.store.SaveConfiguration(config, map[string]*string{"custom": &key}); err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(s.gateway.Status())
	out := httptest.NewRecorder()
	s.serveChat(out, httptest.NewRequest("GET", "/v1/models", nil))
	if out.Code != 200 || !strings.Contains(out.Body.String(), "custom/enabled") {
		t.Fatal(out.Code, out.Body.String())
	}
	invalid := httptest.NewRecorder()
	s.serveChat(invalid, httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"custom/disabled"}`)))
	if invalid.Code != 400 {
		t.Fatal("disabled model accepted", invalid.Code)
	}
	after, _ := json.Marshal(s.gateway.Status())
	if string(before) != string(after) || s.gateway.Status().Running {
		t.Fatal("Chat changed public gateway settings")
	}
}
