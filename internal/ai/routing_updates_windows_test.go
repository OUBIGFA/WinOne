//go:build windows

package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// Exercise the packaged router and its provider transport, not an upstream
// in-process server: a second adapter must not overwrite a routing decision.
func TestPackagedRoutingRuleEffortSurvivesProviderDefaults(t *testing.T) {
	var mu sync.Mutex
	var sent []map[string]any
	vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		mu.Lock()
		sent = append(sent, body)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"rule proof"},"finish_reason":"stop"}]}`)
	}))
	defer vendor.Close()
	providers := []RuntimeProvider{{Provider: Provider{ID: "fixture", Name: "Fixture", AuthMode: "api-key", Protocol: "chat", BaseURL: vendor.URL, Models: []string{"model"}, ModelMetadata: map[string]ModelMetadata{"model": {Efforts: []string{"low", "medium", "high"}, ReasoningEffort: "medium"}}}, Key: "fixture-key"}}
	router, ctx := startUpdateRouter(t, providers, nil)
	for _, c := range []struct{ use, asked, want string }{{"low", "high", "low"}, {"max", "low", "high"}, {"high", "", "high"}} {
		updateRouterCall(t, ctx, router, "POST", "/api/groups/save", map[string]any{"id": "ruled", "name": "Ruled", "members": []string{"fixture/model"}, "rules": []any{map[string]any{"use": "fixture/model", "tokens": 1000, "useEffort": c.use}}})
		body := map[string]any{"model": "group/ruled", "messages": []any{map[string]any{"role": "user", "content": strings.Repeat("word ", 2000)}}}
		if c.asked != "" {
			body["reasoning_effort"] = c.asked
		}
		raw := updateRouterCall(t, ctx, router, "POST", "/v1/chat/completions", body)
		if !bytes.Contains(raw, []byte("rule proof")) {
			t.Fatalf("missing answer: %s", raw)
		}
		mu.Lock()
		last := sent[len(sent)-1]
		mu.Unlock()
		if last["reasoning_effort"] != c.want {
			t.Fatalf("rule %s, client %s: vendor received %v, want %s", c.use, c.asked, last["reasoning_effort"], c.want)
		}
		trace := updateRouterCall(t, ctx, router, "GET", "/api/gateway/trace", nil)
		if !bytes.Contains(trace, []byte("ruleEffort")) {
			t.Fatalf("rule effort missing from trace: %s", trace)
		}
	}
}

// Inner patience wins even when the outer budget is shorter. Count attempts
// instead of asserting elapsed seconds, which is unreliable on loaded hosts.
func TestPackagedNestedRoutingPatience(t *testing.T) {
	for _, c := range []struct {
		name                       string
		inner, outer, wantA, wantB int
	}{{"inner budget", 10, 4, 3, 3}, {"inner off", -1, 10, 1, 3}, {"outer fallback", 0, 4, 2, 2}} {
		t.Run(c.name, func(t *testing.T) {
			var mu sync.Mutex
			attempts := map[string]int{}
			vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct{ Model string }
				_ = json.NewDecoder(r.Body).Decode(&body)
				mu.Lock()
				attempts[body.Model]++
				mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(529)
				io.WriteString(w, `{"error":{"message":"Our servers are currently overloaded. Please try again later.","code":"server_is_overloaded"}}`)
			}))
			defer vendor.Close()
			providers := []RuntimeProvider{}
			for _, id := range []string{"a", "b"} {
				providers = append(providers, RuntimeProvider{Provider: Provider{ID: id, Name: id, AuthMode: "api-key", Protocol: "chat", BaseURL: vendor.URL, Models: []string{id}}, Key: "fixture-key"})
			}
			router, ctx := startUpdateRouter(t, providers, nil)
			updateRouterCall(t, ctx, router, "POST", "/api/groups/save", map[string]any{"id": "inner", "name": "Inner", "members": []string{"a/a", "b/b"}, "routing": "order", "patience": c.inner})
			updateRouterCall(t, ctx, router, "POST", "/api/groups/save", map[string]any{"id": "outer", "name": "Outer", "members": []string{"group/inner"}, "routing": "order", "patience": c.outer})
			requestBody := []byte(`{"model":"group/outer","messages":[{"role":"user","content":"hi"}]}`)
			req, _ := http.NewRequestWithContext(ctx, "POST", router.URL+"/v1/chat/completions", bytes.NewReader(requestBody))
			req.Header.Set("Authorization", "Bearer "+router.Key)
			req.Header.Set("Content-Type", "application/json")
			response, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			raw, _ := io.ReadAll(response.Body)
			response.Body.Close()
			if response.StatusCode != 529 {
				t.Fatalf("status %d: %s", response.StatusCode, raw)
			}
			mu.Lock()
			defer mu.Unlock()
			// Off disables group replanning. Magpie still permits its normal
			// last-candidate transient retries (one attempt for a, three for b).
			if attempts["a"] != c.wantA || attempts["b"] != c.wantB {
				t.Fatalf("attempts %v, want a=%d b=%d", attempts, c.wantA, c.wantB)
			}
			if c.inner < 0 {
				trace := updateRouterCall(t, ctx, router, "GET", "/api/gateway/trace", nil)
				if bytes.Contains(trace, []byte("\"replan\":")) {
					t.Fatalf("inner Off replanned: %s", trace)
				}
			}
			if c.inner > 0 {
				trace := updateRouterCall(t, ctx, router, "GET", "/api/gateway/trace", nil)
				if !bytes.Contains(trace, []byte(`"patience":10`)) {
					t.Fatalf("inner patience not traced: %s", trace)
				}
			}
		})
	}
}

func TestPackagedRoutingPublishesDisabledGroupFeature(t *testing.T) {
	router, ctx := startUpdateRouter(t, nil, &MagpieSettings{BuiltinPreset: "essentials"})
	var state struct {
		Settings struct {
			Builtins []struct {
				ID string
				On bool
			}
		}
	}
	raw := updateRouterCall(t, ctx, router, "GET", "/api/state", nil)
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatal(err)
	}
	for _, feature := range state.Settings.Builtins {
		if feature.ID == "groups" {
			if feature.On {
				t.Fatal("groups must be off for Essentials without a group")
			}
			return
		}
	}
	t.Fatal("routing page did not receive feature state")
}

func startUpdateRouter(t *testing.T, providers []RuntimeProvider, prefs *MagpieSettings) (*RoutingProcess, context.Context) {
	t.Helper()
	exe := os.Getenv("WINONE_ROUTING_TEST_EXE")
	if exe == "" {
		t.Skip("explicit isolated Magpie bundle")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	t.Cleanup(cancel)
	if prefs == nil {
		prefs = &MagpieSettings{NoStats: true, NoUsageStats: true}
	}
	router, err := (Translator{Executable: exe, DataDirectory: filepath.Join(t.TempDir(), "runtime")}).StartRouting(ctx, prefs, providers)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(router.Close)
	return router, ctx
}

func updateRouterCall(t *testing.T, ctx context.Context, router *RoutingProcess, method, path string, body any) []byte {
	t.Helper()
	var payload []byte
	var err error
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, router.URL+path, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+router.Key)
	req.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 200 {
		t.Fatalf("%s %s: %d %s", method, path, response.StatusCode, raw)
	}
	return raw
}
