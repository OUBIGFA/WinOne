//go:build windows

package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"winone/internal/ai"
)

func TestGatewayRoutesFullProtocolThroughMagpie(t *testing.T) {
	executable := os.Getenv("WINONE_AI_TEST_EXE")
	if executable == "" {
		t.Skip("set the built magpie adapter for full HTTP checks")
	}
	called := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		var body map[string]json.RawMessage
		if json.NewDecoder(r.Body).Decode(&body) != nil || string(body["model"]) != `"test-model"` || !strings.Contains(string(body["tools"]), "weather") || r.Header.Get("Authorization") != "Bearer fixture-key" {
			t.Error("route, tools or credentials lost")
		}
		if string(body["stream"]) == "true" {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hello\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"fixture","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}]}`)
	}))
	defer upstream.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := newAIService(ctx, t.TempDir(), filepath.Join(t.TempDir(), "WinOne.Host.exe"))
	s.translator.Executable = executable
	s.gatewaySlots = make(chan struct{}, 4)
	s.initGateway()
	s.store, _ = ai.Open(s.storePath)
	config := s.store.Configuration()
	config.Providers = []ai.Provider{{ID: "custom", Name: "Custom", AuthMode: "api-key", Protocol: "chat", BaseURL: upstream.URL + "/v1", Models: []string{"test-model"}}, {ID: "account", Name: "Account", AuthMode: "account", Agent: "codex", Models: []string{"account-model"}}}
	key := "fixture-key"
	if _, err := s.store.SaveConfiguration(config, map[string]*string{"custom": &key}); err != nil {
		t.Fatal(err)
	}
	models := httptest.NewRecorder()
	s.serveGateway(models, httptest.NewRequest("GET", "/v1/models", nil))
	if !strings.Contains(models.Body.String(), "custom/test-model") || !strings.Contains(models.Body.String(), "account/account-model") {
		t.Fatal(models.Body.String())
	}
	for _, stream := range []bool{false, true} {
		body, _ := json.Marshal(map[string]any{"model": "custom/test-model", "messages": []any{map[string]any{"role": "user", "content": "hello"}}, "tools": []any{map[string]any{"type": "function", "function": map[string]any{"name": "weather", "parameters": map[string]any{"type": "object", "properties": map[string]any{}}}}}, "stream": stream})
		output := httptest.NewRecorder()
		s.serveGateway(output, httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(string(body))))
		if output.Code != 200 || !strings.Contains(output.Body.String(), "hello") {
			t.Fatalf("stream=%v status=%d body=%s", stream, output.Code, output.Body.String())
		}
	}
	invalid := httptest.NewRecorder()
	s.serveGateway(invalid, httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"custom/disabled"}`)))
	if invalid.Code != 400 || called != 2 {
		t.Fatalf("invalid model reached vendor: %d/%d", invalid.Code, called)
	}
}
