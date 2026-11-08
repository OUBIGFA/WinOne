//go:build windows

package ai

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// Real processes share the ledger, while each launch has a different bridge
// credential. This catches double billing and identities that change on restart.
func TestRoutingLedgerOnceAndStableAcrossRestart(t *testing.T) {
	exe := os.Getenv("WINONE_ROUTING_TEST_EXE")
	if exe == "" {
		t.Skip("explicit isolated AI bundle")
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer ledger-secret" {
			t.Error("selected key lost")
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"ledger-response","model":"ledger-model","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":100,"completion_tokens":10,"total_tokens":110}}`)
	}))
	defer upstream.Close()
	dir := t.TempDir()
	ledger := filepath.Join(dir, "usage.jsonl")
	tr := Translator{Executable: exe, DataDirectory: filepath.Join(dir, "runtime"), UsagePath: ledger}
	providers := []RuntimeProvider{{Provider: Provider{ID: "ledger-provider", Name: "Ledger", AuthMode: "api-key", Protocol: "chat", BaseURL: upstream.URL, Models: []string{"ledger-model"}}, Key: "ledger-secret", Keys: []RuntimeKey{{ID: "primary", Key: "ledger-secret"}}}}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	for i := 0; i < 2; i++ {
		router, err := tr.StartRouting(ctx, &MagpieSettings{NoStats: true, NoUsageStats: true}, providers)
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest("POST", "http://localhost/v1/chat/completions", bytes.NewBufferString(`{"model":"ledger-provider/ledger-model","messages":[{"role":"user","content":"hello"}]}`)).WithContext(ctx)
		request.Header.Set("User-Agent", "codex_cli_rs/1.0")
		request.Header.Set("X-WinOne-Product", "1")
		request.Header.Set("X-WinOne-Session", "01J00000000000000000000000")
		if i == 1 {
			request = request.WithContext(WithProductSession(request.Context(), "01J00000000000000000000000"))
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		router.Close()
		if response.Code != 200 {
			t.Fatalf("call %d: %d %s", i, response.Code, response.Body.String())
		}
	}
	raw, err := os.ReadFile(ledger)
	if err != nil {
		t.Fatal(err)
	}
	rows := bytes.Split(bytes.TrimSpace(raw), []byte("\n"))
	if len(rows) != 2 {
		t.Fatalf("one row per call required: %d rows\n%s", len(rows), raw)
	}
	fingerprint := sha256.Sum256([]byte("ledger-secret"))
	for i, row := range rows {
		var r struct {
			Provider, Agent, ProviderKeyID string
			Session                        string
			Input                          int `json:"in"`
		}
		if err = json.Unmarshal(row, &r); err != nil {
			t.Fatal(err)
		}
		if r.Provider != "ledger-provider" || r.ProviderKeyID != hex.EncodeToString(fingerprint[:5]) || r.Input != 100 || r.Agent == "gateway" {
			t.Fatalf("lost execution identity: %s", row)
		}
		if i == 0 && (r.Agent == "winone" || r.Session == "01J00000000000000000000000") || i == 1 && (r.Agent != "winone" || r.Session != "01J00000000000000000000000") {
			t.Fatalf("untrusted or missing product attribution: %s", row)
		}
	}
}

func TestRoutingCatalogRefreshAndClientAlias(t *testing.T) {
	exe := os.Getenv("WINONE_ROUTING_TEST_EXE")
	if exe == "" {
		t.Skip("explicit isolated AI bundle")
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Model string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Model != "new-model" {
			t.Errorf("alias sent to vendor: %q", body.Model)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"new answer"},"finish_reason":"stop"}]}`)
	}))
	defer upstream.Close()
	p := Provider{ID: "catalog", Name: "Catalog", AuthMode: "api-key", Protocol: "chat", BaseURL: upstream.URL, Models: []string{"old-model"}}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	router, err := (Translator{Executable: exe, DataDirectory: t.TempDir()}).StartRouting(ctx, &MagpieSettings{NoStats: true, NoUsageStats: true}, []RuntimeProvider{{Provider: p, Key: "fixture"}})
	if err != nil {
		t.Fatal(err)
	}
	defer router.Close()
	p.Models = []string{"new-model"}
	p.ModelMetadata = map[string]ModelMetadata{"new-model": {Alias: "my-model"}}
	if err = router.SyncCatalog(ctx, p); err != nil {
		t.Fatal(err)
	}
	allowed := func(s Selection) bool { return s.Provider == "catalog" && s.Model == "new-model" }
	if ok, err := router.AllowsModel(ctx, "catalog/my-model", allowed, false); err != nil || !ok {
		t.Fatalf("alias policy: %v %v", ok, err)
	}
	if ok, err := router.AllowsModel(ctx, "catalog/my-model", func(Selection) bool { return false }, false); err != nil || ok {
		t.Fatalf("alias bypassed policy: %v %v", ok, err)
	}
	response := httptest.NewRecorder()
	router.ServeBody(response, httptest.NewRequest("POST", "http://localhost/v1/chat/completions", nil).WithContext(ctx), "/v1/chat/completions", []byte(`{"model":"catalog/my-model","messages":[{"role":"user","content":"hello"}]}`))
	if response.Code != 200 || !bytes.Contains(response.Body.Bytes(), []byte("new answer")) {
		t.Fatalf("updated catalog failed: %d %s", response.Code, response.Body.String())
	}
}

// A fallback is another execution target, so it must meet the same Host policy.
func TestRoutingFallbackRespectsGatewaySelection(t *testing.T) {
	exe := os.Getenv("WINONE_ROUTING_TEST_EXE")
	if exe == "" {
		t.Skip("explicit isolated AI bundle")
	}
	var backupCalls atomic.Int32
	vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer primary" {
			http.Error(w, `{"error":{"message":"upstream unavailable"}}`, 503)
			return
		}
		backupCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"fallback answer"},"finish_reason":"stop"}]}`)
	}))
	defer vendor.Close()
	providers := []RuntimeProvider{
		{Provider: Provider{ID: "primary", Name: "Primary", AuthMode: "api-key", Protocol: "chat", BaseURL: vendor.URL, Models: []string{"model"}, ProviderPreferences: ProviderPreferences{Fallback: []string{"backup/model"}}}, Key: "primary"},
		{Provider: Provider{ID: "backup", Name: "Backup", AuthMode: "api-key", Protocol: "chat", BaseURL: vendor.URL, Models: []string{"model"}}, Key: "backup"},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	router, err := (Translator{Executable: exe, DataDirectory: t.TempDir()}).StartRouting(ctx, &MagpieSettings{NoStats: true, NoUsageStats: true}, providers)
	if err != nil {
		t.Fatal(err)
	}
	defer router.Close()
	for _, allowBackup := range []bool{false, true} {
		models := []string{"primary/model"}
		if allowBackup {
			models = append(models, "backup/model")
		}
		response := httptest.NewRecorder()
		router.ServeAllowedBody(response, httptest.NewRequest("POST", "http://localhost/v1/chat/completions", nil).WithContext(ctx), "/v1/chat/completions", []byte(`{"model":"primary/model","messages":[{"role":"user","content":"hello"}]}`), models)
		if allowBackup {
			if response.Code != 200 || backupCalls.Load() != 1 {
				t.Fatalf("allowed fallback did not execute: %d %s calls=%d", response.Code, response.Body.String(), backupCalls.Load())
			}
		} else if response.Code == 200 || backupCalls.Load() != 0 {
			t.Fatalf("fallback escaped policy: %d %s", response.Code, response.Body.String())
		}
	}
}

// Exercise the real exporter: the saved switch must affect outgoing requests,
// and body collection must remain separate from enabling trace metadata.
func TestRoutingObservabilityOptIn(t *testing.T) {
	exe := os.Getenv("WINONE_ROUTING_TEST_EXE")
	if exe == "" {
		t.Skip("explicit isolated AI bundle")
	}
	vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"ai-test-private-answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":100,"completion_tokens":10}}`)
	}))
	defer vendor.Close()
	for _, mode := range []string{"off", "metadata", "bodies"} {
		t.Run(mode, func(t *testing.T) {
			exported := make(chan []byte, 8)
			collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/traces" || r.Header.Get("X-Collector-Test") != "fixture" {
					t.Errorf("collector route/header: %s", r.URL.Path)
				}
				b, _ := io.ReadAll(r.Body)
				exported <- b
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, `{}`)
			}))
			defer collector.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			prefs := &MagpieSettings{NoStats: true, NoUsageStats: true, MagpieClientSettings: MagpieClientSettings{OTel: Observability{Enabled: mode != "off", Endpoint: collector.URL, Headers: map[string]string{"X-Collector-Test": "fixture"}, Bodies: mode == "bodies"}}}
			router, err := (Translator{Executable: exe, DataDirectory: t.TempDir()}).StartRouting(ctx, prefs, []RuntimeProvider{{Provider: Provider{ID: "observed", Name: "Observed", AuthMode: "api-key", Protocol: "chat", BaseURL: vendor.URL, Models: []string{"model"}}, Key: "collector-private-key"}})
			if err != nil {
				t.Fatal(err)
			}
			defer router.Close()
			response := httptest.NewRecorder()
			router.ServeBody(response, httptest.NewRequest("POST", "http://localhost/v1/chat/completions", nil).WithContext(ctx), "/v1/chat/completions", []byte(`{"model":"observed/model","messages":[{"role":"user","content":"ai-test-private-prompt"}]}`))
			if response.Code != 200 {
				t.Fatalf("request: %d %s", response.Code, response.Body.String())
			}
			select {
			case body := <-exported:
				if mode == "off" {
					t.Fatal("disabled exporter sent data")
				}
				if !json.Valid(body) || bytes.Contains(body, []byte("collector-private-key")) {
					t.Fatalf("invalid/unsafe export: %s", body)
				}
				hasBody := bytes.Contains(body, []byte("ai-test-private-prompt")) && bytes.Contains(body, []byte("ai-test-private-answer"))
				if hasBody != (mode == "bodies") {
					t.Fatalf("body opt-in not honored: %s", body)
				}
				var payload struct {
					Resources []struct {
						Scopes []struct {
							Spans []json.RawMessage `json:"spans"`
						} `json:"scopeSpans"`
					} `json:"resourceSpans"`
				}
				if err := json.Unmarshal(body, &payload); err != nil {
					t.Fatal(err)
				}
				count := 0
				for _, r := range payload.Resources {
					for _, s := range r.Scopes {
						count += len(s.Spans)
					}
				}
				if count != 2 {
					t.Fatalf("one request root and one attempt required, got %d", count)
				}
				roots, attempts := 0, 0
				trace := ""
				for _, resource := range payload.Resources {
					for _, scope := range resource.Scopes {
						for _, raw := range scope.Spans {
							var span struct {
								TraceID  string `json:"traceId"`
								ParentID string `json:"parentSpanId"`
							}
							if err := json.Unmarshal(raw, &span); err != nil {
								t.Fatal(err)
							}
							if trace != "" && trace != span.TraceID {
								t.Fatal("one request exported as separate traces")
							}
							trace = span.TraceID
							if span.ParentID == "" {
								roots++
							} else {
								attempts++
							}
						}
					}
				}
				if roots != 1 || attempts != 1 {
					t.Fatalf("duplicate observer roots: roots=%d attempts=%d", roots, attempts)
				}
			case <-time.After(8 * time.Second):
				if mode != "off" {
					t.Fatal("saved observability opt-in exported nothing")
				}
			}
		})
	}
}
