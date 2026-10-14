package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/klauspost/compress/zstd"
	"github.com/yetone/magpie/internal/provider"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// This fails if the gateway loses the provider/model, prompt, key or response.
func TestModelSettingsReachRealUpstreamRequest(t *testing.T) {
	called := false
	vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["max_completion_tokens"] != float64(2048) && body["max_tokens"] != float64(2048) {
			t.Errorf("output limit lost: %v", body)
		}
		if body["reasoning_effort"] != "high" || body["service_tier"] != "priority" {
			t.Errorf("model choices lost: %v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"ok"}}]}`)
	}))
	defer vendor.Close()
	raw := fmt.Sprintf(`{"protocol":"chat","baseUrl":%q,"model":"gpt-5","text":"hello","modelMetadata":{"contextWindow":128000,"maxTokens":2048,"reasoningEffort":"high","tier":"priority"}}`, vendor.URL+"/v1")
	got, err := run(context.Background(), strings.NewReader(raw))
	if err != nil || got != "ok" || !called {
		t.Fatal(got, called, err)
	}
}

func TestTranslationThroughMagpie(t *testing.T) {
	vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer test-only" {
			t.Errorf("wrong upstream request: %s", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["model"] != "model-test" {
			t.Errorf("wrong model: %v", body["model"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"test","object":"chat.completion","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"你好"}}]}`))
	}))
	defer vendor.Close()
	raw, _ := json.Marshal(request{Protocol: "chat", BaseURL: vendor.URL + "/v1", Key: "test-only", Model: "model-test", System: "Translate to Chinese.", Text: "Hello"})
	actual, err := run(context.Background(), strings.NewReader(string(raw)))
	if err != nil || actual != "你好" {
		t.Fatalf("result %q: %v", actual, err)
	}
}

func TestCustomHeadersApplyToModelDiscoveryAndGeneration(t *testing.T) {
	calls := 0
	vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("X-Team") != "fixture" || r.Header.Get("Authorization") != "Custom fixture" {
			t.Error("custom headers were lost")
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/models" {
			fmt.Fprint(w, `{"data":[{"id":"model-test"}]}`)
			return
		}
		fmt.Fprint(w, `{"choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"ok"}}]}`)
	}))
	defer vendor.Close()
	in := request{Protocol: "chat", BaseURL: vendor.URL + "/v1", Key: "default-key", Model: "model-test", Text: "hello", Headers: map[string]string{"X-Team": "fixture", "Authorization": "Custom fixture"}}
	models, err := fetchModels(context.Background(), in)
	if err != nil || len(models) != 1 {
		t.Fatal(models, err)
	}
	raw, _ := json.Marshal(in)
	reply, err := run(context.Background(), strings.NewReader(string(raw)))
	if err != nil || reply != "ok" || calls != 2 {
		t.Fatal(reply, calls, err)
	}
}

func TestRejectRemotePlainHTTP(t *testing.T) {
	_, err := run(context.Background(), strings.NewReader(`{"protocol":"chat","baseUrl":"http://example.com/v1","model":"test","text":"Hello"}`))
	if err == nil {
		t.Fatal("remote plaintext endpoint accepted")
	}
}

func TestImageRecognitionUsesSharedProviderModel(t *testing.T) {
	const png = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aZ1cAAAAASUVORK5CYII="
	called := false
	vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		var body struct {
			Model    string `json:"model"`
			Messages []struct {
				Content json.RawMessage `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.Model != "shared-vision" || r.Header.Get("Authorization") != "Bearer shared-key" {
			t.Error("shared model or credential missing")
		}
		if len(body.Messages) != 2 || !strings.Contains(string(body.Messages[1].Content), "data:image/png;base64,"+png) {
			t.Error("image did not reach selected provider")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"Recognized text"}}]}`)
	}))
	defer vendor.Close()
	raw, _ := json.Marshal(map[string]string{"protocol": "chat", "baseUrl": vendor.URL + "/v1", "key": "shared-key", "model": "shared-vision", "text": "Read the image", "imagePng": png})
	got, err := run(context.Background(), strings.NewReader(string(raw)))
	if err != nil || got != "Recognized text" || !called {
		t.Fatalf("shared image recognition %q: %v, called=%v", got, err, called)
	}
}

func TestConversationHistoryReachesMagpieInOrder(t *testing.T) {
	vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if len(payload.Messages) != 4 || payload.Messages[1].Content != "Original: Hello" || payload.Messages[2].Role != "assistant" || payload.Messages[2].Content != "你好" || payload.Messages[3].Content != "Explain it" {
			t.Errorf("conversation changed: %+v", payload.Messages)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"An explanation"}}]}`)
	}))
	defer vendor.Close()
	raw, _ := json.Marshal(map[string]any{"protocol": "chat", "baseUrl": vendor.URL + "/v1", "model": "model-test", "system": "Use the supplied context.", "messages": []map[string]string{{"role": "user", "content": "Original: Hello"}, {"role": "assistant", "content": "你好"}, {"role": "user", "content": "Explain it"}}})
	got, err := run(context.Background(), strings.NewReader(string(raw)))
	if err != nil || got != "An explanation" {
		t.Fatalf("conversation %q: %v", got, err)
	}
}

func TestTextGenerationUsesTheSignedInAgent(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("USERPROFILE", home)
	// Current Magpie follows Codex's explicit home before USERPROFILE.
	// The fixture must bind both so a developer's account is never consulted.
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	t.Setenv("APPDATA", filepath.Join(home, "AppData/Roaming"))
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData/Local"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	jwt := func(value map[string]any) string {
		data, _ := json.Marshal(value)
		return "eyJhbGciOiJub25lIn0." + base64.RawURLEncoding.EncodeToString(data) + ".fixture"
	}
	token := jwt(map[string]any{"exp": time.Now().Add(time.Hour).Unix()})
	id := jwt(map[string]any{"email": "winone@example.invalid", "https://api.openai.com/auth": map[string]string{"chatgpt_account_id": "fixture-account", "chatgpt_plan_type": "plus"}})
	auth, _ := json.Marshal(map[string]any{"auth_mode": "chatgpt", "tokens": map[string]string{"id_token": id, "access_token": token, "refresh_token": "unused-fixture", "account_id": "fixture-account"}})
	if err := os.WriteFile(filepath.Join(home, ".codex", "auth.json"), auth, 0600); err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Error("account credential did not reach its provider")
		}
		// Current magpie reads the signed-in model's reasoning capabilities
		// before routing. Model discovery uses the same account credential.
		if r.Method == http.MethodGet && r.URL.Path == "/models" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"models":[{"slug":"fixture-model","display_name":"Fixture model","visibility":"list","supported_reasoning_levels":[]}]}`)
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/wham/usage" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"plan_type":"plus","rate_limit":{"allowed":true,"limit_reached":false,"primary_window":{"used_percent":0,"limit_window_seconds":18000,"reset_after_seconds":18000}}}`)
			return
		}
		if r.Method != http.MethodPost || r.URL.Path != "/responses" {
			t.Errorf("unexpected account request: %s %s", r.Method, r.URL.Path)
		}
		// Magpie now follows Codex's native account transport: /responses is
		// zstd on the wire. Decode the real request instead of disabling it.
		if r.Header.Get("Content-Encoding") != "zstd" {
			t.Error("Codex account turn was not compressed")
		}
		decoded, err := zstd.NewReader(r.Body)
		if err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		defer decoded.Close()
		var body map[string]any
		if err := json.NewDecoder(decoded).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["model"] != "fixture-model" {
			t.Errorf("unexpected model %v", body["model"])
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"output_index\":0,\"content_index\":0,\"delta\":\"账号译文\"}\n\nevent: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"fixture\",\"status\":\"completed\",\"output\":[]}}\n\n")
	}))
	defer upstream.Close()
	previous := provider.CodexBase
	provider.CodexBase = upstream.URL
	t.Cleanup(func() { provider.CodexBase = previous })
	provider.WinOneProviders = nil
	provider.Changed()
	raw, _ := json.Marshal(request{Agent: "codex", Model: "fixture-model", System: "Translate only.", Text: "Hello"})
	got, err := run(context.Background(), strings.NewReader(string(raw)))
	if err != nil || got != "账号译文" {
		t.Fatalf("account generation %q: %v", got, err)
	}
}
