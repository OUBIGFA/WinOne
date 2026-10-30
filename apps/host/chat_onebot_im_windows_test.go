//go:build windows

package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"golang.org/x/sys/windows"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
	"winone/internal/ai"
	"winone/internal/chat"
)

// This is a real Host -> OneBot worker -> Pi worker -> Magpie -> HTTP fixture
// round trip. It deliberately has no IM account or public network dependency;
// transport verification and text/card rendering have their own Bun tests.
func TestOneBotIMHostRoundTripWithRealPi(t *testing.T) {
	bun, onebot, pi, aiExe := os.Getenv("WINONE_CHAT_BUN_EXE"), os.Getenv("WINONE_ONEBOT_WORKER"), os.Getenv("WINONE_CHAT_WORKER"), os.Getenv("WINONE_AI_TEST_EXE")
	if bun == "" || onebot == "" || pi == "" || aiExe == "" {
		t.Skip("explicit isolated OneBot/Pi/AI runtime paths are required")
	}
	root := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	bundle := filepath.Join(root, "bundle")
	chatDir := filepath.Join(bundle, "shared", "chat")
	if err := os.MkdirAll(chatDir, 0700); err != nil {
		t.Fatal(err)
	}
	bunDir := filepath.Join(bundle, "shared", "ai", "runtime")
	if err := os.MkdirAll(bunDir, 0700); err != nil {
		t.Fatal(err)
	}
	copyOneBotTestRuntime(t, bun, filepath.Join(bunDir, "bun.exe"))
	// Link only known build outputs; preserve the immutable source bundle.
	for _, file := range []string{"worker.js", "codemode-worker.js", "script-runner.js", "quickjs.wasm", "photon_rs_bg.wasm"} {
		if err := os.Link(filepath.Join(filepath.Dir(pi), file), filepath.Join(chatDir, file)); err != nil {
			t.Fatal(err)
		}
	}
	copyOneBotTestWorker(t, onebot, filepath.Join(chatDir, "onebot.js"))
	type modelReply struct {
		tool string
		args any
		text string
	}
	replies := []modelReply{
		{tool: "write", args: map[string]string{"path": "owner-proof.txt", "content": "OWNER_APPROVED"}},
		{tool: "ask_user_question", args: map[string]any{"questions": []any{map[string]any{"question": "输出格式", "options": []string{"文本", "表格"}}}}},
		{text: "OWNER_DONE"},
		{tool: "write", args: map[string]string{"path": "guest-proof.txt", "content": "MUST_NOT_EXIST"}},
		{text: "GUEST_DENIED"},
	}
	var mu sync.Mutex
	var requests, learningRequests, reviewRequests []json.RawMessage
	var failingModel string
	var taskRequests, taskResponses int
	taskContinue := make(chan struct{})
	var taskRelease sync.Once
	releaseTask := func() { taskRelease.Do(func() { close(taskContinue) }) }
	defer releaseTask()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			http.Error(w, "read", 500)
			return
		}
		if r.Header.Get("Authorization") != "Bearer onebot-fixture-key" {
			t.Error("synthetic provider credential missing")
		}
		mu.Lock()
		var requested struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(raw, &requested); err != nil {
			mu.Unlock()
			t.Error(err)
			http.Error(w, "invalid model request", 400)
			return
		}
		if requested.Model == "helper-model" {
			// Per-reply learning is now live. It must not consume this fixture's
			// scripted conversational tool/answer FIFO or corrupt request counts.
			// Keep real auxiliary HTTP calls and record their separate contracts.
			if bytes.Contains(raw, []byte("Review a completed teammate exchange")) {
				reviewRequests = append(reviewRequests, raw)
			} else {
				learningRequests = append(learningRequests, raw)
			}
		} else {
			requests = append(requests, raw)
		}
		if failingModel != "" && requested.Model == failingModel {
			mu.Unlock()
			http.Error(w, "Service temporarily unavailable", http.StatusServiceUnavailable)
			return
		}
		interactionID := ""
		for i := len(requested.Messages) - 1; i >= 0; i-- {
			if requested.Messages[i].Role != "user" {
				continue
			}
			var text string
			_ = json.Unmarshal(requested.Messages[i].Content, &text)
			if strings.HasPrefix(text, "[任务需要你处理] task_id: ") {
				interactionID = strings.Fields(strings.TrimPrefix(text, "[任务需要你处理] task_id: "))[0]
			}
			break
		}
		var next modelReply
		if requested.Model == "helper-model" {
			next = modelReply{text: "{}"}
		} else if interactionID != "" {
			if requested.Messages[len(requested.Messages)-1].Role == "tool" {
				next = modelReply{text: "QUESTION_ANSWERED"}
			} else {
				next = modelReply{tool: "message_session_task", args: map[string]any{"task_id": interactionID, "answers": map[string]string{"验证确认": "继续"}}}
			}
		} else if requested.Model == "task-model" {
			taskRequests++
			mu.Unlock()
			select {
			case <-taskContinue:
			case <-r.Context().Done():
				return
			case <-ctx.Done():
				return
			}
			mu.Lock()
			taskResponses++
			if taskResponses == 1 {
				next = modelReply{tool: "write", args: map[string]string{"path": "task-proof.txt", "content": "TASK_EXECUTED"}}
			} else if taskResponses == 2 {
				next = modelReply{tool: "ask_user_question", args: map[string]any{"questions": []any{map[string]any{"question": "验证确认", "options": []string{"继续", "取消"}}}}}
			} else if taskResponses == 3 {
				next = modelReply{text: "TASK_COMPLETED"}
			} else {
				mu.Unlock()
				http.Error(w, "duplicate task execution", 500)
				return
			}
		} else {
			if len(replies) == 0 {
				mu.Unlock()
				http.Error(w, "unexpected model request", 500)
				return
			}
			next = replies[0]
			replies = replies[1:]
		}
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		emit := func(delta any, finish any) {
			b, _ := json.Marshal(map[string]any{"id": "fixture", "object": "chat.completion.chunk", "model": "model", "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}}})
			fmt.Fprintf(w, "data: %s\n\n", b)
		}
		emit(map[string]string{"role": "assistant"}, nil)
		if next.tool != "" {
			args, _ := json.Marshal(next.args)
			emit(map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "reused-call-id", "type": "function", "function": map[string]string{"name": next.tool, "arguments": string(args)}}}}, nil)
			emit(map[string]any{}, "tool_calls")
		} else {
			emit(map[string]string{"content": next.text}, nil)
			emit(map[string]any{}, "stop")
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer upstream.Close()
	data := filepath.Join(root, "data")
	service := newAIService(ctx, data, filepath.Join(bundle, "WinOne.Host.exe"))
	var err error
	service.store, err = ai.Open(service.storePath)
	if err != nil {
		t.Fatal(err)
	}
	service.gatewaySlots = make(chan struct{}, 4)
	service.translator = ai.Translator{Executable: aiExe, DataDirectory: filepath.Join(data, "ai-runtime")}
	cfg := service.store.Configuration()
	// A realistic context window: the scripted replies must not be spent on an
	// automatic compaction caused only by the teammate tool surface.
	window := ai.ModelMetadata{ContextWindow: 200000}
	cfg.Providers = []ai.Provider{{ID: "fixture", Name: "Fixture", AuthMode: "api-key", Protocol: "chat", BaseURL: upstream.URL, Models: []string{"model", "model-next", "task-model", "helper-model"},
		ModelMetadata: map[string]ai.ModelMetadata{"model": window, "model-next": window, "task-model": window, "helper-model": window}}}
	cfg.Tasks["chat"] = ai.Selection{Provider: "fixture", Model: "model"}
	key := "onebot-fixture-key"
	if _, err = service.store.SaveConfiguration(cfg, map[string]*string{"fixture": &key}); err != nil {
		t.Fatal(err)
	}
	store, err := chat.Open(filepath.Join(data, "chat"))
	if err != nil {
		t.Fatal(err)
	}
	skillDirectory := filepath.Join(root, "skills")
	if err = os.MkdirAll(filepath.Join(skillDirectory, "next-skill"), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(skillDirectory, "next-skill", "SKILL.md"), []byte("---\nname: next-skill\ndescription: Skill selected for the next runtime\n---\nUse on demand."), 0600); err != nil {
		t.Fatal(err)
	}
	prefs := store.Preferences()
	prefs.TitleModel = ai.Selection{Provider: "fixture", Model: "helper-model"}
	prefs.SkillDirectories = []string{skillDirectory}
	if _, err = store.SavePreferences(prefs); err != nil {
		t.Fatal(err)
	}
	bot := newChatOneBot(&chatControl{ctx: ctx, directory: data, desktop: filepath.Join(bundle, "WinOne.exe"), language: func() string { return "zh-CN" }, store: store, ai: service})
	defer func() {
		end, stop := context.WithTimeout(context.Background(), 8*time.Second)
		defer stop()
		if err := bot.close(end); err != nil {
			t.Error(err)
		}
	}()
	if _, err = bot.call(ctx, "profiles.create", json.RawMessage(`{"input":{"id":"im-fixture","name":"IM Fixture","identitySource":"IM_IDENTITY_PROOF","capabilities":{"permissions":"bypassPermissions"}}}`)); err != nil {
		t.Fatal(err)
	}
	post := func(route string, input any, status int) []byte {
		t.Helper()
		body, _ := json.Marshal(input)
		req, _ := http.NewRequestWithContext(ctx, "POST", bot.imBridge.BaseURL+"/"+route, bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+bot.imBridge.Key)
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		raw, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != status {
			t.Fatalf("%s: HTTP %d: %s", route, response.StatusCode, raw)
		}
		return raw
	}
	route := func(receipt string, owner bool, routeKey, prompt string, attachments []string) string {
		t.Helper()
		raw := post("im/route", map[string]any{"botId": "im-fixture", "channel": "telegram", "connectionId": "fixture12345", "requestId": receipt, "routeKey": routeKey, "group": true, "ownerAuthorized": owner, "prompt": prompt, "attachments": attachments}, 200)
		var result struct {
			SessionID string `json:"sessionId"`
		}
		if err := json.Unmarshal(raw, &result); err != nil {
			t.Fatal(err)
		}
		return result.SessionID
	}
	const oldReceipt = "im-stale-owner-receipt"
	const ownerReceipt = "im-current-owner-receipt"
	const guestReceipt = "im-current-guest-receipt"
	lane := "im:fixture-group"
	sessionID := route(oldReceipt, true, lane, "Unused old message", nil)
	if next := route(ownerReceipt, true, lane, "OWNER_WRITE", nil); next != sessionID {
		t.Fatal("owner group lane duplicated")
	}
	invalidReceipt := oldReceipt
	var beforeApproval func()
	execute := func(receipt, prompt string, wantApprovals int) {
		t.Helper()
		post("prepare", map[string]any{"schedule": map[string]string{"targetSessionId": sessionID}, "runId": receipt}, 200)
		body, _ := json.Marshal(map[string]string{"id": sessionID, "runId": receipt, "prompt": prompt})
		req, _ := http.NewRequestWithContext(ctx, "POST", bot.imBridge.BaseURL+"/execute", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+bot.imBridge.Key)
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != 200 {
			raw, _ := io.ReadAll(response.Body)
			t.Fatalf("execute HTTP %d: %s", response.StatusCode, raw)
		}
		scanner := bufio.NewScanner(response.Body)
		scanner.Buffer(make([]byte, 4096), 4<<20)
		handled := map[string]bool{}
		settled := false
		approvalCount := 0
		var trace []json.RawMessage
		for scanner.Scan() {
			trace = append(trace, append(json.RawMessage(nil), scanner.Bytes()...))
			var page struct {
				Approvals []struct {
					ID        string `json:"id"`
					RequestID string `json:"requestId"`
					Kind      string `json:"kind"`
				} `json:"approvals"`
				Events []struct {
					Data struct {
						Type      string `json:"type"`
						RequestID string `json:"requestId"`
						Status    string `json:"status"`
						Error     string `json:"error"`
					} `json:"data"`
				} `json:"events"`
				Terminal json.RawMessage `json:"terminal"`
			}
			if err := json.Unmarshal(scanner.Bytes(), &page); err != nil {
				t.Fatal(err)
			}
			if len(page.Terminal) > 0 {
				t.Fatalf("unconfirmed terminal: %s", page.Terminal)
			}
			for _, approval := range page.Approvals {
				if handled[approval.ID+approval.Kind] {
					continue
				}
				handled[approval.ID+approval.Kind] = true
				approvalCount++
				if approval.RequestID != receipt {
					t.Fatalf("approval borrowed another receipt: %+v", approval)
				}
				input := map[string]any{"sessionId": sessionID, "requestId": invalidReceipt, "id": approval.ID, "kind": approval.Kind, "allow": true, "answers": []string{"文本"}}
				rejected := post("im/interaction", input, 400)
				if !strings.Contains(string(rejected), "another input") {
					t.Fatalf("wrong receipt error: %s", rejected)
				}
				input["requestId"] = receipt
				if beforeApproval != nil {
					run := beforeApproval
					beforeApproval = nil
					run()
				}
				post("im/interaction", input, 200)
			}
			for _, event := range page.Events {
				if (event.Data.Type == "settled" || event.Data.Type == "input_settled") && event.Data.RequestID == receipt {
					if event.Data.Status != "completed" || event.Data.Error != "" {
						t.Fatalf("turn failed: %+v", event.Data)
					}
					settled = true
				}
			}
		}
		if err := scanner.Err(); err != nil {
			t.Fatal(err)
		}
		if !settled || approvalCount != wantApprovals {
			t.Fatalf("settled=%v approvals=%d want=%d trace=%s", settled, approvalCount, wantApprovals, trace)
		}
	}
	execute(ownerReceipt, "OWNER_WRITE", 2)
	conversation, err := bot.sessions.store.Conversation(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if raw, err := os.ReadFile(filepath.Join(conversation.Directory, "owner-proof.txt")); err != nil || string(raw) != "OWNER_APPROVED" {
		t.Fatalf("owner write: %s %v", raw, err)
	}
	// Recreate the Host control and both actual worker processes. The old owner
	// receipt remains verifiable but cannot authorize the next guest's tools.
	oldRunner := bot.runner
	if err := bot.close(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-oldRunner.Done():
	default:
		t.Fatal("OneBot worker survived Host close")
	}
	bot = newChatOneBot(bot.control)
	if _, err := bot.ensure(ctx); err != nil {
		t.Fatal(err)
	}
	if restored, err := bot.inputGrant(sessionID, ownerReceipt); err != nil || !restored.Owner {
		t.Fatalf("owner receipt did not survive Host restart: %+v %v", restored, err)
	}
	if next := route(guestReceipt, false, lane, "GUEST_WRITE", nil); next != sessionID {
		t.Fatal("guest did not reuse isolated group lane")
	}
	execute(guestReceipt, "GUEST_WRITE", 0)
	if _, err := os.Stat(filepath.Join(conversation.Directory, "guest-proof.txt")); !os.IsNotExist(err) {
		t.Fatalf("guest write escaped: %v", err)
	}
	post("im/route", map[string]any{"botId": "im-fixture", "channel": "telegram", "connectionId": "fixture12345", "requestId": "im-guest-canonical-denied", "routeKey": "canonical", "ownerAuthorized": false}, 400)
	canonical, err := bot.openSession(ctx, "im-fixture")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bot.sessions.engine.Open(ctx, canonical.ID, 0); err != nil {
		t.Fatal(err)
	}
	// A desktop owner turn on the canonical session receives its project facts
	// from the same Host catalog as the settings page, through the real Pi bridge.
	projectDirectory := filepath.Join(root, "registered-project")
	if err := os.Mkdir(projectDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddProject(projectDirectory, "Registered project"); err != nil {
		t.Fatal(err)
	}
	projectRaw, err := bot.call(ctx, "projects.list", json.RawMessage(`{"botId":"im-fixture"}`))
	if err != nil {
		t.Fatal(err)
	}
	var projectOptions []struct {
		ID     string `json:"id"`
		Linked bool   `json:"linked"`
	}
	if err = json.Unmarshal(projectRaw, &projectOptions); err != nil || len(projectOptions) != 1 {
		t.Fatalf("project catalog: %s %v", projectRaw, err)
	}
	mu.Lock()
	replies = append(replies, modelReply{tool: "list_teammate_projects", args: map[string]any{"include_available": true}}, modelReply{tool: "set_teammate_project", args: map[string]any{"project_id": projectOptions[0].ID, "linked": true}}, modelReply{text: "Project linked"})
	mu.Unlock()
	channelSessionID := sessionID
	sessionID = canonical.ID
	if err = bot.grantInput("desktop-project-turn", oneBotInputGrant{SessionID: canonical.ID, Owner: true, Digest: fmt.Sprintf("%x", sha256.Sum256([]byte("Link the registered project")))}); err != nil {
		t.Fatal(err)
	}
	execute("desktop-project-turn", "Link the registered project", 0)
	sessionID = channelSessionID
	projectRaw, err = bot.call(ctx, "projects.list", json.RawMessage(`{"botId":"im-fixture"}`))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(projectRaw, &projectOptions); err != nil || len(projectOptions) != 1 || !projectOptions[0].Linked {
		t.Fatalf("Pi project link did not persist: %s %v", projectRaw, err)
	}
	// A real pending question holds the old run while a profile save notifies
	// this already-open Worker. Its queued input must adopt the new runtime.
	mu.Lock()
	replies = append(replies, modelReply{tool: "ask_user_question", args: map[string]any{"questions": []any{map[string]any{"question": "继续执行", "options": []string{"文本", "表格"}}}}}, modelReply{text: "Old runtime completed"}, modelReply{text: "New runtime completed"})
	mu.Unlock()
	sessionID = canonical.ID
	invalidReceipt = "stale-desktop-input"
	if err = bot.grantInput(invalidReceipt, oneBotInputGrant{SessionID: canonical.ID, Owner: true, Digest: fmt.Sprintf("%x", sha256.Sum256([]byte("Unused desktop input")))}); err != nil {
		t.Fatal(err)
	}
	if err = bot.grantInput("runtime-held", oneBotInputGrant{SessionID: canonical.ID, Owner: true, Digest: fmt.Sprintf("%x", sha256.Sum256([]byte("Wait for my answer")))}); err != nil {
		t.Fatal(err)
	}
	beforeApproval = func() {
		profileRaw, err := bot.call(ctx, "profiles.get", json.RawMessage(`{"botId":"im-fixture"}`))
		if err != nil {
			t.Fatal(err)
		}
		var profile struct {
			CurrentVersion int `json:"currentVersion"`
		}
		if err = json.Unmarshal(profileRaw, &profile); err != nil {
			t.Fatal(err)
		}
		patch, _ := json.Marshal(map[string]any{"input": map[string]any{"id": "im-fixture", "expectedVersion": profile.CurrentVersion, "identitySource": "UPDATED_IDENTITY_PROOF", "capabilities": map[string]any{"modelChainOverride": []any{map[string]any{"harness": "pi", "providerId": "fixture", "model": "model-next", "fastMode": true}}, "skills": []string{"next-skill"}}}})
		if _, err = bot.call(ctx, "profiles.update", patch); err != nil {
			t.Fatal(err)
		}
		current, err := bot.sessions.store.Conversation(canonical.ID)
		if err != nil || current.Model.Model != "model" {
			t.Fatalf("active tool's model changed: %+v %v", current, err)
		}
		if err = bot.grantInput("runtime-queued", oneBotInputGrant{SessionID: canonical.ID, Owner: true, Digest: fmt.Sprintf("%x", sha256.Sum256([]byte("Use the updated settings")))}); err != nil {
			t.Fatal(err)
		}
		if _, err = bot.sessions.engine.Prompt(ctx, canonical.ID, "runtime-queued", "Use the updated settings"); err != nil {
			t.Fatal(err)
		}
	}
	execute("runtime-held", "Wait for my answer", 1)
	var cursor uint64
	for {
		page, err := bot.sessions.engine.WaitEvents(ctx, canonical.ID, cursor)
		if err != nil {
			t.Fatal(err)
		}
		cursor = page.Cursor
		if !page.Running {
			break
		}
	}
	current, err := bot.sessions.store.Conversation(canonical.ID)
	if err != nil || current.Model.Model != "model-next" || !current.FastMode {
		t.Fatalf("queued runtime not committed: %+v %v", current, err)
	}
	state, err := bot.sessions.engine.Command(ctx, canonical.ID, "state", nil)
	var runtimeState struct {
		Resources struct {
			Skills []struct {
				Name string `json:"name"`
			} `json:"skills"`
		} `json:"resources"`
	}
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(state, &runtimeState); err != nil || len(runtimeState.Resources.Skills) != 1 || runtimeState.Resources.Skills[0].Name != "next-skill" {
		t.Fatalf("selected skill not loaded: %s %v", state, err)
	}
	mu.Lock()
	if len(requests) != 11 || !bytes.Contains(requests[9], []byte(`"model":"model"`)) || bytes.Contains(requests[9], []byte("UPDATED_IDENTITY_PROOF")) || !bytes.Contains(requests[10], []byte(`"model":"model-next"`)) || !bytes.Contains(requests[10], []byte("UPDATED_IDENTITY_PROOF")) {
		t.Errorf("runtime boundary trace: %q", requests)
	}
	mu.Unlock()
	sessionID = channelSessionID
	// Persist the effective fallback, so reopening the same configured chain
	// cannot silently send the next input to its broken primary again.
	profileRaw, err := bot.call(ctx, "profiles.get", json.RawMessage(`{"botId":"im-fixture"}`))
	if err != nil {
		t.Fatal(err)
	}
	var modelProfile struct {
		CurrentVersion int `json:"currentVersion"`
	}
	if err = json.Unmarshal(profileRaw, &modelProfile); err != nil {
		t.Fatal(err)
	}
	modelPatch, _ := json.Marshal(map[string]any{"input": map[string]any{"id": "im-fixture", "expectedVersion": modelProfile.CurrentVersion, "capabilities": map[string]any{"modelChainOverride": []any{
		map[string]any{"harness": "pi", "providerId": "fixture", "model": "model-next", "effort": "low", "fastMode": false},
		map[string]any{"harness": "pi", "providerId": "fixture", "model": "model", "effort": "high", "fastMode": true},
	}}}})
	if _, err = bot.call(ctx, "profiles.update", modelPatch); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	// Magpie owns its transport attempts before Pi sees a terminal failure.
	// Keep the primary unavailable across every such attempt.
	fallbackStart := len(requests)
	failingModel = "model-next"
	replies = append(replies, modelReply{text: "Fallback completed"})
	mu.Unlock()
	sessionID = canonical.ID
	if err = bot.grantInput("model-fallback", oneBotInputGrant{SessionID: canonical.ID, Owner: true, Digest: fmt.Sprintf("%x", sha256.Sum256([]byte("Continue through the declared chain")))}); err != nil {
		t.Fatal(err)
	}
	execute("model-fallback", "Continue through the declared chain", 0)
	current, err = bot.openSession(ctx, "im-fixture")
	if err != nil || current.Model.Model != "model" || current.ThinkingLevel != "high" || !current.FastMode {
		t.Fatalf("fallback selection not preserved: %+v %v", current, err)
	}
	mu.Lock()
	fallbackRequests := requests[fallbackStart:]
	if len(fallbackRequests) < 2 || !bytes.Contains(fallbackRequests[len(fallbackRequests)-1], []byte(`"model":"model"`)) {
		t.Errorf("fallback route trace: %q", requests)
	}
	for _, failed := range fallbackRequests[:len(fallbackRequests)-1] {
		if !bytes.Contains(failed, []byte(`"model":"model-next"`)) {
			t.Errorf("unexpected fallback route: %s", failed)
		}
	}
	failingModel = ""
	mu.Unlock()
	sessionID = channelSessionID
	// Independent tasks use the real ordinary Chat service, not a test native
	// dispatch callback. The companion model invokes the published Pi tool.
	bot.control.engine = chat.NewService(ctx, store, chat.ServiceOptions{Bun: filepath.Join(bunDir, "bun.exe"), Worker: filepath.Join(chatDir, "worker.js"), Configure: bot.control.configure})
	defer func() {
		end, done := context.WithTimeout(context.Background(), 8*time.Second)
		defer done()
		if err := bot.control.engine.Stop(end); err != nil {
			t.Error(err)
		}
	}()
	mu.Lock()
	replies = append(replies,
		modelReply{tool: "start_session_task", args: map[string]any{"instruction": "Write task-proof.txt and return TASK_COMPLETED", "model_selection": map[string]any{"id": "fixture/task-model", "effort": "low"}}},
		modelReply{text: "TASK_STARTED"}, modelReply{text: "TASK_REPORTED"})
	mu.Unlock()
	if err := bot.grantInput("native-task-start", oneBotInputGrant{SessionID: canonical.ID, Owner: true, Digest: fmt.Sprintf("%x", sha256.Sum256([]byte("Start independent work")))}); err != nil {
		t.Fatal(err)
	}
	sessionID = canonical.ID
	execute("native-task-start", "Start independent work", 0)
	// This draft is authored after the initiating owner input. Only the later
	// host completion reaches it; starting a new user turn may clear its own draft.
	if err := bot.sessions.store.SaveDraft(canonical.ID, chat.Draft{Text: "task draft"}); err != nil {
		t.Fatal(err)
	}
	// Pause the actual task and resume behind its durable input barrier. It
	// keeps the same task/session and refuses an ordinary supplement while paused.
	waitTaskRequest := func(count int) {
		t.Helper()
		until := time.Now().Add(5 * time.Second)
		for {
			mu.Lock()
			ready := taskRequests >= count
			mu.Unlock()
			if ready {
				return
			}
			if time.Now().After(until) {
				child := store.Snapshot().Conversations[0]
				native, _ := bot.control.engine.Command(ctx, child.ID, "execution.inspect", nil)
				raw, _ := bot.control.engine.Command(ctx, child.ID, "state", nil)
				var state struct {
					Notice   string `json:"notice"`
					Messages []struct {
						Role  string `json:"role"`
						Error string `json:"errorMessage"`
					} `json:"messages"`
				}
				_ = json.Unmarshal(raw, &state)
				tasks, _ := bot.runner.Call(ctx, "tasks.list", map[string]string{"callerSessionId": canonical.ID})
				stack := make([]byte, 1<<20)
				size := runtime.Stack(stack, true)
				_ = os.WriteFile(filepath.Join(os.TempDir(), "blocked-host-stack.txt"), stack[:size], 0600)
				_ = os.WriteFile(filepath.Join(os.TempDir(), "blocked-child-state.json"), raw, 0600)
				t.Fatalf("task request not admitted: native=%s state=%+v tasks=%s", native, state, tasks)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	waitTaskRequest(1)
	liveTask := store.Snapshot().Conversations[0]
	controlTask := func(method string, fields map[string]any) json.RawMessage {
		t.Helper()
		fields["callerSessionId"] = canonical.ID
		fields["delegationId"] = liveTask.OneBotTaskID
		fields["requestId"] = "native-task-start"
		fields["ownerAuthorized"] = true
		raw, err := bot.runner.Call(ctx, method, fields)
		if err != nil {
			t.Fatal(method, err)
		}
		return raw
	}
	if raw := controlTask("tasks.stop", map[string]any{"mode": "pause"}); !bytes.Contains(raw, []byte(`"ok":true`)) {
		t.Fatalf("task pause failed: %s", raw)
	}
	if raw := controlTask("tasks.get", map[string]any{}); !bytes.Contains(raw, []byte(`"state":"paused"`)) {
		t.Fatalf("pause did not reach native idle: %s", raw)
	}
	if raw := controlTask("tasks.message", map[string]any{"input": map[string]any{"kind": "message", "text": "must stay blocked"}}); !bytes.Contains(raw, []byte("TASK_PAUSED")) {
		t.Fatalf("paused task accepted input: %s", raw)
	}
	if raw := controlTask("tasks.message", map[string]any{"input": map[string]any{"kind": "resume"}}); !bytes.Contains(raw, []byte(`"ok":true`)) {
		t.Fatalf("task resume failed: %s", raw)
	}
	waitTaskRequest(2)
	// Restart only the OneBot coordinator while ordinary Pi remains in flight.
	// The new worker must attach to the accepted receipt, not launch another task.
	if err := bot.runner.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := bot.ensure(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := bot.runner.Call(ctx, "tasks.recover", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	releaseTask()
	var task struct {
		Delegations []struct {
			ID             string `json:"id"`
			ChildSessionID string `json:"childSessionId"`
			Status         string `json:"status"`
			ResultSummary  string `json:"resultSummary"`
		} `json:"delegations"`
	}
	deadlineTask := time.Now().Add(12 * time.Second)
	for {
		raw, err := bot.runner.Call(ctx, "tasks.list", map[string]string{"callerSessionId": canonical.ID})
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &task); err != nil {
			t.Fatal(err)
		}
		state, err := bot.sessions.engine.Command(ctx, canonical.ID, "state", nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(task.Delegations) == 1 && task.Delegations[0].Status == "completed" && bytes.Contains(state, []byte("TASK_REPORTED")) {
			break
		}
		if time.Now().After(deadlineTask) {
			t.Fatalf("task not settled: %s\nparent: %s", raw, state)
		}
		time.Sleep(20 * time.Millisecond)
	}
	child, err := store.Conversation(task.Delegations[0].ChildSessionID)
	if err != nil || child.OneBotParentID != canonical.ID || child.Model.Model != "task-model" || child.ThinkingLevel != "low" {
		t.Fatalf("task route/ownership: %+v %v", child, err)
	}
	if proof, err := os.ReadFile(filepath.Join(child.Directory, "task-proof.txt")); err != nil || string(proof) != "TASK_EXECUTED" {
		t.Fatalf("task proof: %s %v", proof, err)
	}
	if draft, err := bot.sessions.store.ReadDraft(canonical.ID); err != nil || draft.Text != "task draft" {
		t.Fatalf("task overwrote unsent draft: %+v %v", draft, err)
	}
	mu.Lock()
	if taskRequests != 4 || taskResponses != 3 {
		t.Errorf("task ran requests=%d responses=%d, want one cancelled request plus write/question/answer", taskRequests, taskResponses)
	}
	requestsBeforeReplay := len(requests)
	mu.Unlock()
	if _, err := bot.control.engine.Open(ctx, child.ID, 0); err != nil {
		t.Fatal(err)
	}
	executionRaw, err := bot.control.engine.Command(ctx, child.ID, "execution.inspect", nil)
	if err != nil {
		t.Fatal(err)
	}
	var currentExecution struct {
		RequestID string `json:"requestId"`
	}
	if err := json.Unmarshal(executionRaw, &currentExecution); err != nil {
		t.Fatal(err)
	}
	receipt, err := bot.control.engine.Command(ctx, child.ID, "input.result", map[string]string{"id": currentExecution.RequestID})
	childState, childStateErr := bot.control.engine.Command(ctx, child.ID, "state", nil)
	bot.control.engine.Release(child.ID, 0)
	if childStateErr != nil || !bytes.Contains(childState, []byte("Start independent work")) || bytes.Contains(childState, []byte("Task ID:")) {
		t.Fatalf("task user presentation lost source or exposed machine instructions: %s %v", childState, childStateErr)
	}
	if err != nil {
		t.Fatal(err)
	}
	var nativeReceipt struct {
		Execution json.RawMessage `json:"execution"`
	}
	if err := json.Unmarshal(receipt, &nativeReceipt); err != nil {
		t.Fatal(err)
	}
	if _, err := bot.runner.Call(ctx, "tasks.settle", map[string]any{"childSessionId": child.ID, "execution": nativeReceipt.Execution, "outcome": "done", "resultText": "DUPLICATE_WRONG"}); err != nil {
		t.Fatal(err)
	}
	parentState, err := bot.sessions.engine.Command(ctx, canonical.ID, "state", nil)
	if err != nil {
		t.Fatal(err)
	}
	var timeline struct {
		Messages []struct {
			Role        string          `json:"role"`
			HiddenInput bool            `json:"hiddenInput"`
			Content     json.RawMessage `json:"content"`
			AgentMeta   struct {
				Collaboration struct {
					Role string `json:"role"`
				} `json:"botCollaboration"`
			} `json:"agentMeta"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(parentState, &timeline); err != nil {
		t.Fatal(err)
	}
	anchors, results, hiddenInputs := 0, 0, 0
	for _, message := range timeline.Messages {
		if message.Role == "user" && message.HiddenInput {
			hiddenInputs++
		}
		switch message.AgentMeta.Collaboration.Role {
		case "delegation-request":
			anchors++
		case "delegation-result":
			results++
		}
	}
	if anchors != 1 || results != 1 || hiddenInputs != 2 || bytes.Contains(parentState, []byte("DUPLICATE_WRONG")) {
		t.Fatalf("task timeline duplicated or overwritten: %s", parentState)
	}
	mu.Lock()
	if len(requests) != requestsBeforeReplay {
		t.Error("duplicate result woke a new model request")
	}
	replies = append(replies, modelReply{text: "ROUTINE_EXECUTED"})
	mu.Unlock()
	// The renderer saves and starts a routine through the actual public Host
	// methods. Its original engine must dispatch Pi in the existing main chat.
	routineCall := func(method string, input map[string]any) json.RawMessage {
		t.Helper()
		input["botId"] = "im-fixture"
		raw, _ := json.Marshal(input)
		result, err := bot.call(ctx, method, raw)
		if err != nil {
			t.Fatal(method, err)
		}
		return result
	}
	if _, err := bot.runner.Call(ctx, "learning.scan", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	var beforeRoutine struct{ PendingTurns int }
	if err := json.Unmarshal(routineCall("routines.nightly.get", map[string]any{}), &beforeRoutine); err != nil {
		t.Fatal(err)
	}
	var routine struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(routineCall("routines.save", map[string]any{"input": map[string]any{
		"name": "Report", "prompt": "Return ROUTINE_EXECUTED", "enabled": true,
		"preRunHook": map[string]any{"command": "exit 0", "timeoutMs": 10000},
		"triggers":   []any{map[string]any{"id": "daily", "kind": "interval", "intervalMs": 60000}},
	}}), &routine); err != nil || routine.ID == "" {
		t.Fatal("routine was not saved", err)
	}
	routineCall("routines.runNow", map[string]any{"id": routine.ID})
	for {
		var runs []struct {
			Status, ResultText, Error string
			FinishedAt                int64
		}
		if err := json.Unmarshal(routineCall("routines.history", map[string]any{"id": routine.ID}), &runs); err != nil {
			t.Fatal(err)
		}
		if len(runs) > 0 && runs[0].FinishedAt != 0 {
			if runs[0].Status != "success" || runs[0].ResultText != "ROUTINE_EXECUTED" {
				t.Fatalf("routine execution: %+v", runs[0])
			}
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("routine did not settle", ctx.Err())
		case <-time.After(20 * time.Millisecond):
		}
	}
	if draft, err := bot.sessions.store.ReadDraft(canonical.ID); err != nil || draft.Text != "task draft" {
		t.Fatal("routine consumed the owner draft", draft, err)
	}
	routineState, err := bot.sessions.engine.Command(ctx, canonical.ID, "state", nil)
	if err != nil || !bytes.Contains(routineState, []byte("ROUTINE_EXECUTED")) {
		t.Fatal("routine did not run in the original canonical chat", err)
	}
	routineCall("routines.remove", map[string]any{"id": routine.ID})
	if _, err := bot.runner.Call(ctx, "learning.scan", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	var afterRoutine struct{ PendingTurns int }
	if err := json.Unmarshal(routineCall("routines.nightly.get", map[string]any{}), &afterRoutine); err != nil || afterRoutine.PendingTurns != beforeRoutine.PendingTurns {
		t.Fatal("automatic routine was counted as owner learning evidence", beforeRoutine, afterRoutine, err)
	}
	// The original nightly executor uses Host AI services, never a Pi tool turn
	// in the main chat. An empty validated plan records a real no-change result.
	mu.Lock()
	requestsBeforeLearning := len(learningRequests)
	mu.Unlock()
	routineCall("routines.nightly.runNow", map[string]any{})
	for {
		var view struct {
			Running      bool
			PendingTurns int
			LastRun      *struct{ Outcome string }
		}
		if err := json.Unmarshal(routineCall("routines.nightly.get", map[string]any{}), &view); err != nil {
			t.Fatal(err)
		}
		if view.LastRun != nil && !view.Running {
			if view.LastRun.Outcome != "nothing" || view.PendingTurns != 0 {
				t.Fatalf("nightly learning failed: %+v", view)
			}
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("nightly did not settle", ctx.Err())
		case <-time.After(20 * time.Millisecond):
		}
	}
	mu.Lock()
	if len(learningRequests) != requestsBeforeLearning+1 {
		t.Error("nightly learning did not use one auxiliary request")
	}
	if len(reviewRequests) == 0 {
		t.Error("completed owner turns never reached the real learning review model")
	}
	var learningRequest map[string]json.RawMessage
	if err := json.Unmarshal(learningRequests[len(learningRequests)-1], &learningRequest); err != nil {
		t.Error(err)
	}
	var outputLimit int
	for _, field := range []string{"max_tokens", "max_completion_tokens", "max_output_tokens"} {
		if len(learningRequest[field]) > 0 {
			_ = json.Unmarshal(learningRequest[field], &outputLimit)
		}
	}
	if outputLimit != 8000 {
		t.Errorf("helper output budget was dropped: %s", learningRequests[len(learningRequests)-1])
	}
	mu.Unlock()
	sessionID = channelSessionID
	if err := bot.sessions.store.SaveDraft(canonical.ID, chat.Draft{Text: "lifecycle draft"}); err != nil {
		t.Fatal(err)
	}
	lifecycle := func(action string) json.RawMessage {
		t.Helper()
		input, _ := json.Marshal(map[string]string{"botId": "im-fixture", "action": action})
		result, err := bot.call(ctx, "lifecycle.action", input)
		if err != nil {
			t.Fatal(action, err)
		}
		return result
	}
	paused := lifecycle("pause")
	if !bytes.Contains(paused, []byte(`"status":"paused"`)) || bytes.Contains(paused, []byte("SESSION_CLOSE_FAILED")) {
		t.Fatalf("pause failed: %s", paused)
	}
	if _, err := bot.contextFor(ctx, map[string]string{"botId": "im-fixture"}); err == nil {
		t.Fatal("paused bot still dispatched work")
	}
	lifecycle("resume")
	restarted := lifecycle("restart")
	if !bytes.Contains(restarted, []byte(`"status":"active"`)) {
		t.Fatalf("restart failed: %s", restarted)
	}
	after, err := bot.openSession(ctx, "im-fixture")
	if err != nil || after.ID != canonical.ID {
		t.Fatal("restart changed canonical session", err)
	}
	draft, err := bot.sessions.store.ReadDraft(canonical.ID)
	if err != nil || draft.Text != "lifecycle draft" {
		t.Fatal("lifecycle lost the draft", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(requests) < 13 || !bytes.Contains(requests[0], []byte("IM_IDENTITY_PROOF")) || !bytes.Contains(requests[4], []byte("非主人")) {
		t.Fatalf("unexpected model trace (%d): %q", len(requests), requests)
	}
	if rows := store.Snapshot().Conversations; len(rows) != 1 || rows[0].OneBotTaskID != task.Delegations[0].ID {
		t.Fatal("ordinary Chat must contain only the independent task, not companion lanes")
	}
	// Deleting the owner detaches its history into the ordinary archive. The
	// original Pi file and unsent draft must survive, without a usable Bot lease.
	historyBefore, err := os.ReadFile(conversation.SessionFile)
	if err != nil {
		t.Fatal(err)
	}
	remove, _ := json.Marshal(map[string]any{"botId": "im-fixture", "action": "delete", "confirmName": "IM Fixture", "keepTaskHistory": true})
	deleted, err := bot.call(ctx, "lifecycle.action", remove)
	if err != nil || !bytes.Contains(deleted, []byte(`"status":"deleted"`)) {
		t.Fatalf("delete with history: %s %v", deleted, err)
	}
	for _, id := range []string{sessionID, canonical.ID} {
		retained, err := store.Conversation(id)
		if err != nil || !retained.Archived {
			t.Fatalf("detached archive %s: %+v %v", id, retained, err)
		}
		if _, err := bot.sessions.store.Conversation(id); err == nil {
			t.Fatal("deleted Bot still owns the task")
		}
	}
	historyAfter, err := os.ReadFile(conversation.SessionFile)
	if err != nil || !bytes.Equal(historyBefore, historyAfter) {
		t.Fatal("deletion changed Pi history", err)
	}
	if draft, err := store.ReadDraft(canonical.ID); err != nil || draft.Text != "lifecycle draft" {
		t.Fatal("deletion lost unsent draft", err)
	}
	if _, err := bot.contextFor(ctx, map[string]string{"sessionId": canonical.ID}); err == nil {
		t.Fatal("deleted Bot can still configure a runtime")
	}
	if _, err := bot.inputGrant(sessionID, ownerReceipt); err == nil {
		t.Fatal("deleted Bot retained an input grant")
	}
	if _, err := os.Stat(filepath.Join(data, "onebot", "bots", "im-fixture")); !os.IsNotExist(err) {
		t.Fatal("deleted Bot Home was not recycled", err)
	}
	// Explicitly declining history recycles both routes, without adding them to
	// ordinary Chat. These are synthetic profiles in this test's isolated root.
	if _, err := bot.call(ctx, "profiles.create", json.RawMessage(`{"input":{"id":"drop-history","name":"Drop"}}`)); err != nil {
		t.Fatal(err)
	}
	dropMain, err := bot.openSession(ctx, "drop-history")
	if err != nil {
		t.Fatal(err)
	}
	dropLane, err := bot.openRoute(ctx, "drop-history", "test-lane", "channel", "Lane")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bot.call(ctx, "lifecycle.action", json.RawMessage(`{"botId":"drop-history","action":"delete","confirmName":"Drop","keepTaskHistory":false}`)); err != nil {
		t.Fatal(err)
	}
	for _, c := range []chat.Conversation{dropMain, dropLane} {
		if _, err := bot.sessions.store.Conversation(c.ID); err == nil {
			t.Fatal("removed history still indexed")
		}
		if _, err := store.Conversation(c.ID); err == nil {
			t.Fatal("declined history entered ordinary archive")
		}
		if _, err := os.Stat(c.SessionDirectory); !os.IsNotExist(err) {
			t.Fatal("history was not recycled", err)
		}
	}
	// A locked Home must surface a warning and retain a cleanup intent, which
	// the next real Host/worker instance replays after the file is released.
	if _, err := bot.call(ctx, "profiles.create", json.RawMessage(`{"input":{"id":"cleanup-retry","name":"Retry"}}`)); err != nil {
		t.Fatal(err)
	}
	retryHome := filepath.Join(data, "onebot", "bots", "cleanup-retry")
	lockedPath, err := windows.UTF16PtrFromString(filepath.Join(retryHome, "SOUL.md"))
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(lockedPath, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	locked := os.NewFile(uintptr(handle), "locked Home")
	defer locked.Close()
	result, err := bot.call(ctx, "lifecycle.action", json.RawMessage(`{"botId":"cleanup-retry","action":"delete","confirmName":"Retry","keepTaskHistory":true}`))
	if err != nil || !bytes.Contains(result, []byte("HOME_CLEANUP_PENDING")) {
		t.Fatalf("cleanup warning missing: %s %v", result, err)
	}
	if err := bot.close(ctx); err != nil {
		t.Fatal(err)
	}
	if err := locked.Close(); err != nil {
		t.Fatal(err)
	}
	bot = newChatOneBot(bot.control)
	if _, err := bot.ensure(ctx); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		pending, err := bot.runner.Call(ctx, "lifecycle.pendingCleanup", map[string]any{})
		if err != nil {
			t.Fatal(err)
		}
		if string(pending) == "[]" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("cleanup not recovered: %s", pending)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, err := os.Stat(retryHome); !os.IsNotExist(err) {
		t.Fatal("restarted Host did not recycle pending Home", err)
	}
}
