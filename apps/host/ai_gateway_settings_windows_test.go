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

func TestGatewayModelDescriptionAppliesMagpieSettings(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := newAIService(ctx, t.TempDir(), filepath.Join(t.TempDir(), "WinOne.Host.exe"))
	s.store, _ = ai.Open(s.storePath)
	c := s.store.Configuration()
	c.MagpieSettings = &ai.MagpieSettings{CodexAgentsV1: true, CodexAutoReview: "account/gpt-5", CompactAt: 300000}
	c.Providers = []ai.Provider{{ID: "account", Name: "Account", AuthMode: "account", Agent: "codex", Models: []string{"gpt-5"}, ModelMetadata: map[string]ai.ModelMetadata{"gpt-5": {ContextWindow: 1000000, Efforts: []string{"low", "high"}}}}}
	if _, err := s.store.SaveConfiguration(c, nil); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	s.serveAIProtocol(w, httptest.NewRequest("GET", "/v1/models", nil), nil, func(ai.Selection) bool { return true }, false)
	var result struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Data) != 1 {
		t.Fatalf("model catalog: %s", w.Body.String())
	}
	m := result.Data[0]
	if m["multi_agent_version"] != "v1" || m["auto_review_model_override"] != "account/gpt-5" || m["context_window"] != float64(300000) {
		t.Fatalf("settings not applied: %s", w.Body.String())
	}
}
