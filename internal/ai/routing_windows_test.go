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
	"sync"
	"testing"
	"time"
)

func TestMagpieRoutingFailsOverKeysAndRetainsCooldown(t *testing.T) {
	executable := os.Getenv("WINONE_ROUTING_TEST_EXE")
	if executable == "" {
		t.Skip("explicit isolated Magpie bundle")
	}
	var mu sync.Mutex
	attempts := map[string]int{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("Authorization")
		mu.Lock()
		attempts[key]++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if key == "Bearer exhausted-key" {
			w.Header().Set("Retry-After", "60")
			w.WriteHeader(429)
			io.WriteString(w, `{"error":{"message":"rate limited","type":"rate_limit_error"}}`)
			return
		}
		if key != "Bearer usable-key" {
			t.Errorf("unexpected credential %q", key)
			w.WriteHeader(401)
			return
		}
		io.WriteString(w, `{"id":"fixture","choices":[{"message":{"role":"assistant","content":"Failover answer"},"finish_reason":"stop"}]}`)
	}))
	defer upstream.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	providers := []RuntimeProvider{{Provider: Provider{ID: "fixture", Name: "Fixture", AuthMode: "api-key", Protocol: "chat", BaseURL: upstream.URL, Models: []string{"model"}}, Key: "exhausted-key", Keys: []RuntimeKey{{ID: "first", Key: "exhausted-key"}, {ID: "second", Key: "usable-key"}}}}
	router, err := (Translator{Executable: executable, DataDirectory: filepath.Join(t.TempDir(), "runtime")}).StartRouting(ctx, &MagpieSettings{NoStats: true, NoUsageStats: true}, providers)
	if err != nil {
		t.Fatal(err)
	}
	defer router.Close()
	for i := 0; i < 2; i++ {
		request, _ := http.NewRequestWithContext(ctx, "POST", router.URL+"/v1/chat/completions", bytes.NewBufferString(`{"model":"fixture/model","messages":[{"role":"user","content":"Test failover"}]}`))
		request.Header.Set("Authorization", "Bearer "+router.Key)
		request.Header.Set("Content-Type", "application/json")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if response.StatusCode != 200 || !bytes.Contains(raw, []byte("Failover answer")) {
			t.Fatalf("call %d: %d %s", i, response.StatusCode, raw)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if attempts["Bearer exhausted-key"] != 1 || attempts["Bearer usable-key"] != 2 {
		t.Fatalf("failed key must stay in cooldown: %v", attempts)
	}
}

func TestMagpieRoutingPersistsGroupRoutesRealRequestsAndRejectsUnauthenticatedClients(t *testing.T) {
	executable := os.Getenv("WINONE_ROUTING_TEST_EXE")
	if executable == "" {
		t.Skip("explicit isolated Magpie bundle")
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fixture-upstream-key" {
			t.Error("provider credential was lost")
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"Routed answer"},"finish_reason":"stop"}]}`)
	}))
	defer upstream.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	runtime := Translator{Executable: executable, DataDirectory: filepath.Join(t.TempDir(), "runtime")}
	providers := []RuntimeProvider{{Provider: Provider{ID: "fixture", Name: "Fixture", AuthMode: "api-key", Protocol: "chat", BaseURL: upstream.URL, Models: []string{"model"}}, Key: "fixture-upstream-key"}}
	router, err := runtime.StartRouting(ctx, &MagpieSettings{NoStats: true, NoUsageStats: true}, providers)
	if err != nil {
		t.Fatal(err)
	}
	defer router.Close()
	call := func(method, path, body string) []byte {
		t.Helper()
		request, _ := http.NewRequestWithContext(ctx, method, router.URL+path, bytes.NewBufferString(body))
		request.Header.Set("Authorization", "Bearer "+router.Key)
		request.Header.Set("Content-Type", "application/json")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		raw, _ := io.ReadAll(response.Body)
		if response.StatusCode != 200 {
			t.Fatalf("%s: %d %s", path, response.StatusCode, raw)
		}
		return raw
	}
	call("POST", "/api/groups/save", `{"id":"test-group","name":"Test group","members":["fixture/model"],"routing":"order"}`)
	// The standalone page's naming and provider controls use the same durable
	// configuration as real routed requests, even after a worker replacement.
	call("POST", "/api/settings/plain-names", `{"mode":"off"}`)
	if raw := call("GET", "/api/settings", ""); !bytes.Contains(raw, []byte(`"plainNames":true`)) {
		t.Fatal("routing preference did not reach the active snapshot", string(raw))
	}
	call("POST", "/api/provider/route", `{"id":"fixture","routing":"rotate"}`)
	call("GET", "/api/usage/quotas", "")
	answer := call("POST", "/v1/chat/completions", `{"model":"group/test-group","messages":[{"role":"user","content":"Test route"}]}`)
	if !bytes.Contains(answer, []byte("Routed answer")) {
		t.Fatal(string(answer))
	}
	if trace := call("GET", "/api/gateway/trace", ""); !bytes.Contains(trace, []byte("fixture")) {
		t.Fatal("real route missing", string(trace))
	}
	denied := func(Selection) bool { return false }
	allowed := func(selection Selection) bool { return selection.Provider == "fixture" && selection.Model == "model" }
	if ok, err := router.AllowsModel(ctx, "group/test-group", allowed); err != nil || !ok {
		t.Fatal("enabled group rejected", ok, err)
	}
	if ok, err := router.AllowsModel(ctx, "group/test-group", denied); err != nil || ok {
		t.Fatal("group bypassed disabled member", ok, err)
	}
	models, err := router.Models(ctx, denied)
	if err != nil || !bytes.Contains(models, []byte(`"data":[]`)) {
		t.Fatal("empty selection exposed models", string(models), err)
	}
	models, err = router.Models(ctx, allowed)
	if err != nil || !bytes.Contains(models, []byte("group/test-group")) {
		t.Fatal("routing group not in enabled model list", string(models), err)
	}
	request, _ := http.NewRequestWithContext(ctx, "GET", router.URL+"/api/groups", nil)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 401 {
		t.Fatal("unauthenticated access", response.StatusCode)
	}
	for _, path := range []string{"/app.js", "/start.js", "/plugins/sessions/client.js", "/api/usage", "/api/provider/save"} {
		request, _ := http.NewRequestWithContext(ctx, "GET", router.URL+path, nil)
		request.Header.Set("Authorization", "Bearer "+router.Key)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != 404 && response.StatusCode != 405 {
			t.Fatalf("non-routing application endpoint %s was exposed: %d", path, response.StatusCode)
		}
	}
	router.Close()
	router, err = runtime.StartRouting(ctx, &MagpieSettings{NoStats: true, NoUsageStats: true}, providers)
	if err != nil {
		t.Fatal(err)
	}
	if raw := call("GET", "/api/settings", ""); !bytes.Contains(raw, []byte(`"plainNames":true`)) {
		t.Fatal("routing preference did not survive restart", string(raw))
	}
	defer router.Close()
	var state struct{ Groups []struct{ ID string } }
	if err = json.Unmarshal(call("GET", "/api/groups", ""), &state); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, group := range state.Groups {
		found = found || group.ID == "test-group"
	}
	if !found {
		t.Fatal("saved routing group was lost")
	}
	store, err := Open(filepath.Join(t.TempDir(), "ai-settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	config := store.Configuration()
	config.RoutingEnabled = true
	if _, err = store.SaveConfiguration(config, nil); err != nil {
		t.Fatal(err)
	}
	again, err := Open(store.path)
	if err != nil || !again.Configuration().RoutingEnabled {
		t.Fatal("routing switch not persisted", err)
	}
	// The old process is fully gone before a replacement starts.
	select {
	case <-router.process.finished:
		t.Fatal("replacement router exited unexpectedly")
	default:
	}
	// Unexpected termination must be observed before another open/request can
	// reuse the dead process. Close remains safe after the background reaper.
	if err := router.process.cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-router.process.finished:
	case <-time.After(2 * time.Second):
		t.Fatal("terminated router was not reaped")
	}
}
