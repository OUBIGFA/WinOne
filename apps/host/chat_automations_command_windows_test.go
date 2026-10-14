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
	"testing"
	"time"
	"winone/internal/ai"
	"winone/internal/chat"
)

func commandFixture(t *testing.T) (*chatAutomations, string, string) {
	t.Helper()
	bun, worker := os.Getenv("WINONE_CHAT_BUN_EXE"), os.Getenv("WINONE_CHAT_WORKER")
	if bun == "" || worker == "" {
		t.Skip("explicit isolated command bundle")
	}
	root := t.TempDir()
	base := filepath.Join(root, "bundle")
	for source, relative := range map[string]string{bun: "shared/ai/runtime/bun.exe", filepath.Join(filepath.Dir(worker), "script-runner.js"): "shared/chat/script-runner.js"} {
		target := filepath.Join(base, relative)
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.Link(source, target); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	store, err := chat.Open(filepath.Join(root, "profile", "chat"))
	if err != nil {
		t.Fatal(err)
	}
	a := newChatAutomations(&chatControl{ctx: ctx, directory: filepath.Join(root, "profile"), desktop: filepath.Join(base, "WinOne.exe"), store: store})
	a.bridge, err = chat.NewPrivateBridge(ctx, http.HandlerFunc(a.serve))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.bridge.Close)
	return a, root, bun
}
func executeFixture(t *testing.T, a *chatAutomations, schedule scheduledChat, mode string) (json.RawMessage, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	raw, err := a.command(ctx, schedule, mode, "fixture-run", time.Now().UnixMilli())
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Value json.RawMessage `json:"value"`
		Error string          `json:"error"`
	}
	if err = json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	return result.Value, result.Error
}
func TestAutomationScriptProtocolUsesRealJobAndSanitizedEnvironment(t *testing.T) {
	a, root, bun := commandFixture(t)
	t.Setenv("WINONE_TEST_SECRET", "must-not-reach-script")
	script := filepath.Join(root, "script.mjs")
	code := `import readline from 'node:readline';const lines=readline.createInterface({input:process.stdin});for await(const line of lines){const frame=JSON.parse(line);if(frame.type==='start'){if(process.env.WINONE_TEST_SECRET)throw Error('secret inherited');process.stdout.write(JSON.stringify({protocol:'cindy-script/1',type:'call',id:'cap',method:'host.capabilities'})+'\n');}else if(frame.type==='call_result'){if(!frame.ok||frame.result.methods.length!==7)throw Error('bad capability response');process.stdout.write(JSON.stringify({protocol:'cindy-script/1',type:'complete',resultText:'中文结果 ✓'})+'\n');}}`
	if err := os.WriteFile(script, []byte(code), 0600); err != nil {
		t.Fatal(err)
	}
	schedule := scheduledChat{ID: "fixture", Name: "Script fixture", ExecutionMode: "script", WorkspaceKind: "project", WorkingDir: root, ScriptConfig: &scheduledCommand{Command: fmt.Sprintf("\"%s\" \"%s\"", bun, script), Capabilities: []string{}}}
	value, failure := executeFixture(t, a, schedule, "script")
	if failure != "" {
		t.Fatal(failure)
	}
	var result struct {
		ResultText string `json:"resultText"`
	}
	if err := json.Unmarshal(value, &result); err != nil || result.ResultText != "中文结果 ✓" {
		t.Fatalf("%s %v", value, err)
	}
}
func TestAutomationHookDecisionsAndScriptTimeoutAreReal(t *testing.T) {
	a, root, bun := commandFixture(t)
	for _, code := range []int{0, 2, 7} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			script := filepath.Join(root, fmt.Sprintf("hook-%d.mjs", code))
			if err := os.WriteFile(script, []byte(fmt.Sprintf("console.log('检查');process.exit(%d)", code)), 0600); err != nil {
				t.Fatal(err)
			}
			schedule := scheduledChat{ID: "hook", Name: "Hook", WorkingDir: root, PreRunHook: &scheduledCommand{Command: fmt.Sprintf("\"%s\" \"%s\"", bun, script)}}
			value, failure := executeFixture(t, a, schedule, "hook")
			if failure != "" {
				t.Fatal(failure)
			}
			var result struct {
				Decision string `json:"decision"`
				Exit     int    `json:"exitCode"`
				Stdout   string `json:"stdout"`
			}
			if err := json.Unmarshal(value, &result); err != nil {
				t.Fatal(err)
			}
			wanted := map[int]string{0: "run", 2: "skip", 7: "block"}[code]
			if result.Decision != wanted || result.Exit != code || !strings.Contains(result.Stdout, "检查") {
				t.Fatalf("%s", value)
			}
		})
	}
	script := filepath.Join(root, "timeout.mjs")
	if err := os.WriteFile(script, []byte("setInterval(()=>{},1000)"), 0600); err != nil {
		t.Fatal(err)
	}
	schedule := scheduledChat{ID: "timeout", Name: "Timeout", ExecutionMode: "script", WorkspaceKind: "project", WorkingDir: root, ScriptConfig: &scheduledCommand{Command: fmt.Sprintf("\"%s\" \"%s\"", bun, script), TimeoutMs: 100}}
	started := time.Now()
	_, failure := executeFixture(t, a, schedule, "script")
	if !strings.Contains(failure, "timed out") {
		t.Fatalf("failure=%q", failure)
	}
	if time.Since(started) > 6*time.Second {
		t.Fatal("timeout cleanup exceeded bounded job lifetime")
	}
}
func TestAutomationDesktopNotificationRecordsDeliveryFailure(t *testing.T) {
	a := newChatAutomations(nil)
	called := false
	a.desktopNotify = func(title, text string) error {
		called = true
		if !strings.Contains(text, "Result") {
			t.Fatal(text)
		}
		return fmt.Errorf("notification rejected")
	}
	schedule := scheduledChat{Name: "Fixture"}
	schedule.Notify.Desktop = true
	if err := a.notifyRun(context.Background(), schedule, automationRunNotice{Status: "success", ResultText: "Result"}); err == nil || !called {
		t.Fatalf("called=%v err=%v", called, err)
	}
}
func TestAutomationScriptCannotOverrideHostOwnedDispatchSettings(t *testing.T) {
	a := newChatAutomations(nil)
	schedule := scheduledChat{TargetSessionID: "bound", ScriptConfig: &scheduledCommand{Capabilities: []string{"sessions.dispatch"}}}
	for _, raw := range []string{`{"message":"hi","permission_mode":"bypassPermissions"}`, `{"message":"hi","working_dir":"C:\\other"}`, `{"message":"hi","target_session_id":"other"}`} {
		if _, err := a.dispatchScript(context.Background(), schedule, json.RawMessage(raw)); err == nil {
			t.Fatal("accepted a script-owned dispatch override", raw)
		}
	}
}
func TestAutomationBoundConversationKeepsUnspecifiedModelOptions(t *testing.T) {
	store, err := chat.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := store.NewConversation("", "", "Bound", ai.Selection{Provider: "test", Model: "model"})
	if err != nil {
		t.Fatal(err)
	}
	conversation, err = store.UpdateConversation(conversation.ID, func(value *chat.Conversation) error { value.ThinkingLevel = "high"; value.FastMode = true; return nil })
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	engine := chat.NewService(ctx, store, chat.ServiceOptions{})
	a := newChatAutomations(&chatControl{ctx: ctx, store: store, engine: engine})
	result, busy, err := a.prepare(ctx, scheduledChat{TargetSessionID: conversation.ID})
	if err != nil || busy || result.ThinkingLevel != "high" || !result.FastMode {
		t.Fatalf("bound options changed: %+v busy=%v err=%v", result, busy, err)
	}
}
func TestAutomationScriptRejectsFalseCompletionAndAcceptsLegacyProtocol(t *testing.T) {
	a, root, bun := commandFixture(t)
	for _, test := range []struct{ name, source, want string }{
		{"missing", `console.error('No complete frame');`, "without a complete frame"},
		{"malformed", `console.log('not json');`, "JSONL protocol"},
		{"duplicate", `console.log(JSON.stringify({protocol:'cindy-script/1',type:'complete',resultText:'first'}));console.log(JSON.stringify({protocol:'cindy-script/1',type:'complete',resultText:'second'}));`, "more than one complete"},
		{"legacy", `console.log(JSON.stringify({protocol:'xdt-maker-script/1',type:'complete',resultText:'legacy result'}));`, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			file := filepath.Join(root, test.name+".mjs")
			if err := os.WriteFile(file, []byte(test.source), 0600); err != nil {
				t.Fatal(err)
			}
			schedule := scheduledChat{ID: test.name, Name: test.name, ExecutionMode: "script", WorkspaceKind: "project", WorkingDir: root, ScriptConfig: &scheduledCommand{Command: fmt.Sprintf("\"%s\" \"%s\"", bun, file)}}
			value, failure := executeFixture(t, a, schedule, "script")
			if test.want == "" {
				if failure != "" || !strings.Contains(string(value), "legacy result") {
					t.Fatalf("%s %s", value, failure)
				}
			} else if !strings.Contains(failure, test.want) {
				t.Fatalf("wanted %q in %q", test.want, failure)
			}
		})
	}
}

func TestAutomationScriptCallsRealLocalMCPTransport(t *testing.T) {
	a, root, bun := commandFixture(t)
	called := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      json.RawMessage `json:"id"`
			Method  string          `json:"method"`
			Params  json.RawMessage `json:"params"`
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
			result = map[string]any{"protocolVersion": "2025-03-26", "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]string{"name": "local-fixture", "version": "1"}}
		case "tools/list":
			result = map[string]any{"tools": []any{map[string]any{"name": "jira_issues", "description": "Local fixture", "inputSchema": map[string]any{"type": "object"}}}}
		case "tools/call":
			called <- string(request.Params)
			result = map[string]any{"content": []any{map[string]any{"type": "text", "text": "{\"issue\":\"EX-1\"}"}}}
		default:
			result = map[string]any{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": result})
	}))
	defer server.Close()
	prefs := a.control.store.Preferences()
	prefs.MCPServers = []chat.MCPServer{{Name: "local", Enabled: true, URL: server.URL}}
	if _, err := a.control.store.SavePreferences(prefs); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{"settings": automationConnections{Methods: map[string]automationMethod{"jira.get": {Server: "local", Tool: "jira_issues", Arguments: map[string]any{"action": "get"}}}}})
	if _, err := a.saveConnections(raw); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(root, "mcp.mjs")
	source := `import readline from 'node:readline';for await(const line of readline.createInterface({input:process.stdin})){const value=JSON.parse(line);if(value.type==='start')console.log(JSON.stringify({protocol:'cindy-script/1',type:'call',id:'jira',method:'jira.get',params:{issue_key:'EX-1'}}));else if(value.type==='call_result'){if(!value.ok)throw Error(JSON.stringify(value.error));console.log(JSON.stringify({protocol:'cindy-script/1',type:'complete',resultText:JSON.stringify(value.result)}));}}`
	if err := os.WriteFile(script, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	schedule := scheduledChat{ID: "mcp", Name: "MCP", ExecutionMode: "script", WorkspaceKind: "project", WorkingDir: root, ScriptConfig: &scheduledCommand{Command: fmt.Sprintf("\"%s\" \"%s\"", bun, script), Capabilities: []string{"jira.read"}}}
	value, failure := executeFixture(t, a, schedule, "script")
	if failure != "" {
		t.Fatal(failure)
	}
	if !strings.Contains(string(value), "EX-1") {
		t.Fatal(string(value))
	}
	select {
	case args := <-called:
		if !strings.Contains(args, "issue_key") || !strings.Contains(args, "get") {
			t.Fatal(args)
		}
	default:
		t.Fatal("MCP transport was not called")
	}
}
