//go:build windows

package main

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"winone/internal/ai"
)

func TestProtocolModelsExposeConfiguredNamesAndMetadata(t *testing.T) {
	executable := os.Getenv("WINONE_AI_TEST_EXE")
	if executable == "" {
		t.Skip("set WINONE_AI_TEST_EXE for the upstream model list")
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	s := newAIService(ctx, t.TempDir(), filepath.Join(t.TempDir(), "WinOne.Host.exe"))
	s.translator.Executable = executable
	defer s.closeGatewayBackend()
	s.initGateway()
	var err error
	s.store, err = ai.Open(s.storePath)
	if err != nil {
		t.Fatal(err)
	}
	config := s.store.Configuration()
	vision := false
	config.Providers = []ai.Provider{{ID: "custom", Name: "Example", AuthMode: "api-key", Protocol: "chat", BaseURL: "https://example.invalid/v1", Models: []string{"model"}, ModelNames: map[string]string{"model": "Readable model"}, ModelMetadata: map[string]ai.ModelMetadata{"model": {ContextWindow: 128000, MaxTokens: 4096, CompactionPercent: 75, Vision: &vision, Tier: "priority"}}}}
	key := "fixture-key"
	if _, err = s.store.SaveConfiguration(config, map[string]*string{"custom": &key}); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	s.serveGateway(w, httptest.NewRequest("GET", "/v1/models", nil))
	var body struct {
		Data []struct {
			Name      string `json:"display_name"`
			Context   int    `json:"context_window"`
			MaxTokens int    `json:"max_output_tokens"`
		} `json:"data"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &body); err != nil || w.Code != 200 || len(body.Data) != 1 {
		t.Fatal(w.Code, w.Body.String(), err)
	}
	model := body.Data[0]
	// Magpie's ordinary model list describes the real model window and uses
	// its own field names. Client-specific compaction stays with that client.
	if model.Name != "Readable model" || model.Context != 128000 || model.MaxTokens != 4096 {
		t.Fatalf("metadata lost: %+v", model)
	}
}
