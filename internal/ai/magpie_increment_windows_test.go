//go:build windows

package ai

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// Per-provider middleware belongs to the router attempt, not its private
// transport. A change for the failing first candidate must not reach the next.
func TestPackagedProviderMiddlewareIsOnceAndCandidateScoped(t *testing.T) {
	var mu sync.Mutex
	observed := map[string][]float64{}
	vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model string
			TopP  float64 `json:"top_p"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		mu.Lock()
		observed[body.Model] = append(observed[body.Model], body.TopP)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if body.Model == "a" {
			w.WriteHeader(http.StatusTooManyRequests)
			io.WriteString(w, `{"error":{"message":"fixture quota","type":"insufficient_quota"}}`)
			return
		}
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"provider scoped"},"finish_reason":"stop"}]}`)
	}))
	defer vendor.Close()
	providers := []RuntimeProvider{}
	for _, id := range []string{"a", "b"} {
		providers = append(providers, RuntimeProvider{Provider: Provider{ID: id, Name: id, AuthMode: "api-key", Protocol: "chat", BaseURL: vendor.URL, Models: []string{id}}, Key: "fixture-key"})
	}
	router, ctx := startUpdateRouter(t, providers, nil)
	fixture := t.TempDir()
	for name, value := range map[string]string{
		"package.json": `{"name":"winone-provider-hook","version":"1.0.0","magpie":{"middleware":"provider.js"}}`,
		"provider.js":  `export function onProvider(body,ctx){return {...body,top_p:(body.top_p||0)+(ctx.provider.id==="a"?0.25:0.5)}}`,
	} {
		if err := os.WriteFile(filepath.Join(fixture, name), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	updateRouterCall(t, ctx, router, "POST", "/api/plugins/add", map[string]string{"spec": fixture})
	updateRouterCall(t, ctx, router, "POST", "/api/groups/save", map[string]any{"id": "scoped", "name": "Scoped", "members": []string{"a/a", "b/b"}, "routing": "order", "patience": -1})
	updateRouterCall(t, ctx, router, "POST", "/v1/chat/completions", map[string]any{"model": "group/scoped", "messages": []any{map[string]string{"role": "user", "content": "Hello"}}})
	mu.Lock()
	defer mu.Unlock()
	if len(observed["a"]) != 1 || len(observed["b"]) != 1 || observed["a"][0] != 0.25 || observed["b"][0] != 0.5 {
		t.Fatalf("middleware repeated or leaked between candidates: %v", observed)
	}
}
