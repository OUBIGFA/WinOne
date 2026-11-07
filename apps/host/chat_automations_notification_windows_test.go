//go:build windows

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"winone/internal/chat"
)

// The real command worker and MCP transport deliver both channels to a local
// fixture. No desktop notification, production bot or account is contacted.
func TestAutomationNotificationsUseMappedTransportAndSurfaceFailures(t *testing.T) {
	a, root, _ := commandFixture(t)
	var mu sync.Mutex
	calls := []map[string]any{}
	failRemote := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			} `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		if len(request.ID) == 0 {
			w.WriteHeader(202)
			return
		}
		var result any
		switch request.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": "2025-03-26", "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]string{"name": "notification-fixture", "version": "1"}}
		case "tools/list":
			result = map[string]any{"tools": []any{map[string]any{"name": "send_notice", "inputSchema": map[string]any{"type": "object"}}}}
		case "tools/call":
			mu.Lock()
			calls = append(calls, request.Params.Arguments)
			fail := failRemote
			mu.Unlock()
			text := "DELIVERED"
			if fail {
				text = "DELIVERY_REFUSED"
			}
			result = map[string]any{"isError": fail, "content": []any{map[string]string{"type": "text", "text": text}}}
		default:
			result = map[string]any{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
	}))
	defer server.Close()
	prefs := a.control.store.Preferences()
	prefs.MCPServers = []chat.MCPServer{{Name: "notify-local", Enabled: true, URL: server.URL}}
	if _, err := a.control.store.SavePreferences(prefs); err != nil {
		t.Fatal(err)
	}
	mappings := automationConnections{Methods: map[string]automationMethod{}}
	for _, channel := range []string{"feishu", "wecom"} {
		mappings.Methods["notify."+channel] = automationMethod{Server: "notify-local", Tool: "send_notice", Arguments: map[string]any{"channel": channel}, ParameterMap: map[string]string{"text": "body"}}
	}
	raw, _ := json.Marshal(map[string]any{"settings": mappings})
	if _, err := a.saveConnections(raw); err != nil {
		t.Fatal(err)
	}
	schedule := scheduledChat{ID: "notify", Name: "Scheduled report", WorkspaceKind: "project", WorkingDir: root}
	schedule.Notify.Feishu = true
	schedule.Notify.Wecom = true
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := a.notifyRun(ctx, schedule, automationRunNotice{ID: "notification-run", Status: "success", ResultText: "Verified local result"}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	delivered := append([]map[string]any(nil), calls...)
	mu.Unlock()
	if len(delivered) != 2 {
		t.Fatalf("expected both channels, got %#v", delivered)
	}
	channels := map[string]bool{}
	for _, call := range delivered {
		channel, _ := call["channel"].(string)
		channels[channel] = true
		if call["body"] != "Scheduled report\nVerified local result" {
			t.Fatalf("wrong notification content: %#v", call)
		}
	}
	if !channels["feishu"] || !channels["wecom"] {
		t.Fatal("channel routing changed", channels)
	}
	mu.Lock()
	failRemote = true
	mu.Unlock()
	schedule.Notify.Wecom = false
	if err := a.notifyRun(ctx, schedule, automationRunNotice{ID: "failed-notify", Status: "success", ResultText: "not delivered"}); err == nil || !strings.Contains(err.Error(), "DELIVERY_REFUSED") {
		t.Fatalf("remote failure was hidden: %v", err)
	}
	prefs.MCPServers[0].Enabled = false
	if _, err := a.control.store.SavePreferences(prefs); err != nil {
		t.Fatal(err)
	}
	if err := a.notifyRun(ctx, schedule, automationRunNotice{ID: "disabled-notify", Status: "success"}); err == nil {
		t.Fatal("disabled connector reported delivery success")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 3 {
		t.Fatalf("disabled connector still called the server: %d", len(calls))
	}
}
