package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/yetone/magpie/internal/catalog"
	"github.com/yetone/magpie/internal/gateway"
	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/settings"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Read the upstream billing cache through its public API; a known free group
// must survive the private slot projection without exposing its fingerprint.
func TestRelayGroupRatesUseOriginalPricingAndPrivateSlots(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	p := provider.Provider{ID: "relay-rate-fixture", Key: "fixture-rate-primary", Keys: []provider.KeyAccount{{Key: "fixture-rate-free"}}, TopUpRate: .0714, PriceRate: .9}
	primary, free := provider.KeyID(p.Key), provider.KeyID(p.Keys[0].Key)
	cache := filepath.Join(filepath.Dir(provider.Path()), "key-rates.json")
	if err := os.MkdirAll(filepath.Dir(cache), 0700); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{
		p.ID + " " + primary: map[string]any{"known": true, "rate": 2.5},
		p.ID + " " + free:    map[string]any{"known": true, "rate": 0},
	})
	if err := os.WriteFile(cache, raw, 0600); err != nil {
		t.Fatal(err)
	}
	rememberGatewayKeySlots(p.ID, request{Keys: []requestKey{{ID: "primary", KeyAccount: provider.KeyAccount{Key: p.Key}}, {ID: "free-slot", KeyAccount: p.Keys[0]}}})
	t.Cleanup(func() { gatewayKeySlots.Delete(p.ID) })
	rows := gatewayKeyRestState(p)
	if len(rows) != 2 || rows[0].ID != "primary" || rows[0].GroupRate == nil || *rows[0].GroupRate != 2.5 || rows[1].ID != "free-slot" || rows[1].GroupRate == nil || *rows[1].GroupRate != 0 {
		t.Fatalf("group rates lost: %+v", rows)
	}
	for key, want := range map[string]float64{primary: .1785, free: 0, "unknown": .9} {
		if got, own := p.KeyPriceRate(key); got != want || own {
			t.Errorf("key %s rate=%g own=%t, want %g", key, got, own, want)
		}
	}
	p.KeyPriceRates = map[string]float64{primary: .3}
	if got, own := p.KeyPriceRate(primary); got != .3 || !own {
		t.Fatalf("explicit rate lost: %g %t", got, own)
	}
	encoded, _ := json.Marshal(rows)
	if bytes.Contains(encoded, []byte(primary)) || bytes.Contains(encoded, []byte("fixture-rate-")) {
		t.Fatalf("private identity leaked: %s", encoded)
	}
}

// Together's top-level array and vendor model types reach the same complete
// catalog used by the native model list; this transport never opens a socket.
func TestTogetherDiscoveryKeepsEmbeddingAndRerankTypes(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Cleanup(func() { provider.WinOneProviders = nil; settings.WinOneSettings.Store(nil) })
	old := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = old })
	http.DefaultClient = &http.Client{Transport: modelBalanceTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.together.xyz" {
			return nil, fmt.Errorf("unexpected external request: %s", r.URL.Host)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`[{"id":"fixture-chat","type":"chat"},{"id":"togethercomputer/m2-bert-80M-32k-retrieval","type":"embedding"},{"id":"Salesforce/Llama-Rank-V1","type":"rerank"},{"id":"fixture-moderation","type":"moderation"}]`))}, nil
	})}
	models, err := fetchModels(t.Context(), request{ProviderID: "together-type-fixture", Protocol: "chat", BaseURL: "https://api.together.xyz/v1", Key: "fixture-catalog-key"})
	if err != nil {
		t.Fatal(err)
	}
	for id, kind := range map[string]string{"fixture-chat": "", "togethercomputer/m2-bert-80M-32k-retrieval": catalog.ServesEmbedding, "Salesforce/Llama-Rank-V1": catalog.ServesRerank} {
		i := slices.IndexFunc(models, func(m modelOption) bool { return m.ID == id })
		if i < 0 || models[i].Metadata.Serves != kind {
			t.Errorf("missing or wrongly typed model %s: %+v", id, models)
		}
	}
	if slices.ContainsFunc(models, func(m modelOption) bool { return m.ID == "fixture-moderation" }) {
		t.Fatal("unsupported moderation model was exposed")
	}
}

// This exercises a real gateway refusal. Its private key-state API must
// identify Host slots and lift the same upstream rest without leaking keys.
func TestGatewayKeyRestExplainsRefusalAndRetriesItsSlot(t *testing.T) {
	if os.Getenv("WINONE_NETWORK_TESTS") != "1" {
		t.Skip("explicit isolated gateway HTTP verification")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Cleanup(func() { provider.WinOneProviders = nil; settings.WinOneSettings.Store(nil) })
	vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		io.WriteString(w, `{"error":{"message":"credit balance is too low; fixture-secret-first"}}`)
	}))
	defer vendor.Close()
	if err := applyRuntimeSettings(request{}); err != nil {
		t.Fatal(err)
	}
	source := request{ProviderID: "rests-fixture", Protocol: "chat", BaseURL: vendor.URL + "/v1", Keys: []requestKey{{ID: "primary", KeyAccount: provider.KeyAccount{Key: "fixture-secret-first"}}, {ID: "second", KeyAccount: provider.KeyAccount{Key: "fixture-secret-second"}}}}
	if err := installAuxiliaryProviders(request{AuxiliaryProviders: []auxiliaryProvider{{Provider: provider.Provider{ID: "rests-fixture", Models: []string{"model"}}, NativeSource: &source}}}); err != nil {
		t.Fatal(err)
	}
	gw := gateway.New()
	p, _ := provider.Find("rests-fixture")
	for _, k := range p.KeyList() {
		key := p.RestKey(k)
		t.Cleanup(func() { gw.UnrestAll(key) })
	}
	turn := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"rests-fixture/model","messages":[{"role":"user","content":"test"}]}`))
	turn.Header.Set("Authorization", "Bearer "+gateway.Token)
	turn.Header.Set("Content-Type", "application/json")
	gw.Handler().ServeHTTP(httptest.NewRecorder(), turn)
	handler := gatewayAdminHandler(gw)
	read := httptest.NewRecorder()
	handler.ServeHTTP(read, httptest.NewRequest("GET", "/api/winone/providers/rests-fixture/key-rests", nil))
	if read.Code != 200 {
		t.Fatalf("key-state API unavailable: %d %s", read.Code, read.Body.String())
	}
	var state struct {
		Keys []struct {
			ID   string
			Rest *provider.KeyRest
		}
	}
	if err := json.Unmarshal(read.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if len(state.Keys) != 2 || state.Keys[0].ID != "primary" || state.Keys[0].Rest == nil || !strings.Contains(state.Keys[0].Rest.Error, "credit balance is too low") || strings.Contains(read.Body.String(), "fixture-secret-") {
		t.Fatalf("rest or protected slot mapping lost: %s", read.Body.String())
	}
	retry := httptest.NewRecorder()
	handler.ServeHTTP(retry, httptest.NewRequest("POST", "/api/winone/providers/rests-fixture/key-rests/retry", strings.NewReader(`{"id":"primary"}`)))
	if retry.Code != 200 {
		t.Fatal(retry.Code, retry.Body.String())
	}
	if _, resting := gateway.RestOf(p.RestKey(p.KeyList()[0])); resting {
		t.Fatal("retry did not lift the original gateway rest")
	}
}

// Plan media are explicit upstream catalogs, separate from a vendor's chat
// /models response. They must remain selectable on WinOne's gateway too.
func TestTokenPlanDiscoveryIncludesSeparateMediaModels(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Cleanup(func() { provider.WinOneProviders = nil; settings.WinOneSettings.Store(nil) })
	oldClient := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = oldClient })
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"data":[{"id":"qwen4-max"}]}`)
	})
	http.DefaultClient = &http.Client{Transport: modelBalanceTransport(func(r *http.Request) (*http.Response, error) {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w.Result(), nil
	})}
	for _, preset := range []string{"qwen-token-plan", "bailian-token-plan", "alibaba-token-plan", "qwencloud-token-plan"} {
		models, err := fetchModels(t.Context(), request{ProviderID: "media-" + preset, Protocol: "chat", BaseURL: "https://plan-fixture.invalid/v1", Preset: preset, Key: "fixture-plan-key"})
		if err != nil {
			t.Fatal(preset, err)
		}
		for _, id := range []string{"qwen4-max", "qwen-image-3.0-pro", "wan2.7-image", "wan3.0-video", "happyhorse-1.1-t2v", "qwen-audio-3.0-tts-plus", "qwen-audio-3.0-asr-flash"} {
			if !slices.ContainsFunc(models, func(m modelOption) bool { return m.ID == id }) {
				t.Errorf("%s omits selectable %s", preset, id)
			}
		}
	}
}

// Default verification exercises the actual slot mapping and credential
// protection without listening on a port or contacting any provider.
func TestGatewayKeyRestMappingAndVendorWordsStayPrivate(t *testing.T) {
	in := request{Keys: []requestKey{{ID: "primary", KeyAccount: provider.KeyAccount{Key: "first-private-secret"}}, {ID: "second", KeyAccount: provider.KeyAccount{Key: "second-private-secret"}}}}
	p := provider.Provider{ID: "private-key-state", Key: "first-private-secret", Keys: []provider.KeyAccount{{Key: "second-private-secret"}}}
	rememberGatewayKeySlots(p.ID, in)
	t.Cleanup(func() { gatewayKeySlots.Delete(p.ID) })
	rows := gatewayKeyRestState(p)
	if len(rows) != 2 || rows[0].ID != "primary" || rows[1].ID != "second" {
		t.Fatalf("private slot identity lost: %+v", rows)
	}
	r := protectedKeyRest(p, gateway.Rest{Why: "credit", By: "credit", Status: 400, Error: "credit balance too low for first-private-secret and second-private-secret"})
	raw, _ := json.Marshal(r)
	if !strings.Contains(r.Error, "credit balance too low") || strings.Contains(string(raw), "private-secret") || r.Status != 400 || r.By != "credit" {
		t.Fatalf("vendor explanation lost or leaked a key: %s", raw)
	}
}

// Normalizing private bridge IDs must apply the upstream Devin migration,
// without widening per-account model restrictions.
func TestPrivateDevinBridgeDoesNotExposeRetiredRouters(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Cleanup(func() { provider.WinOneProviders = nil; settings.WinOneSettings.Store(nil) })
	if err := applyRuntimeSettings(request{}); err != nil {
		t.Fatal(err)
	}
	in := request{AuxiliaryProviders: []auxiliaryProvider{{Provider: provider.Provider{ID: "private-devin", Chat: "http://127.0.0.1:9/v1", Models: []string{"Adaptive", "swe-2", "fusion"}}, AccountKind: "devin"}}}
	if err := installAuxiliaryProviders(in); err != nil {
		t.Fatal(err)
	}
	p, err := provider.Find("private-devin")
	if err != nil || !slices.Equal(p.Models, []string{"swe-2"}) {
		t.Fatalf("private bridge retains retired routers: %+v %v", p, err)
	}
}

// The provider's GUI and gateway use OutputWithin when publishing a model.
// In particular, models.dev's output can exceed the vendor's own context.
func TestDiscoveredModelUsesTheUpstreamPublishedOutput(t *testing.T) {
	p := provider.Provider{ID: "fixture"}
	m := catalog.Model{ID: "fixture-model", Context: 128000, Output: 384000}
	got := describeModel(p, m, modelMetadata{})
	if got.ContextWindow != 128000 || got.MaxTokens != 128000 {
		t.Fatalf("discovered model differs from the upstream published pair: %+v", got)
	}
	if m.Output != 384000 {
		t.Fatal("publishing a model changed its original request limit")
	}
}

func TestModelSystemPromptAndKindUseUpstreamPreferences(t *testing.T) {
	if os.Getenv("WINONE_NETWORK_TESTS") != "1" {
		t.Skip("network tests are excluded from automatic verification; set WINONE_NETWORK_TESTS=1 to run explicitly")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Cleanup(func() { provider.WinOneProviders = nil; settings.WinOneSettings.Store(nil) })
	seen := false
	vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		if !bytes.Contains(body, []byte("caller-instructions")) || !bytes.Contains(body, []byte("model-instructions")) {
			t.Errorf("system prompt lost: %s", body)
		}
		seen = true
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`)
	}))
	defer vendor.Close()
	in := request{ProviderID: "fixture", Protocol: "chat", BaseURL: vendor.URL + "/v1", Model: "text-model", Text: "hello", System: "caller-instructions", ModelMetadata: modelMetadata{SystemPrompt: "model-instructions"}}
	raw, _ := json.Marshal(in)
	if text, err := run(t.Context(), bytes.NewReader(raw)); err != nil || text != "ok" || !seen {
		t.Fatal(text, err)
	}
	provider.WinOneProviders[0].Models = append(provider.WinOneProviders[0].Models, "odd-media-id")
	provider.Changed()
	if err := installModelPreferences("fixture", "odd-media-id", modelMetadata{Kind: provider.KindVideo}); err != nil {
		t.Fatal(err)
	}
	if p, err := provider.Find("fixture"); err != nil || p.ModelKind("odd-media-id") != provider.KindVideo {
		t.Fatal("upstream video-model classification was not installed", err)
	}
	if err := installModelPreferences("fixture", "odd-media-id", modelMetadata{}); err != nil {
		t.Fatal(err)
	}
	if settings.Load().ModelKinds["fixture/odd-media-id"] != "" {
		t.Fatal("restoring defaults kept the media override")
	}
}

func TestConfiguredProviderAcceptsHTTPHostsAcrossProtocols(t *testing.T) {
	for _, endpoint := range []string{"http://relay.example:3000/v1", "http://203.0.113.7:3000/v1", "http://192.168.50.10:3000/v1", "http://[2001:db8::7]:3000/v1", "https://relay.example/v1"} {
		for _, protocol := range []string{"chat", "responses", "anthropic", "gemini", "decide"} {
			p, err := configuredProvider(request{BaseURL: endpoint, Protocol: protocol, Key: "fixture-key"})
			if err != nil {
				t.Fatalf("%s %s: %v", protocol, endpoint, err)
			}
			actual := map[string]string{"chat": p.Chat, "responses": p.Responses, "anthropic": p.Anthropic, "gemini": p.Gemini, "decide": p.Decide}[protocol]
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

func TestCompleteEndpointsAndKeyOptionsReachUpstreamProvider(t *testing.T) {
	p, err := configuredProvider(request{Protocol: "chat", BaseURL: "https://primary.invalid/v1", Key: "fixture",
		KeyID: "saved-slot", KeyOptions: &KeyOptions{Name: "Team", Protocol: "anthropic", Weight: 4},
		ProviderPreferences: &ProviderPreferences{Endpoints: &ProviderEndpoints{Responses: "https://response.invalid/v1", Anthropic: "https://messages.invalid", Gemini: "https://gemini.invalid/v1beta", Decide: "https://decision.invalid"}, ListOff: true, NativeCompaction: true, PinUpstream: true, KeySpendCaps: map[string]SpendCap{"saved-slot": {USD: 2, Period: "day"}}, KeyPriceRates: map[string]float64{"saved-slot": .5}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.Chat != "https://primary.invalid/v1" || p.Gemini != "https://gemini.invalid/v1beta" || p.Decide != "https://decision.invalid" || p.Responses != "https://response.invalid/v1" || !p.ListOff || !p.NativeCompaction || !p.PinUpstream {
		t.Fatal("provider options not applied", p)
	}
	if p.KeyName != "Team" || p.KeyProtocol != provider.Anthropic || p.KeyWeight != 4 || p.KeySpendCaps[provider.KeyID("fixture")].USD != 2 || p.KeyPriceRates[provider.KeyID("fixture")] != .5 {
		t.Fatal("key options or cap identity lost")
	}
}

func TestModelHeadersReachGenerationButNotDiscovery(t *testing.T) {
	if os.Getenv("WINONE_NETWORK_TESTS") != "1" {
		t.Skip("network tests are excluded from automatic verification; set WINONE_NETWORK_TESTS=1 to run explicitly")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Cleanup(func() { provider.WinOneProviders = nil; settings.WinOneSettings.Store(nil) })
	var paths []string
	vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/models" {
			if r.Header.Get("X-Route") != "provider" || r.Header.Get("X-Model") != "" {
				t.Error("model header leaked into discovery")
			}
			io.WriteString(w, `{"data":[{"id":"header-model"}]}`)
			return
		}
		if r.Header.Get("X-Route") != "model" || r.Header.Get("X-Model") != "header-model" {
			t.Error("generation lost model header precedence")
		}
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`)
	}))
	defer vendor.Close()
	in := request{Protocol: "chat", BaseURL: vendor.URL + "/v1", Model: "header-model", Text: "hello", Headers: map[string]string{"X-Route": "provider"}, ModelMetadata: modelMetadata{Headers: map[string]string{"X-Route": "model", "X-Model": "header-model"}}}
	if _, err := fetchModels(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(in)
	if result, err := run(context.Background(), bytes.NewReader(data)); err != nil || result != "ok" {
		t.Fatal(result, err)
	}
	if len(paths) < 2 {
		t.Fatal("discovery and generation did not reach the server")
	}
}

func TestHTTPDiscoveryAndGenerationBeyondTheOldLoopbackAllowlist(t *testing.T) {
	if os.Getenv("WINONE_NETWORK_TESTS") != "1" {
		t.Skip("network tests are excluded from automatic verification; set WINONE_NETWORK_TESTS=1 to run explicitly")
	}
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
	if os.Getenv("WINONE_NETWORK_TESTS") != "1" {
		t.Skip("network tests are excluded from automatic verification; set WINONE_NETWORK_TESTS=1 to run explicitly")
	}
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

type modelBalanceTransport func(*http.Request) (*http.Response, error)

func (f modelBalanceTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSavedKeyBalancesKeepVendorSplitAndRedactFailures(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	provider.ForgetBalances()
	previousClient, previousProviders := http.DefaultClient, provider.WinOneProviders
	t.Cleanup(func() { http.DefaultClient = previousClient; provider.WinOneProviders = previousProviders })
	http.DefaultClient = &http.Client{Transport: modelBalanceTransport(func(r *http.Request) (*http.Response, error) {
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
	if os.Getenv("WINONE_NETWORK_TESTS") != "1" {
		t.Skip("network tests are excluded from automatic verification; set WINONE_NETWORK_TESTS=1 to run explicitly")
	}
	var received map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer server.Close()
	in := request{Protocol: "chat", BaseURL: server.URL + "/v1", Model: "model-fixture", MaxTokens: 16000, Messages: []conversationMessage{{Role: "user", Content: "Original history"}, {Role: "assistant", Content: "Earlier response"}, {Role: "user", Content: "Continue"}}, ModelMetadata: modelMetadata{ContextWindow: 256000, MaxTokens: 8000, CompactionPercent: 72.5, ListedContextWindow: 128000, MaxContextWindow: 1000000}}
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
	if configured.Chat != "https://opencode.ai/inference/go/openai/v1" || configured.Responses != "https://opencode.ai/inference/go/openai/v1" || configured.Anthropic != "https://opencode.ai/inference/go/anthropic" || configured.Gemini != "https://opencode.ai/inference/go/google/v1beta" || configured.Catalog != "opencode-go" {
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
	if os.Getenv("WINONE_NETWORK_TESTS") != "1" {
		t.Skip("network tests are excluded from automatic verification; set WINONE_NETWORK_TESTS=1 to run explicitly")
	}
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
	if os.Getenv("WINONE_NETWORK_TESTS") != "1" {
		t.Skip("network tests are excluded from automatic verification; set WINONE_NETWORK_TESTS=1 to run explicitly")
	}
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

func TestDescribedModelKeepsVendorLevelsAndReasonlessUsesUpstream(t *testing.T) {
	metadata := describeModel(provider.Provider{ID: "winone"}, catalog.Model{ID: "mimo-v2.5-free", Efforts: []string{"light", "balanced", "deep"}}, modelMetadata{})
	if !slices.Equal(metadata.Levels, []string{"light", "balanced", "deep"}) || !slices.Equal(metadata.Efforts, metadata.Levels) || metadata.Reasoning == nil || !*metadata.Reasoning {
		t.Fatalf("vendor levels lost: %+v", metadata)
	}
	prior := settings.Load()
	t.Cleanup(func() { settings.WinOneUse(prior) })
	off := false
	if err := installModelPreferences("winone", "mimo-v2.5-free", modelMetadata{Levels: metadata.Levels, Efforts: []string{"deep"}, Reasoning: &off}); err != nil {
		t.Fatal(err)
	}
	s := settings.Load()
	if !s.ModelReasonless["winone/mimo-v2.5-free"] || s.ModelEfforts["winone/mimo-v2.5-free"] != nil {
		t.Fatalf("upstream preferences not installed: reasonless=%v efforts=%v", s.ModelReasonless, s.ModelEfforts)
	}
	if err := installModelPreferences("winone", "mimo-v2.5-free", modelMetadata{Levels: metadata.Levels, Efforts: metadata.Levels}); err != nil {
		t.Fatal(err)
	}
	if s = settings.Load(); s.ModelReasonless["winone/mimo-v2.5-free"] || s.ModelEfforts["winone/mimo-v2.5-free"] != nil {
		t.Fatalf("restored model kept narrowed preferences: reasonless=%v efforts=%v", s.ModelReasonless, s.ModelEfforts)
	}
}
