//go:build windows

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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

// Only the remote model is a fixture: Cindy, SQLite, the Host bridge and Pi
// run as their real shipped processes under a repository-local test profile.
func TestAutomationsDispatchPiPersistRunsAndReusePersistentConversation(t *testing.T) {
	bun, worker := os.Getenv("WINONE_CHAT_BUN_EXE"), os.Getenv("WINONE_CHAT_WORKER")
	if bun == "" || worker == "" {
		t.Skip("explicit bundled Chat fixture")
	}
	root := t.TempDir()
	bundle := filepath.Join(root, "bundle")
	schedulerScript := filepath.Join(filepath.Dir(worker), "scheduler.js")
	for source, relative := range map[string]string{bun: "shared/ai/runtime/bun.exe", worker: "shared/chat/worker.js", schedulerScript: "shared/chat/scheduler.js", filepath.Join(filepath.Dir(worker), "script-runner.js"): "shared/chat/script-runner.js"} {
		target := filepath.Join(bundle, relative)
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.Link(source, target); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	profile := filepath.Join(root, "profile")
	store, err := chat.Open(filepath.Join(profile, "chat"))
	if err != nil {
		t.Fatal(err)
	}
	modelService := newAIService(ctx, profile, filepath.Join(bundle, "WinOne.Host.exe"))
	modelService.store, err = ai.Open(modelService.storePath)
	if err != nil {
		t.Fatal(err)
	}
	config := modelService.store.Configuration()
	config.Providers = []ai.Provider{{ID: "test", Name: "Fixture", AuthMode: "api-key", Protocol: "chat", BaseURL: "https://example.com/v1", Models: []string{"model"}}}
	config.Tasks = map[string]ai.Selection{"chat": {Provider: "test", Model: "model"}}
	key := "fixture-key"
	if _, err = modelService.store.SaveConfiguration(config, map[string]*string{"test": &key}); err != nil {
		t.Fatal(err)
	}
	var holdModel atomic.Bool
	schedulerKilled := false
	blockedModel := make(chan struct{}, 1)
	cancelledModel := make(chan struct{}, 1)
	engine := chat.NewService(ctx, store, chat.ServiceOptions{Bun: bun, Worker: worker, Configure: func(c chat.Conversation) (map[string]any, func(), error) {
		bridge, err := chat.NewPrivateBridge(ctx, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			io.Copy(io.Discard, r.Body)
			w.Header().Set("Content-Type", "text/event-stream")
			if holdModel.Load() {
				fmt.Fprint(w, "data: {\"id\":\"blocked\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"},\"finish_reason\":null}]}\n\n")
				w.(http.Flusher).Flush()
				blockedModel <- struct{}{}
				<-r.Context().Done()
				cancelledModel <- struct{}{}
				return
			}
			fmt.Fprint(w, "data: {\"id\":\"test\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Automation result saved\"},\"finish_reason\":null}]}\n\ndata: {\"id\":\"test\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
		}))
		if err != nil {
			return nil, nil, err
		}
		return map[string]any{"baseUrl": bridge.BaseURL, "key": bridge.Key, "agentDir": filepath.Join(profile, "agents", c.ID), "model": map[string]string{"id": "test/model", "name": "Fixture"}}, bridge.Close, nil
	}})
	control := &chatControl{ctx: ctx, directory: profile, desktop: filepath.Join(bundle, "WinOne.exe"), store: store, engine: engine, ai: modelService}
	automations := newChatAutomations(control)
	control.automations = automations
	defer func() {
		closing, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		if err := automations.close(closing); err != nil && !schedulerKilled {
			t.Error(err)
		}
		if err := engine.Stop(closing); err != nil {
			t.Error(err)
		}
	}()
	call := func(method string, params any) json.RawMessage {
		t.Helper()
		raw, _ := json.Marshal(params)
		result, err := automations.call(ctx, method, raw)
		if err != nil {
			t.Fatal(method, err)
		}
		return result
	}
	created := call("create", map[string]any{"input": map[string]any{"name": "Scheduled review", "prompt": "Return the requested result.", "kind": "cron", "cronExpr": "0 9 * * *", "timezone": "Asia/Shanghai", "recurring": true, "manual": true, "persistentSession": true, "agentKind": "pi", "modelAgentKind": "pi", "model": "model", "providerId": "test", "workspaceKind": "dialogue", "useWorktree": false, "permissionMode": "ask", "notify": map[string]bool{"desktop": false, "feishu": false}}})
	var task struct {
		ID string `json:"id"`
	}
	if err = json.Unmarshal(created, &task); err != nil {
		t.Fatal(err)
	}
	var firstSession string
	for n := 0; n < 2; n++ {
		var accepted struct {
			RunID string `json:"runId"`
		}
		if err = json.Unmarshal(call("run", map[string]string{"id": task.ID}), &accepted); err != nil {
			t.Fatal(err)
		}
		if accepted.RunID == "" {
			t.Fatal("no real run id")
		}
		cursor := uint64(0)
		for {
			var runs []struct {
				ID        string `json:"id"`
				Status    string `json:"status"`
				SessionID string `json:"sessionId"`
				Result    string `json:"resultText"`
				Error     string `json:"errorMsg"`
			}
			if err = json.Unmarshal(call("runs", map[string]string{"id": task.ID}), &runs); err != nil {
				t.Fatal(err)
			}
			done := false
			for _, run := range runs {
				if run.ID == accepted.RunID && run.Status != "running" {
					if run.Status != "success" || run.Result != "Automation result saved" {
						t.Fatalf("unexpected execution: %+v", run)
					}
					if n == 0 {
						firstSession = run.SessionID
					} else if run.SessionID != firstSession {
						t.Fatal("persistent schedule created a different conversation")
					}
					done = true
				}
			}
			if done {
				break
			}
			var page automationPage
			if err = json.Unmarshal(call("events", map[string]uint64{"cursor": cursor}), &page); err != nil {
				t.Fatal(err)
			}
			cursor = page.Cursor
		}
	}
	if len(store.Snapshot().Conversations) != 1 {
		t.Fatal("persistent automation duplicated conversation")
	}
	// A definite worker rejection must release running state and retain the
	// pending input so the user can remove an incompatible attachment and retry.
	imagePath := filepath.Join(root, "unsupported.png")
	if err = os.WriteFile(imagePath, []byte{137, 80, 78, 71, 13, 10, 26, 10}, 0600); err != nil {
		t.Fatal(err)
	}
	attached, err := store.AddAttachments(firstSession, []string{imagePath})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = engine.Open(ctx, firstSession, 42); err != nil {
		t.Fatal(err)
	}
	if _, err = engine.Prompt(ctx, firstSession, "unsupported-image", "Read this image", []string{attached[0].Path}); err == nil {
		t.Fatal("text-only model accepted an image")
	}
	state, err := engine.Poll(firstSession, 0)
	if err != nil || state.Running {
		t.Fatal("known input rejection left conversation stuck", state, err)
	}
	draft, err := store.ReadDraft(firstSession)
	if err != nil || draft.Text != "Read this image" || len(draft.Attachments) != 1 {
		t.Fatal("rejected message lost its draft", draft, err)
	}
	engine.Release(firstSession, 42)
	// A persistent task recreates an archived target, while an explicit binding
	// records one failure and pauses. This crosses the real HTTP error contract.
	if _, err = store.UpdateConversation(firstSession, func(value *chat.Conversation) error { value.Archived = true; return nil }); err != nil {
		t.Fatal(err)
	}
	waitRun := func(taskID string) (string, string) {
		t.Helper()
		var accepted struct {
			RunID string `json:"runId"`
		}
		if err := json.Unmarshal(call("run", map[string]string{"id": taskID}), &accepted); err != nil {
			t.Fatal(err)
		}
		cursor := uint64(0)
		for {
			var runs []struct {
				ID        string `json:"id"`
				Status    string `json:"status"`
				SessionID string `json:"sessionId"`
			}
			if err := json.Unmarshal(call("runs", map[string]string{"id": taskID}), &runs); err != nil {
				t.Fatal(err)
			}
			for _, run := range runs {
				if run.ID == accepted.RunID && run.Status != "running" {
					return run.Status, run.SessionID
				}
			}
			var page automationPage
			if err := json.Unmarshal(call("events", map[string]uint64{"cursor": cursor}), &page); err != nil {
				t.Fatal(err)
			}
			cursor = page.Cursor
		}
	}
	status, replacement := waitRun(task.ID)
	if status != "success" || replacement == "" || replacement == firstSession {
		t.Fatal("persistent target was not recreated", status, replacement)
	}
	bound := call("create", map[string]any{"input": map[string]any{"name": "Explicit old binding", "prompt": "Run only in this chat", "kind": "cron", "cronExpr": "0 9 * * *", "timezone": "Asia/Shanghai", "recurring": true, "manual": true, "targetSessionId": firstSession, "agentKind": "pi", "modelAgentKind": "pi", "model": "model", "providerId": "test", "workspaceKind": "dialogue", "useWorktree": false, "permissionMode": "ask", "notify": map[string]bool{"desktop": false, "feishu": false}}})
	var boundTask struct {
		ID string `json:"id"`
	}
	if err = json.Unmarshal(bound, &boundTask); err != nil {
		t.Fatal(err)
	}
	status, _ = waitRun(boundTask.ID)
	if status != "failed" {
		t.Fatal("missing explicit target was not a failed run", status)
	}
	var paused struct {
		Status string `json:"status"`
	}
	if err = json.Unmarshal(call("get", map[string]string{"id": boundTask.ID}), &paused); err != nil || paused.Status != "paused" {
		t.Fatal("missing explicit target did not pause", paused, err)
	}
	// A real script dispatches through the Host, then exposes only the genuine
	// conversation returned by that dispatch as its primary session.
	scriptPath := filepath.Join(root, "dispatch.mjs")
	scriptSource := `import readline from 'node:readline';for await(const line of readline.createInterface({input:process.stdin})){const frame=JSON.parse(line);if(frame.type==='start')console.log(JSON.stringify({protocol:'cindy-script/1',type:'call',id:'dispatch',method:'sessions.dispatch',params:{message:'Return the requested result.',title:'Script child'}}));else if(frame.type==='call_result'){if(!frame.ok)throw Error(JSON.stringify(frame.error));console.log(JSON.stringify({protocol:'cindy-script/1',type:'complete',primarySessionId:frame.result.target_session_id,resultText:'Dispatched'}));}}`
	if err = os.WriteFile(scriptPath, []byte(scriptSource), 0600); err != nil {
		t.Fatal(err)
	}
	var scriptTask struct {
		ID string `json:"id"`
	}
	scriptDefinition := map[string]any{"name": "Script dispatch", "prompt": "", "kind": "cron", "cronExpr": "0 9 * * *", "timezone": "UTC", "recurring": true, "manual": true, "agentKind": "pi", "model": "model", "providerId": "test", "executionMode": "script", "workspaceKind": "project", "workingDir": root, "useWorktree": false, "scriptConfig": map[string]any{"command": fmt.Sprintf("\"%s\" \"%s\"", bun, scriptPath), "capabilities": []string{"sessions.dispatch"}}, "notify": map[string]bool{"desktop": false, "feishu": false}}
	if err = json.Unmarshal(call("create", map[string]any{"input": scriptDefinition}), &scriptTask); err != nil {
		t.Fatal(err)
	}
	status, scriptSession := waitRun(scriptTask.ID)
	if status != "success" || scriptSession == "" {
		t.Fatal("script dispatch did not complete", status, scriptSession)
	}
	if _, err = store.Conversation(scriptSession); err != nil {
		t.Fatal("script returned an unknown conversation", err)
	}
	holdModel.Store(true)
	call("run", map[string]string{"id": task.ID})
	select {
	case <-blockedModel:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	process, err := os.FindProcess(automations.runner.PID())
	if err != nil {
		t.Fatal(err)
	}
	if err = process.Kill(); err != nil {
		t.Fatal(err)
	}
	schedulerKilled = true
	select {
	case <-cancelledModel:
	case <-time.After(5 * time.Second):
		t.Fatal("scheduler worker death left its Pi model request running")
	}
}

func TestAutomationHTTPDisconnectStopsOnlyItsOwnPiTurn(t *testing.T) {
	bun, worker := os.Getenv("WINONE_CHAT_BUN_EXE"), os.Getenv("WINONE_CHAT_WORKER")
	if bun == "" || worker == "" {
		t.Skip("explicit bundled Chat fixture")
	}
	root := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store, err := chat.Open(filepath.Join(root, "profile"))
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := store.NewConversation("", "", "Disconnect", ai.Selection{})
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{}, 4)
	cancelled := make(chan struct{}, 4)
	engine := chat.NewService(ctx, store, chat.ServiceOptions{Bun: bun, Worker: worker, Configure: func(c chat.Conversation) (map[string]any, func(), error) {
		bridge, err := chat.NewPrivateBridge(ctx, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			io.Copy(io.Discard, r.Body)
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"id\":\"blocked\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"},\"finish_reason\":null}]}\n\n")
			w.(http.Flusher).Flush()
			started <- struct{}{}
			<-r.Context().Done()
			cancelled <- struct{}{}
		}))
		if err != nil {
			return nil, nil, err
		}
		return map[string]any{"baseUrl": bridge.BaseURL, "key": bridge.Key, "agentDir": filepath.Join(root, "agents", c.ID), "model": map[string]string{"id": "test/model", "name": "Fixture"}}, bridge.Close, nil
	}})
	defer func() {
		ending, stop := context.WithTimeout(context.Background(), 8*time.Second)
		defer stop()
		_ = engine.Stop(ending)
	}()
	control := &chatControl{ctx: ctx, directory: root, store: store, engine: engine}
	automations := newChatAutomations(control)
	server := httptest.NewServer(http.HandlerFunc(automations.serve))
	defer server.Close()
	if _, err := engine.Open(ctx, conversation.ID, 42); err != nil {
		t.Fatal(err)
	}
	runCtx, stopRun := context.WithCancel(ctx)
	body := fmt.Sprintf(`{"id":%q,"runId":"cancelled-run","prompt":"Wait for cancellation"}`, conversation.ID)
	req, _ := http.NewRequestWithContext(runCtx, "POST", server.URL+"/v1/execute", strings.NewReader(body))
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatal("execute endpoint rejected fixture", response.StatusCode)
	}
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	stopRun()
	response.Body.Close()
	select {
	case <-cancelled:
	case <-time.After(5 * time.Second):
		t.Fatal("disconnect left the real model request running")
	}
	until := time.Now().Add(3 * time.Second)
	for {
		page, err := engine.Poll(conversation.ID, 0)
		if err != nil {
			t.Fatal(err)
		}
		if !page.Running {
			break
		}
		if time.Now().After(until) {
			t.Fatal("cancelled request remained running")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := engine.Prompt(ctx, conversation.ID, "new-user-turn", "Continue later"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if err := engine.StopRequest(ctx, conversation.ID, "cancelled-run"); err != nil {
		t.Fatal(err)
	}
	page, err := engine.Poll(conversation.ID, 0)
	if err != nil || !page.Running {
		t.Fatal("late cancellation stopped the newer turn", page, err)
	}
	if err := engine.StopRequest(ctx, conversation.ID, "new-user-turn"); err != nil {
		t.Fatal(err)
	}
	engine.Release(conversation.ID, 42)
}
