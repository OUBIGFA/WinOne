package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/settings"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRuntimeRedactionRestoresResponseAndUsesWireName(t *testing.T) {
	old := settings.WinOneSettings.Load()
	defer func() {
		settings.WinOneSettings.Store(old)
		provider.WinOneSearchAPIs = nil
		provider.WinOneProviders = nil
	}()
	var receivedModel, receivedText string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/models") {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]string{"id": "gpt-4.1"}}})
			return
		}
		var body struct {
			Model    string `json:"model"`
			Messages []struct {
				Content any `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		receivedModel = body.Model
		for _, m := range body.Messages {
			if value, ok := m.Content.(string); ok && strings.Contains(value, "project") {
				receivedText = value
			}
		}
		if receivedText == "" {
			for _, m := range body.Messages {
				if value, ok := m.Content.(string); ok {
					receivedText = value
				}
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "test", "object": "chat.completion", "model": body.Model, "choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": receivedText}, "finish_reason": "stop"}}})
	}))
	defer server.Close()
	in := request{Protocol: "chat", BaseURL: server.URL + "/v1", Model: "gpt-4.1", Text: "private-project", ModelMetadata: modelMetadata{UpstreamName: "wire-model"}, MagpieSettings: json.RawMessage(`{"redact":true,"redactWords":["private-project"]}`)}
	encoded, _ := json.Marshal(in)
	out, err := run(context.Background(), bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	if receivedModel != "wire-model" {
		t.Fatalf("wire name not used: %q", receivedModel)
	}
	if strings.Contains(receivedText, "private-project") {
		t.Fatal("unmasked text reached vendor")
	}
	if out != "private-project" {
		t.Fatalf("response was not restored: %q", out)
	}
}

func TestTraySelectionPreservesWholeAccountWindows(t *testing.T) {
	q := provider.SubscriptionQuota{Provider: "codex", User: "a", Windows: []provider.QuotaWindow{{Name: "week", Span: 7 * 24 * time.Hour}, {Name: "model", Model: "one"}, {Name: "extra", Aside: true}, {Name: "five", Span: 5 * time.Hour}}}
	cards := trayCards([]provider.SubscriptionQuota{q}, []string{"saved-provider|a"}, "saved-provider", time.Now())
	if len(cards) != 1 || len(cards[0].Windows) != 2 || cards[0].Windows[0].Name != "five" {
		t.Fatalf("wrong tray windows: %+v", cards)
	}
}

// These assertions exercise the snapshots consumed by the upstream gateway,
// not a duplicate WinOne redaction or search implementation.
func TestRuntimeSettingsReachUpstream(t *testing.T) {
	in := request{MagpieSettings: json.RawMessage(`{"redact":true,"redactPersonal":true,"redactWords":["private-project"],"vision":"off","searcher":"off","searchFirst":"api","noLoopGuard":true,"searchAPIs":[{"vendor":"tavily","key":"test-key"}]}`)}
	if err := applyRuntimeSettings(in); err != nil {
		t.Fatal(err)
	}
	defer func() { settings.WinOneSettings.Store(nil); provider.WinOneSearchAPIs = nil }()
	s := settings.Load()
	if !s.Redact || !s.RedactPersonal || s.Vision != "off" || s.Searcher != "off" || s.SearchFirst != "api" || !s.NoLoopGuard {
		t.Fatalf("settings not applied: %+v", s)
	}
	if !s.NoWSLAgents {
		t.Fatal("embedded runtime must never discover real WSL agent profiles")
	}
	apis := provider.SearchAPIs()
	if len(apis) != 1 || apis[0].Key != "test-key" {
		t.Fatal("search API was not installed")
	}
}

func TestRuntimeRejectsInvalidRedactionRule(t *testing.T) {
	if err := applyRuntimeSettings(request{MagpieSettings: json.RawMessage(`{"redactRules":[{"kind":"PRIVATE","regex":"["}]}`)}); err == nil {
		t.Fatal("invalid regex accepted")
	}
}
