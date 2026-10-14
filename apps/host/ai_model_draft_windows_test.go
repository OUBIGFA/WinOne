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
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"winone/internal/ai"
	"winone/internal/host"
)

func TestModelDiscoveryUsesUnsavedConnectionWithoutSaving(t *testing.T) {
	exe := os.Getenv("WINONE_AI_TEST_EXE")
	if exe == "" {
		t.Skip("explicit packaged AI worker required")
	}
	for _, scenario := range []string{"new", "saved-key", "replacement", "anonymous", "failover"} {
		t.Run(scenario, func(t *testing.T) {
			var received []string
			var mu sync.Mutex
			vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				key := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
				mu.Lock()
				received = append(received, key)
				mu.Unlock()
				if key == "limited" {
					w.WriteHeader(429)
					fmt.Fprint(w, `{"error":"private error body"}`)
					return
				}
				fmt.Fprint(w, `{"data":[{"id":"draft-model","name":"Draft model"}]}`)
			}))
			defer vendor.Close()
			service := newAIService(context.Background(), t.TempDir(), exe)
			service.translator.Executable = exe
			store, err := ai.Open(service.storePath)
			if err != nil {
				t.Fatal(err)
			}
			service.store = store
			c := store.Configuration()
			c.Providers = []ai.Provider{{ID: "stored", Name: "Stored", AuthMode: "api-key", Protocol: "chat", BaseURL: "https://saved.example/v1", Models: []string{}}}
			c.Tasks = map[string]ai.Selection{}
			savedKey := "saved-secret"
			if _, err = store.SaveConfiguration(c, map[string]*string{"stored": &savedKey}); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(service.storePath)
			if err != nil {
				t.Fatal(err)
			}
			id := "unsaved"
			keys := []map[string]any{{"id": "primary", "enabled": true}}
			edits := map[string]string{"primary": "draft-secret"}
			want := []string{"draft-secret"}
			switch scenario {
			case "saved-key":
				id = "stored"
				edits = nil
				want = []string{"saved-secret"}
			case "replacement":
				id = "stored"
				edits["primary"] = "replacement-secret"
				want = []string{"replacement-secret"}
			case "anonymous":
				keys = []map[string]any{}
				edits = nil
				want = []string{""}
			case "failover":
				keys = []map[string]any{{"id": "a", "enabled": true}, {"id": "b", "enabled": true}}
				edits = map[string]string{"a": "limited", "b": "fallback"}
				want = []string{"limited", "fallback"}
			}
			payload, _ := json.Marshal(map[string]any{"provider": id, "connection": map[string]any{"protocol": "chat", "baseUrl": vendor.URL + "/v1", "keys": keys}, "keyUpdates": edits})
			response := service.handle(host.Request{ID: "models", Method: "models.fetch", PeerPID: uint32(os.Getpid()), Params: payload})
			if response.Error != nil {
				t.Fatal(response.Error)
			}
			if !strings.Contains(string(response.Result), "draft-model") || strings.Contains(string(response.Result), "secret") {
				t.Fatal("model response invalid or exposed credentials")
			}
			mu.Lock()
			actual := append([]string(nil), received...)
			mu.Unlock()
			if !reflect.DeepEqual(actual, want) {
				t.Fatalf("discovery keys = %v, want %v", received, want)
			}
			after, err := os.ReadFile(service.storePath)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("model discovery changed saved configuration")
			}
			loaded, err := ai.Open(filepath.Clean(service.storePath))
			if err != nil {
				t.Fatal(err)
			}
			_, key, err := loaded.ProviderForModels("stored")
			if err != nil || key != savedKey {
				t.Fatal("model discovery replaced the saved credential")
			}
		})
	}
}
