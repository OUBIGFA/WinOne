package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/yetone/magpie/internal/catalog"
	"github.com/yetone/magpie/internal/provider"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestModelCatalogPreservesVendorCreditRates(t *testing.T) {
	metadata := describeModel(provider.Provider{ID: "winone"}, catalog.Model{ID: "discounted", Rate: 0.5, RateWas: 1}, modelMetadata{})
	raw, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err = json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	if value["rate"] != 0.5 || value["rateWas"] != float64(1) {
		t.Fatalf("vendor credit rates lost: %s", raw)
	}
}

func TestModelCatalogUsesMagpieReportedWindowMaximumAndOutput(t *testing.T) {
	p := provider.Provider{ID: "winone", Contexts: map[string]int{"model": 400000}}
	model := catalog.Model{ID: "model", Name: "Model", Context: 272000, MaxContext: 872000, Output: 64000, Compact: 200000}
	metadata := describeModel(p, model, modelMetadata{CompactionPercent: 72.5, CompactionThreshold: 12345, MaxTokens: 32000})
	if metadata.ContextWindow != 400000 || metadata.ListedContextWindow != 272000 || metadata.MaxContextWindow != 872000 || metadata.MaxTokens != 32000 || metadata.CompactionThreshold != 290000 || metadata.CompactionPercent != 72.5 {
		t.Fatalf("native metadata/percentage precedence lost: %+v", metadata)
	}
	legacy := describeModel(p, model, modelMetadata{CompactionThreshold: 150000})
	if legacy.CompactionThreshold != 150000 || legacy.MaxTokens != 64000 {
		t.Fatalf("legacy/native output changed: %+v", legacy)
	}
	unknown := describeModel(provider.Provider{ID: "winone"}, catalog.Model{ID: "unknown-model-id-fixture"}, modelMetadata{})
	if unknown.MaxContextWindow != 0 || unknown.ListedContextWindow != 0 {
		t.Fatal("unknown model maximum was invented")
	}
}

func TestPercentageMetadataIsAcceptedOnRealGenerationWithoutDroppingHistory(t *testing.T) {
	var received map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer server.Close()
	in := request{Protocol: "chat", BaseURL: server.URL + "/v1", Model: "model-fixture", Messages: []conversationMessage{{Role: "user", Content: "Original history"}, {Role: "assistant", Content: "Earlier response"}, {Role: "user", Content: "Continue"}}, ModelMetadata: modelMetadata{ContextWindow: 256000, MaxTokens: 8000, CompactionPercent: 72.5, ListedContextWindow: 128000, MaxContextWindow: 1000000}}
	encoded, _ := json.Marshal(in)
	if result, err := run(context.Background(), strings.NewReader(string(encoded))); err != nil || result != "ok" {
		t.Fatal(result, err)
	}
	encoded, _ = json.Marshal(received["messages"])
	if !strings.Contains(string(encoded), "Original history") || !strings.Contains(string(encoded), "Earlier response") || !strings.Contains(string(encoded), "Continue") {
		t.Fatal("stateless generation discarded history")
	}
	if received["max_completion_tokens"] != float64(8000) && received["max_tokens"] != float64(8000) {
		t.Fatal("output limit was not sent")
	}
}

func TestServicePresetRetainsItsModelProtocolsAndScope(t *testing.T) {
	var input request
	if err := json.Unmarshal([]byte(`{"preset":"opencode-go","protocol":"chat","baseUrl":"https://opencode.ai/zen/go/v1","key":"fixture-key"}`), &input); err != nil {
		t.Fatal(err)
	}
	configured, err := configuredProvider(input)
	if err != nil {
		t.Fatal(err)
	}
	if configured.Responses != "https://opencode.ai/zen/go/v1" || configured.Anthropic != "https://opencode.ai/zen/go" || configured.Catalog != "opencode-go" {
		t.Fatal("OpenCode lost its model-specific protocols")
	}
	if err := json.Unmarshal([]byte(`{"preset":"clinepass","protocol":"chat","baseUrl":"https://api.cline.bot/api/v1","key":"fixture-key"}`), &input); err != nil {
		t.Fatal(err)
	}
	configured, err = configuredProvider(input)
	if err != nil || configured.Preset != "clinepass" {
		t.Fatal("ClinePass lost its preset model scope", err)
	}
	input.BaseURL = "http://127.0.0.1:1234/v1"
	configured, err = configuredProvider(input)
	if err != nil || configured.Chat != input.BaseURL || configured.Responses != "" || configured.Anthropic != "" {
		t.Fatal("custom endpoint kept unrelated preset destinations", err)
	}
}

func TestModelDiscoveryPreservesSafeFailureMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "90")
		w.WriteHeader(429)
		_, _ = w.Write([]byte(`{"error":"secret model body"}`))
	}))
	defer server.Close()
	_, err := fetchModels(context.Background(), request{Protocol: "chat", BaseURL: server.URL + "/v1", Key: "fixture"})
	var failure *upstreamFailure
	if !errors.As(err, &failure) || failure.Status != 429 || failure.RetryAfterSeconds != 90 {
		t.Fatalf("discovery error=%v", err)
	}
}

func TestConfiguredModelDiscoveryUsesMagpieAndPreservesManualFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" || r.Header.Get("Authorization") != "Bearer fixture-model-key" {
			t.Error("wrong model endpoint or credential")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"remote-a"},{"id":"remote-b"}]}`))
	}))
	defer server.Close()
	models, err := fetchModels(context.Background(), request{Protocol: "chat", BaseURL: server.URL + "/v1", Key: "fixture-model-key"})
	if err != nil || len(models) != 2 {
		t.Fatal(models, err)
	}
	missing := httptest.NewServer(http.NotFoundHandler())
	defer missing.Close()
	if _, err = fetchModels(context.Background(), request{Protocol: "chat", BaseURL: missing.URL + "/v1"}); err == nil {
		t.Fatal("unsupported discovery reported success")
	}
}
