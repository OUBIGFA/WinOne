//go:build windows

package ai

import (
	"bytes"
	"context"
	"encoding/base64"
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
func TestPackagedLimitFailureMetadataReachesHost(t *testing.T) {
	executable := os.Getenv("WINONE_AI_TEST_EXE")
	if executable == "" {
		t.Skip("set WINONE_AI_TEST_EXE")
	}
	vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(403)
		io.WriteString(w, `{"error":{"message":"Your quota has been exhausted private-vendor-data","type":"insufficient_quota"}}`)
	}))
	defer vendor.Close()
	runtime := Translator{Executable: executable, DataDirectory: filepath.Join(t.TempDir(), "runtime")}
	p := Preferences{Engine: "llm", AuthMode: "api-key", ProviderID: "fixture", Protocol: "chat", BaseURL: vendor.URL + "/v1", Model: "gpt-4o", LimitRestOff: true, MagpieSettings: &MagpieSettings{NoStats: true, NoUsageStats: true}}
	check := func(err error) {
		t.Helper()
		var failure *AttemptFailure
		if !errors.As(err, &failure) || failure.Status != 403 || !failure.Limited {
			t.Fatalf("lost quota classification: %v", err)
		}
		if strings.Contains(err.Error(), "private-vendor-data") {
			t.Fatal("vendor body leaked")
		}
	}
	_, err := runtime.GenerateStream(context.Background(), p, "fixture", Generation{Text: "hello"}, func(Delta) error { t.Fatal("failed request emitted content"); return nil })
	check(err)
	response := httptest.NewRecorder()
	_, err = runtime.GatewayRequest(context.Background(), p, "fixture", "/v1/chat/completions", json.RawMessage(`{"messages":[{"role":"user","content":"hello"}]}`), nil, response)
	check(err)
	if response.Body.Len() != 0 {
		t.Fatal("failed gateway forwarded a body")
	}
	_, err = runtime.Models(context.Background(), Provider{ID: "fixture", AuthMode: "api-key", Protocol: "chat", BaseURL: vendor.URL + "/v1", LimitRestOff: true}, "fixture")
	check(err)
}

func TestPackagedGlobalProxyPreservesLoginAndRoutesOnlyThroughSelectedProxy(t *testing.T) {
	executable := os.Getenv("WINONE_AI_TEST_EXE")
	if executable == "" {
		t.Skip("set WINONE_AI_TEST_EXE")
	}
	var calls atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Proxy-Authorization") != "Basic "+base64.StdEncoding.EncodeToString([]byte("na@me:p:ss")) {
			t.Error("proxy login was not encoded correctly")
		}
		if r.URL.Host != "vendor.invalid" {
			t.Error("unexpected proxy destination")
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/models" {
			io.WriteString(w, `{"data":[{"id":"gpt-4o"}]}`)
			return
		}
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"via proxy"},"finish_reason":"stop"}]}`)
	}))
	defer proxy.Close()
	address := strings.Replace(proxy.URL, "http://", "http://na%40me:p%3Ass@", 1)
	settings := &MagpieSettings{Proxy: address, ProxyCustom: address, NoStats: true, NoUsageStats: true}
	runtime := Translator{Executable: executable, DataDirectory: filepath.Join(t.TempDir(), "runtime")}
	p := Preferences{Engine: "llm", AuthMode: "api-key", ProviderID: "fixture", Protocol: "chat", BaseURL: "http://vendor.invalid/v1", Model: "gpt-4o", MagpieSettings: settings}
	reply, err := runtime.Generate(context.Background(), p, "fixture-key", Generation{Text: "hello"})
	if err != nil || reply != "via proxy" {
		t.Fatal(reply, err)
	}
	if calls.Load() != 1 {
		t.Fatalf("proxy calls=%d", calls.Load())
	}
}

func TestPackagedLocalArchiveIsOptInSharedAndFinishedBeforeReply(t *testing.T) {
	executable := os.Getenv("WINONE_AI_TEST_EXE")
	if executable == "" {
		t.Skip("set WINONE_AI_TEST_EXE")
	}
	vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"archive reply"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":2}}`)
	}))
	defer vendor.Close()
	root := t.TempDir()
	runtime := Translator{Executable: executable, DataDirectory: root}
	archiveRoot := filepath.Join(root, "config", "magpie", "request-archive")
	files := func() []string {
		var out []string
		err := filepath.WalkDir(archiveRoot, func(path string, d os.DirEntry, err error) error {
			if os.IsNotExist(err) {
				return nil
			}
			if err != nil {
				return err
			}
			if !d.IsDir() {
				out = append(out, path)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	p := Preferences{Engine: "llm", AuthMode: "api-key", Protocol: "chat", BaseURL: vendor.URL + "/v1", Model: "gpt-4o", MagpieSettings: &MagpieSettings{NoStats: true, NoUsageStats: true}}
	send := func(provider string) {
		t.Helper()
		p.ProviderID = provider
		reply, err := runtime.ForProvider(provider).Generate(context.Background(), p, "archive-secret-fixture", Generation{Text: "archive-content-fixture"})
		if err != nil || reply != "archive reply" {
			t.Fatal(reply, err)
		}
	}
	send("first")
	if len(files()) != 0 {
		t.Fatal("archive was enabled by default")
	}
	p.MagpieSettings.RequestArchiveLocal = true
	p.MagpieSettings.RequestArchiveLocalMB = 16
	p.MagpieSettings.RequestArchiveMaxMB = 1
	send("first")
	send("second")
	archived := files()
	if len(archived) != 2 {
		t.Fatalf("expected two completed files in shared archive, got %v", archived)
	}
	for _, file := range archived {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(raw, []byte("archive-content-fixture")) || !bytes.Contains(raw, []byte("archive reply")) {
			t.Fatal("archive did not keep both bodies")
		}
		if bytes.Contains(raw, []byte("archive-secret-fixture")) {
			t.Fatal("archive leaked the authorization key")
		}
	}
	p.MagpieSettings.RequestArchiveLocal = false
	send("first")
	if len(files()) != 2 {
		t.Fatal("turning off archive removed old files or kept a new call")
	}
	// Read the archived body through the same worker used by the settings UI.
	relative, err := filepath.Rel(archiveRoot, archived[0])
	if err != nil {
		t.Fatal(err)
	}
	id := strings.TrimSuffix(filepath.ToSlash(relative), ".json")
	preview, err := runtime.RequestArchive(context.Background(), id)
	if err != nil {
		t.Fatalf("archive preview failed: %v", err)
	}
	var record struct {
		ID                string `json:"id"`
		Request, Response struct {
			Body string `json:"body"`
		}
	}
	if json.Unmarshal(preview, &record) != nil || record.ID != id || !strings.Contains(record.Request.Body, "archive-content-fixture") || !strings.Contains(record.Response.Body, "archive reply") {
		t.Fatal("archive preview lost request or response")
	}
	exported, err := runtime.invokeBounded(context.Background(), map[string]any{"operation": "archive.file", "archive": id}, 2<<20)
	original, readErr := os.ReadFile(archived[0])
	if err != nil || readErr != nil || !bytes.Equal(exported, original) {
		t.Fatal("archive export did not preserve complete file", err, readErr)
	}
	var streamed bytes.Buffer
	if err := runtime.CopyRequestArchive(context.Background(), id, &streamed); err != nil || !bytes.Equal(streamed.Bytes(), original) {
		t.Fatal("streamed archive export differs", err)
	}
	// The original bodies remain exportable when either upstream's text limit
	// or escaped JSON's desktop-frame limit makes an inline preview too large.
	for _, body := range []string{strings.Repeat("x", (256<<10)+1), strings.Repeat("\x01", 180<<10)} {
		full, _ := json.Marshal(map[string]any{"id": id, "request": map[string]any{"body": body, "headers": map[string]any{}}, "response": map[string]any{"body": "ok", "headers": map[string]any{}}})
		if err := os.WriteFile(archived[0], full, 0600); err != nil {
			t.Fatal(err)
		}
		preview, err := runtime.RequestArchive(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		var out struct {
			Large   bool
			Request struct {
				Omitted bool
				Body    string
			}
		}
		if json.Unmarshal(preview, &out) != nil || (!out.Large && !out.Request.Omitted) || len(preview) > 768<<10 {
			t.Fatal("large preview did not offer full-file export")
		}
		streamed.Reset()
		if err := runtime.CopyRequestArchive(context.Background(), id, &streamed); err != nil || !bytes.Equal(streamed.Bytes(), full) {
			t.Fatal("large export truncated", err)
		}
	}
	for _, bad := range []string{"../settings", "2026-10-26/../../secret", "2026-10-26/missing"} {
		if _, err := runtime.invoke(context.Background(), map[string]any{"operation": "archive.read", "archive": bad}); err == nil {
			t.Fatalf("invalid or missing archive accepted: %q", bad)
		}
	}
}

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
