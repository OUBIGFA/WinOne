//go:build windows

package main

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"winone/internal/ai"
	"winone/internal/host"
)

func TestGatewayModelSelectionControlsDiscoveryAndCalls(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dir := t.TempDir()
	s := newAIService(ctx, dir, filepath.Join(dir, "WinOne.Host.exe"))
	s.initGateway()
	s.store, _ = ai.Open(s.storePath)
	config := s.store.Configuration()
	config.Providers = []ai.Provider{{ID: "one", Name: "Work", AuthMode: "api-key", Protocol: "chat", BaseURL: "https://example.invalid/v1", Models: []string{"same", "second"}}, {ID: "two", Name: "Private", AuthMode: "account", Agent: "codex", Models: []string{"same"}}}
	if _, err := s.store.SaveConfiguration(config, nil); err != nil {
		t.Fatal(err)
	}
	save := func(revision int, models any) host.Response {
		params, _ := json.Marshal(map[string]any{"settings": map[string]any{"revision": revision, "enabled": false, "port": 3426, "models": models}})
		return s.handleGateway(host.Request{ID: "save", Method: "gateway.save", Params: params})
	}
	if r := save(0, []ai.Selection{{Provider: "one", Model: "same"}}); r.Error != nil {
		t.Fatal(r.Error)
	}
	list := func() string {
		w := httptest.NewRecorder()
		s.serveGateway(w, httptest.NewRequest("GET", "/v1/models", nil))
		if w.Code != 200 {
			t.Fatal(w.Code)
		}
		return w.Body.String()
	}
	if models := list(); !strings.Contains(models, "one/same") || strings.Contains(models, "one/second") || strings.Contains(models, "two/same") {
		t.Fatal("gateway selection was not applied:", models)
	}
	for _, route := range []string{"one/second", "two/same"} {
		w := httptest.NewRecorder()
		body, _ := json.Marshal(map[string]string{"model": route})
		s.serveGateway(w, httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(string(body))))
		if w.Code != 403 {
			t.Fatalf("disabled model %s was not rejected: %d", route, w.Code)
		}
	}
	// Older callers omit the selection. They must not reopen disabled routes.
	if r := save(1, nil); r.Error != nil {
		t.Fatal(r.Error)
	}
	if strings.Contains(list(), "two/same") {
		t.Fatal("legacy save reset model choices")
	}
	if r := save(2, []ai.Selection{}); r.Error != nil {
		t.Fatal(r.Error)
	}
	if models := list(); !strings.Contains(models, `"data":[]`) {
		t.Fatal("empty selection reopened every model:", models)
	}
	if len(s.store.Configuration().Providers[0].Models) != 2 {
		t.Fatal("gateway edited feature model choices")
	}
}
