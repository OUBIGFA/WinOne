package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/yetone/magpie/internal/catalog"
	"github.com/yetone/magpie/internal/provider"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestConfiguredProviderAcceptsHTTPHostsAcrossProtocols(t *testing.T) {
	for _, endpoint := range []string{"http://relay.example:3000/v1", "http://203.0.113.7:3000/v1", "http://192.168.50.10:3000/v1", "http://[2001:db8::7]:3000/v1", "https://relay.example/v1"} {
		for _, protocol := range []string{"chat", "responses", "anthropic"} {
			p, err := configuredProvider(request{BaseURL: endpoint, Protocol: protocol, Key: "fixture-key"})
			if err != nil {
				t.Fatalf("%s %s: %v", protocol, endpoint, err)
			}
			actual := map[string]string{"chat": p.Chat, "responses": p.Responses, "anthropic": p.Anthropic}[protocol]
			if actual != endpoint {
				t.Fatal("explicit endpoint was changed")
			}
		}
	}
	for _, endpoint := range []string{"ftp://relay.example/v1", "https:///v1", "http://user:secret@relay.example/v1", "http://relay.example/v1?key=secret", "http://relay.example/v1#fragment"} {
		if _, err := configuredProvider(request{BaseURL: endpoint, Protocol: "chat"}); err == nil {
			t.Fatal("invalid endpoint accepted")
		}
	}
}

func TestHTTPDiscoveryAndGenerationBeyondTheOldLoopbackAllowlist(t *testing.T) {
	// A second loopback address exercises the old exact-host rejection without
	// listening on the LAN or calling an external provider during default tests.
	listener, err := net.Listen("tcp", "127.0.0.2:0")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fixture-http-key" {
			t.Error("key was not forwarded")
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/models":
			io.WriteString(w, `{"data":[{"id":"fixture-http-model"}]}`)
		case "/v1/chat/completions":
			io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"HTTP_OK"},"finish_reason":"stop"}]}`)
		default:
			t.Error("base URL was not preserved", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	server.Listener.Close()
	server.Listener = listener
	server.Start()
	defer server.Close()
	input := request{Protocol: "chat", BaseURL: server.URL + "/v1", Key: "fixture-http-key", Model: "fixture-http-model", Text: "fixture request"}
	models, err := fetchModels(context.Background(), input)
	if err != nil || len(models) != 1 || models[0].ID != input.Model {
		t.Fatal("HTTP model discovery failed", err)
	}
	encoded, _ := json.Marshal(input)
	text, err := run(context.Background(), bytes.NewReader(encoded))
	if err != nil || text != "HTTP_OK" {
		t.Fatal("HTTP generation failed", err)
	}
}

func TestDiscoveryPreserves2500Models(t *testing.T) {
	models := make([]map[string]string, 2500)
	for i := range models {
		models[i] = map[string]string{"id": fmt.Sprintf("fixture-%04d", i)}
	}
	vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": models})
	}))
	defer vendor.Close()
	got, err := fetchModels(context.Background(), request{Protocol: "chat", BaseURL: vendor.URL + "/v1"})
	if err != nil || len(got) != 2500 {
		t.Fatalf("large discovery lost models: count=%d error=%v", len(got), err)
	}
	if got[2499].ID != "fixture-2499" {
		t.Fatal("last model missing")
	}
}

func TestSavedKeyBalancesKeepVendorSplitAndRedactFailures(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	provider.ForgetBalances()
	previousClient, previousProviders := http.DefaultClient, provider.WinOneProviders
	t.Cleanup(func() { http.DefaultClient = previousClient; provider.WinOneProviders = previousProviders })
	http.DefaultClient = &http.Client{Transport: usagePriceTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.deepseek.com" || r.URL.Path != "/user/balance" {
			t.Errorf("unexpected balance endpoint: %s", r.URL.Host)
		}
		if r.Header.Get("Authorization") != "Bearer fixture-primary-secret" {
			return &http.Response{StatusCode: 401, Status: "401 Unauthorized", Body: io.NopCloser(strings.NewReader(`{"error":"fixture-secondary-secret must never appear"}`)), Header: make(http.Header)}, nil
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"is_available":true,"balance_infos":[{"currency":"CNY","total_balance":"12.00","granted_balance":"12.00","topped_up_balance":"0.00"}]}`)), Header: make(http.Header)}, nil
	})}
	var out bytes.Buffer
	err := readKeyBalances(context.Background(), request{ProviderID: "saved", Protocol: "chat", BaseURL: "https://api.deepseek.com/v1", BalanceKeys: []provider.KeyAccount{{Name: "first", Key: "fixture-primary-secret"}, {Name: "second", Key: "fixture-secondary-secret"}}}, &out)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Quotas []provider.SubscriptionQuota `json:"quotas"`
	}
	if json.Unmarshal(out.Bytes(), &result) != nil || len(result.Quotas) != 2 {
		t.Fatalf("invalid balance result: %s", out.String())
	}
	if result.Quotas[0].Provider != "saved" || len(result.Quotas[0].BalanceSplit) != 2 || result.Quotas[0].BalanceSplit[1].Text != "¥0.00" {
		t.Fatalf("split lost: %+v", result.Quotas[0])
	}
	if result.Quotas[1].Error == "" || strings.Contains(out.String(), "fixture-") {
		t.Fatalf("failure or credentials leaked: %s", out.String())
	}
}

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

func TestModelCatalogPreservesVendorTiersEffortsAndPrices(t *testing.T) {
	model := catalog.Model{ID: "offered", Tiers: []string{"priority", "ultrafast"}, Efforts: []string{"low", "high"}, Price: &catalog.Price{Input: 2, Output: 8}}
	metadata := describeModel(provider.Provider{ID: "winone"}, model, modelMetadata{})
	raw, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err = json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	if len(value["tiers"].([]any)) != 2 || len(value["efforts"].([]any)) != 2 || value["price"].(map[string]any)["input"] != float64(2) {
		t.Fatalf("vendor model capabilities lost: %s", raw)
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
