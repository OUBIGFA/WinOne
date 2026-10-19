//go:build windows

package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Exercise WinOne's owned worker and Magpie's image guard with real local
// HTTP requests. Merely accepting the saved setting would miss propagation.
func TestPackagedPrivateImageFetchSetting(t *testing.T) {
	executable := os.Getenv("WINONE_AI_TEST_EXE")
	if executable == "" {
		t.Skip("set WINONE_AI_TEST_EXE")
	}
	var pngBody bytes.Buffer
	if err := png.Encode(&pngBody, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	var downloads, vendorCalls atomic.Int32
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		downloads.Add(1)
		if r.URL.Path == "/not-image" {
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"fixture-private-value":true}`)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngBody.Bytes())
	}))
	defer source.Close()
	vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vendorCalls.Add(1)
		if !strings.HasSuffix(r.URL.Path, "/images/edits") {
			t.Errorf("unexpected vendor path: %s", r.URL.Path)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"created":1,"data":[{"b64_json":"cHJvb2Y="}]}`)
	}))
	defer vendor.Close()
	runtime := Translator{Executable: executable, DataDirectory: filepath.Join(t.TempDir(), "runtime")}
	prefs := Preferences{Engine: "llm", AuthMode: "api-key", Protocol: "chat", BaseURL: vendor.URL + "/v1", Model: "gpt-image-1", MagpieSettings: &MagpieSettings{NoStats: true, NoUsageStats: true}}
	send := func(path string) error {
		t.Helper()
		body, _ := json.Marshal(map[string]any{"model": "gpt-image-1", "prompt": "fixture edit", "image": source.URL + path})
		_, err := runtime.GatewayRequest(context.Background(), prefs, "fixture-private-key", "/v1/images/edits", body, nil, httptest.NewRecorder())
		return err
	}
	blocked := func(err error) {
		t.Helper()
		var failure *AttemptFailure
		if !errors.As(err, &failure) || failure.Status != 400 {
			t.Fatalf("expected private/image rejection, got %v", err)
		}
	}
	blocked(send("/image.png"))
	if downloads.Load() != 0 || vendorCalls.Load() != 0 {
		t.Fatal("default request reached a private service")
	}
	prefs.MagpieSettings.FetchPrivate = []string{"127.0.0.1"}
	if err := send("/image.png"); err != nil {
		t.Fatal("explicit fetch allowlist did not reach worker", err)
	}
	if downloads.Load() != 1 || vendorCalls.Load() != 1 {
		t.Fatal("allowed image was not fetched exactly once")
	}
	blocked(send("/not-image"))
	if vendorCalls.Load() != 1 {
		t.Fatal("non-image content reached vendor")
	}
	prefs.MagpieSettings.FetchPrivate = nil
	blocked(send("/image.png"))
	if downloads.Load() != 2 {
		t.Fatal("allowlist escaped its request settings")
	}
}

// This crosses persistence, immutable task selection, the shared scheduler,
// worker stdin and Magpie before inspecting what the local vendor received.
func TestPackagedModelSettingsAndSharedRequestLimits(t *testing.T) {
	executable := os.Getenv("WINONE_AI_TEST_EXE")
	if executable == "" {
		t.Skip("set WINONE_AI_TEST_EXE")
	}
	var calls atomic.Int32
	vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := calls.Add(1)
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		want := float64(2048)
		if call == 2 {
			want = 30
		}
		if body["max_completion_tokens"] != want && body["max_tokens"] != want {
			t.Errorf("output limit missing: %v", body)
		}
		if body["reasoning_effort"] != "high" || body["service_tier"] != "priority" {
			t.Errorf("model choices missing: %v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"ok"}}]}`)
	}))
	defer vendor.Close()
	directory := t.TempDir()
	store, err := Open(filepath.Join(directory, "ai.json"))
	if err != nil {
		t.Fatal(err)
	}
	config := store.Configuration()
	config.Providers = []Provider{{ID: "test", Name: "Test", AuthMode: "api-key", Protocol: "chat", BaseURL: vendor.URL + "/v1", Models: []string{"gpt-5"}, ModelMetadata: map[string]ModelMetadata{"gpt-5": {ContextWindow: 128000, MaxTokens: 2048, CompactionPercent: 72.5, ListedContextWindow: 128000, MaxContextWindow: 1000000, ReasoningEffort: "high", Tier: "priority"}}, RequestLimits: RequestLimits{MaxRPM: 1}}}
	if _, err = store.SaveConfiguration(config, nil); err != nil {
		t.Fatal(err)
	}
	store, err = Open(filepath.Join(directory, "ai.json"))
	if err != nil {
		t.Fatal(err)
	}
	p, _, err := store.ForTaskKeys("chat", &Selection{Provider: "test", Model: "gpt-5"})
	if err != nil {
		t.Fatal(err)
	}
	translator := (Translator{Executable: executable, DataDirectory: filepath.Join(directory, "runtime")}).ForProvider("test")
	if reply, err := translator.Generate(context.Background(), p, "key", Generation{Text: "hello"}); err != nil || reply != "ok" {
		t.Fatal(reply, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	body := json.RawMessage(`{"model":"gpt-5","messages":[{"role":"user","content":"again"}],"max_tokens":30}`)
	if _, err = translator.GatewayRequest(ctx, p, "key", "/v1/chat/completions", body, nil, httptest.NewRecorder()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("gateway did not share result RPM", err)
	}
	if calls.Load() != 1 {
		t.Fatal("rate-limited call reached upstream")
	}
	response := httptest.NewRecorder()
	if _, err = translator.GatewayRequest(context.Background(), p, "second-key", "/v1/chat/completions", body, nil, response); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 || response.Code != 200 {
		t.Fatal("independent key or raw request failed", calls.Load(), response.Code)
	}
}
