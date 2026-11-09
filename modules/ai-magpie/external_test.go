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

	"github.com/coder/websocket"

	"github.com/yetone/magpie/internal/agent"
	"github.com/yetone/magpie/internal/appdir"
	"github.com/yetone/magpie/internal/browserext"
	"github.com/yetone/magpie/internal/gateway"
	"github.com/yetone/magpie/internal/library"
	"github.com/yetone/magpie/internal/otlp"
	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/sessions"
	"github.com/yetone/magpie/internal/settings"
)

// Each test owns a private Magpie store and generated external-client folders.
// The build entry also removes inherited profile selectors before Go starts.
func externalFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	priorDirectory := appdir.Portable()
	priorSettings := settings.WinOneSettings.Load()
	priorProviders := provider.WinOneProviders
	if err := appdir.WinOneExternalRoot(root); err != nil {
		t.Fatal(err)
	}
	settings.WinOneSettings.Store(nil)
	writeExternalFixture(t, settings.Path(), `{"noWSLAgents":true}`)
	library.WinOneExternalUseGateway("", "")
	browserext.WinOneExternal = false
	browserext.WinOneExternalKey = ""
	agent.WinOneExternalKey = ""
	sessions.Reset()
	t.Cleanup(func() {
		settings.WinOneSettings.Store(priorSettings)
		provider.WinOneProviders = priorProviders
		if priorDirectory == "" {
			appdir.UseExecutable("")
		} else {
			appdir.WinOneExternalRoot(priorDirectory)
		}
		settings.WinOneExternalOTel(nil)
		sessions.Reset()
		browserext.WinOneExternal = false
		browserext.WinOneExternalKey = ""
		agent.WinOneExternalKey = ""
		library.WinOneExternalUseGateway("", "")
	})
	t.Setenv("WINONE_AI_EXTERNAL_DATA", root)
	t.Setenv("MAGPIE_ADDR", "")
	t.Setenv("MAGPIE_OTEL_ENABLED", "")
	os.Unsetenv("MAGPIE_OTEL_ENABLED")
	return root
}

func writeExternalFixture(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestExternalDirectServersDoNotKeepWorkerAlive(t *testing.T) {
	root := externalFixture(t)
	writeExternalFixture(t, filepath.Join(root, "library.json"), `{"mcp":[{"name":"direct","transport":"http","url":"https://example.invalid/mcp"}]}`)
	s := &externalService{}
	if err := s.refresh(false); err != nil {
		t.Fatal(err)
	}
	if s.Enabled {
		t.Fatal("a direct remote MCP server incorrectly keeps the WinOne worker alive")
	}
}

func TestExternalDisablingLastServiceReleasesPort(t *testing.T) {
	root := externalFixture(t)
	writeExternalFixture(t, filepath.Join(root, "library.json"), `{"mcpHub":true}`)
	s := &externalService{}
	defer s.close()
	if err := s.start(); err != nil {
		t.Fatal(err)
	}
	if err := s.refresh(false); err != nil {
		t.Fatal(err)
	}
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(s.Port))
	writeExternalFixture(t, filepath.Join(root, "library.json"), `{"mcpHub":false,"skillRouter":false}`)
	if err := s.refresh(false); err != nil {
		t.Fatal(err)
	}
	conn, err := net.DialTimeout("tcp", address, time.Second)
	if err == nil {
		conn.Close()
		t.Fatal("disabled client service still listens")
	}
	if s.Enabled || s.server != nil {
		t.Fatal("disabled client service remains retained")
	}
}

func TestExternalCompletedSignInRestartsRelay(t *testing.T) {
	root := externalFixture(t)
	s := &externalService{}
	defer s.close()
	if err := s.start(); err != nil {
		t.Fatal(err)
	}
	if err := s.refresh(false); err != nil {
		t.Fatal(err)
	}
	writeExternalFixture(t, filepath.Join(root, "library.json"), `{"mcp":[{"name":"signed-in","transport":"http","url":"https://example.invalid/mcp"}]}`)
	writeExternalFixture(t, filepath.Join(root, "mcp-signins.json"), `{"signed-in":{"url":"https://example.invalid/mcp","access":"fixture-token"}}`)
	if err := s.refresh(false); err != nil {
		t.Fatal(err)
	}
	if !s.Enabled || s.server == nil {
		t.Fatal("completed asynchronous MCP sign-in did not restore its actual relay listener")
	}
}

func TestExternalBrowserConnectionIsClosedWhenDisabled(t *testing.T) {
	externalFixture(t)
	s := &externalService{}
	defer s.close()
	if err := s.start(); err != nil {
		t.Fatal(err)
	}
	if err := s.refresh(true); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, gateway.URL()+gateway.BrowserExtensionPath+"?key="+s.Key, &websocket.DialOptions{HTTPHeader: http.Header{"Origin": []string{browserext.Origin}}})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	s.mu.Lock()
	s.Browser = false
	s.mu.Unlock()
	if err := s.refresh(false); err != nil {
		t.Fatal(err)
	}
	if _, _, err := conn.Read(ctx); err == nil {
		t.Fatal("disabled browser connection still reads")
	}
	if s.Enabled || s.server != nil {
		t.Fatal("disabled browser kept service listening")
	}
}

func externalHTTP(t *testing.T, method, url, key, body string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	client := http.Client{Timeout: 5 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, data
}

func TestExternalEndpointPersistsAndServesAuthenticatedMCP(t *testing.T) {
	root := externalFixture(t)
	writeExternalFixture(t, filepath.Join(root, "library.json"), `{"mcpHub":true}`)
	s := &externalService{}
	defer s.close()
	if err := s.start(); err != nil {
		t.Fatal(err)
	}
	if err := s.refresh(false); err != nil {
		t.Fatal(err)
	}
	endpoint, key, port := gateway.URL(), s.Key, s.Port
	init := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"WinOne fixture","version":"1"}}}`
	if code, _ := externalHTTP(t, "POST", endpoint+gateway.HubMCPAll, "", init); code != 401 {
		t.Fatalf("unkeyed MCP status %d", code)
	}
	if code, data := externalHTTP(t, "POST", endpoint+gateway.HubMCPAll, key, init); code != 200 || !strings.Contains(string(data), "serverInfo") {
		t.Fatalf("MCP initialize: %d %s", code, data)
	}
	if code, _ := externalHTTP(t, "POST", endpoint+"/v1/chat/completions", key, `{}`); code != 404 {
		t.Fatalf("external endpoint exposed model route: %d", code)
	}
	s.close()
	restored, err := loadExternalService()
	if err != nil {
		t.Fatal(err)
	}
	defer restored.close()
	if !restored.Enabled || restored.Key != key || restored.Port != port {
		t.Fatal("persistent endpoint identity changed")
	}
	if err := restored.start(); err != nil {
		t.Fatal(err)
	}
	if code, data := externalHTTP(t, "POST", endpoint+gateway.HubMCPAll, key, init); code != 200 || !strings.Contains(string(data), "serverInfo") {
		t.Fatalf("restored MCP initialize: %d %s", code, data)
	}
}

func startExternalFixture(t *testing.T) (string, string) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	input, send := io.Pipe()
	output, receive := io.Pipe()
	done := make(chan error, 1)
	key := strings.Repeat("fixture-key-", 6)
	go func() { done <- serveExternal(ctx, request{Key: key}, input, receive) }()
	var state struct{ URL string }
	if err := json.NewDecoder(output).Decode(&state); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		send.Close()
		input.Close()
		receive.Close()
		output.Close()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("external worker did not stop")
		}
	})
	return state.URL, key
}

func TestExternalSessionConsentTakesEffectImmediately(t *testing.T) {
	root := externalFixture(t)
	client := filepath.Join(root, "client-claude")
	t.Setenv("CLAUDE_CONFIG_DIR", client)
	writeExternalFixture(t, filepath.Join(client, "projects", "work", "fixture.jsonl"), `{"type":"user","timestamp":"2026-09-20T10:00:00Z","message":{"role":"user","content":"private-fixture"},"cwd":"/work","sessionId":"fixture"}`+"\n")
	endpoint, key := startExternalFixture(t)
	// Startup and status restore neither a transcript index nor an exporter.
	code, data := externalHTTP(t, "GET", endpoint+"/api/winone/external/status", key, "")
	if code != 200 || strings.Contains(string(data), `"exporting":true`) {
		t.Fatalf("startup status %d %s", code, data)
	}
	code, data = externalHTTP(t, "POST", endpoint+"/api/sessions/reads", key, `{"all":false,"agents":{"claude":true}}`)
	if code != 200 {
		t.Fatalf("consent: %d %s", code, data)
	}
	code, data = externalHTTP(t, "GET", endpoint+"/api/sessions/manage", key, "")
	if code != 200 || !strings.Contains(string(data), `"id":"fixture"`) {
		t.Fatalf("consent not applied to actual session read: %d %s", code, data)
	}
	if !strings.Contains(string(data), `"terminal":true`) {
		t.Fatal("Windows session resume was hidden although upstream supports its terminal")
	}
	code, data = externalHTTP(t, "POST", endpoint+"/api/sessions/reads", key, `{"agents":{"claude":false}}`)
	if code != 200 {
		t.Fatalf("revoke: %d %s", code, data)
	}
	code, data = externalHTTP(t, "GET", endpoint+"/api/sessions/manage", key, "")
	if code != 200 || strings.Contains(string(data), `"id":"fixture"`) {
		t.Fatalf("revoked session still visible: %d %s", code, data)
	}
}

func TestExternalExporterRequiresHostAndExplicitStart(t *testing.T) {
	externalFixture(t)
	endpoint, key := startExternalFixture(t)
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, `{}`) }))
	defer collector.Close()
	body := `{"on":true,"magpieSettings":{"otel":{"enabled":true,"endpoint":"` + collector.URL + `","headers":{"Authorization":"Bearer fixture-collector"}}}}`
	if code, _ := externalHTTP(t, "POST", endpoint+"/session/"+key+"/api/winone/external/settings", "", body); code != 401 {
		t.Fatalf("page capability started exporter: %d", code)
	}
	if code, data := externalHTTP(t, "POST", endpoint+"/api/winone/external/settings", key, body); code != 200 {
		t.Fatalf("host start: %d %s", code, data)
	}
	if code, data := externalHTTP(t, "GET", endpoint+"/api/winone/external/status", key, ""); code != 200 || !strings.Contains(string(data), `"exporting":true`) {
		t.Fatalf("exporter not running: %d %s", code, data)
	}
	if code, data := externalHTTP(t, "POST", endpoint+"/api/winone/external/settings", key, `{"on":false}`); code != 200 {
		t.Fatalf("host stop: %d %s", code, data)
	}
	if code, data := externalHTTP(t, "GET", endpoint+"/api/winone/external/status", key, ""); code != 200 || strings.Contains(string(data), `"exporting":true`) {
		t.Fatalf("exporter retained: %d %s", code, data)
	}
	saved, _ := os.ReadFile(settings.Path())
	if strings.Contains(string(saved), "fixture-collector") {
		t.Fatal("Host collector credential persisted by external page")
	}
}

func TestExternalExporterActuallyExportsNewFixtureSessionAndStops(t *testing.T) {
	root := externalFixture(t)
	pi := filepath.Join(root, "pi")
	t.Setenv("PI_CODING_AGENT_DIR", pi)
	t.Setenv("PI_CODING_AGENT_SESSION_DIR", "")
	path := filepath.Join(pi, "sessions", "work", "fixture_session.jsonl")
	old := time.Now().Add(-time.Minute)
	header, _ := json.Marshal(map[string]any{"type": "session", "id": "export-fixture", "timestamp": old})
	writeExternalFixture(t, path, string(header)+"\n")
	if err := settings.SetSessionReads(func(*settings.SessionReads) *settings.SessionReads {
		return &settings.SessionReads{Asked: true, Agents: map[string]bool{"pi": true}}
	}); err != nil {
		t.Fatal(err)
	}
	priorEvery, priorSessions := otlp.OTelEvery, otlp.OTelSessionsEvery
	otlp.OTelEvery, otlp.OTelSessionsEvery = 50*time.Millisecond, 20*time.Millisecond
	t.Cleanup(func() { otlp.OTelEvery, otlp.OTelSessionsEvery = priorEvery, priorSessions })
	received := make(chan []byte, 16)
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fixture-collector" {
			t.Error("saved Host collector credential missing")
		}
		data, _ := io.ReadAll(r.Body)
		received <- data
		io.WriteString(w, `{}`)
	}))
	defer collector.Close()
	endpoint, key := startExternalFixture(t)
	select {
	case <-received:
		t.Fatal("default external worker exported sessions")
	case <-time.After(100 * time.Millisecond):
	}
	body := `{"on":true,"magpieSettings":{"otel":{"enabled":true,"endpoint":"` + collector.URL + `","headers":{"Authorization":"Bearer fixture-collector"}}}}`
	if code, data := externalHTTP(t, "POST", endpoint+"/api/winone/external/settings", key, body); code != 200 {
		t.Fatalf("export start %d %s", code, data)
	}
	appendTurn := func(id string) {
		t.Helper()
		now := time.Now()
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		for _, entry := range []map[string]any{
			{"type": "message", "id": id, "timestamp": now, "message": map[string]any{"role": "user", "content": "PRIVATE-USER"}},
			{"type": "message", "id": id + "-reply", "parentId": id, "timestamp": now.Add(time.Millisecond), "message": map[string]any{"role": "assistant", "timestamp": now.UnixMilli(), "model": "fixture-model", "provider": "magpie", "stopReason": "stop", "content": "PRIVATE-REPLY", "usage": map[string]int{"input": 10, "output": 5}}},
			{"type": "custom", "parentId": id + "-reply", "customType": "timing-final", "timestamp": now.Add(2 * time.Millisecond), "data": map[string]any{"totalMs": 2, "endAt": now.Add(2 * time.Millisecond).UnixMilli()}},
		} {
			data, _ := json.Marshal(entry)
			if _, err := f.Write(append(data, '\n')); err != nil {
				t.Fatal(err)
			}
		}
	}
	appendTurn("fresh")
	select {
	case data := <-received:
		if !strings.Contains(string(data), "resourceSpans") || strings.Contains(string(data), "PRIVATE-USER") || strings.Contains(string(data), "PRIVATE-REPLY") {
			t.Fatalf("session trace/privacy: %s", data)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("actual new fixture session was never exported")
	}
	if code, data := externalHTTP(t, "POST", endpoint+"/api/winone/external/settings", key, `{"on":false}`); code != 200 {
		t.Fatalf("export stop %d %s", code, data)
	}
	for len(received) > 0 {
		<-received
	}
	appendTurn("after-stop")
	select {
	case <-received:
		t.Fatal("session exported after explicit stop")
	case <-time.After(150 * time.Millisecond):
	}
}

func TestExternalBuiltinModelServerNeedsAnEnabledHostGateway(t *testing.T) {
	externalFixture(t)
	s := &externalService{}
	defer s.close()
	if err := s.start(); err != nil {
		t.Fatal(err)
	}
	if _, err := library.InstallServer("magpie-web-search", nil, nil); err == nil {
		t.Fatal("built-in model MCP installed an endpoint without a running Host gateway")
	}
}

func TestExternalBuiltinInstallUsesHostGatewayAndBrowserUsesDedicatedService(t *testing.T) {
	root := externalFixture(t)
	s := &externalService{}
	defer s.close()
	if err := s.start(); err != nil {
		t.Fatal(err)
	}
	library.WinOneExternalUseGateway("http://127.0.0.1:45678", "fixture-host-key")
	for _, id := range []string{"magpie-web-search", "magpie-vision", "magpie-image", "magpie-browser"} {
		if _, err := library.InstallServer(id, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := os.ReadFile(filepath.Join(root, "library.json"))
	if err != nil {
		t.Fatal(err)
	}
	var lib library.Library
	if err := json.Unmarshal(raw, &lib); err != nil {
		t.Fatal(err)
	}
	if len(lib.MCP) != 4 {
		t.Fatalf("installed built-ins: %d", len(lib.MCP))
	}
	for _, server := range lib.MCP {
		if server.Name == "magpie-browser" {
			if server.Env["WINONE_AI_MCP_KEY"] != s.Key || server.Env["MAGPIE_GATEWAY"] != "" {
				t.Fatal("browser sent to model gateway")
			}
		} else if server.Env["WINONE_AI_MCP_KEY"] != "fixture-host-key" || server.Env["MAGPIE_GATEWAY"] != "http://127.0.0.1:45678" {
			t.Fatal("model built-in lost authenticated Host endpoint")
		}
	}
}

func TestExternalBrowserReadPreservesExistingExtensionFiles(t *testing.T) {
	root := externalFixture(t)
	t.Setenv("USERPROFILE", filepath.Join(root, "client-home"))
	t.Setenv("HOME", filepath.Join(root, "client-home"))
	path := filepath.Join(browserext.Dir(), "manifest.json")
	writeExternalFixture(t, path, `{"version":"user-owned-fixture"}`)
	endpoint, key := startExternalFixture(t)
	if code, data := externalHTTP(t, "GET", endpoint+"/api/library/browser", key, ""); code != 200 {
		t.Fatalf("browser state %d %s", code, data)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != `{"version":"user-owned-fixture"}` {
		t.Fatal("reading browser state rewrote the installed extension")
	}
}

func TestExternalBrowserStopOverridesSavedBrowserEntry(t *testing.T) {
	root := externalFixture(t)
	writeExternalFixture(t, filepath.Join(root, "library.json"), `{"mcp":[{"name":"magpie-browser","transport":"stdio","command":"WinOne.AI.exe","args":["mcp","browser"]}]}`)
	s := &externalService{}
	defer s.close()
	if err := s.start(); err != nil {
		t.Fatal(err)
	}
	if err := s.refresh(false); err != nil {
		t.Fatal(err)
	}
	s.Browser = false
	s.BrowserOff = true
	if err := s.refresh(false); err != nil {
		t.Fatal(err)
	}
	if s.Enabled || s.server != nil {
		t.Fatal("saved browser entry undid explicit service stop")
	}
	restored, err := loadExternalService()
	if err != nil {
		t.Fatal(err)
	}
	if restored.Enabled || !restored.BrowserOff {
		t.Fatal("browser stop was not persistent")
	}
}

func TestExternalBrowserStopLeavesHubAliveAndDisconnectsExtension(t *testing.T) {
	root := externalFixture(t)
	writeExternalFixture(t, filepath.Join(root, "library.json"), `{"mcpHub":true}`)
	s := &externalService{}
	defer s.close()
	if err := s.start(); err != nil {
		t.Fatal(err)
	}
	if err := s.refresh(true); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, gateway.URL()+gateway.BrowserExtensionPath+"?key="+s.Key, &websocket.DialOptions{HTTPHeader: http.Header{"Origin": []string{browserext.Origin}}})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	if err := s.stopBrowser(); err != nil {
		t.Fatal(err)
	}
	if !s.Enabled || s.server == nil {
		t.Fatal("stopping browser stopped enabled MCP hub")
	}
	if _, _, err := conn.Read(ctx); err == nil {
		t.Fatal("stopped browser extension still connects")
	} else if ctx.Err() != nil {
		t.Fatal("browser extension remained open until the test deadline")
	}
	if code, _ := externalHTTP(t, "GET", gateway.URL()+gateway.BrowserStatusPath, s.Key, ""); code != 503 {
		t.Fatalf("stopped browser still accepted calls: %d", code)
	}
	if code, data := externalHTTP(t, "POST", gateway.URL()+gateway.HubMCPAll, s.Key, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"fixture","version":"1"}}}`); code != 200 || !strings.Contains(string(data), "serverInfo") {
		t.Fatalf("hub no longer serves %d %s", code, data)
	}
}

func TestExternalPluginManagerReadsPrivateStatusWithoutClientAccess(t *testing.T) {
	root := externalFixture(t)
	shared := filepath.Join(root, "shared-plugins")
	spec := filepath.Join(root, "fixture-plugin.mjs")
	t.Setenv("WINONE_AI_PLUGINS_HOME", shared)
	libraryPath := filepath.Join(root, "external", "library.json")
	t.Setenv("WINONE_AI_EXTERNAL_LIBRARY", libraryPath)
	registry, _ := json.Marshal(map[string]any{"plugins": []any{map[string]string{"spec": spec}}})
	names, _ := json.Marshal(map[string][]string{spec: {"ping"}})
	writeExternalFixture(t, filepath.Join(shared, "plugins.json"), string(registry))
	writeExternalFixture(t, filepath.Join(shared, "plugin-tools.json"), string(names))
	offered := library.PluginServers()
	if len(offered) != 1 {
		t.Fatalf("Installed did not offer cached plugin tools: %+v", offered)
	}
	var name string
	for id := range offered {
		name = id
	}
	if offered[name].On {
		t.Fatal("new tools unexpectedly enabled for clients")
	}
	if _, err := os.Stat(libraryPath); !os.IsNotExist(err) {
		t.Fatal("reading Installed wrote the external library")
	}
	data, _ := json.Marshal(map[string]any{"mcp": []any{map[string]any{"name": "fixture", "plugin": name, "agents": []string{"codex"}}}})
	writeExternalFixture(t, libraryPath, string(data))
	if state := library.PluginServers(); !state[name].On {
		t.Fatalf("Installed did not read external private status: %+v", state)
	}
	data, _ = json.Marshal(map[string]any{"mcp": []any{map[string]any{"name": "fixture", "plugin": name, "hidden": true, "agents": []string{}}}, "pluginsOffered": []string{name}})
	writeExternalFixture(t, libraryPath, string(data))
	if state := library.PluginServers(); state[name].On {
		t.Fatalf("Installed lost hidden status: %+v", state)
	}
}

func TestExternalUnassignedPluginDoesNotKeepServiceAlive(t *testing.T) {
	root := externalFixture(t)
	writeExternalFixture(t, filepath.Join(root, "library.json"), `{"mcp":[{"name":"fixture","plugin":"fixture","transport":"http","url":"http://127.0.0.1:1/mcp/magpie/tools/fixture","agents":[]}]}`)
	s := &externalService{}
	defer s.close()
	if err := s.start(); err != nil {
		t.Fatal(err)
	}
	if err := s.refresh(false); err != nil {
		t.Fatal(err)
	}
	if s.Enabled || s.server != nil {
		t.Fatal("viewing unassigned plugin started a persistent service")
	}
}
