//go:build windows

package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGatewayRecordingConsentPersistsWithoutExternalHistory(t *testing.T) {
	exe := os.Getenv("WINONE_ROUTING_TEST_EXE")
	if exe == "" {
		t.Skip("explicit isolated AI bundle")
	}
	vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"recorded fixture answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":4}}`)
	}))
	defer vendor.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	tr := Translator{Executable: exe, DataDirectory: filepath.Join(t.TempDir(), "private")}
	prefs := &MagpieSettings{NoStats: true, NoUsageStats: true, MagpieClientSettings: MagpieClientSettings{GitHubToken: new("fixture-github-secret"), OTel: Observability{Headers: map[string]string{"Authorization": "fixture-collector-secret"}}}}
	providers := []RuntimeProvider{{Provider: Provider{ID: "fixture", Name: "Fixture", AuthMode: "api-key", Protocol: "chat", BaseURL: vendor.URL, Models: []string{"one"}}, Key: "fixture-vendor-key"}}
	start := func() *RoutingProcess {
		r, err := tr.StartRouting(ctx, prefs, providers)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	router := start()
	defer func() { router.Close() }()
	call := func(method, path string, body any) []byte {
		t.Helper()
		raw, _ := json.Marshal(body)
		r, _ := http.NewRequestWithContext(ctx, method, router.URL+path, bytes.NewReader(raw))
		r.Header.Set("Authorization", "Bearer "+router.Key)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Magpie-Session", "recording-fixture")
		r.Header.Set("User-Agent", "WinOne-Test")
		res, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		data, _ := io.ReadAll(res.Body)
		if res.StatusCode != 200 {
			t.Fatalf("%s %d: %s", path, res.StatusCode, data)
		}
		return data
	}
	state := func() map[string]json.RawMessage {
		var out map[string]json.RawMessage
		if err := json.Unmarshal(call("GET", "/api/winone/gateway/conversations", nil), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	if string(state()["recording"]) != "false" {
		t.Fatal("recording enabled without consent")
	}
	public := call("GET", "/api/settings", nil)
	if bytes.Contains(public, []byte("fixture-github-secret")) || bytes.Contains(public, []byte("fixture-collector-secret")) {
		t.Fatal("private settings API exposed runtime credentials")
	}
	call("POST", "/api/settings/plain-names", map[string]string{"mode": "off"})
	if err := filepath.WalkDir(tr.DataDirectory, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && entry.Name() == "settings.json" {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if bytes.Contains(data, []byte("fixture-github-secret")) || bytes.Contains(data, []byte("fixture-collector-secret")) {
				t.Error("runtime credentials persisted by a native preference save")
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	call("POST", "/api/winone/gateway/conversations/recording", map[string]bool{"on": true})
	request := map[string]any{"model": "fixture/one", "messages": []any{map[string]string{"role": "user", "content": "recorded fixture question"}}}
	call("POST", "/v1/chat/completions", request)
	var sessions []struct{ Agent, ID string }
	_ = json.Unmarshal(state()["sessions"], &sessions)
	if len(sessions) != 1 {
		t.Fatalf("gateway-only sessions: %v", sessions)
	}
	read := func() string {
		return string(call("POST", "/api/winone/gateway/conversations/read", map[string]string{"agent": sessions[0].Agent, "id": sessions[0].ID}))
	}
	if !strings.Contains(read(), "recorded fixture answer") {
		t.Fatal("recording did not reach actual request")
	}
	router.Close()
	router = start()
	if string(state()["recording"]) != "true" || !strings.Contains(read(), "recorded fixture question") {
		t.Fatal("consent or transcript lost on restart")
	}
	call("POST", "/api/winone/gateway/conversations/recording", map[string]bool{"on": false})
	if !strings.Contains(read(), "recorded fixture answer") {
		t.Fatal("stopping deleted existing record")
	}
	call("POST", "/api/winone/gateway/conversations/recording", map[string]bool{"clear": true})
	if string(state()["recorded"]) != "false" {
		t.Fatal("explicit clear retained recorded text")
	}
}
