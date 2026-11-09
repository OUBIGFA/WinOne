//go:build windows

package main

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"winone/internal/ai"
	"winone/internal/host"
)

func TestExternalExportStopDoesNotDependOnSavedPreferences(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/winone/external/settings" || r.Header.Get("Authorization") != "Bearer fixture-key" {
			t.Error("incorrect Host export channel")
		}
		var in struct{ On bool }
		if json.NewDecoder(r.Body).Decode(&in) != nil || in.On {
			t.Error("stop was not sent")
		}
		called = true
		w.Write([]byte(`{}`))
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "broken-settings.json")
	if err := os.WriteFile(path, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	s := &aiService{ctx: context.Background(), storePath: path}
	s.external.process = &ai.RoutingProcess{URL: server.URL, Key: "fixture-key"}
	s.external.viewers = map[string]bool{"fixture": true}
	response := s.handleExternal(host.Request{Method: "external.export", Params: json.RawMessage(`{"viewer":"fixture","on":false}`)})
	if response.Error != nil || !called {
		t.Fatalf("explicit stop blocked by unrelated settings: %+v", response.Error)
	}
}

func TestExternalExportRejectsAViewerWithoutAnOpenLease(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	defer server.Close()
	s := &aiService{ctx: context.Background()}
	s.external.process = &ai.RoutingProcess{URL: server.URL, Key: "fixture-key"}
	response := s.handleExternal(host.Request{Method: "external.export", Params: json.RawMessage(`{"viewer":"fixture","on":true}`)})
	if response.Error == nil || called {
		t.Fatal("an unopened viewer reached the external exporter")
	}
}

func TestExternalHostKeepsEnabledServiceAndRestoresItsEndpoint(t *testing.T) {
	exe := os.Getenv("WINONE_AI_TEST_EXE")
	if exe == "" {
		t.Skip("set WINONE_AI_TEST_EXE for actual worker lifecycle")
	}
	root := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service := newAIService(ctx, root, filepath.Join(root, "WinOne.Host.exe"))
	service.translator.Executable = exe
	defer service.closeExternal()
	request := func(s *aiService, method string) {
		t.Helper()
		response := s.handleExternal(host.Request{Method: method, Params: json.RawMessage(`{"viewer":"fixture"}`)})
		if response.Error != nil {
			t.Fatalf("%s: %+v", method, response.Error)
		}
	}
	request(service, "external.open")
	first := service.external.process
	library := func(process *ai.RoutingProcess, on bool) {
		t.Helper()
		payload := `{"on":false}`
		if on {
			payload = `{"on":true}`
		}
		r, err := http.NewRequest(http.MethodPost, process.URL+"/api/library/servers/hub", strings.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Authorization", "Bearer "+process.Key)
		response, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != 200 {
			data, _ := io.ReadAll(response.Body)
			t.Fatalf("library switch %d %s", response.StatusCode, data)
		}
	}
	library(first, true)
	state, err := first.ExternalState(ctx)
	if err != nil || !state.Enabled || !state.Listening {
		t.Fatalf("real MCP service did not start: %+v %v", state, err)
	}
	request(service, "external.close")
	if service.external.process != first || !first.Alive() {
		t.Fatal("closing page terminated enabled client service")
	}
	service.closeExternal()
	if first.Alive() {
		t.Fatal("WinOne exit retained external worker")
	}
	restored := newAIService(ctx, root, filepath.Join(root, "WinOne.Host.exe"))
	restored.translator.Executable = exe
	defer restored.closeExternal()
	restored.restoreExternal()
	if restored.external.process == nil {
		t.Fatal("saved enabled service was not restored")
	}
	state2, err := restored.external.process.ExternalState(ctx)
	if err != nil || !state2.Enabled || !state2.Listening || state2.Exporting || state2.URL != state.URL {
		t.Fatalf("restored service: %+v %v", state2, err)
	}
	request(restored, "external.open")
	library(restored.external.process, false)
	request(restored, "external.close")
	if restored.external.process != nil {
		t.Fatal("disabled worker retained after last page closed")
	}
	address := strings.TrimPrefix(state.URL, "http://")
	conn, err := net.DialTimeout("tcp", address, time.Second)
	if err == nil {
		conn.Close()
		t.Fatal("disabled MCP listener retained its port")
	}
}

// The manager and external client pages must see the same installed plugin.
// A real stdio tool verifies the relay beyond merely accepting configuration.
func TestExternalHostSharesPluginToolsWithClientLibrary(t *testing.T) {
	exe := os.Getenv("WINONE_AI_TEST_EXE")
	if exe == "" {
		t.Skip("set WINONE_AI_TEST_EXE for actual plugin runtime")
	}
	root := t.TempDir()
	runtimeRoot := filepath.Join(root, "ai-runtime")
	settingsDir := filepath.Join(runtimeRoot, "external-clients", "config", "magpie")
	if err := os.MkdirAll(settingsDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(settingsDir, "settings.json"), []byte(`{"noWSLAgents":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	shared := filepath.Join(runtimeRoot, "third-party")
	if err := os.MkdirAll(shared, 0700); err != nil {
		t.Fatal(err)
	}
	spec := filepath.Join(root, "fixture-tool.mjs")
	if err := os.WriteFile(spec, []byte(`export default async()=>({tool:{ping:{description:"Fixture ping",args:{},execute:async()=>"external-plugin-fixture-ok"}}})`), 0600); err != nil {
		t.Fatal(err)
	}
	registry, _ := json.Marshal(map[string]any{"plugins": []any{map[string]string{"spec": spec}}})
	names, _ := json.Marshal(map[string][]string{spec: {"ping"}})
	for name, data := range map[string][]byte{"plugins.json": registry, "plugin-tools.json": names} {
		if err := os.WriteFile(filepath.Join(shared, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	s := newAIService(ctx, root, filepath.Join(root, "WinOne.Host.exe"))
	s.translator.Executable = exe
	defer s.closeExternal()
	response := s.handleExternal(host.Request{Method: "external.open", Params: json.RawMessage(`{"viewer":"fixture"}`)})
	if response.Error != nil {
		t.Fatal(response.Error)
	}
	p := s.external.process
	call := func(method, path, key, body string) []byte {
		t.Helper()
		r, err := http.NewRequestWithContext(ctx, method, path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Authorization", "Bearer "+key)
		r.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != 200 {
			t.Fatalf("%s %s: %d %s", method, path, resp.StatusCode, raw)
		}
		return raw
	}
	raw := call("GET", p.URL+"/api/library", p.Key, "")
	var view struct {
		Servers []struct{ Name, Plugin, URL string }
	}
	if err := json.Unmarshal(raw, &view); err != nil {
		t.Fatal(err)
	}
	var server struct{ Name, Plugin, URL string }
	for _, candidate := range view.Servers {
		if candidate.Plugin != "" {
			server = candidate
		}
	}
	if server.Plugin == "" {
		t.Fatalf("installed plugin was absent from external library: %s", raw)
	}
	body, _ := json.Marshal(map[string]any{"name": server.Plugin, "on": true})
	call("POST", p.URL+"/api/library/servers/plugin-on", p.Key, string(body))
	var marker struct {
		Key  string
		Port int
	}
	data, err := os.ReadFile(filepath.Join(runtimeRoot, "external-clients", "config", "magpie", "external-service.json"))
	if err != nil {
		t.Fatal(err)
	}
	json.Unmarshal(data, &marker)
	if !strings.HasPrefix(server.URL, "http://127.0.0.1:"+strconv.Itoa(marker.Port)+"/") {
		t.Fatalf("plugin server used wrong endpoint: %s, owned port %d", server.URL, marker.Port)
	}
	tools := call("POST", server.URL, marker.Key, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	if !strings.Contains(string(tools), `"ping"`) {
		noted, _ := os.ReadFile(filepath.Join(shared, "plugin-tools.json"))
		t.Fatalf("plugin tools missing at %s: %s; noted=%s", server.URL, tools, noted)
	}
	result := call("POST", server.URL, marker.Key, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"ping","arguments":{}}}`)
	if !strings.Contains(string(result), "external-plugin-fixture-ok") {
		t.Fatalf("plugin tool did not execute: %s", result)
	}
	body, _ = json.Marshal(map[string]any{"name": server.Plugin, "on": false})
	call("POST", p.URL+"/api/library/servers/plugin-on", p.Key, string(body))
	raw = call("GET", p.URL+"/api/library", p.Key, "")
	if !strings.Contains(string(raw), `"hidden":true`) {
		t.Fatalf("hidden state not kept: %s", raw)
	}
}
