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

func TestGatewayModelDescriptionPreservesProviderMetadata(t *testing.T) {
	if os.Getenv("WINONE_NETWORK_TESTS") != "1" {
		t.Skip("network tests are excluded from automatic verification; set WINONE_NETWORK_TESTS=1 to run explicitly")
	}
	executable := os.Getenv("WINONE_AI_TEST_EXE")
	if executable == "" {
		t.Skip("set WINONE_AI_TEST_EXE to test the actual upstream catalog")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := newAIService(ctx, t.TempDir(), filepath.Join(t.TempDir(), "WinOne.Host.exe"))
	s.translator.Executable = executable
	defer s.closeGatewayBackend()
	s.initGateway()
	s.store, _ = ai.Open(s.storePath)
	c := s.store.Configuration()
	c.Providers = []ai.Provider{{ID: "account", Name: "Account", AuthMode: "account", Agent: "codex", Models: []string{"gpt-5"}, ModelMetadata: map[string]ai.ModelMetadata{"gpt-5": {ContextWindow: 1000000, CompactionThreshold: 300000, Efforts: []string{"low", "high"}}}}}
	if _, err := s.store.SaveConfiguration(c, nil); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	s.serveGateway(w, httptest.NewRequest("GET", "/v1/models", nil))
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
	// The ordinary OpenAI list publishes the actual model window. Magpie's
	// client-specific catalogs own compaction; the Host must not replace it.
	if m["context_window"] != float64(1000000) || len(m["supported_reasoning_levels"].([]any)) != 2 {
		t.Fatalf("settings not applied: %s", w.Body.String())
	}
}
