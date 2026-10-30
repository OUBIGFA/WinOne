//go:build windows

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"winone/internal/ai"
	"winone/internal/chat"
)

func TestOneBotGroupMethodBoundary(t *testing.T) {
	for _, method := range []string{"groups.list", "groups.get", "groups.create", "groups.update", "groups.setMembers", "groups.delete", "groups.send", "groups.continue", "groups.stop", "groups.plan.start", "groups.plan.dismiss", "groups.plan.continue", "groups.plan.retry", "groups.plan.edit", "groups.attachments.stage", "groups.attachments.image"} {
		allowed, readOnly := oneBotMethod(method)
		if !allowed || readOnly != (method == "groups.list" || method == "groups.get") {
			t.Fatalf("%s: %v %v", method, allowed, readOnly)
		}
	}
	for _, method := range []string{"group-lane", "group-lanes-close", "group-worktree", "group-folder-recycle", "groups.dispatch"} {
		if allowed, _ := oneBotMethod(method); allowed {
			t.Fatalf("private route exposed: %s", method)
		}
	}
}

func TestOneBotGroupProjectAndRecycleBoundaries(t *testing.T) {
	root := t.TempDir()
	bot := newChatOneBot(&chatControl{directory: root})
	managed := filepath.Join(root, "onebot", "bots", "member", "workspace")
	project := filepath.Join(root, "project")
	for _, dir := range []string{managed, project} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, item := range []struct {
		dir string
		ok  bool
	}{{project, true}, {managed, false}, {"relative", false}, {filepath.Join(root, "missing"), false}} {
		raw, _ := json.Marshal(map[string]string{"directory": item.dir})
		result, err := bot.groupProjectDirectory(raw)
		if err != nil || result.(map[string]any)["ok"] != item.ok {
			t.Fatalf("%s: %v %v", item.dir, result, err)
		}
	}
	// Rejected paths never reach the recycle API (no system mutation in this test).
	for _, dir := range []string{root, project, filepath.Join(root, "onebot", "bot-groups"), filepath.Join(root, "onebot", "bot-groups", "g", "files")} {
		raw, _ := json.Marshal(map[string]string{"directory": dir})
		if _, err := bot.recycleGroupFolder(context.Background(), raw); err == nil {
			t.Fatalf("accepted non-group root: %s", dir)
		}
	}
	for _, route := range []string{"canonical", "im:group", "group:../outside", "group:g:thread:remote"} {
		raw, _ := json.Marshal(map[string]string{"routeKey": route})
		if _, err := bot.groupLane(context.Background(), raw); err == nil {
			t.Fatalf("accepted route: %s", route)
		}
	}
}

// The manifest is an input receipt, not a cache of the selected source file.
// Reopening and changing the source must never change a submitted attachment.
func TestOneBotGroupAttachmentsKeepNamesAndReplayAfterReopen(t *testing.T) {
	root := t.TempDir()
	store, err := chat.Open(filepath.Join(root, "onebot", "chat"))
	if err != nil {
		t.Fatal(err)
	}
	row, err := store.NewConversation("", root, "Group lane", ai.Selection{})
	if err != nil {
		t.Fatal(err)
	}
	newHost := func() *chatOneBot {
		bot := newChatOneBot(&chatControl{directory: root})
		bot.sessions = &chatControl{store: store}
		return bot
	}
	bot := newHost()
	source := filepath.Join(root, "source.txt")
	if err = os.WriteFile(source, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	input := []oneBotGroupAttachment{{Name: "notes.txt", Path: source}}
	first, err := bot.groupAttachments(row.ID, "input-1", input)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(source, []byte("edited after send"), 0600); err != nil {
		t.Fatal(err)
	}
	bot = newHost()
	repeated, err := bot.groupAttachments(row.ID, "input-1", input)
	if err != nil || len(repeated) != 1 || repeated[0] != first[0] {
		t.Fatal("receipt changed after reopening", repeated, err)
	}
	data, err := os.ReadFile(repeated[0])
	if err != nil || string(data) != "original" || filepath.Base(repeated[0]) != "notes.txt" {
		t.Fatal("snapshot/name changed", string(data), repeated, err)
	}
	if _, err = bot.groupAttachments(row.ID, "input-1", []oneBotGroupAttachment{{Name: "changed.txt", Path: source}}); err == nil {
		t.Fatal("reused receipt with different input")
	}
	fresh, err := bot.groupAttachments(row.ID, "input-2", input)
	if err != nil || fresh[0] == first[0] {
		t.Fatal("new input reused old bytes", fresh, err)
	}
	data, err = os.ReadFile(fresh[0])
	if err != nil || string(data) != "edited after send" {
		t.Fatal("new input lost current source", string(data), err)
	}
	blobRoot := filepath.Join(root, "onebot", "media", "blobs")
	if err = os.MkdirAll(blobRoot, 0700); err != nil {
		t.Fatal(err)
	}
	png, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==")
	if err != nil {
		t.Fatal(err)
	}
	blob := strings.Repeat("a", 64) + ".png"
	if err = os.WriteFile(filepath.Join(blobRoot, blob), png, 0600); err != nil {
		t.Fatal(err)
	}
	image, err := bot.groupAttachments(row.ID, "image-1", []oneBotGroupAttachment{{Name: "chart.png", URL: "cindy-media://blobs/" + blob}})
	if err != nil || filepath.Base(image[0]) != "chart.png" {
		t.Fatal("managed image lost its name", image, err)
	}
	for i, invalid := range []oneBotGroupAttachment{{Name: "x", URL: "https://example.test/image.png"}, {Name: "x", Path: "relative"}, {Name: "../x", Path: source}, {Name: "x", URL: "cindy-media://blobs/" + strings.Repeat("b", 64) + ".png"}} {
		if _, err = bot.groupAttachments(row.ID, fmt.Sprintf("invalid-%d", i), []oneBotGroupAttachment{invalid}); err == nil {
			t.Fatal("invalid attachment accepted", invalid)
		}
	}
}

// Actual Host -> group service -> private member lane -> Pi -> Magpie -> local
// HTTP, not a callback substitute. It also covers per-input file authority,
// draft preservation and group removal without touching the companion itself.
func TestOneBotGroupHostRoundTripWithRealPi(t *testing.T) {
	bun, worker, pi, aiExe := os.Getenv("WINONE_CHAT_BUN_EXE"), os.Getenv("WINONE_ONEBOT_WORKER"), os.Getenv("WINONE_CHAT_WORKER"), os.Getenv("WINONE_AI_TEST_EXE")
	if bun == "" || worker == "" || pi == "" || aiExe == "" {
		t.Skip("explicit isolated OneBot/Pi/AI runtime paths are required")
	}
	root := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Second)
	defer cancel()
	bundle, data := filepath.Join(root, "bundle"), filepath.Join(root, "data")
	chatDir, bunDir := filepath.Join(bundle, "shared", "chat"), filepath.Join(bundle, "shared", "ai", "runtime")
	for _, dir := range []string{chatDir, bunDir} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	copyOneBotTestRuntime(t, bun, filepath.Join(bunDir, "bun.exe"))
	for _, file := range []string{"worker.js", "codemode-worker.js", "script-runner.js", "quickjs.wasm", "photon_rs_bg.wasm"} {
		if err := os.Link(filepath.Join(filepath.Dir(pi), file), filepath.Join(chatDir, file)); err != nil {
			t.Fatal(err)
		}
	}
	copyOneBotTestWorker(t, worker, filepath.Join(chatDir, "onebot.js"))
	var mu sync.Mutex
	var requests []string
	var protectedCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/audio/speech") {
			w.Header().Set("Content-Type", "audio/pcm")
			_, _ = w.Write([]byte{0, 0, 1, 0, 2, 0, 3, 0})
			return
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		mu.Lock()
		requests = append(requests, string(raw))
		protectedStep := 0
		if strings.Contains(string(raw), "PROTECTED_CONTENT_NOT_ARCHIVED") {
			protectedCalls++
			protectedStep = protectedCalls
		}
		mu.Unlock()
		var input struct {
			Tools    []any
			Messages []struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"messages"`
		}
		if err = json.Unmarshal(raw, &input); err != nil {
			t.Error(err)
			return
		}
		// One real tool call proves a local owner's group work is not accidentally
		// downgraded to the guest-IM read-only policy.
		w.Header().Set("Content-Type", "text/event-stream")
		emit := func(delta any, finish any) {
			b, _ := json.Marshal(map[string]any{"id": "group-fixture", "object": "chat.completion.chunk", "model": "model", "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}}})
			fmt.Fprintf(w, "data: %s\n\n", b)
		}
		emit(map[string]string{"role": "assistant"}, nil)
		if protectedStep == 2 {
			args, _ := json.Marshal(map[string]any{"questions": []any{map[string]any{"question": "Confirm protected result?", "options": []string{"Yes", "No"}}}})
			emit(map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "protected-question", "type": "function", "function": map[string]string{"name": "ask_user_question", "arguments": string(args)}}}}, nil)
			emit(map[string]any{}, "tool_calls")
		} else if len(input.Tools) == 0 {
			// The native voice dialogue consumes NDJSON actions, including partial
			// lines across SSE chunks, rather than ordinary chat text.
			emit(map[string]string{"content": "{\"say\":\"VOICE_"}, nil)
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			emit(map[string]string{"content": "REPLY\"}\n"}, nil)
			emit(map[string]any{}, "stop")
		} else if len(input.Messages) > 0 && input.Messages[len(input.Messages)-1].Role == "tool" {
			emit(map[string]string{"content": "GROUP_COMPLETED"}, nil)
			emit(map[string]any{}, "stop")
		} else {
			args, _ := json.Marshal(map[string]string{"path": "group-proof.txt", "content": "GROUP_OWNER_REQUEST"})
			emit(map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "group-write", "type": "function", "function": map[string]string{"name": "write", "arguments": string(args)}}}}, nil)
			emit(map[string]any{}, "tool_calls")
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer upstream.Close()
	service := newAIService(ctx, data, filepath.Join(bundle, "WinOne.Host.exe"))
	var err error
	service.store, err = ai.Open(service.storePath)
	if err != nil {
		t.Fatal(err)
	}
	service.gatewaySlots = make(chan struct{}, 4)
	service.translator = ai.Translator{Executable: aiExe, DataDirectory: filepath.Join(data, "ai-runtime")}
	cfg := service.store.Configuration()
	cfg.Providers = []ai.Provider{{ID: "fixture", Name: "Fixture", AuthMode: "api-key", Protocol: "chat", BaseURL: upstream.URL, Models: []string{"model", "tts-1", "whisper-1"}, ModelMetadata: map[string]ai.ModelMetadata{"model": {ContextWindow: 200000}}}}
	cfg.Tasks["chat"] = ai.Selection{Provider: "fixture", Model: "model"}
	key := "isolated-key"
	if _, err = service.store.SaveConfiguration(cfg, map[string]*string{"fixture": &key}); err != nil {
		t.Fatal(err)
	}
	store, err := chat.Open(filepath.Join(data, "chat"))
	if err != nil {
		t.Fatal(err)
	}
	bot := newChatOneBot(&chatControl{ctx: ctx, directory: data, desktop: filepath.Join(bundle, "WinOne.exe"), language: func() string { return "zh-CN" }, store: store, ai: service})
	defer func() {
		end, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if err := bot.close(end); err != nil {
			t.Error(err)
		}
	}()
	call := func(method string, input any) json.RawMessage {
		t.Helper()
		raw, _ := json.Marshal(input)
		result, err := bot.call(ctx, method, raw)
		if err != nil {
			t.Fatalf("%s: %v", method, err)
		}
		return result
	}
	for _, id := range []string{"alpha", "beta"} {
		call("profiles.create", map[string]any{"input": map[string]any{"id": id, "name": id, "capabilities": map[string]string{"permissions": "bypassPermissions"}}})
	}
	var created struct {
		Ok      bool
		GroupID string
	}
	if err = json.Unmarshal(call("groups.create", map[string]any{"input": map[string]any{"name": "Native team", "botIds": []string{"alpha", "beta"}}}), &created); err != nil || !created.Ok {
		t.Fatalf("create: %+v %v", created, err)
	}
	call("groups.update", map[string]any{"input": map[string]any{"groupId": created.GroupID, "replyMode": "mentioned"}})
	// Create the same hidden lane twice; its identity and model follow the member.
	laneInput, _ := json.Marshal(map[string]string{"botId": "alpha", "routeKey": "group:" + created.GroupID, "title": "Native team"})
	lane, err := bot.groupLane(ctx, laneInput)
	if err != nil {
		t.Fatal(err)
	}
	laneID := lane.(map[string]any)["sessionId"].(string)
	again, err := bot.groupLane(ctx, laneInput)
	if err != nil || again.(map[string]any)["sessionId"] != laneID {
		t.Fatal("group lane duplicated", again, err)
	}
	row, err := bot.sessions.store.Conversation(laneID)
	if err != nil {
		t.Fatal(err)
	}
	if err = bot.sessions.store.SaveDraft(laneID, chat.Draft{Text: "UNSENT_OWNER_DRAFT"}); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "notes.txt")
	if err = os.WriteFile(file, []byte("GROUP_ATTACHMENT_CONTENT"), 0600); err != nil {
		t.Fatal(err)
	}
	call("groups.send", map[string]any{"input": map[string]any{"groupId": created.GroupID, "clientId": "native-group-input", "text": "@alpha execute this file task", "mentions": map[string]any{"botIds": []string{"alpha"}, "all": false}, "attachments": []any{map[string]any{"name": "notes.txt", "path": file, "mimeType": "text/plain", "category": "text"}}}})
	var detail struct {
		Group struct {
			Round    struct{ Status string }
			Messages []struct{ AuthorKind, Content string }
		}
	}
	for {
		if err = json.Unmarshal(call("groups.get", map[string]string{"groupId": created.GroupID}), &detail); err != nil {
			t.Fatal(err)
		}
		if detail.Group.Round.Status == "idle" && len(detail.Group.Messages) > 1 {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("group did not settle", ctx.Err())
		case <-time.After(30 * time.Millisecond):
		}
	}
	found := false
	for _, m := range detail.Group.Messages {
		if m.AuthorKind == "bot" && m.Content == "GROUP_COMPLETED" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing actual reply: %+v", detail)
	}
	proof, err := os.ReadFile(filepath.Join(row.Directory, "group-proof.txt"))
	if err != nil || string(proof) != "GROUP_OWNER_REQUEST" {
		t.Fatal("group tool was not executed", string(proof), err)
	}
	draft, err := bot.sessions.store.ReadDraft(laneID)
	if err != nil || draft.Text != "UNSENT_OWNER_DRAFT" {
		t.Fatal("group consumed unsent draft", draft, err)
	}
	mu.Lock()
	joined := strings.Join(requests, "\n")
	mu.Unlock()
	if !strings.Contains(joined, "GROUP_ATTACHMENT_CONTENT") {
		t.Fatal("attachment did not reach the member model")
	}
	// A lost native acknowledgement must replay the same immutable attachment
	// copy, not import a new random path and conflict with the accepted input.
	replayInput, _ := json.Marshal(map[string]any{"targetSessionId": laneID, "clientId": "group-replay-input", "message": "Repeat receipt only", "attachments": []oneBotGroupAttachment{{Name: "notes.txt", Path: file}}})
	if _, err = bot.taskNative(ctx, "dispatch", replayInput); err != nil {
		t.Fatal("first attachment dispatch", err)
	}
	if _, err = bot.taskNative(ctx, "dispatch", replayInput); err != nil {
		t.Fatal("attachment acknowledgement replay", err)
	}
	if len(store.Snapshot().Conversations) != 0 {
		t.Fatal("group lane leaked into ordinary Chat")
	}
	// The first step uses the identity already registered for its worktree.
	const worktreeSessionID = "ZYXWVUTSRQPONMLKJIHGFEDCBA"
	planInput, _ := json.Marshal(map[string]string{"botId": "alpha", "routeKey": "group:" + created.GroupID + ":plan:native", "workingDir": root, "sessionId": worktreeSessionID})
	planLane, err := bot.groupLane(ctx, planInput)
	if err != nil || planLane.(map[string]any)["sessionId"] != worktreeSessionID {
		t.Fatal("worktree registration identity was lost", planLane, err)
	}
	call("groups.delete", map[string]string{"groupId": created.GroupID})
	row, err = bot.sessions.store.Conversation(laneID)
	if err != nil || !row.Archived {
		t.Fatal("deleted group lane not archived", row, err)
	}
	call("profiles.get", map[string]string{"botId": "alpha"})
	// Voice uses the same local AI service and canonical task queue. Its private
	// dialogue is durable presentation metadata, and the composer stays untouched.
	var canonical chat.Conversation
	if err = json.Unmarshal(call("session.open", map[string]string{"botId": "alpha"}), &canonical); err != nil || canonical.ID == "" {
		t.Fatal("voice canonical", err)
	}
	if err = bot.sessions.store.SaveDraft(canonical.ID, chat.Draft{Text: "VOICE_UNSENT_DRAFT"}); err != nil {
		t.Fatal(err)
	}
	var speechModels []map[string]any
	if err = json.Unmarshal(call("voice.models", map[string]any{}), &speechModels); err != nil || len(speechModels) != 1 {
		t.Fatal("speech catalog", speechModels, err)
	}
	selection := map[string]any{"callId": "native-call", "botId": "alpha", "sessionId": canonical.ID, "providerId": "fixture", "agent": "pi", "modelId": "tts-1", "protocol": "openai", "voice": "alloy"}
	call("voice.start", map[string]any{"input": selection})
	call("voice.submitTurn", map[string]any{"callId": "native-call", "inputId": "voice-small-talk", "text": "VOICE_CASUAL_UTTERANCE", "forceHandoff": false})
	for {
		raw, readErr := bot.sessions.engine.Open(ctx, canonical.ID, 0)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if strings.Contains(string(raw), "VOICE_REPLY") {
			break
		}
		if strings.Contains(string(raw), "\"failed\":true") {
			t.Fatal("voice turn failed", string(raw))
		}
		select {
		case <-ctx.Done():
			t.Fatal("voice reply not persisted", string(raw))
		case <-time.After(40 * time.Millisecond):
		}
	}
	call("voice.speak", map[string]any{"input": map[string]string{"callId": "native-call", "requestId": "speech-one", "text": "VOICE_REPLY"}})
	var speech struct{ Pcm string }
	if err = json.Unmarshal(call("voice.readSpeech", map[string]string{"callId": "native-call", "requestId": "speech-one"}), &speech); err != nil || speech.Pcm == "" {
		t.Fatal("voice audio", err)
	}
	call("voice.submitTurn", map[string]any{"callId": "native-call", "inputId": "voice-file-task", "text": "Save the requested file", "forceHandoff": true})
	for {
		raw, readErr := bot.sessions.engine.Open(ctx, canonical.ID, 0)
		if readErr != nil {
			t.Fatal(readErr)
		}
		// The original dialogue folds the actual task result into its owned call
		// record. A main-chat bubble alone does not prove this return path.
		var state struct {
			Messages []struct {
				AgentMeta struct{ VoiceCall json.RawMessage }
			}
		}
		if err = json.Unmarshal(raw, &state); err != nil {
			t.Fatal(err)
		}
		returned := false
		for _, message := range state.Messages {
			if strings.Contains(string(message.AgentMeta.VoiceCall), "GROUP_COMPLETED") {
				returned = true
			}
		}
		if returned {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("voice task result did not return to the call", string(raw))
		case <-time.After(40 * time.Millisecond):
		}
	}
	voiceProof, err := os.ReadFile(filepath.Join(canonical.Directory, "group-proof.txt"))
	if err != nil || string(voiceProof) != "GROUP_OWNER_REQUEST" {
		t.Fatal("voice task did not execute", string(voiceProof), err)
	}
	call("voice.end", map[string]string{"callId": "native-call"})
	if err = bot.sessions.engine.CloseSession(ctx, canonical.ID); err != nil {
		t.Fatal(err)
	}
	reopened, err := bot.sessions.engine.Open(ctx, canonical.ID, 0)
	if err != nil || !strings.Contains(string(reopened), "VOICE_CASUAL_UTTERANCE") || !strings.Contains(string(reopened), "VOICE_REPLY") {
		t.Fatal("voice history not retained", string(reopened), err)
	}
	kept, err := bot.sessions.store.ReadDraft(canonical.ID)
	if err != nil || kept.Text != "VOICE_UNSENT_DRAFT" {
		t.Fatal("voice replaced the composer draft", kept, err)
	}
	// A protected turn keeps the real permission/question flow and prior
	// context, but does not mutate its canonical history or learning projection.
	canonical, err = bot.sessions.store.Conversation(canonical.ID)
	if err != nil {
		t.Fatal(err)
	}
	beforeProtected, err := os.ReadFile(canonical.SessionFile)
	if err != nil {
		t.Fatal(err)
	}
	protected, _ := json.Marshal(map[string]any{"connection": map[string]string{"id": "protected123", "botId": "alpha", "channel": "telegram"}, "message": map[string]any{"text": "PROTECTED_CONTENT_NOT_ARCHIVED", "protectedContent": true}, "requestId": "protected-native-input", "routeKey": "canonical", "ownerAuthorized": true, "group": true})
	native := httptest.NewServer(http.HandlerFunc(bot.serveIM))
	defer native.Close()
	request, _ := http.NewRequestWithContext(ctx, http.MethodPost, native.URL+"/v1/im/protected", bytes.NewReader(protected))
	response, err := native.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(response.Body)
		t.Fatal("protected turn did not start", response.StatusCode, string(raw))
	}
	confirmed, answered, completed := false, false, false
	scanner := bufio.NewScanner(response.Body)
	for scanner.Scan() {
		var event struct {
			SessionID string
			Approval  *struct{ ID, Kind string }
			Result    json.RawMessage
			Error     string
		}
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil || event.Error != "" {
			t.Fatal("protected event", string(scanner.Bytes()), err)
		}
		if event.Approval != nil {
			answer, _ := json.Marshal(map[string]any{"sessionId": event.SessionID, "requestId": "protected-native-input", "id": event.Approval.ID, "kind": event.Approval.Kind, "allow": true, "answers": []string{"Yes"}})
			reply := httptest.NewRecorder()
			bot.serveIM(reply, httptest.NewRequest(http.MethodPost, "/v1/im/interaction", bytes.NewReader(answer)).WithContext(ctx))
			if reply.Code != http.StatusOK {
				t.Fatal("protected interaction", reply.Code, reply.Body.String())
			}
			if event.Approval.Kind == "question" {
				answered = true
			} else {
				confirmed = true
			}
		}
		if strings.Contains(string(event.Result), "GROUP_COMPLETED") {
			completed = true
		}
	}
	if err := scanner.Err(); err != nil || !confirmed || !answered || !completed {
		t.Fatal("protected flow incomplete", confirmed, answered, completed, err)
	}
	savedHistory, err := os.ReadFile(canonical.SessionFile)
	if err != nil || !bytes.Equal(beforeProtected, savedHistory) {
		t.Fatal("protected turn changed durable history", err)
	}
}
