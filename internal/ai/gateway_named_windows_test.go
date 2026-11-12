//go:build windows

package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Exercise the public listener, real runtime, original access/budget store and
// vendor HTTP boundary together. No cloud account or installed client is used.
func TestGatewayNamedKeysScopesBudgetRestartAndRevocation(t *testing.T) {
	exe := os.Getenv("WINONE_GATEWAY_TEST_EXE")
	if exe == "" {
		t.Skip("explicit isolated AI bundle")
	}
	var vendorCalls atomic.Int32
	vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vendorCalls.Add(1)
		if r.Header.Get("Authorization") != "Bearer vendor-fixture" {
			t.Error("caller key reached vendor")
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"model":"one","choices":[{"message":{"role":"assistant","content":"answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":100,"completion_tokens":10,"total_tokens":110}}`)
	}))
	defer vendor.Close()
	dir := t.TempDir()
	tr := Translator{Executable: exe, DataDirectory: filepath.Join(dir, "runtime"), UsagePath: filepath.Join(dir, "usage.jsonl")}
	providers := []RuntimeProvider{{Provider: Provider{ID: "fixture", Name: "Fixture", AuthMode: "api-key", Protocol: "chat", BaseURL: vendor.URL, Models: []string{"one", "two"}}, Key: "vendor-fixture"}}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	start := func() *GatewayProcess {
		r, err := tr.StartGateway(ctx, &MagpieSettings{NoStats: true, NoUsageStats: true}, providers)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	router := start()
	defer func() { router.Close() }()
	admin := func(path string, body any, want int) map[string]json.RawMessage {
		t.Helper()
		raw, _ := json.Marshal(body)
		r, _ := http.NewRequestWithContext(ctx, "POST", router.URL+path, bytes.NewReader(raw))
		r.Header.Set("Authorization", "Bearer "+router.Key)
		r.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		data, _ := io.ReadAll(res.Body)
		if res.StatusCode != want {
			t.Fatalf("admin %s: HTTP %d: %s", path, res.StatusCode, data)
		}
		var out map[string]json.RawMessage
		_ = json.Unmarshal(data, &out)
		return out
	}
	created := admin("/api/caller-keys/add-key", map[string]any{"name": "Laptop"}, 200)
	var secret string
	_ = json.Unmarshal(created["secret"], &secret)
	var keys []struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(created["keys"], &keys)
	if secret == "" || len(keys) != 1 {
		t.Fatal("key creation failed")
	}
	id := keys[0].ID
	policy := map[string]any{"id": id, "name": "Laptop", "models": []string{"fixture/one"}, "issues": true, "limit": map[string]any{"period": "day", "tokens": 100}}
	admin("/api/winone/gateway/configure", policy, 200)
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	allowed := []string{"fixture/one", "fixture/two"}
	g := NewGateway(ctx, filepath.Join(dir, "gateway.json"), http.NotFoundHandler(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { router.ServeMedia(w, r, allowed) }))
	defer g.Close()
	status, err := g.Save(GatewaySettings{Enabled: true, Port: port}, false)
	if err != nil {
		t.Fatal(err)
	}
	// The holder page is public HTML; its data and every model call remain
	// authenticated. Short API paths retain named-key policy enforcement.
	base := strings.TrimSuffix(status.BaseURL, "/v1")
	res, err := http.Get(base + "/me")
	if err != nil {
		t.Fatal(err)
	}
	pageBody, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || !bytes.Contains(pageBody, []byte("<html")) {
		t.Fatalf("holder page: %d", res.StatusCode)
	}
	res, err = http.Get(base + "/v1/magpie/me")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 401 {
		t.Fatal("holder data allowed without key")
	}
	call := func(token, path string, body any, want int) []byte {
		t.Helper()
		raw, _ := json.Marshal(body)
		r, _ := http.NewRequestWithContext(ctx, "POST", status.BaseURL+path, bytes.NewReader(raw))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-WinOne-Caller-Key", "forged")
		r.Header.Set("X-WinOne-Gateway-Credential", secret)
		res, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		data, _ := io.ReadAll(res.Body)
		if res.StatusCode != want {
			t.Fatalf("call %s: HTTP %d, want %d: %s", path, res.StatusCode, want, data)
		}
		return data
	}
	prompt := func(model string) map[string]any {
		return map[string]any{"model": "fixture/" + model, "messages": []map[string]string{{"role": "user", "content": "hello"}}}
	}
	call("invalid-key", "/chat/completions", prompt("one"), 401)
	call(secret, "/chat/completions", prompt("two"), 403)
	call(secret, "/chat/completions", prompt("one"), 200)
	call(secret, "/chat/completions", prompt("one"), 429)
	if vendorCalls.Load() != 1 {
		t.Fatal("blocked request reached vendor")
	}
	// The key secret is protected on disk, and the ledger stores identity only.
	found := false
	if err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && (d.Name() == "caller-keys.json" || d.Name() == "usage.jsonl") {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if bytes.Contains(data, []byte(secret)) {
				t.Error("plaintext key persisted")
			}
			if d.Name() == "caller-keys.json" {
				found = true
				if !bytes.HasPrefix(data, []byte("WinOne-DPAPI:")) {
					t.Error("key store not protected")
				}
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("missing key store")
	}
	router.Close()
	router = start()
	call(secret, "/chat/completions", prompt("one"), 429)
	admin("/api/caller-keys/reset-limit-key", map[string]string{"key": id}, 200)
	call(secret, "/chat/completions", prompt("one"), 200)
	delete(policy, "limit")
	admin("/api/winone/gateway/configure", policy, 200)
	issued := call(secret, "/magpie/keys", map[string]any{"name": "Job", "ttl": "1h", "models": []string{"fixture/one"}}, 201)
	var child struct {
		Key string `json:"key"`
	}
	if json.Unmarshal(issued, &child) != nil || child.Key == "" {
		t.Fatal("no issued key")
	}
	call(child.Key, "/chat/completions", prompt("one"), 200)
	allowed = []string{"fixture/two"}
	call(child.Key, "/chat/completions", prompt("one"), 403)
	allowed = []string{"fixture/one", "fixture/two"}
	admin("/api/caller-keys/off-key", map[string]string{"key": id}, 200)
	call(child.Key, "/chat/completions", prompt("one"), 401)
	shortRequest, _ := http.NewRequestWithContext(ctx, "POST", base+"/chat/completions", strings.NewReader(`{"model":"fixture/one"}`))
	shortRequest.Header.Set("Authorization", "Bearer "+secret)
	res, err = http.DefaultClient.Do(shortRequest)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 401 {
		t.Fatalf("short protocol path skipped key policy: %d", res.StatusCode)
	}
}
