//go:build windows

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"winone/internal/ai"
)

// Real Host -> current packaged AI adapter -> local HTTP provider. This covers
// ordinary Pi chat (not the already-tested media-only provider transport).
func TestChatGatewayCallsItsSelectedProviderWithTheOriginalToolPayload(t *testing.T) {
	executable := os.Getenv("WINONE_AI_TEST_EXE")
	if executable == "" {
		t.Skip("explicit packaged AI runtime")
	}
	var calls atomic.Int32
	vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer chat-gateway-fixture-key" {
			t.Error("selected dummy credential was not used")
		}
		var input struct {
			Model    string
			Tools    []json.RawMessage
			Messages []json.RawMessage
		}
		if json.NewDecoder(r.Body).Decode(&input) != nil || input.Model != "model" || len(input.Tools) != 1 || len(input.Messages) == 0 {
			t.Errorf("original Pi payload changed: %+v", input)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"CHAT_GATEWAY_DONE\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer vendor.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	root := t.TempDir()
	service := newAIService(ctx, root, filepath.Join(root, "WinOne.Host.exe"))
	service.translator.Executable = executable
	service.gatewaySlots = make(chan struct{}, 4)
	store, err := service.magpieStore()
	if err != nil {
		t.Fatal(err)
	}
	configuration := store.Configuration()
	configuration.Providers = []ai.Provider{{ID: "fixture", Name: "Fixture", AuthMode: "api-key", Protocol: "chat", BaseURL: vendor.URL, Models: []string{"model"}}}
	configuration.Tasks = map[string]ai.Selection{"chat": {Provider: "fixture", Model: "model"}}
	configuration.MagpieSettings = &ai.MagpieSettings{NoStats: true, NoUsageStats: true}
	key := "chat-gateway-fixture-key"
	if _, err = store.SaveConfiguration(configuration, map[string]*string{"fixture": &key}); err != nil {
		t.Fatal(err)
	}
	body := `{"model":"fixture/model","messages":[{"role":"user","content":"Reply once."}],"tools":[{"type":"function","function":{"name":"fixture_tool","parameters":{"type":"object"}}}],"stream":true}`
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/v1/chat/completions", strings.NewReader(body)).WithContext(ctx)
	response := httptest.NewRecorder()
	service.serveChat(response, request)
	if response.Code != http.StatusOK || calls.Load() != 1 || !strings.Contains(response.Body.String(), "CHAT_GATEWAY_DONE") {
		prefs, _, e := store.ForTaskKeys("chat", &ai.Selection{Provider: "fixture", Model: "model"})
		if e == nil {
			probe := httptest.NewRecorder()
			_, e = service.translator.ChatRequest(ctx, prefs, key, "/v1/chat/completions", json.RawMessage(body), http.Header{}, probe)
		}
		t.Fatalf("ordinary chat gateway: status=%d requests=%d body=%s adapter=%v", response.Code, calls.Load(), response.Body.String(), e)
	}
}
