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
	"sync/atomic"
	"testing"
	"time"

	"winone/internal/ai"
	"winone/internal/chat"
)

// This fixture owns a real catalog and AI configuration under t.TempDir. Model
// and session mapping tests do not open Pi, a plugin sandbox or any account.
func legacyGhostFixture(t *testing.T, endpoint string) (*chatGhosts, *chat.Store) {
	t.Helper()
	root := t.TempDir()
	store, err := chat.Open(filepath.Join(root, "chat"))
	if err != nil {
		t.Fatal(err)
	}
	service := newAIService(context.Background(), root, filepath.Join(root, "unused-host.exe"))
	settings, err := service.magpieStore()
	if err != nil {
		t.Fatal(err)
	}
	if endpoint == "" {
		endpoint = "https://example.invalid/v1"
	}
	config := settings.Configuration()
	config.Providers = []ai.Provider{{ID: "fixture", Name: "Fixture", AuthMode: "api-key", Protocol: "chat", BaseURL: endpoint, Models: []string{"model", "another"}}}
	config.Tasks = map[string]ai.Selection{"chat": {Provider: "fixture", Model: "model"}}
	if _, err = settings.SaveConfiguration(config, nil); err != nil {
		t.Fatal(err)
	}
	control := &chatControl{ctx: context.Background(), store: store, ai: service, directory: root}
	g := newChatGhosts(control)
	if err = os.MkdirAll(filepath.Join(root, "chat", "ghosts"), 0700); err != nil {
		t.Fatal(err)
	}
	return g, store
}

func TestLegacyGhostErrandSettingsPreserveUserModelAndReplaceOnlyTheMappedTask(t *testing.T) {
	g, store := legacyGhostFixture(t, "")
	request := ghostLegacyErrandRequest{GhostID: "fixture", SessionKey: "report", PermissionMode: "ask", TaskConfig: &ghostLegacyTaskConfig{}}
	first, err := g.ensureLegacyErrand(request)
	if err != nil {
		t.Fatal(err)
	}
	if first.Permission != "ask" || first.Model.Model != "model" || first.PluginID != "" || first.Hidden {
		t.Fatalf("not an ordinary visible errand: %+v", first)
	}
	_, err = store.UpdateConversation(first.ID, func(row *chat.Conversation) error {
		row.Model.Model, row.ThinkingLevel = "another", "high"
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	reused, err := g.ensureLegacyErrand(request)
	if err != nil || reused.ID != first.ID || reused.Model.Model != "another" || reused.ThinkingLevel != "high" {
		t.Fatalf("an unconfigured errand lost the owner's task model: %+v %v", reused, err)
	}
	effort := "adaptive-vendor"
	request.TaskConfig = &ghostLegacyTaskConfig{ProviderID: "fixture", Model: "model", Effort: &effort, FastMode: json.RawMessage(`true`)}
	request.PermissionMode = "auto"
	next, err := g.ensureLegacyErrand(request)
	if err != nil || next.ID == first.ID || next.Model.Model != "model" || next.ThinkingLevel != effort || !next.FastMode || next.Permission != "auto" {
		t.Fatalf("explicit user settings did not allocate the exact new task: %+v %v", next, err)
	}
	old, err := store.Conversation(first.ID)
	if err != nil || old.Archived || old.Model.Model != "another" || old.Permission != "ask" {
		t.Fatalf("replacing the mapping rewrote the old task: %+v %v", old, err)
	}
	request.TaskConfig.FastMode = json.RawMessage(`"ultrafast"`)
	if _, err = g.ensureLegacyErrand(request); err == nil {
		t.Fatal("invalid Fast mode was silently changed")
	}
	if g.errands[g.errandKey("fixture", "report")] != next.ID || len(store.Snapshot().Conversations) != 2 {
		t.Fatal("failed configuration changed the persisted mapping or allocated a task")
	}
	request.TaskConfig.FastMode = json.RawMessage(`true`)
	request.TaskConfig.Model = "missing"
	if _, err = g.ensureLegacyErrand(request); err == nil {
		t.Fatal("an unavailable selected model used an implicit fallback")
	}
	request.TaskConfig.Model = "model"
	request.WorkingDir = filepath.Join(g.control.directory, "project")
	if err = os.MkdirAll(request.WorkingDir, 0700); err != nil {
		t.Fatal(err)
	}
	project, err := g.ensureLegacyErrand(request)
	if err != nil || project.ID == next.ID || project.Directory != request.WorkingDir {
		t.Fatalf("configured project directory: %+v %v", project, err)
	}
	request.WorkingDir += string(filepath.Separator)
	again, err := g.ensureLegacyErrand(request)
	if err != nil || again.ID != project.ID {
		t.Fatalf("path normalization kept creating errands: %+v %v", again, err)
	}
	reopened := newChatGhosts(g.control)
	if reopened.errands[g.errandKey("fixture", "report")] != project.ID {
		t.Fatal("the accepted sessionKey mapping did not survive Host reopening")
	}
	request.SessionKey = "another-object"
	other, err := reopened.ensureLegacyErrand(request)
	if err != nil || other.ID == project.ID {
		t.Fatalf("independent business keys reused one task: %+v %v", other, err)
	}
}

func TestLegacyGhostWorkspaceCreatesAnIdleDraftAndReusesTheNewestVisibleTask(t *testing.T) {
	g, store := legacyGhostFixture(t, "")
	g.control.engine = chat.NewService(context.Background(), store, chat.ServiceOptions{})
	directory := filepath.Join(g.control.directory, "project")
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	created, err := g.createLegacyDraft(directory, "Authorized project", nil, "auto")
	if err != nil {
		t.Fatal(err)
	}
	id := created.(map[string]any)["sessionId"].(string)
	row, err := store.Conversation(id)
	if err != nil || row.Permission != "auto" || row.Directory != directory || row.HasMessages || g.control.engine.Active() {
		t.Fatalf("workspace draft started an Agent or lost its config: %+v %v", row, err)
	}
	visible, err := store.NewConfiguredConversation("", directory, "Visible plugin-created task", ai.Selection{Provider: "fixture", Model: "model"}, "ask", "", false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.UpdateConversation(visible.ID, func(value *chat.Conversation) error { value.PluginID = "other-plugin"; return nil })
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"hidden", "worker", "archived"} {
		unavailable, err := store.NewConfiguredConversation("", directory, kind, ai.Selection{Provider: "fixture", Model: "model"}, "ask", "", false)
		if err != nil {
			t.Fatal(err)
		}
		_, err = store.UpdateConversation(unavailable.ID, func(value *chat.Conversation) error {
			value.Hidden, value.Archived = kind == "hidden", kind == "archived"
			if kind == "worker" {
				value.OrcaRole = "worker"
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	found, err := g.findLegacyWorkspace(directory)
	if err != nil || found.(map[string]any)["sessionId"] != visible.ID {
		t.Fatalf("workspace ensure reused a hidden task or missed an ordinary plugin task: %+v %v", found, err)
	}
	if g.control.engine.Active() {
		t.Fatal("workspace dedupe opened Pi")
	}
}

// The deterministic local HTTP model is the only model seam. Real Pi writes
// receipts and history; this checks fast completion, retirement/reopening, an
// empty reply and a long reply against the exact input, not the latest answer.
func TestLegacyGhostCurrentAgentModesKeepSourceAuthorityAndUseTheRealQueue(t *testing.T) {
	bun, worker := os.Getenv("WINONE_CHAT_BUN_EXE"), os.Getenv("WINONE_CHAT_WORKER")
	if bun == "" || worker == "" {
		t.Skip("explicit bundled Pi runtime fixture")
	}
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		number := calls.Add(1)
		if number == 1 {
			close(started)
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"MODE_RESULT_%d\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n", number)
	}))
	defer model.Close()
	g, store := legacyGhostFixture(t, model.URL+"/v1")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	g.control.ctx = ctx
	g.control.engine = chat.NewService(ctx, store, chat.ServiceOptions{Bun: bun, Worker: worker, Configure: func(row chat.Conversation) (map[string]any, func(), error) {
		return map[string]any{"baseUrl": model.URL + "/v1", "key": "legacy-agent-fixture-key", "agentDir": filepath.Join(g.control.directory, "agents", row.ID), "model": map[string]any{"id": "fixture/" + row.Model.Model, "name": "Fixture", "efforts": []string{"high"}}, "runtimeSettings": map[string]any{"toolPreset": "chat-only", "autoRetry": false}}, func() {}, nil
	}})
	defer func() {
		end, stop := context.WithTimeout(context.Background(), 8*time.Second)
		defer stop()
		if err := g.control.engine.Stop(end); err != nil {
			t.Error(err)
		}
	}()
	source, err := store.NewConfiguredConversation("", "", "Owner task", ai.Selection{Provider: "fixture", Model: "another"}, "auto", "high", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = g.control.engine.Open(ctx, source.ID, 0); err != nil {
		t.Fatal(err)
	}
	defer g.control.engine.Release(source.ID, 0)
	if _, err = g.control.engine.Prompt(ctx, source.ID, "owner-input", "Owner-authored request"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	run := func(mode, title string) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(map[string]string{"GhostID": "fixture", "SourceSessionID": source.ID, "Mode": mode, "Prompt": "Plugin-authored continuation", "Title": title, "PersistedContent": `{"kind":"delegated-continuation"}`})
		result, err := g.runLegacyAgent(ctx, raw)
		if err != nil {
			t.Fatal(err)
		}
		value := result.(map[string]any)
		if value["ok"] != true {
			t.Fatalf("%s: %+v", mode, value)
		}
		return value
	}
	queued := run("continue", "")
	if queued["sessionId"] != source.ID || queued["disposition"] != "queued" {
		t.Fatalf("active input was not queued on its source: %+v", queued)
	}
	close(release)
	await := func(id, want string) {
		t.Helper()
		if _, err := g.control.engine.Open(ctx, id, 0); err != nil {
			t.Fatal(err)
		}
		defer g.control.engine.Release(id, 0)
		cursor := uint64(0)
		lastState := ""
		for {
			page, err := g.control.engine.WaitEvents(ctx, id, cursor)
			if err != nil {
				t.Fatalf("waiting for %s; requests=%d last state=%s: %v", want, calls.Load(), lastState, err)
			}
			cursor = page.Cursor
			raw, err := g.control.engine.Command(ctx, id, "state", nil)
			if err != nil {
				t.Fatalf("state read for %s failed; model calls=%d cursor=%d last=%s: %v", want, calls.Load(), cursor, lastState, err)
			}
			lastState = string(raw)
			var state struct {
				Messages []struct {
					Role    string
					Content json.RawMessage
				}
			}
			if json.Unmarshal(raw, &state) != nil {
				t.Fatal("invalid native message page")
			}
			seen := false
			for _, m := range state.Messages {
				if m.Role == "assistant" {
					var blocks []struct{ Type, Text string }
					if err := json.Unmarshal(m.Content, &blocks); err != nil {
						t.Fatal(err)
					}
					for _, b := range blocks {
						if b.Type == "text" && b.Text == want {
							seen = true
						}
					}
				}
			}
			if seen && !page.Running {
				return
			}
			if !page.Running && g.control.engine.Activity()[id].Phase == "error" {
				t.Fatalf("%s failed before the expected reply; requests=%d state=%s", want, calls.Load(), raw)
			}
			if ctx.Err() != nil {
				t.Fatalf("waiting for %s; requests=%d state=%s: %v", want, calls.Load(), raw, ctx.Err())
			}
		}
	}
	await(source.ID, "MODE_RESULT_2")
	created := run("new", "New from owner")
	newID := created["sessionId"].(string)
	if newID == source.ID || created["disposition"] != "created" {
		t.Fatalf("new mode: %+v", created)
	}
	row, err := store.Conversation(newID)
	if err != nil || row.Directory != source.Directory || row.Model != source.Model || row.Permission != source.Permission || row.ThinkingLevel != source.ThinkingLevel || row.FastMode != source.FastMode {
		t.Fatalf("new mode replaced owner authority: %+v %v", row, err)
	}
	await(newID, "MODE_RESULT_3")
	forked := run("fork", "Fork latest owner reply")
	forkID := forked["sessionId"].(string)
	if forkID == source.ID || forked["disposition"] != "forked" {
		t.Fatalf("fork mode: %+v", forked)
	}
	row, err = store.Conversation(forkID)
	if err != nil || row.Model != source.Model || row.Permission != source.Permission || row.ThinkingLevel != source.ThinkingLevel || row.FastMode != source.FastMode {
		t.Fatalf("fork changed owner authority: %+v %v", row, err)
	}
	await(forkID, "MODE_RESULT_4")
	if _, err = store.UpdateConversation(source.ID, func(row *chat.Conversation) error { row.Archived = true; return nil }); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]string{"GhostID": "fixture", "SourceSessionID": source.ID, "Mode": "continue", "Prompt": "Do not resurrect"})
	rejected, err := g.runLegacyAgent(ctx, raw)
	if err != nil || rejected.(map[string]any)["ok"] != false || calls.Load() != 4 {
		t.Fatalf("archived source was resurrected: %+v %v", rejected, err)
	}
}

func TestLegacyGhostErrandReturnsItsRealPiResultAfterRetirement(t *testing.T) {
	bun, worker := os.Getenv("WINONE_CHAT_BUN_EXE"), os.Getenv("WINONE_CHAT_WORKER")
	if bun == "" || worker == "" {
		t.Skip("explicit bundled Pi runtime fixture")
	}
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []struct{ Content json.RawMessage }
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		last := ""
		if len(request.Messages) > 0 {
			_ = json.Unmarshal(request.Messages[len(request.Messages)-1].Content, &last)
			if last == "" {
				var parts []struct{ Text string }
				_ = json.Unmarshal(request.Messages[len(request.Messages)-1].Content, &parts)
				for _, part := range parts {
					last += part.Text
				}
			}
		}
		answer := "FIRST_RESULT"
		if strings.Contains(last, "EMPTY_REPLY") {
			answer = ""
		}
		if strings.Contains(last, "LONG_REPLY") {
			answer = strings.Repeat("LONG_TEXT_", 8000)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":%q},\"finish_reason\":null}]}\n\n", answer)
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer model.Close()
	g, store := legacyGhostFixture(t, model.URL+"/v1")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	g.control.ctx = ctx
	g.control.engine = chat.NewService(ctx, store, chat.ServiceOptions{Bun: bun, Worker: worker, Configure: func(row chat.Conversation) (map[string]any, func(), error) {
		return map[string]any{"baseUrl": model.URL + "/v1", "key": "legacy-native-fixture-key", "agentDir": filepath.Join(g.control.directory, "agents", row.ID), "model": map[string]any{"id": "fixture/" + row.Model.Model, "name": "Fixture"}, "runtimeSettings": map[string]any{"toolPreset": "chat-only", "autoRetry": false}}, func() {}, nil
	}})
	defer func() {
		closeCtx, stop := context.WithTimeout(context.Background(), 8*time.Second)
		defer stop()
		if err := g.control.engine.Stop(closeCtx); err != nil {
			t.Error(err)
		}
	}()
	var id string
	for _, text := range []string{"FIRST_REPLY", "EMPTY_REPLY", "LONG_REPLY"} {
		started, err := g.startLegacyErrand(ctx, ghostLegacyErrandRequest{GhostID: "fixture", SessionKey: "results", Message: text, PermissionMode: "ask", TaskConfig: &ghostLegacyTaskConfig{}})
		if err != nil || started["ok"] != true {
			t.Fatalf("start: %+v %v", started, err)
		}
		id = started["sessionId"].(string)
		requestID := started["requestId"].(string)
		// Delay pickup until the exact input is terminal, then retire its Worker.
		// The waiter must consult the persisted receipt before waiting for events.
		for {
			if _, err := g.control.engine.Open(ctx, id, 0); err != nil {
				t.Fatal(err)
			}
			raw, err := g.control.engine.Command(ctx, id, "input.result", map[string]string{"id": requestID})
			g.control.engine.Release(id, 0)
			if err != nil {
				t.Fatal(err)
			}
			var result struct{ Status string }
			_ = json.Unmarshal(raw, &result)
			if result.Status != "" {
				break
			}
			select {
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			case <-time.After(20 * time.Millisecond):
			}
		}
		if err := g.control.engine.CloseSession(ctx, id); err != nil {
			t.Fatal(err)
		}
		result, err := g.waitLegacyErrand(ctx, id, requestID)
		if err != nil || result["ok"] != true {
			t.Fatalf("pickup: %+v %v", result, err)
		}
		want := "FIRST_RESULT"
		if text == "EMPTY_REPLY" {
			want = "(本轮没有可取回的文字回复)"
		}
		if text == "LONG_REPLY" {
			want = strings.Repeat("LONG_TEXT_", 8000)
		}
		if result["text"] != want {
			t.Fatalf("%s returned the wrong or silently truncated reply: %d chars", text, len(result["text"].(string)))
		}
	}
	if len(store.Snapshot().Conversations) != 1 {
		t.Fatal("result pickup or retirement created a replacement errand")
	}
}
