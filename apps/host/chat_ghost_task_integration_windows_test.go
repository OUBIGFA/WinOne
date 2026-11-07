//go:build windows

package main

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"winone/internal/ai"
	"winone/internal/chat"
)

// Genuine plugin package -> native WebView sandbox -> Cindy task service ->
// private Go Host -> Pi -> deterministic HTTP model. No simulated task receipts.
func TestGhostTaskRoundTripWithRealPiAndSandbox(t *testing.T) {
	bun, worker, plugin := os.Getenv("WINONE_CHAT_BUN_EXE"), os.Getenv("WINONE_CHAT_WORKER"), os.Getenv("WINONE_GHOST_SANDBOX")
	if bun == "" || worker == "" || plugin == "" {
		t.Skip("explicit bundled Pi and native plugin sandbox required")
	}
	root := t.TempDir()
	bundle := filepath.Join(root, "bundle")
	assets := filepath.Join(bundle, "shared", "chat")
	bunDir := filepath.Join(bundle, "shared", "ai", "runtime")
	for _, dir := range []string{assets, bunDir} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	copyOneBotTestRuntime(t, bun, filepath.Join(bunDir, "bun.exe"))
	copyOneBotTestRuntime(t, plugin, filepath.Join(bundle, "WinOne.Plugin.exe"))
	for _, name := range []string{"worker.js", "codemode-worker.js", "quickjs.wasm", "photon_rs_bg.wasm", "ghost.js", "ghost-sdk.js", "ghost-mail-sdk.js", "ghost-snapshot.js", "nodeRuntimeWorkerProcess.js", "libraryDbWorker.js", "library-sqlite.js", "worktree-recovery-worker.js"} {
		copyOneBotTestRuntime(t, filepath.Join(filepath.Dir(worker), name), filepath.Join(assets, name))
	}
	var requests atomic.Int32
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Role    string
				Content json.RawMessage
			}
			Stream bool
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		requests.Add(1)
		if !body.Stream {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"verdict\":\"allow\",\"reason\":\"Approved local fixture task\"}"}}]}`)
			return
		}
		last := ""
		if len(body.Messages) > 0 {
			_ = json.Unmarshal(body.Messages[len(body.Messages)-1].Content, &last)
			if last == "" {
				var parts []struct{ Text string }
				_ = json.Unmarshal(body.Messages[len(body.Messages)-1].Content, &parts)
				for _, part := range parts {
					last += part.Text
				}
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if strings.Contains(last, "START_ORCA_WORKER") {
			args := `{"agent":"pi","role":"reviewer","label":"native","model":"model","provider_id":"fixture","initial_task":"ORCA_WORKER_EXECUTE"}`
			value := map[string]any{"id": "orca-fixture", "object": "chat.completion.chunk", "choices": []any{map[string]any{"index": 0, "delta": map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"index": 0, "id": "create-orca-worker", "type": "function", "function": map[string]string{"name": "create_worker", "arguments": args}}}}}}}
			encoded, _ := json.Marshal(value)
			fmt.Fprintf(w, "data: %s\n\n", encoded)
			fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n\n")
			return
		}
		text := "PLUGIN_TASK_DONE"
		if strings.Contains(last, "ORCA_WORKER_EXECUTE") {
			text = "ORCA_WORKER_DONE"
		} else if strings.Contains(last, "Worker native completed") {
			text = "ORCA_REPORT_RECORDED"
		} else if len(body.Messages) > 0 && body.Messages[len(body.Messages)-1].Role == "tool" {
			text = "ORCA_COORDINATOR_DONE"
		}
		fmt.Fprintf(w, "data: {\"id\":\"fixture\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"fixture/model\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":%q},\"finish_reason\":null}]}\n\n", text)
		fmt.Fprint(w, "data: {\"id\":\"fixture\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"fixture/model\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer model.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	directory := filepath.Join(root, "profile")
	t.Cleanup(func() {
		if t.Failed() {
			if data, err := os.ReadFile(filepath.Join(directory, "logs", "host.log")); err == nil {
				t.Logf("isolated Host diagnostic: %s", data)
			}
		}
	})
	store, err := chat.Open(filepath.Join(directory, "chat"))
	if err != nil {
		t.Fatal(err)
	}
	aiService := newAIService(ctx, directory, filepath.Join(bundle, "WinOne.Host.exe"))
	aiStore, err := aiService.magpieStore()
	if err != nil {
		t.Fatal(err)
	}
	configuration := aiStore.Configuration()
	configuration.Providers = []ai.Provider{{ID: "fixture", Name: "Fixture", AuthMode: "api-key", Protocol: "chat", BaseURL: model.URL + "/v1", Models: []string{"model", "another"}}}
	configuration.Tasks = map[string]ai.Selection{"chat": {Provider: "fixture", Model: "model"}}
	key := "plugin-model-fixture-key"
	if _, err = aiStore.SaveConfiguration(configuration, map[string]*string{"fixture": &key}); err != nil {
		t.Fatal(err)
	}
	c := &chatControl{ctx: ctx, store: store, ai: aiService, directory: directory, desktop: filepath.Join(bundle, "WinOne.exe"), language: func() string { return "zh-CN" }}
	g := newChatGhosts(c)
	c.ghosts = g
	c.engine = chat.NewService(ctx, store, chat.ServiceOptions{Bun: filepath.Join(bunDir, "bun.exe"), Worker: filepath.Join(assets, "worker.js"), Configure: func(row chat.Conversation) (map[string]any, func(), error) {
		if err := g.restoreWorkerWorktree(ctx, row); err != nil {
			return nil, nil, err
		}
		bridge, err := chat.NewPrivateBridge(ctx, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !g.toolCall(w, r, row) {
				http.NotFound(w, r)
			}
		}))
		if err != nil {
			return nil, nil, err
		}
		config := map[string]any{"agentDir": filepath.Join(root, "agents", row.ID), "language": "zh-CN", "baseUrl": model.URL + "/v1", "key": key, "model": map[string]any{"id": "fixture/" + row.Model.Model, "name": "Fixture"}, "taskDelivery": true, "ghostBridge": map[string]string{"baseUrl": bridge.BaseURL, "key": bridge.Key}, "runtimeSettings": map[string]any{"toolPreset": "chat-only"}}
		config["orca"] = map[string]any{"agentKind": "pi", "workingDir": row.Directory, "sessionId": row.ID, "vendorOptions": map[string]string{"orcaRole": row.OrcaRole, "orcaWorkflowId": row.OrcaTeamID, "orcaLeadSessionId": row.OrcaLeadID, "orcaWorkerId": row.OrcaWorkerID}}
		config["models"] = []map[string]any{{"id": "fixture/model", "name": "Fixture", "provider": "fixture"}, {"id": "fixture/another", "name": "Another", "provider": "fixture"}}
		if row.OrcaRole == "lead" || row.OrcaRole == "worker" {
			config["runtimeSettings"] = map[string]any{"codeMode": false, "autoRetry": false, "subagents": true}
		}
		release := bridge.Close
		if row.PluginID != "" {
			admission, err := g.taskAdmissionBridge(row)
			if err != nil {
				bridge.Close()
				return nil, nil, err
			}
			config["taskBridge"] = map[string]string{"baseUrl": admission.BaseURL, "key": admission.Key}
			release = func() { admission.Close(); bridge.Close() }
		}
		return config, release, nil
	}})
	defer func() {
		closeCtx, done := context.WithTimeout(context.Background(), 10*time.Second)
		defer done()
		_ = g.close(closeCtx)
		_ = c.engine.Stop(closeCtx)
	}()
	packageFile := filepath.Join(root, "fixture.cindy")
	out, err := os.Create(packageFile)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(out)
	manifest := `{"schemaVersion":3,"minCindyVersion":"0.0.0","id":"fixture","name":"Fixture","version":"1.0.0","description":"Native task fixture","entry":"main.js","kind":"chip","launch":"resident","subscribe":{"topics":["turn","activity"],"hooks":["will-assistant-message"]},"agent":{"tasks":true,"targets":true},"tools":[{"name":"tasks","description":"Exercise task API","parameters":{"type":"object","properties":{"request":{"type":"object"}}}}]}`
	main := `cindy.onHostMessage(async message=>{if(message.type==='event'&&message.name==='will-assistant-message'){await cindy.send({type:'event-verdict',hookId:message.hookId,action:'rewrite',text:'DISPLAY_'+message.data.text});return;}if(message.type!=='tool-call')return;const result=await cindy.send({type:'tasks-request',...message.args.request});await cindy.send({type:'tool-result',callId:message.callId,ok:true,result});});`
	for name, content := range map[string]string{"ghost.json": manifest, "main.js": main} {
		file, e := archive.Create(name)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = file.Write([]byte(content)); e != nil {
			t.Fatal(e)
		}
	}
	if err = archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err = out.Close(); err != nil {
		t.Fatal(err)
	}
	input, _ := json.Marshal(map[string]string{"path": packageFile})
	inspected, err := g.call(ctx, "inspect", input)
	if err != nil {
		t.Fatal(err)
	}
	var inspection struct{ Token string }
	_ = json.Unmarshal(inspected, &inspection)
	input, _ = json.Marshal(map[string]any{"token": inspection.Token, "confirmed": true})
	installedRaw, err := g.call(ctx, "install", input)
	if err != nil {
		t.Fatal(err)
	}
	var installed struct {
		PluginID string `json:"pluginId"`
	}
	if err := json.Unmarshal(installedRaw, &installed); err != nil || installed.PluginID == "" {
		t.Fatalf("installed identity: %s %v", installedRaw, err)
	}
	runner, err := g.ensure(ctx)
	if err != nil {
		t.Fatal(err)
	}
	call := func(request map[string]any) json.RawMessage {
		t.Helper()
		raw, e := runner.Call(ctx, "agent-call", map[string]any{"ghostId": "fixture", "tool": "tasks", "args": map[string]any{"request": request}})
		if e != nil {
			t.Fatal(e)
		}
		var response struct {
			OK     bool
			Result struct {
				OK    bool
				Data  json.RawMessage
				Error any
			}
			Message string
		}
		if e = json.Unmarshal(raw, &response); e != nil || !response.OK || !response.Result.OK {
			t.Fatalf("task request %v: %s (%v)", request, raw, e)
		}
		return response.Result.Data
	}
	created := call(map[string]any{"kind": "create", "requestKey": "first", "title": "Plugin task"})
	var task struct {
		TaskID   string
		Revision uint64
	}
	if err = json.Unmarshal(created, &task); err != nil || task.TaskID == "" {
		t.Fatalf("task view: %s %v", created, err)
	}
	repeated := call(map[string]any{"kind": "create", "requestKey": "first", "title": "Plugin task"})
	if string(created) != string(repeated) {
		t.Fatalf("idempotent create changed: %s -> %s", created, repeated)
	}
	sent := call(map[string]any{"kind": "send", "taskId": task.TaskID, "requestKey": "turn", "expectedRevision": task.Revision, "text": "Return PLUGIN_TASK_DONE."})
	var run struct{ RunID, Status string }
	if err = json.Unmarshal(sent, &run); err != nil || run.RunID == "" {
		t.Fatalf("run: %s %v", sent, err)
	}
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		raw := call(map[string]any{"kind": "getRun", "runId": run.RunID})
		_ = json.Unmarshal(raw, &run)
		if run.Status == "completed" {
			break
		}
		if run.Status == "failed" {
			t.Fatalf("plugin task failed: %s", raw)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if run.Status != "completed" {
		t.Fatalf("run did not reach its actual terminal: %+v", run)
	}
	history := call(map[string]any{"kind": "readMessages", "taskId": task.TaskID})
	if !strings.Contains(string(history), "PLUGIN_TASK_DONE") {
		t.Fatalf("history lost actual reply: %s", history)
	}
	if requests.Load() != 1 {
		t.Fatalf("replayed a model input: %d requests", requests.Load())
	}
	archived := call(map[string]any{"kind": "archive", "taskId": task.TaskID})
	if !strings.Contains(string(archived), `"archived"`) {
		t.Fatalf("archive: %s", archived)
	}
	project, err := store.CreateProject(root, "target-project")
	if err != nil {
		t.Fatal(err)
	}
	ordinary, err := store.NewConversation("", "", "Existing task", ai.Selection{Provider: "fixture", Model: "model"})
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []map[string]string{{"kind": "project", "id": project.Path}, {"kind": "task", "id": ordinary.ID}} {
		key := "target-" + target["kind"]
		request := map[string]any{"type": "targets-request", "kind": "send", "requestKey": key, "target": target, "text": "Return PLUGIN_TASK_DONE.", "title": "Project target"}
		value := call(request)
		var delivery struct {
			TaskID   string
			Accepted bool
			Delivery string
		}
		if err := json.Unmarshal(value, &delivery); err != nil || !delivery.Accepted || delivery.Delivery != "queue" {
			t.Fatalf("target delivery: %s %v", value, err)
		}
		again := call(request)
		if string(again) != string(value) {
			t.Fatalf("target replay changed: %s -> %s", value, again)
		}
		if target["kind"] == "project" {
			row, err := store.Conversation(delivery.TaskID)
			if err != nil || row.ProjectID != project.ID || row.PluginID != installed.PluginID {
				t.Fatalf("target did not create a real project task: %+v %v", row, err)
			}
		}
		deadline := time.Now().Add(20 * time.Second)
		settled := false
		for time.Now().Before(deadline) {
			value := call(map[string]any{"type": "targets-request", "kind": "get", "requestKey": key})
			var status struct {
				Settled bool
				Result  struct{ Outcome string }
			}
			if err := json.Unmarshal(value, &status); err != nil {
				t.Fatal(err)
			}
			if status.Settled {
				if status.Result.Outcome != "completed" {
					t.Fatalf("target terminal: %s", value)
				}
				settled = true
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		if !settled {
			t.Fatalf("target %s did not settle", target["kind"])
		}
	}
	if requests.Load() != 3 {
		t.Fatalf("target replay submitted extra model work: %d", requests.Load())
	}
	deadline = time.Now().Add(10 * time.Second)
	var overrides json.RawMessage
	for time.Now().Before(deadline) {
		overrides, err = runner.Call(ctx, "reply-overrides", map[string]string{"sessionId": task.TaskID})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(overrides), "DISPLAY_PLUGIN_TASK_DONE") {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !strings.Contains(string(overrides), "DISPLAY_PLUGIN_TASK_DONE") || !strings.Contains(string(overrides), "messageId") {
		t.Fatalf("actual Pi terminal did not reach the pinned reply hook: %s", overrides)
	}
	// Plugin -> actual Orca MCP in Pi -> ordinary Worker -> persistent result.
	// Enabling a team must not allocate a Worker or call the model by itself.
	input, _ = json.Marshal(map[string]any{"pluginId": installed.PluginID, "taskPreferences": map[string]string{"permissionMode": "auto"}})
	if _, err = g.call(ctx, "configure", input); err != nil {
		t.Fatal(err)
	}
	created = call(map[string]any{"kind": "create", "requestKey": "orca-coordinator", "title": "Orca coordinator"})
	_ = json.Unmarshal(created, &task)
	before := requests.Load()
	started := call(map[string]any{"kind": "startTeam", "taskId": task.TaskID})
	if !strings.Contains(string(started), `"workerPermissionMode":"auto"`) || requests.Load() != before {
		t.Fatalf("startTeam submitted work: %s calls=%d", started, requests.Load()-before)
	}
	created = call(map[string]any{"kind": "get", "taskId": task.TaskID})
	_ = json.Unmarshal(created, &task)
	sent = call(map[string]any{"kind": "send", "taskId": task.TaskID, "requestKey": "orca-first", "expectedRevision": task.Revision, "text": "START_ORCA_WORKER"})
	var team struct {
		Workers []struct {
			WorkerID    string `json:"worker_id"`
			SessionID   string `json:"session_id"`
			CompletedAt int64  `json:"completedAt"`
			Status      string `json:"status"`
		}
	}
	deadline = time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		value := call(map[string]any{"kind": "getTeam", "taskId": task.TaskID})
		if err = json.Unmarshal(value, &team); err != nil {
			t.Fatal(err)
		}
		if len(team.Workers) == 1 && team.Workers[0].CompletedAt > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if len(team.Workers) != 1 || team.Workers[0].CompletedAt <= 0 {
		t.Fatalf("actual Orca Worker did not complete: %s team=%+v", sent, team)
	}
	member := team.Workers[0]
	row, err := store.Conversation(member.SessionID)
	if err != nil || row.PluginID != "" || row.Permission != "auto" || row.OrcaLeadID != task.TaskID {
		t.Fatalf("ordinary Worker metadata: %+v %v", row, err)
	}
	transcript, err := os.ReadFile(row.SessionFile)
	if err != nil || !strings.Contains(string(transcript), "ORCA_WORKER_DONE") {
		t.Fatalf("Worker never executed real Pi: %v %s", err, transcript)
	}
	bad, err := runner.Call(ctx, "agent-call", map[string]any{"ghostId": "fixture", "tool": "tasks", "args": map[string]any{"request": map[string]any{"kind": "releaseWorker", "taskId": task.TaskID, "workerId": member.WorkerID, "completedAt": member.CompletedAt + 1}}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(bad), "STALE_REVISION") {
		t.Fatalf("forged completion was accepted: %s", bad)
	}
	_ = call(map[string]any{"kind": "releaseWorker", "taskId": task.TaskID, "workerId": member.WorkerID, "completedAt": member.CompletedAt})
	row, err = store.Conversation(member.SessionID)
	if err != nil || !row.Archived {
		t.Fatalf("Worker release did not archive its real task: %+v %v", row, err)
	}
}
