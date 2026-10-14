//go:build windows

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"winone/internal/ai"
	"winone/internal/host"
)

// The real Magpie worker probes only a local fixture. Draft credentials and
// mappings must reach that fixture without changing the saved configuration.
func TestModelConnectivityWithRealWorkerAndUnsavedMapping(t *testing.T) {
	exe := os.Getenv("WINONE_AI_TEST_EXE")
	if exe == "" {
		t.Skip("explicit packaged AI worker required")
	}
	var failed atomic.Bool
	var probes atomic.Int32
	vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			fmt.Fprint(w, `{"data":[{"id":"actual-model"}]}`)
			return
		}
		probes.Add(1)
		if r.Header.Get("Authorization") != "Bearer draft-fixture" || r.Header.Get("X-Draft") != "current" {
			t.Error("draft credential or header was not used")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["model"] != "actual-model" {
			t.Errorf("request model = %v; mapping did not reach provider", body["model"])
		}
		if failed.Load() {
			w.WriteHeader(403)
			fmt.Fprint(w, `{"error":{"message":"fixture model unavailable"}}`)
			return
		}
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"OK"},"finish_reason":"stop"}]}`)
	}))
	defer vendor.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	engine := newAIService(ctx, t.TempDir(), exe)
	engine.translator.Executable = exe
	store, err := ai.Open(engine.storePath)
	if err != nil {
		t.Fatal(err)
	}
	engine.store = store
	config := store.Configuration()
	config.Providers = []ai.Provider{{ID: "saved", Name: "Saved", AuthMode: "api-key", Protocol: "chat", BaseURL: "https://saved.invalid/v1", Models: []string{"saved-model"}}}
	secret := "saved-fixture"
	if _, err = store.SaveConfiguration(config, map[string]*string{"saved": &secret}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(engine.storePath)
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]any{"provider": "saved", "models": []string{"alias-model"}, "metadata": map[string]any{"alias-model": map[string]any{"upstreamName": "actual-model", "api": "chat"}}, "connection": map[string]any{"protocol": "chat", "baseUrl": vendor.URL + "/v1", "headers": map[string]string{"X-Draft": "current"}, "keys": []map[string]any{{"id": "primary", "enabled": true}}}, "keyUpdates": map[string]string{"primary": "draft-fixture"}})
	for _, failure := range []bool{false, true} {
		failed.Store(failure)
		response := engine.handle(host.Request{ID: "probe", Method: "models.test", Params: payload})
		if response.Error != nil {
			t.Fatal(response.Error)
		}
		var value struct {
			Results []struct {
				OK       bool   `json:"ok"`
				Status   int    `json:"status"`
				Model    string `json:"model"`
				Protocol string `json:"protocol"`
			}
		}
		if err = json.Unmarshal(response.Result, &value); err != nil {
			t.Fatal(err)
		}
		if len(value.Results) != 1 || value.Results[0].OK == failure || value.Results[0].Model != "alias-model" || value.Results[0].Protocol != "chat" {
			t.Fatalf("incorrect protocol test result: %s", response.Result)
		}
		if failure && value.Results[0].Status != 403 {
			t.Fatalf("vendor failure status was lost: %s", response.Result)
		}
		if strings.Contains(string(response.Result), "draft-fixture") {
			t.Fatal("test result exposed credential")
		}
	}
	if probes.Load() != 2 {
		t.Fatalf("expected two vendor probes, got %d", probes.Load())
	}
	after, err := os.ReadFile(engine.storePath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("connectivity test saved the draft")
	}
}
