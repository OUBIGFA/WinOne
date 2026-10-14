//go:build windows

package main

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"winone/internal/ai"
)

func TestProtocolModelsExposeConfiguredNamesAndMetadata(t *testing.T) {
	s := newAIService(context.Background(), t.TempDir(), filepath.Join(t.TempDir(), "WinOne.Host.exe"))
	var err error
	s.store, err = ai.Open(s.storePath)
	if err != nil {
		t.Fatal(err)
	}
	config := s.store.Configuration()
	// This test asserts the configured model name and metadata. Keep the
	// upstream provider suffix disabled so naming policy is not part of the
	// assertion; that policy has its own Magpie settings path.
	config.MagpieSettings.PlainNames = true
	vision := false
	config.Providers = []ai.Provider{{ID: "custom", Name: "Example", AuthMode: "api-key", Protocol: "chat", BaseURL: "https://example.invalid/v1", Models: []string{"model"}, ModelNames: map[string]string{"model": "Readable model"}, ModelMetadata: map[string]ai.ModelMetadata{"model": {ContextWindow: 128000, MaxTokens: 4096, CompactionPercent: 75, Vision: &vision, Tier: "priority"}}}}
	if _, err = s.store.SaveConfiguration(config, nil); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	s.serveAIProtocol(w, httptest.NewRequest("GET", "/v1/models", nil), nil, func(ai.Selection) bool { return true }, false)
	var body struct {
		Data []struct {
			Name      string           `json:"name"`
			Context   int              `json:"context_window"`
			MaxTokens int              `json:"max_tokens"`
			Metadata  ai.ModelMetadata `json:"metadata"`
		} `json:"data"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &body); err != nil || w.Code != 200 || len(body.Data) != 1 {
		t.Fatal(w.Code, w.Body.String(), err)
	}
	model := body.Data[0]
	// A 75% compaction setting exposes 96,000 as the active context and keeps
	// 128,000 as the maximum context in the protocol metadata.
	if model.Name != "Readable model" || model.Context != 96000 || model.MaxTokens != 4096 || model.Metadata.CompactionPercent != 75 || model.Metadata.Vision == nil || *model.Metadata.Vision || model.Metadata.Tier != "priority" {
		t.Fatalf("metadata lost: %+v", model)
	}
}
