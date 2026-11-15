//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
	"winone/internal/ai"
	"winone/internal/host"
)

// Magpie's provider concurrency is the admission policy for model requests.
// Five live calls catch a second, hard-coded four-job limit in the Host.
func TestAIModelRequestsUseUpstreamAdmissionBeyondFourJobs(t *testing.T) {
	if os.Getenv("WINONE_NETWORK_TESTS") != "1" {
		t.Skip("network tests are excluded from automatic verification; set WINONE_NETWORK_TESTS=1 to run explicitly")
	}
	service := gatewayTestService(t)
	release := make(chan struct{})
	entered := make(chan struct{}, 5)
	vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ADMITTED\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer vendor.Close()
	defer close(release)
	store, err := service.magpieStore()
	if err != nil {
		t.Fatal(err)
	}
	configuration := store.Configuration()
	configuration.MagpieSettings = &ai.MagpieSettings{NoStats: true, NoUsageStats: true}
	configuration.Providers = []ai.Provider{{ID: "admission", Name: "Admission", AuthMode: "api-key", Protocol: "chat", BaseURL: vendor.URL + "/v1", Models: []string{"model"}, RequestLimits: ai.RequestLimits{MaxConcurrency: new(5)}}}
	configuration.Tasks["chat"] = ai.Selection{Provider: "admission", Model: "model"}
	key := "fixture-admission"
	if _, err = store.SaveConfiguration(configuration, map[string]*string{"admission": &key}); err != nil {
		t.Fatal(err)
	}
	var jobs []aiJob
	call := func(method string, params any) host.Response {
		raw, _ := json.Marshal(params)
		return service.handle(host.Request{ID: "admission", Method: method, PeerPID: uint32(os.Getpid()), Params: raw})
	}
	defer func() {
		for _, job := range jobs {
			call("jobs.cancel", map[string]string{"id": job.ID})
		}
	}()
	for i := 0; i < 5; i++ {
		response := call("generate.start", map[string]string{"text": "fixture"})
		if response.Error != nil {
			t.Fatalf("Host rejected model request %d before Magpie admission: %v", i+1, response.Error)
		}
		var job aiJob
		if err = json.Unmarshal(response.Result, &job); err != nil {
			t.Fatal(err)
		}
		jobs = append(jobs, job)
	}
	for range jobs {
		select {
		case <-entered:
		case <-time.After(10 * time.Second):
			t.Fatal("an admitted model request did not reach its vendor")
		}
	}
}

// Saving a provider changes the next request, without destroying the upstream
// Server that is still delivering a reply to an existing result window.
func TestAIConfigurationSaveKeepsRunningModelAndAppliesNewConnection(t *testing.T) {
	if os.Getenv("WINONE_NETWORK_TESTS") != "1" {
		t.Skip("network tests are excluded from automatic verification; set WINONE_NETWORK_TESTS=1 to run explicitly")
	}
	service := gatewayTestService(t)
	release, entered := make(chan struct{}), make(chan struct{}, 1)
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"FIRST\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer first.Close()
	defer unblock()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"SECOND\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer second.Close()
	store, err := service.magpieStore()
	if err != nil {
		t.Fatal(err)
	}
	configuration := store.Configuration()
	configuration.MagpieSettings = &ai.MagpieSettings{NoStats: true, NoUsageStats: true}
	configuration.Providers = []ai.Provider{{ID: "connection", Name: "Connection", AuthMode: "api-key", Protocol: "chat", BaseURL: first.URL + "/v1", Models: []string{"model"}}}
	configuration.Tasks["chat"] = ai.Selection{Provider: "connection", Model: "model"}
	key := "fixture-connection"
	if _, err = store.SaveConfiguration(configuration, map[string]*string{"connection": &key}); err != nil {
		t.Fatal(err)
	}
	call := func(method string, params any) host.Response {
		raw, _ := json.Marshal(params)
		return service.handle(host.Request{ID: "connection", Method: method, PeerPID: uint32(os.Getpid()), Params: raw})
	}
	start := func() aiJob {
		t.Helper()
		response := call("generate.start", map[string]string{"text": "fixture"})
		if response.Error != nil {
			t.Fatal(response.Error)
		}
		var job aiJob
		if err := json.Unmarshal(response.Result, &job); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { call("jobs.cancel", map[string]string{"id": job.ID}) })
		return job
	}
	complete := func(job aiJob, want string) {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for job.State == "running" && time.Now().Before(deadline) {
			response := call("jobs.get", map[string]string{"id": job.ID})
			if response.Error != nil || json.Unmarshal(response.Result, &job) != nil {
				t.Fatal("could not read the model result", response.Error)
			}
			if job.State == "running" {
				time.Sleep(10 * time.Millisecond)
			}
		}
		if job.State != "completed" || job.Text != want {
			t.Fatalf("expected %s from the selected connection, got %+v", want, job)
		}
	}
	oldJob := start()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("the initial model request did not reach its provider")
	}
	configuration = store.Configuration()
	configuration.Providers[0].BaseURL = second.URL + "/v1"
	if response := call("configuration.save", map[string]any{"configuration": configuration}); response.Error != nil {
		t.Fatal(response.Error)
	}
	complete(start(), "SECOND")
	unblock()
	complete(oldJob, "FIRST")
}

// Text size is an IPC boundary, not a model-window estimate. A translation
// beyond the former 20,000-character/100,000-byte caps reaches Magpie unchanged.
func TestAITranslationBeyondOldByteLimitUsesUpstreamInput(t *testing.T) {
	if os.Getenv("WINONE_NETWORK_TESTS") != "1" {
		t.Skip("network tests are excluded from automatic verification; set WINONE_NETWORK_TESTS=1 to run explicitly")
	}
	service := gatewayTestService(t)
	text := strings.Repeat("word ", 25000)
	vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct{ Content string }
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Messages) < 2 || body.Messages[1].Content != text {
			t.Error("the translation's complete input did not reach the provider", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"TRANSLATED\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer vendor.Close()
	store, err := service.magpieStore()
	if err != nil {
		t.Fatal(err)
	}
	configuration := store.Configuration()
	configuration.Engine = "llm"
	configuration.MagpieSettings = &ai.MagpieSettings{NoStats: true, NoUsageStats: true}
	configuration.Providers = []ai.Provider{{ID: "translation", Name: "Translation", AuthMode: "api-key", Protocol: "chat", BaseURL: vendor.URL + "/v1", Models: []string{"model"}}}
	configuration.Tasks["default"] = ai.Selection{Provider: "translation", Model: "model"}
	key := "fixture-translation"
	if _, err = store.SaveConfiguration(configuration, map[string]*string{"translation": &key}); err != nil {
		t.Fatal(err)
	}
	call := func(method string, params any) host.Response {
		raw, _ := json.Marshal(params)
		return service.handle(host.Request{ID: "translation", Method: method, PeerPID: uint32(os.Getpid()), Params: raw})
	}
	response := call("translate.start", map[string]string{"text": text})
	if response.Error != nil {
		t.Fatal("Host rejected text before the model's own context handling", response.Error)
	}
	var job aiJob
	if err = json.Unmarshal(response.Result, &job); err != nil {
		t.Fatal(err)
	}
	defer call("jobs.cancel", map[string]string{"id": job.ID})
	deadline := time.Now().Add(10 * time.Second)
	for job.State == "running" && time.Now().Before(deadline) {
		response = call("jobs.get", map[string]string{"id": job.ID})
		if response.Error != nil || json.Unmarshal(response.Result, &job) != nil {
			t.Fatal("could not read translation result", response.Error)
		}
		if job.State == "running" {
			time.Sleep(10 * time.Millisecond)
		}
	}
	if job.State != "completed" || job.Text != "TRANSLATED" {
		t.Fatal("the long translation was not delivered", job.State, job.Error)
	}
}
