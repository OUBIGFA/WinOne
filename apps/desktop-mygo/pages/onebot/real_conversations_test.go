package onebot

import (
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

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/chat/model"
	"winone/desktopmygo/host"
	"winone/desktopmygo/i18n"
	"winone/internal/ai"
)

// This entry can be selected without repeating profile/template/browser checks
// that already passed against the same production native components.
func TestRealHostNativeConversationCombinations(t *testing.T) {
	p, tt, settle, setView := realOneBotUI(t)
	realConversationCombinations(t, p, tt, settle, setView)
}

// Continue the existing explicit profile matrix with real local conversations.
// Only the HTTP model output is deterministic. All native controls, receipts,
// permissions, question handling, Pi tools, topic and group stores are real.
func realConversationCombinations(t *testing.T, p *page, tt *ui.Tester, settle func(), setView func(func(*ui.Context))) {
	t.Helper()
	client, err := host.NewClient(os.Getenv("WINONE_NATIVE_UI_REAL_PIPE"))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Cancel()
	call := func(method string, params, out any) {
		t.Helper()
		if err := client.CallInto(method, params, out); err != nil {
			t.Fatalf("%s: %v", method, err)
		}
	}
	var original ai.Configuration
	call("capture.ai.configuration.get", nil, &original)
	var calls atomic.Int64
	videoSource := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer native-matrix-key" {
			t.Logf("fixture request %s %s rejected: expected dummy authorization=%v", r.Method, r.URL.Path, r.Header.Get("Authorization") == "Bearer native-matrix-key")
			http.Error(w, "unexpected fixture credential", 401)
			return
		}
		var input struct {
			Tools    []json.RawMessage
			Messages []struct {
				Role      string
				Content   json.RawMessage
				ToolCalls []struct{ Function struct{ Name string } } `json:"tool_calls"`
			}
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil || json.Unmarshal(raw, &input) != nil {
			t.Logf("fixture request %s %s has unsupported JSON shape (%d bytes)", r.Method, r.URL.Path, len(raw))
			http.Error(w, "invalid request", 400)
			return
		}
		t.Logf("fixture request %s %s tools=%d messages=%d", r.Method, r.URL.Path, len(input.Tools), len(input.Messages))
		lastUser, lastTool := "", ""
		for _, message := range input.Messages {
			if message.Role == "user" {
				lastUser = string(message.Content)
			}
			if len(message.ToolCalls) > 0 {
				lastTool = message.ToolCalls[len(message.ToolCalls)-1].Function.Name
			}
		}
		tool, text, args := "", "Native fixture reply", any(nil)
		switch {
		case len(input.Tools) == 0:
			text = "Native fixture title"
		case strings.Contains(lastUser, "GROUP_NATIVE_PERMISSION"):
			lastRole := ""
			if len(input.Messages) > 0 {
				lastRole = input.Messages[len(input.Messages)-1].Role
			}
			if lastRole != "tool" {
				tool = "write"
				args = map[string]string{"path": "native-group-proof.txt", "content": "NATIVE_GROUP_TOOL_EXECUTED"}
			} else if lastTool == "write" {
				tool = "ask_user_question"
				args = map[string]any{"questions": []any{map[string]any{"question": "Confirm native group result?", "options": []string{"Yes", "No"}}}}
			} else {
				text = "GROUP_NATIVE_REPLY"
			}
		case strings.Contains(lastUser, "TOPIC_NATIVE"):
			text = "TOPIC_NATIVE_REPLY"
		case strings.Contains(lastUser, "MAIN_NATIVE"):
			node, _ := json.Marshal(map[string]any{"version": 1, "seq": 1, "op": "create", "id": "video", "kind": "video", "props": map[string]string{"label": "Inline native video", "src": videoSource}})
			text = "MAIN_NATIVE_REPLY\n\n```cindy-ui\n" + string(node) + "\n```"
		}
		w.Header().Set("Content-Type", "text/event-stream")
		emit := func(delta any, finish any) {
			value, _ := json.Marshal(map[string]any{"id": "native-fixture", "object": "chat.completion.chunk", "model": "model", "choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}}})
			fmt.Fprintf(w, "data: %s\n\n", value)
		}
		emit(map[string]string{"role": "assistant"}, nil)
		if tool != "" {
			body, _ := json.Marshal(args)
			emit(map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "native-" + tool, "type": "function", "function": map[string]string{"name": tool, "arguments": string(body)}}}}, nil)
			emit(map[string]any{}, "tool_calls")
		} else {
			emit(map[string]string{"content": text}, nil)
			emit(map[string]any{}, "stop")
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	configuration := original
	configuration.Providers = []ai.Provider{{ID: "native-matrix", Name: "Native matrix", AuthMode: "api-key", Protocol: "chat", BaseURL: server.URL, Models: []string{"model"}, ModelMetadata: map[string]ai.ModelMetadata{"model": {ContextWindow: 200000}}}}
	configuration.Tasks = map[string]ai.Selection{"chat": {Provider: "native-matrix", Model: "model"}, "default": {Provider: "native-matrix", Model: "model"}}
	call("capture.ai.configuration.save", map[string]any{"configuration": configuration, "keys": map[string]string{"native-matrix": "native-matrix-key"}}, nil)
	defer func() {
		var latest ai.Configuration
		call("capture.ai.configuration.get", nil, &latest)
		original.Revision = latest.Revision
		call("capture.ai.configuration.save", map[string]any{"configuration": original}, nil)
	}()
	var alpha, beta Profile
	for index, result := range []*Profile{&alpha, &beta} {
		call("chat.onebot.profiles.create", map[string]any{"input": map[string]any{"name": fmt.Sprint("Conversation matrix ", index), "locale": "en", "capabilities": map[string]any{"permissions": "ask", "memory": false, "modelChainOverride": []any{map[string]any{"harness": "pi", "providerId": "native-matrix", "model": "model"}}}}}, result)
	}
	p.loadModels()
	p.refresh()
	settle()
	wait := func(label string, predicate func() bool) {
		t.Helper()
		deadline := time.Now().Add(20 * time.Second)
		for time.Now().Before(deadline) {
			settle()
			if predicate() {
				return
			}
			time.Sleep(30 * time.Millisecond)
		}
		if p.chatSession != "" {
			var diagnostic model.Snapshot
			if err := client.CallInto("chat.onebot.chat.state", map[string]string{"id": p.chatSession}, &diagnostic); err == nil {
				for _, message := range diagnostic.Messages {
					if message.ErrorMessage != "" {
						t.Logf("real Pi request failure: %s", message.ErrorMessage)
					}
				}
			}
		}
		t.Fatalf("%s did not complete after %d fixture requests: page=%q texts=%q", label, calls.Load(), p.error, tt.Texts())
	}
	var main struct{ ID, Directory string }
	groupOnly := os.Getenv("WINONE_NATIVE_UI_GROUP_ONLY") == "1"
	if !groupOnly {
		call("chat.onebot.session.open", map[string]string{"botId": alpha.ID}, &main)
		video, err := os.ReadFile(os.Getenv("WINONE_NATIVE_UI_VIDEO"))
		if err != nil {
			t.Fatal(err)
		}
		videoSource = filepath.Join(main.Directory, "native-inline.mp4")
		if err := os.WriteFile(videoSource, video, 0600); err != nil {
			t.Fatal(err)
		}
		p.showConversation(alpha.ID, main.ID)
		setView(p.chat.View)
		settle()
		if err := tt.Click(i18n.T("chat.message")); err != nil {
			t.Fatal(err)
		}
		tt.Type("MAIN_NATIVE")
		tt.Key(0, ui.KeyEnter)
		wait("actual native main reply", func() bool { return tt.HasText("MAIN_NATIVE_REPLY") && tt.HasText("Inline native video") })
		var state model.Snapshot
		call("chat.onebot.chat.state", map[string]string{"id": main.ID}, &state)
		root := ""
		for _, message := range state.Messages {
			if message.Role == "user" && strings.Contains(message.Text(), "MAIN_NATIVE") {
				root = message.RequestID
			}
		}
		if root == "" {
			t.Fatal("actual main input lost its native receipt")
		}
		if err := tt.Click("Inline native video"); err != nil {
			t.Fatal(err)
		}
		wait("inline media decoded through the real Host grant", func() bool { return tt.HasText(i18n.T("nativeWorkspace.mediaPlay")) })
		// The fixture has no audio. Hide it by switching to the real topic controller;
		// closing the owner later must release the lazily-owned decoder as well.
		p.openThread(alpha.ID, main.ID, root, "Native topic")
		settle()
		if p.topic == nil || p.topicSession == main.ID {
			t.Fatalf("native topic did not create its own owner: %q", p.topicError)
		}
		setView(p.topic.View)
		settle()
		if err := tt.Click(i18n.T("chat.message")); err != nil {
			t.Fatal(err)
		}
		tt.Type("TOPIC_NATIVE")
		tt.Key(0, ui.KeyEnter)
		wait("actual native topic reply", func() bool { return tt.HasText("TOPIC_NATIVE_REPLY") })
		call("chat.onebot.chat.state", map[string]string{"id": main.ID}, &state)
		for _, message := range state.Messages {
			if strings.Contains(message.Text(), "TOPIC_NATIVE_REPLY") {
				t.Fatal("topic reply contaminated the main conversation")
			}
		}
		setView(p.View)
		settle()
		realProfileShots(t, "main-and-topic", p.View)
	}
	p.closeContent()
	setView(p.View)
	settle()
	var created struct{ GroupID string }
	call("chat.onebot.groups.create", map[string]any{"input": map[string]any{"name": "Native conversation group", "botIds": []string{alpha.ID, beta.ID}}}, &created)
	call("chat.onebot.groups.update", map[string]any{"input": map[string]any{"groupId": created.GroupID, "replyMode": "mentioned"}}, nil)
	g := newGroupPane(p, created.GroupID)
	defer g.Close()
	p.group = g
	p.section = "group"
	setView(g.View)
	settle()
	file := filepath.Join(filepath.Dir(p.env.Directory), "native-group-attachment.txt")
	if err := os.WriteFile(file, []byte("Native group attachment"), 0600); err != nil {
		t.Fatal(err)
	}
	g.stagePaths([]string{file})
	settle()
	if len(g.draft.Attachments) != 1 || g.error != "" {
		t.Fatalf("native group upload: %+v %s", g.draft, g.error)
	}
	g.draft.Mentioned = []string{alpha.ID}
	if err := tt.Click(nt("groupMessage")); err != nil {
		t.Fatal(err)
	}
	tt.Type("GROUP_NATIVE_PERMISSION")
	if err := tt.Click(nt("send")); err != nil {
		t.Fatal(err)
	}
	wait("real group permission", func() bool { g.Refresh(false); return tt.HasText(i18n.T("chat.permissionTitle")) })
	groupSession := ""
	for _, speaker := range g.data.Round.Speakers {
		if speaker.BotID == alpha.ID {
			groupSession = speaker.SessionID
		}
	}
	if groupSession == "" {
		t.Fatal("group permission lost its owning session")
	}
	if err := tt.Click(i18n.T("chat.allowOnce") + "  ↵"); err != nil {
		t.Fatal(err)
	}
	wait("real group question", func() bool { g.Refresh(false); return tt.HasText("Confirm native group result?") })
	if err := tt.Click("Yes"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click(i18n.T("chat.questionSubmit")); err != nil {
		t.Fatal(err)
	}
	wait("real group completion", func() bool { g.Refresh(false); return tt.HasText("GROUP_NATIVE_REPLY") })
	found := false
	for _, message := range g.data.Messages {
		found = found || message.AuthorKind == "bot" && message.Content == "GROUP_NATIVE_REPLY"
	}
	if !found || len(g.prompts) != 0 {
		t.Fatal("group did not release its real waiting prompt after completion")
	}
	// Verify the write in the exact session that owned the inline permission,
	// rather than treating the model's completion prose as execution evidence.
	var catalog struct {
		Conversations []struct{ ID, Directory string }
	}
	call("chat.onebot.chat.catalog", nil, &catalog)
	proof := false
	for _, row := range catalog.Conversations {
		if row.ID == groupSession {
			data, err := os.ReadFile(filepath.Join(row.Directory, "native-group-proof.txt"))
			proof = err == nil && string(data) == "NATIVE_GROUP_TOOL_EXECUTED"
		}
	}
	if !proof {
		t.Fatal("group approval did not execute the real Pi write")
	}
	setView(p.View)
	settle()
	realProfileShots(t, "group-conversation", p.View)
	g.Close()
	p.group = nil
	p.section = "home"
	setView(p.View)
	settle()
	if !groupOnly {
		var receipts []struct {
			SessionID, RootClientID string
			ReplyCount              int
		}
		call("chat.onebot.dmThread.list", map[string]string{"botId": alpha.ID, "rootSessionId": main.ID}, &receipts)
		if len(receipts) != 1 || receipts[0].ReplyCount < 1 {
			t.Fatal("real topic summary was not persisted")
		}
	}
	t.Logf("real native group/attachment/permission/question/Pi-write combinations (groupOnly=%v); %d actual gateway requests", groupOnly, calls.Load())
}
