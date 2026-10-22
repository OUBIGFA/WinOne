//go:build windows

package main

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"winone/internal/ai"
)

// This uses the real authenticated public listener and packaged Magpie, not a
// replacement discovery handler. No request leaves the local fixture servers.
func TestOllamaDiscoveryMatchesGatewayModelPolicy(t *testing.T) {
	s := privacyTestService(t)
	var vendorCalls atomic.Int32
	vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vendorCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"Ollama fixture"},"finish_reason":"stop"}]}`)
	}))
	defer vendor.Close()
	store, err := s.magpieStore()
	if err != nil {
		t.Fatal(err)
	}
	config := store.Configuration()
	config.MagpieSettings = &ai.MagpieSettings{NoStats: true, NoUsageStats: true, NoAccountDiscovery: true, FullContext: true}
	vision := true
	config.Providers = []ai.Provider{
		{ID: "one", Name: "Allowed", AuthMode: "api-key", Protocol: "chat", BaseURL: vendor.URL, Models: []string{"gpt-4.1", "hidden"}, ModelMetadata: map[string]ai.ModelMetadata{"gpt-4.1": {ContextWindow: 128000, Vision: &vision}}},
		{ID: "two", Name: "Hidden", AuthMode: "api-key", Protocol: "chat", BaseURL: vendor.URL, Models: []string{"private-model"}},
		{ID: "account", Name: "Private account", AuthMode: "account", Agent: "codex", Models: []string{"gpt-5"}},
	}
	credential := "fixture-key"
	if _, err = store.SaveConfiguration(config, map[string]*string{"one": &credential, "two": &credential}); err != nil {
		t.Fatal(err)
	}
	s.initGateway()
	t.Cleanup(s.gateway.Close)
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	status, err := s.gateway.Save(ai.GatewaySettings{Enabled: true, Port: port, Models: []ai.Selection{{Provider: "one", Model: "gpt-4.1"}}}, false)
	if err != nil {
		t.Fatal(err)
	}
	key, err := s.gateway.Key()
	if err != nil {
		t.Fatal(err)
	}
	base := strings.TrimSuffix(status.BaseURL, "/v1")
	call := func(method, path, body, token, origin string, want int) []byte {
		t.Helper()
		r, err := http.NewRequestWithContext(s.ctx, method, base+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", origin)
		response, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		raw, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != want {
			t.Fatalf("%s %s: %d, want %d: %s", method, path, response.StatusCode, want, raw)
		}
		return raw
	}
	call("GET", "/api/tags", "", "invalid", "", 401)
	call("GET", "/api/tags", "", key, "https://example.invalid", 403)
	call("POST", "/api/version", `{}`, key, "", 405)
	call("GET", "/api/show", "", key, "", 405)
	version := call("GET", "/api/version", "", key, "", 200)
	if !strings.Contains(string(version), `"version"`) {
		t.Fatalf("upstream version lost: %s", version)
	}
	var tags struct {
		Models []struct {
			Name, Model, Digest string
			Capabilities        []string
		}
	}
	if err := json.Unmarshal(call("GET", "/api/tags", "", key, "", 200), &tags); err != nil {
		t.Fatal(err)
	}
	if len(tags.Models) != 1 || tags.Models[0].Model != "one/gpt-4.1" || tags.Models[0].Name != tags.Models[0].Model || len(tags.Models[0].Digest) != 64 {
		t.Fatalf("Ollama exposed a disabled model or lost metadata: %+v", tags)
	}
	for _, body := range []string{`{"model":"one/gpt-4.1"}`, `{"name":"one/gpt-4.1"}`} {
		var shown struct {
			Info         map[string]any `json:"model_info"`
			Capabilities []string
		}
		if err := json.Unmarshal(call("POST", "/api/show", body, key, "", 200), &shown); err != nil {
			t.Fatal(err)
		}
		if shown.Info["magpie.context_length"] != float64(128000) || !strings.Contains(strings.Join(shown.Capabilities, ","), "vision") {
			t.Fatalf("Ollama model capabilities lost: %+v", shown)
		}
	}
	call("POST", "/api/show", `{"model":"two/private-model"}`, key, "", 404)
	call("POST", "/api/show", `{"model":"account/gpt-5"}`, key, "", 404)
	call("POST", "/api/show", `{}`, key, "", 400)
	call("POST", "/api/show", `{`, key, "", 400)
	call("POST", "/api/show", strings.Repeat(" ", (1<<20)+1), key, "", 413)
	call("POST", "/v1/chat/completions", `{"model":"two/private-model","messages":[{"role":"user","content":"hello"}]}`, key, "", 403)
	call("POST", "/v1/chat/completions", `{"model":"one/gpt-4.1","messages":[{"role":"user","content":"hello"}]}`, key, "", 200)
	for _, endpoint := range []string{"/api/chat", "/api/generate"} {
		call("POST", endpoint, `{"model":"one/gpt-4.1","prompt":"hello","messages":[{"role":"user","content":"hello"}],"stream":false}`, "invalid", "", 401)
		call("POST", endpoint, `{"model":"two/private-model","prompt":"hello","stream":false}`, key, "", 403)
		raw := call("POST", endpoint, `{"model":"one/gpt-4.1","prompt":"hello","messages":[{"role":"user","content":"hello"}],"stream":false}`, key, "", 200)
		if !strings.Contains(string(raw), "Ollama fixture") || !strings.Contains(string(raw), `"done":true`) { t.Fatalf("Ollama generation lost its response: %s", raw) }
	}
	if vendorCalls.Load() != 3 {
		t.Fatalf("discovery/denied requests reached vendor: %d", vendorCalls.Load())
	}
	value := status.GatewaySettings
	value.Models = []ai.Selection{}
	if _, err := s.gateway.Save(value, false); err != nil {
		t.Fatal(err)
	}
	if raw := call("GET", "/api/tags", "", key, "", 200); !strings.Contains(string(raw), `"models":[]`) {
		t.Fatalf("empty allowlist reopened models: %s", raw)
	}
	call("POST", "/api/show", `{"model":"one/gpt-4.1"}`, key, "", 404)
}
