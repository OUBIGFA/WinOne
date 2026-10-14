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
	"sync"
	"testing"
	"time"
	"winone/internal/ai"
	"winone/internal/host"
)

func TestAIConfigurationAcceptsPerKeyEditsWithoutReturningSecrets(t *testing.T) {
	s := newAIService(context.Background(), t.TempDir(), "unused.exe")
	store, err := ai.Open(filepath.Join(t.TempDir(), "ai.json"))
	if err != nil {
		t.Fatal(err)
	}
	s.store = store
	c := store.Configuration()
	c.Providers = []ai.Provider{{ID: "one", Name: "One", AuthMode: "api-key", Protocol: "chat", BaseURL: "https://example.invalid/v1", Models: []string{"m"}, Keys: []ai.KeySlot{{ID: "a", Enabled: true}, {ID: "b", Enabled: true}}}}
	input, _ := json.Marshal(map[string]any{"configuration": c, "keyUpdates": map[string]any{"one": map[string]string{"a": "fixture-secret-a", "b": "fixture-secret-b"}}})
	response := s.handle(host.Request{ID: "keys", Method: "configuration.save", PeerPID: uint32(os.Getpid()), Params: input})
	if response.Error != nil {
		t.Fatal(response.Error)
	}
	if strings.Contains(string(response.Result), "fixture-secret") {
		t.Fatal("save response returned plaintext")
	}
	if len(store.Configuration().Providers[0].Keys) != 2 {
		t.Fatal("per-key updates were not saved")
	}
}

func TestAIRequestsUseKeyRotationAndRespectStreamFailoverBoundary(t *testing.T) {
	exe := os.Getenv("WINONE_AI_TEST_EXE")
	if exe == "" {
		t.Skip("explicit packaged magpie test")
	}
	for _, scenario := range []string{"rotate", "unauthorized", "bad-request", "partial", "models"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			defer cancel()
			var mu sync.Mutex
			var keys []string
			vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				key := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
				mu.Lock()
				keys = append(keys, key)
				mu.Unlock()
				if scenario == "models" {
					if key == "first" {
						w.Header().Set("Retry-After", "120")
						w.WriteHeader(429)
						fmt.Fprint(w, `{"error":"private vendor body"}`)
					} else {
						fmt.Fprint(w, `{"data":[{"id":"remote-model"}]}`)
					}
					return
				}
				if key == "first" && (scenario == "unauthorized" || scenario == "bad-request") {
					status := 401
					if scenario == "bad-request" {
						status = 400
					}
					w.WriteHeader(status)
					fmt.Fprint(w, `{"error":{"message":"private vendor body"}}`)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				if scenario == "partial" {
					fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"partial answer\"}}]}\n\n")
					w.(http.Flusher).Flush()
					return
				}
				fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"complete answer\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
			}))
			defer vendor.Close()
			service := newAIService(ctx, t.TempDir(), exe)
			service.translator.Executable = exe
			store, err := ai.Open(filepath.Join(t.TempDir(), "ai.json"))
			if err != nil {
				t.Fatal(err)
			}
			service.store = store
			c := store.Configuration()
			c.Providers = []ai.Provider{{ID: "one", Name: "One", AuthMode: "api-key", Protocol: "chat", BaseURL: vendor.URL + "/v1", Models: []string{"m"}, Keys: []ai.KeySlot{{ID: "a", Enabled: true}, {ID: "b", Enabled: true}}}}
			c.Tasks["chat"] = ai.Selection{Provider: "one", Model: "m"}
			first, second := "first", "second"
			if _, err = store.SaveConfigurationWithKeys(c, nil, ai.CredentialUpdates{"one": {"a": &first, "b": &second}}); err != nil {
				t.Fatal(err)
			}
			call := func(method string, params any) host.Response {
				raw, _ := json.Marshal(params)
				return service.handle(host.Request{ID: "key-test", Method: method, PeerPID: uint32(os.Getpid()), Params: raw})
			}
			if scenario == "models" {
				response := call("models.fetch", map[string]string{"provider": "one"})
				if response.Error != nil || !strings.Contains(string(response.Result), "remote-model") {
					t.Fatalf("model failover: %v %s", response.Error, response.Result)
				}
			} else {
				jobs := 1
				if scenario == "rotate" {
					jobs = 3
				}
				if scenario == "unauthorized" {
					jobs = 2
				}
				for range jobs {
					response := call("generate.start", map[string]string{"text": "fixture"})
					if response.Error != nil {
						t.Fatal(response.Error)
					}
					var job struct{ ID, State, Text, Error string }
					if err = json.Unmarshal(response.Result, &job); err != nil {
						t.Fatal(err)
					}
					for job.State == "running" && ctx.Err() == nil {
						time.Sleep(10 * time.Millisecond)
						response = call("jobs.get", map[string]string{"id": job.ID})
						if response.Error != nil {
							t.Fatal(response.Error)
						}
						if err = json.Unmarshal(response.Result, &job); err != nil {
							t.Fatal(err)
						}
					}
					want := "completed"
					if scenario == "bad-request" || scenario == "partial" {
						want = "failed"
					}
					if job.State != want {
						t.Fatalf("job state=%s error=%s", job.State, job.Error)
					}
					if strings.Contains(job.Error, "private vendor body") {
						t.Fatal("vendor error body leaked")
					}
					if scenario == "partial" && !strings.Contains(job.Text, "partial answer") {
						t.Fatal("partial stream was not retained")
					}
				}
			}
			mu.Lock()
			defer mu.Unlock()
			want := []string{"first", "second"}
			switch scenario {
			case "rotate":
				want = []string{"first", "second", "first"}
			case "unauthorized":
				want = []string{"first", "second", "second"}
			case "bad-request", "partial":
				want = []string{"first"}
			}
			if strings.Join(keys, ",") != strings.Join(want, ",") {
				t.Fatalf("HTTP key sequence=%v want=%v", keys, want)
			}
		})
	}
}
