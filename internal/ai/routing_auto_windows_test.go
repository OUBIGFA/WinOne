//go:build windows

package ai

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestPackagedAutoGroupOldAndNewIDsSharePermissionsAndExecution(t *testing.T) {
	var calls atomic.Int32
	vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"auto group fixture"},"finish_reason":"stop"}]}`)
	}))
	defer vendor.Close()
	providers := []RuntimeProvider{}
	for _, id := range []string{"a", "b"} {
		providers = append(providers, RuntimeProvider{Provider: Provider{ID: id, Name: id, AuthMode: "api-key", Protocol: "chat", BaseURL: vendor.URL, Models: []string{"gpt-4.1"}}, Key: "fixture-key"})
	}
	router, ctx := startUpdateRouter(t, providers, nil)
	var state struct{ Groups []routingGroup }
	if err := json.Unmarshal(updateRouterCall(t, ctx, router, "GET", "/api/groups", nil), &state); err != nil {
		t.Fatal(err)
	}
	var group routingGroup
	for _, candidate := range state.Groups {
		if strings.HasPrefix(candidate.Ref, "auto/") {
			group = candidate
			break
		}
	}
	if group.ID == "" {
		t.Fatalf("new auto/<model> ID not published: %+v", state)
	}
	allow := func(selection Selection) bool {
		return (selection.Provider == "a" || selection.Provider == "b") && selection.Model == "gpt-4.1"
	}
	denyMember := func(selection Selection) bool { return selection.Provider == "a" && selection.Model == "gpt-4.1" }
	for _, ref := range []string{group.Ref, "group/" + group.ID} {
		if ok, err := router.AllowsModel(ctx, ref, allow); err != nil || !ok {
			t.Fatalf("enabled alias rejected %s: %v %v", ref, ok, err)
		}
		if ok, err := router.AllowsModel(ctx, ref, denyMember); err != nil || ok {
			t.Fatalf("alias bypassed a disabled member %s: %v %v", ref, ok, err)
		}
		raw := updateRouterCall(t, ctx, router, "POST", "/v1/chat/completions", map[string]any{"model": ref, "messages": []any{map[string]string{"role": "user", "content": "hello"}}})
		if !bytes.Contains(raw, []byte("auto group fixture")) {
			t.Fatalf("alias request failed %s: %s", ref, raw)
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("aliases did not execute exactly once: %d", calls.Load())
	}
	allowed, err := router.Models(ctx, allow)
	if err != nil || !bytes.Contains(allowed, []byte(group.Ref)) {
		t.Fatalf("new ID missing from permitted discovery: %s %v", allowed, err)
	}
	hidden, err := router.Models(ctx, denyMember)
	if err != nil || bytes.Contains(hidden, []byte(group.Ref)) {
		t.Fatalf("group with disabled member reappeared: %s %v", hidden, err)
	}
	tags, err := router.OllamaTags(ctx, map[string]bool{group.Ref: true})
	if err != nil || !bytes.Contains(tags, []byte(group.Ref)) || bytes.Contains(tags, []byte(`"model":"a/`)) {
		t.Fatalf("Ollama group permissions differ: %s %v", tags, err)
	}
}
