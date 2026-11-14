//go:build windows

package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Inspect the context at the real runtime boundary, without waiting ten
// minutes. No implicit whole-turn deadline may override Magpie's own transport.
func TestUpstreamModelRequestsKeepCallerDeadline(t *testing.T) {
	for _, method := range []string{"generate", "translate"} {
		t.Run(method, func(t *testing.T) {
			runtime := Translator{RuntimeRequest: func(ctx context.Context, input map[string]any) (io.ReadCloser, error) {
				if deadline, ok := ctx.Deadline(); ok {
					t.Errorf("model request added a whole-turn deadline: %v", deadline)
				}
				return io.NopCloser(strings.NewReader("{\"type\":\"text\",\"delta\":\"UPSTREAM\"}\n{\"type\":\"done\"}\n")), nil
			}}
			preferences := Preferences{Engine: "llm", ProviderID: "fixture", Model: "model"}
			var text string
			var err error
			if method == "translate" {
				text, err = runtime.Translate(context.Background(), preferences, "", "fixture")
			} else {
				text, err = runtime.Generate(context.Background(), preferences, "", Generation{Text: "fixture"})
			}
			if err != nil || text != "UPSTREAM" {
				t.Fatal("the upstream runtime response was not delivered", text, err)
			}
		})
	}
}

func TestUpstreamCatalogDefaultsStayLiveAfterSelection(t *testing.T) {
	executable := os.Getenv("WINONE_AI_TEST_EXE")
	if executable == "" {
		t.Skip("set WINONE_AI_TEST_EXE")
	}
	var window atomic.Int64
	window.Store(128000)
	vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"data":[{"id":"live-model","context_length":%d,"max_output_tokens":384000,"supported_reasoning_levels":[{"effort":"light"},{"effort":"deep"}],"modalities":{"input":["text","image","audio","video","pdf"],"output":["text"]}}]}`, window.Load())
			return
		}
		var body map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		for _, key := range []string{"max_tokens", "max_completion_tokens", "reasoning_effort", "service_tier"} {
			if _, set := body[key]; set {
				t.Errorf("model default was forced into %s", key)
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"DEFAULT_OK\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer vendor.Close()
	directory := t.TempDir()
	store, err := Open(filepath.Join(directory, "ai.json"))
	if err != nil {
		t.Fatal(err)
	}
	runtime := Translator{Executable: executable, DataDirectory: filepath.Join(directory, "runtime")}
	p := Provider{ID: "relay", Name: "Relay", AuthMode: "api-key", Protocol: "chat", BaseURL: vendor.URL + "/v1"}
	refresh := func() {
		t.Helper()
		models, err := runtime.Models(t.Context(), p, "fixture-key")
		if err != nil {
			t.Fatal(err)
		}
		p.RefreshModels(models)
		p.Models = []string{"live-model"}
		configuration := store.Configuration()
		configuration.Providers = []Provider{p}
		configuration.Tasks["default"] = Selection{Provider: p.ID, Model: "live-model"}
		key := "fixture-key"
		if _, err = store.SaveConfiguration(configuration, map[string]*string{p.ID: &key}); err != nil {
			t.Fatal(err)
		}
		p = store.Configuration().Providers[0]
		if !reflect.DeepEqual(p.ModelMetadataFor("live-model"), ModelMetadata{}) {
			t.Fatal("a vendor report became an override")
		}
	}
	refresh()
	providers, err := store.MagpieProviders()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	backend, err := runtime.StartGateway(ctx, nil, providers)
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	check := func(want int) {
		t.Helper()
		w := httptest.NewRecorder()
		backend.ServeMedia(w, httptest.NewRequest("GET", "/v1/models", nil), []string{"relay/live-model"})
		var result struct {
			Data []map[string]any `json:"data"`
		}
		if json.Unmarshal(w.Body.Bytes(), &result) != nil || len(result.Data) != 1 {
			t.Fatalf("catalog: %d %s", w.Code, w.Body.String())
		}
		model := result.Data[0]
		if model["context_window"] != float64(want) || model["max_output_tokens"] != float64(want) {
			t.Fatalf("upstream model limits: %v", model)
		}
		modalities, _ := model["modalities"].(map[string]any)
		if !reflect.DeepEqual(modalities["input"], []any{"text", "image", "audio", "video", "pdf"}) {
			t.Fatalf("lost model input capabilities: %v", model)
		}
	}
	check(128000)
	window.Store(256000)
	refresh()
	if err = backend.SyncCatalog(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	check(256000)
	preferences, _, err := store.ForTaskKeys("chat", nil)
	if err != nil {
		t.Fatal(err)
	}
	runtime.RuntimeRequest = backend.Generate
	if text, err := runtime.Generate(t.Context(), preferences, "", Generation{Text: "hello"}); err != nil || text != "DEFAULT_OK" {
		t.Fatal(text, err)
	}
}

// Exercise the actual upstream Server instead of testing a second Host
// implementation of ordering or concurrency with a simulated clock.
func TestUpstreamRuntimeOwnsKeyRotationAndConcurrency(t *testing.T) {
	executable := os.Getenv("WINONE_AI_TEST_EXE")
	if executable == "" {
		t.Skip("set WINONE_AI_TEST_EXE")
	}
	for _, scenario := range []string{"rotation", "concurrency"} {
		t.Run(scenario, func(t *testing.T) {
			var mu sync.Mutex
			var seen []string
			entered, release := make(chan struct{}, 3), make(chan struct{})
			var active, maximum atomic.Int32
			vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				key := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
				mu.Lock()
				seen = append(seen, key)
				mu.Unlock()
				n := active.Add(1)
				defer active.Add(-1)
				for old := maximum.Load(); n > old && !maximum.CompareAndSwap(old, n); old = maximum.Load() {
				}
				if scenario == "concurrency" {
					entered <- struct{}{}
					select {
					case <-release:
					case <-r.Context().Done():
						return
					}
				}
				w.Header().Set("Content-Type", "text/event-stream")
				io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"OK\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
			}))
			defer vendor.Close()
			p := Provider{ID: "keys", Name: "Keys", AuthMode: "api-key", Protocol: "chat", BaseURL: vendor.URL + "/v1", Models: []string{"model"}, ProviderPreferences: ProviderPreferences{Routing: "rotate", Affinity: "off"}}
			keys := []RuntimeKey{{ID: "first", Key: "first"}, {ID: "second", Key: "second"}}
			if scenario == "concurrency" {
				keys = keys[:1]
				p.RequestLimits = RequestLimits{MaxConcurrency: new(1), QueueLimit: 1}
			}
			runtime := Translator{Executable: executable, DataDirectory: t.TempDir()}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			backend, err := runtime.StartGateway(ctx, nil, []RuntimeProvider{{Provider: p, Key: keys[0].Key, Keys: keys}})
			if err != nil {
				t.Fatal(err)
			}
			defer backend.Close()
			runtime.RuntimeRequest = backend.Generate
			preferences := Preferences{ProviderID: p.ID, Model: "model", AuthMode: "api-key", Engine: "llm"}
			call := func(ctx context.Context) error {
				_, err := runtime.Generate(ctx, preferences, "", Generation{Text: "fixture"})
				return err
			}
			if scenario == "rotation" {
				for range 3 {
					if err = call(t.Context()); err != nil {
						t.Fatal(err)
					}
				}
				mu.Lock()
				got := append([]string{}, seen...)
				mu.Unlock()
				if !reflect.DeepEqual(got, []string{"first", "second", "first"}) {
					t.Fatalf("upstream rotation lost: %v", got)
				}
				return
			}
			finished := make(chan error, 1)
			go func() { finished <- call(t.Context()) }()
			select {
			case <-entered:
			case <-time.After(10 * time.Second):
				t.Fatal("first request did not reach the vendor")
			}
			waiting, stop := context.WithTimeout(t.Context(), 200*time.Millisecond)
			err = call(waiting)
			stop()
			close(release)
			if err == nil {
				t.Fatal("a queued canceled request reported success")
			}
			if err = <-finished; err != nil {
				t.Fatal(err)
			}
			if maximum.Load() != 1 {
				t.Fatalf("upstream concurrency was bypassed: %d", maximum.Load())
			}
			if err = call(t.Context()); err != nil {
				t.Fatal("canceled queue entry retained a slot", err)
			}
		})
	}
}
