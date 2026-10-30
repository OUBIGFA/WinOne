//go:build windows

package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"winone/internal/ai"
	"winone/internal/chat"
)

// Windows image locks cover every hard link to an executable. A test bundle
// needs its own file so another Bun build cannot masquerade as a leaked child
// during TempDir cleanup. A child that really survives still locks this copy.
func copyOneBotTestRuntime(t *testing.T, source, destination string) {
	t.Helper()
	input, err := os.Open(source)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
	if err != nil {
		t.Fatal(err)
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		t.Fatal(copyErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
}

// OneBot's bundle now includes Sharp/SILK and the original marketplace packages.
// Copy the reviewed runtime siblings with the worker, rather than accidentally
// testing a JS-only installation that cannot load its native image decoder.
func copyOneBotTestWorker(t *testing.T, source, destination string) {
	t.Helper()
	copyOneBotTestRuntime(t, source, destination)
	for _, name := range []string{"node_modules", "bot-packages"} {
		if err := os.CopyFS(filepath.Join(filepath.Dir(destination), name), os.DirFS(filepath.Join(filepath.Dir(source), name))); err != nil {
			t.Fatal(err)
		}
	}
	copyOneBotTestRuntime(t, filepath.Join(filepath.Dir(source), "wechat-silk-worker.js"), filepath.Join(filepath.Dir(destination), "wechat-silk-worker.js"))
}

func TestOneBotHostMethodBoundary(t *testing.T) {
	for _, method := range []string{"profiles.list", "profiles.get", "memory.read", "skills.query", "todos.read", "history.search", "routines.list", "routines.history", "routines.sources", "routines.nightly.get", "autonomy.glance", "autonomy.settings"} {
		if allowed, readOnly := oneBotMethod(method); !allowed || !readOnly {
			t.Fatalf("read method %s: %v %v", method, allowed, readOnly)
		}
	}
	for _, method := range []string{"lifecycle.action", "profiles.create", "profiles.update", "memory.write", "memory.delete", "skills.save", "todos.patch", "routines.save", "routines.remove", "routines.runNow", "routines.nightly.setEnabled", "routines.nightly.setFrequency", "routines.nightly.runNow", "routines.nightly.revert", "routines.nightly.candidate", "autonomy.setSettings", "autonomy.resetSettings", "todos.dailyCheck"} {
		if allowed, readOnly := oneBotMethod(method); !allowed || readOnly {
			t.Fatalf("write method %s: %v %v", method, allowed, readOnly)
		}
	}
	control := newChatOneBot(&chatControl{})
	for _, method := range []string{"init", "close", "cloud.connect", "todos.prepareAction", "todos.settleAction", "profiles.deleteEverything", "history.query", "learning.scan", "routines.tick", "routines.start", "authorize-routine", "autonomy.activity"} {
		if _, err := control.call(context.Background(), method, nil); err == nil {
			t.Fatalf("accepted private method %s", method)
		}
	}
	if control.runner != nil {
		t.Fatal("rejected methods started the worker")
	}
}

func TestOneBotDiagnosticsUseSharedLogWithoutContentDetails(t *testing.T) {
	directory := t.TempDir()
	control := newChatOneBot(&chatControl{directory: directory})
	control.event(json.RawMessage(`{"type":"onebot_event","event":{"type":"diagnostic","scope":"content","level":"warn","message":"index unavailable","detail":["private memory body"]}}`))
	raw, err := os.ReadFile(filepath.Join(directory, "logs", "host.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "index unavailable") || strings.Contains(string(raw), "private memory body") {
		t.Fatalf("invalid diagnostic: %s", raw)
	}
}

// Dictation settings and recognized text use distinct original push channels.
// Losing either leaves the real microphone view waiting despite a good ASR reply.
func TestOneBotVoiceInputEventsReachDesktopSubscribers(t *testing.T) {
	control := newChatOneBot(&chatControl{})
	for _, kind := range []string{"voice-input", "voice-data"} {
		raw, _ := json.Marshal(map[string]any{"type": "onebot_event", "event": map[string]any{"type": kind, "payload": map[string]any{"runId": "owned-run", "text": "语音识别"}}})
		control.event(raw)
	}
	if len(control.events) != 2 {
		t.Fatal("voice input pushes were lost", len(control.events))
	}
	for _, event := range control.events {
		if !strings.Contains(string(event.data), "owned-run") || !strings.Contains(string(event.data), "语音识别") {
			t.Fatal("voice event payload was lost", string(event.data))
		}
	}
}

func TestOneBotCanonicalSessionIsDurableAndSeparateFromChat(t *testing.T) {
	bun, worker := os.Getenv("WINONE_CHAT_BUN_EXE"), os.Getenv("WINONE_ONEBOT_WORKER")
	if bun == "" || worker == "" {
		t.Skip("explicit isolated runtime paths are required")
	}
	root := t.TempDir()
	bundle := filepath.Join(root, "bundle")
	if err := os.MkdirAll(filepath.Join(bundle, "shared", "ai", "runtime"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(bundle, "shared", "chat"), 0700); err != nil {
		t.Fatal(err)
	}
	copyOneBotTestRuntime(t, bun, filepath.Join(bundle, "shared", "ai", "runtime", "bun.exe"))
	copyOneBotTestWorker(t, worker, filepath.Join(bundle, "shared", "chat", "onebot.js"))
	var err error
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	data := filepath.Join(root, "data")
	if err = os.MkdirAll(data, 0700); err != nil {
		t.Fatal(err)
	}
	aiService := newAIService(ctx, data, filepath.Join(bundle, "WinOne.Host.exe"))
	aiService.store, err = ai.Open(aiService.storePath)
	if err != nil {
		t.Fatal(err)
	}
	config := aiService.store.Configuration()
	config.Providers = []ai.Provider{{ID: "fixture", Name: "Fixture", AuthMode: "api-key", Protocol: "chat", BaseURL: "https://example.test/v1", Models: []string{"model"}}}
	delete(config.Tasks, "chat")
	config.Tasks["default"] = ai.Selection{Provider: "fixture", Model: "model"}
	key := "fixture-key"
	if _, err = aiService.store.SaveConfiguration(config, map[string]*string{"fixture": &key}); err != nil {
		t.Fatal(err)
	}
	store, err := chat.Open(filepath.Join(data, "chat"))
	if err != nil {
		t.Fatal(err)
	}
	control := newChatOneBot(&chatControl{ctx: ctx, directory: data, desktop: filepath.Join(bundle, "WinOne.exe"), language: func() string { return "zh-CN" }, ai: aiService, store: store})
	defer control.close(context.Background())
	if _, err = control.call(ctx, "profiles.create", json.RawMessage(`{"input":{"id":"canonical-fixture","name":"独立伙伴"}}`)); err != nil {
		t.Fatal(err)
	}
	first, err := control.openSession(ctx, "canonical-fixture")
	if err != nil {
		t.Fatal(err)
	}
	second, err := control.openSession(ctx, "canonical-fixture")
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || len(store.Snapshot().Conversations) != 0 || len(control.sessions.store.Snapshot().Conversations) != 1 {
		t.Fatal("canonical identity was duplicated or leaked into ordinary Chat")
	}
	if err = control.sessions.store.SaveDraft(first.ID, chat.Draft{Text: "Unsent owner text"}); err != nil {
		t.Fatal(err)
	}
	profileRaw, err := control.call(ctx, "profiles.get", json.RawMessage(`{"botId":"canonical-fixture"}`))
	if err != nil {
		t.Fatal(err)
	}
	var profile struct {
		CurrentVersion int `json:"currentVersion"`
	}
	if err = json.Unmarshal(profileRaw, &profile); err != nil {
		t.Fatal(err)
	}
	// Cindy's create-canonical-session is ensure/recovery, not new-chat.
	// Its renderer and invitation both call it again for a healthy main chat.
	for _, expected := range []any{first.ID, nil} {
		request, _ := json.Marshal(map[string]any{"botId": "canonical-fixture", "expectedCanonicalSessionId": expected, "expectedProfileVersion": profile.CurrentVersion})
		reply, err := control.call(ctx, "bots.createCanonicalSession", request)
		if err != nil {
			t.Fatal(err)
		}
		var result struct {
			Created            bool   `json:"created"`
			CanonicalSessionID string `json:"canonicalSessionId"`
		}
		if err = json.Unmarshal(reply, &result); err != nil {
			t.Fatal(err)
		}
		if result.Created || result.CanonicalSessionID != first.ID || len(control.sessions.store.Snapshot().Conversations) != 1 {
			t.Fatalf("healthy canonical was replaced: %s", reply)
		}
	}
	draft, err := control.sessions.store.ReadDraft(first.ID)
	if err != nil || draft.Text != "Unsent owner text" {
		t.Fatal("canonical lookup lost draft", draft, err)
	}
	if err = control.close(ctx); err != nil {
		t.Fatal(err)
	}
	control.resume()
	third, err := control.openSession(ctx, "canonical-fixture")
	if err != nil || third.ID != first.ID {
		t.Fatalf("reopen: %+v %v", third, err)
	}
}

func TestOneBotMasterKeyUsesExistingDPAPIBoundary(t *testing.T) {
	directory := t.TempDir()
	first, err := oneBotMasterKey(directory)
	if err != nil {
		t.Fatal(err)
	}
	second, err := oneBotMasterKey(directory)
	if err != nil || first != second {
		t.Fatal("master key not stable", err)
	}
	raw, err := os.ReadFile(filepath.Join(directory, "im-master-key.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), first) {
		t.Fatal("master key persisted in plaintext")
	}
}

func TestOneBotHostUsesRealWorkerAndClosesItsJob(t *testing.T) {
	bun, worker := os.Getenv("WINONE_CHAT_BUN_EXE"), os.Getenv("WINONE_ONEBOT_WORKER")
	if bun == "" || worker == "" {
		t.Skip("explicit isolated OneBot runtime paths are required")
	}
	directory := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	runner, err := chat.StartRunner(ctx, bun, worker, directory, nil)
	if err != nil {
		t.Fatal(err)
	}
	control := newChatOneBot(&chatControl{ctx: ctx, directory: directory})
	control.runner = runner
	defer control.close(context.Background())
	if _, err := runner.Call(ctx, "init", map[string]any{"directory": directory}); err != nil {
		t.Fatal(err)
	}
	call := func(method string, params any) json.RawMessage {
		t.Helper()
		raw, err := json.Marshal(params)
		if err != nil {
			t.Fatal(err)
		}
		result, err := control.call(ctx, method, raw)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	created := call("profiles.create", map[string]any{"input": map[string]any{"id": "host-fixture", "name": "Host OneBot"}})
	if !strings.Contains(string(created), `"harness":"pi"`) {
		t.Fatalf("profile: %s", created)
	}
	call("memory.write", map[string]any{"botId": "host-fixture", "input": map[string]any{"type": "user", "name": "pref", "title": "偏好", "description": "沟通偏好", "body": "简明表达"}})
	if raw := call("memory.list", map[string]any{"botId": "host-fixture"}); !strings.Contains(string(raw), "user_pref.md") {
		t.Fatalf("memory: %s", raw)
	}
	// Exercise the real Worker -> Runner -> Host error text used by the unchanged
	// memory editor. A stale save/delete must remain recognizable as a conflict.
	for _, method := range []string{"memory.update", "memory.delete"} {
		input, err := json.Marshal(map[string]any{"botId": "host-fixture", "filename": "user_pref.md",
			"expectedUpdatedAt": "2000-01-01T00:00:00.000Z", "title": "Stale", "body": "Must not replace the saved text"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := control.call(ctx, method, input); err == nil || !strings.Contains(err.Error(), "[PRECONDITION_FAILED] bot-memory-changed") {
			t.Fatalf("%s lost editor conflict information: %v", method, err)
		}
	}
	if raw := call("memory.read", map[string]any{"botId": "host-fixture", "filename": "user_pref.md"}); !strings.Contains(string(raw), "简明表达") {
		t.Fatalf("stale editor changed the memory: %s", raw)
	}
	if err := control.close(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-runner.Done():
	default:
		t.Fatal("OneBot worker is still alive after close")
	}
	file := filepath.Join(directory, "bots", "host-fixture", "memories", "fts.db")
	if err := os.Rename(file, file+".closed"); err != nil {
		t.Fatal(err)
	}
	if _, err := control.call(ctx, "profiles.list", nil); err == nil {
		t.Fatal("closed service restarted unexpectedly")
	}
}
