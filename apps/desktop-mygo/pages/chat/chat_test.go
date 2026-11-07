package chat

import (
	"encoding/json"
	"fmt"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/chat/model"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/testhost"
	"winone/desktopmygo/widgets"
)

// fakeChat is the Host's Chat service as the page sees it: a catalog,
// one conversation's snapshot and the events each poll returns.
type fakeChat struct {
	mu       sync.Mutex
	t        *testing.T
	catalog  Catalog
	snapshot model.Snapshot
	// pages are the poll pages to return in order; the last repeats.
	pages   []map[string]any
	polls   int
	calls   []string
	prompts []map[string]any
	drafts  []map[string]any
	queue   []map[string]any
	answers []map[string]any
	created int
	export  map[string]any
	// settings is what chat.settings.save received last.
	settings     map[string]any
	connections  map[string]any
	previewURL   string
	fileText     string
	fileRevision int
}

func newFakeChat(t *testing.T) *fakeChat {
	f := &fakeChat{t: t, fileText: "Native document body", fileRevision: 1, catalog: Catalog{
		Projects: []Project{{ID: "p1", Name: "WinOne", Path: `E:\WinOne`, Permission: model.PermissionAsk}},
		Conversations: []Conversation{
			{ID: "c1", ProjectID: "p1", Title: "Fix the build", Directory: `E:\WinOne`, Model: aiSelection("one", "gpt-a"), Permission: model.PermissionAsk, HasMessages: true, UpdatedAt: "2026-11-01T10:00:00Z"},
			{ID: "c2", Title: "Plan a trip", Directory: `C:\Users\me`, Model: aiSelection("one", "gpt-a"), Permission: model.PermissionAsk, Pinned: true, UpdatedAt: "2026-11-01T09:00:00Z"},
		}},
		snapshot: model.Snapshot{Messages: []model.Message{
			{ID: "u1", Role: "user", Content: model.TextContent("Why does the build fail?"), TurnID: "u1"},
			{ID: "a1", Role: "assistant", TurnID: "u1", Content: model.Content{Parts: []model.Part{
				{Type: "thinking", Thinking: "Read the log first."},
				{Type: "toolCall", ID: "t1", Name: "read", Arguments: map[string]any{"path": "build.log"}}}}},
			{ID: "r1", Role: "toolResult", TurnID: "u1", ToolCallID: "t1", ToolName: "read", Content: model.TextContent("error: missing icon")},
			{ID: "a2", Role: "assistant", TurnID: "u1", Content: model.TextContent("The **icon** is missing. Run `node build/generate-mygo-icons.mjs`.")},
		}},
		pages: []map[string]any{{"events": []any{}, "cursor": 0, "running": false, "status": "idle", "approvals": []any{}}},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		fmt.Fprint(w, f.fileText)
	}))
	f.previewURL = server.URL
	t.Cleanup(server.Close)
	return f
}

func aiSelection(provider, model string) widgets.AISelection {
	return widgets.AISelection{Provider: provider, Model: model}
}

func (f *fakeChat) handle(method string, params json.RawMessage) (any, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var p map[string]any
	_ = json.Unmarshal(params, &p)
	f.calls = append(f.calls, method)
	switch method {
	case "chat.ghosts.reply-overrides":
		return []any{}, nil
	case "chat.ghosts.session-focus":
		return true, nil
	case "chat.ghosts.candidates":
		return []any{}, nil
	case "chat.catalog":
		return f.catalog, nil
	case "capture.ai.configuration.get":
		return map[string]any{"providers": []any{map[string]any{"id": "one", "name": "Relay", "models": []any{"gpt-a", "gpt-b"}}},
			"tasks": map[string]any{"chat": map[string]any{"provider": "one", "model": "gpt-a"}}}, nil
	case "chat.activity":
		return map[string]any{"c1": map[string]any{"phase": "idle", "attention": false}}, nil
	case "chat.open", "chat.state":
		return f.snapshot, nil
	case "chat.draft.get":
		return map[string]any{"text": ""}, nil
	case "chat.draft.save":
		f.drafts = append(f.drafts, p)
		return nil, nil
	case "chat.poll":
		page := f.pages[min(f.polls, len(f.pages)-1)]
		f.polls++
		return page, nil
	case "chat.ghosts.recommendations":
		return json.RawMessage("[]"), nil
	case "chat.prompt":
		f.prompts = append(f.prompts, p)
		return map[string]any{"accepted": true}, nil
	case "chat.queue":
		f.queue = append(f.queue, p)
		return f.snapshot, nil
	case "chat.release", "chat.stop", "chat.compact", "chat.pause", "chat.resume", "chat.title.generate":
		return nil, nil
	case "chat.permission", "chat.question":
		f.answers = append(f.answers, p)
		return nil, nil
	case "chat.conversation.new":
		f.created++
		c := Conversation{ID: "new1", Title: "Chat", Directory: `C:\Users\me`, Model: aiSelection("one", "gpt-a"), Permission: model.PermissionAsk}
		f.catalog.Conversations = append(f.catalog.Conversations, c)
		f.snapshot = model.Snapshot{}
		return c, nil
	case "chat.file.preview", "chat.file.save":
		if method == "chat.file.save" {
			if p["revision"] != fmt.Sprint(f.fileRevision) {
				return nil, fmt.Errorf("document changed")
			}
			f.fileText, _ = p["text"].(string)
			f.fileRevision++
		}
		path, _ := p["path"].(string)
		return map[string]any{"path": path, "name": filepath.Base(path), "kind": "text", "mimeType": "text/plain", "url": f.previewURL, "revision": fmt.Sprint(f.fileRevision), "size": len(f.fileText)}, nil
	case "chat.review.snapshot":
		return map[string]any{"root": `E:\WinOne`, "scope": "working", "revision": "review-one", "files": []any{}}, nil
	case "chat.review.metadata":
		return map[string]any{"repository": false}, nil
	case "chat.files.list":
		return map[string]any{"entries": []any{}}, nil
	case "chat.resources":
		return map[string]any{"agents": []any{}, "skills": []any{}, "plugins": []any{}, "mcpServers": []any{}}, nil
	case "chat.settings.get":
		if f.settings != nil {
			return f.settings, nil
		}
		return map[string]any{"workspaceDirectory": `C:\Users\me\workspaces`, "titleModel": map[string]any{"provider": "", "model": ""},
			"skillDirectories": []any{}, "pluginDirectories": []any{},
			"mcpServers":  []any{map[string]any{"name": "jira", "enabled": true, "command": "jira-mcp"}},
			"automations": []any{map[string]any{"id": "a1", "name": "Daily", "prompt": "Summarize"}}}, nil
	case "chat.settings.save":
		f.settings, _ = p["settings"].(map[string]any)
		return f.settings, nil
	case "chat.automations.connections.get":
		return map[string]any{"settings": map[string]any{"methods": map[string]any{}}, "servers": []any{map[string]any{"name": "jira", "enabled": true}}}, nil
	case "chat.automations.connections.save":
		f.connections = p
		return nil, nil
	case "chat.export":
		f.export = p
		if p["excludeMedia"] == true {
			return map[string]any{"status": "ok", "filePath": p["targetPath"], "fidelity": "partial", "mediaExcluded": 3}, nil
		}
		return map[string]any{"status": "oversize", "totalBytes": 300 << 20, "mediaBytes": 200 << 20, "limitBytes": 256 << 20}, nil
	}
	f.t.Errorf("unexpected %s %s", method, params)
	return nil, nil
}

func (f *fakeChat) called(method string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, m := range f.calls {
		if m == method {
			n++
		}
	}
	return n
}

// ticks collects the page's scheduled reads, for the test to run.
type ticks struct{ due []func() }

func (k *ticks) after(_ time.Duration, fn func()) func() {
	k.due = append(k.due, fn)
	return func() {}
}

// run runs the reads scheduled so far.
func (k *ticks) run() {
	due := k.due
	k.due = nil
	for _, fn := range due {
		fn()
	}
}

// dark draws the tests' pages in the dark theme, for the screenshots.
var dark bool

func setup(t *testing.T) (*fakeChat, *testhost.Harness, *Chat, *ui.Tester, *ticks) {
	f := newFakeChat(t)
	h, p, tt, k := setupHost(t, f.handle)
	return f, h, p, tt, k
}

// setupWith draws the page on handler, a Host that extends f's.
func setupWith(t *testing.T, f *fakeChat, handler testhost.Handler) (*testhost.Harness, *Chat, *ui.Tester) {
	h, p, tt, _ := setupHost(t, handler)
	return h, p, tt
}

func setupHost(t *testing.T, handler testhost.Handler) (*testhost.Harness, *Chat, *ui.Tester, *ticks) {
	i18n.SetLocale(i18n.Chinese)
	h := testhost.New(t, handler)
	p := New(h.Env)
	k := &ticks{}
	p.after = k.after
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(kit.Theme(c.Preferences(), dark))
		p.View(c)
	}, 1100, 760)
	settle(h, tt)
	t.Cleanup(p.Close)
	return h, p, tt, k
}

func settle(h *testhost.Harness, tt *ui.Tester) {
	for range 3 {
		h.Settle(tt)
	}
}

// shot saves what the tester shows when WINONE_SHOTS names a folder.
func shot(t *testing.T, tt *ui.Tester, name string) {
	dir := os.Getenv("WINONE_SHOTS")
	if dir == "" {
		return
	}
	file, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, tt.Image()); err != nil {
		t.Fatal(err)
	}
}

func open(t *testing.T, h *testhost.Harness, p *Chat, tt *ui.Tester, id string) {
	t.Helper()
	p.run(func(done func(error)) { p.choose(id, nil, done) })
	settle(h, tt)
	if p.activeID != id || p.opening {
		t.Fatalf("opened %q opening %v err %q", p.activeID, p.opening, p.err)
	}
}

func TestTheListShowsProjectsAndPinnedConversations(t *testing.T) {
	_, _, _, tt, _ := setup(t)
	shot(t, tt, "chat-welcome")
	for _, text := range []string{"Fix the build", "Plan a trip", "WinOne", i18n.T("chat.pinned")} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %q", text, tt.Texts())
		}
	}
}

func TestOpeningAConversationShowsItsTurnsAndFollowsItsEvents(t *testing.T) {
	f, h, p, tt, k := setup(t)
	open(t, h, p, tt, "c1")
	if f.called("chat.draft.get") != 1 || f.called("chat.open") != 1 {
		t.Fatalf("calls %v", f.calls)
	}
	if !tt.HasText("Why does the build fail?") || !tt.HasText(i18n.T("chat.processWorked", "duration", "0s")) {
		t.Fatalf("texts %q", tt.Texts())
	}
	shot(t, tt, "chat-conversation")
	// The next poll streams a new reply.
	f.mu.Lock()
	f.pages = []map[string]any{{"events": []any{
		map[string]any{"sequence": 1, "data": map[string]any{"type": "pi_event", "eventType": "message_start", "message": map[string]any{"id": "u2", "requestId": "u2", "turnId": "u2", "role": "user", "content": "And now?"}}},
		map[string]any{"sequence": 2, "data": map[string]any{"type": "delta", "kind": "text_delta", "turnId": "u2", "timestamp": 5, "text": "Streaming answer"}},
	}, "cursor": 2, "running": true, "status": "running", "approvals": []any{}}}
	f.mu.Unlock()
	k.run()
	settle(h, tt)
	if !tt.HasText("And now?") || !p.view.Running || p.session.cursor != 2 {
		t.Fatalf("after poll running %v cursor %v texts %q", p.view.Running, p.session.cursor, tt.Texts())
	}
}

func TestSendingClearsTheDraftAndSendsOneReceipt(t *testing.T) {
	f, h, p, tt, _ := setup(t)
	open(t, h, p, tt, "c1")
	p.updateText("Please fix it", nil, "c1")
	settle(h, tt)
	p.requestSend("")
	settle(h, tt)
	if len(f.prompts) != 1 {
		t.Fatalf("prompts %v err %q", f.prompts, p.err)
	}
	prompt := f.prompts[0]
	if prompt["text"] != "Please fix it" || prompt["deliveryMode"] != "queue" || prompt["requestId"] == "" || prompt["id"] != "c1" {
		t.Fatalf("prompt %v", prompt)
	}
	if p.text != "" || p.draftDirty {
		t.Fatalf("draft %q dirty %v", p.text, p.draftDirty)
	}
	found := 0
	for _, m := range p.view.Messages {
		if m.RequestID == prompt["requestId"] {
			found++
		}
	}
	if found != 1 || p.awaitingInput != prompt["requestId"] || !p.view.Running {
		t.Fatalf("optimistic rows %d awaiting %q running %v", found, p.awaitingInput, p.view.Running)
	}
}

func TestARunningTurnAsksWhetherToQueueOrSteer(t *testing.T) {
	f, h, p, tt, _ := setup(t)
	open(t, h, p, tt, "c1")
	p.view.Running = true
	p.updateText("One more thing", nil, "c1")
	settle(h, tt)
	p.requestSend("")
	settle(h, tt)
	if p.queuedSend == nil || len(f.prompts) != 0 {
		t.Fatalf("no confirmation: %v", f.prompts)
	}
	if !tt.HasText(i18n.T("chat.confirmFollowup")) {
		t.Fatalf("texts %q", tt.Texts())
	}
	shot(t, tt, "chat-queue-confirm")
	if err := tt.Click(i18n.T("chat.steerSend")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if len(f.prompts) != 1 || f.prompts[0]["deliveryMode"] != "steer" {
		t.Fatalf("prompts %v", f.prompts)
	}
}

func TestANewChatIsCreatedOnItsFirstMessage(t *testing.T) {
	f, h, p, tt, _ := setup(t)
	p.updateText("Hello", nil, "")
	p.requestSend("")
	settle(h, tt)
	if f.created != 1 || len(f.prompts) != 1 || f.prompts[0]["id"] != "new1" || p.activeID != "new1" {
		t.Fatalf("created %d prompts %v active %q err %q", f.created, f.prompts, p.activeID, p.err)
	}
}

func TestApprovalsAnswerWithTheirKeys(t *testing.T) {
	f, h, p, tt, _ := setup(t)
	open(t, h, p, tt, "c1")
	p.view.Approvals = []model.Approval{{ID: "ap1", Tool: "powershell", Args: map[string]any{"command": "npm run build"}, ScopeLabel: "npm run *"}}
	settle(h, tt)
	shot(t, tt, "chat-approval")
	tt.Key(ui.Ctrl, ui.KeyEnter)
	settle(h, tt)
	if len(f.answers) != 1 || f.answers[0]["allow"] != true || f.answers[0]["scope"] != "session" || len(p.view.Approvals) != 0 {
		t.Fatalf("answers %v approvals %v", f.answers, p.view.Approvals)
	}
}

func TestNavigationWaitsForDraftSaves(t *testing.T) {
	f, h, p, tt, _ := setup(t)
	open(t, h, p, tt, "c1")
	p.updateText("draft one", nil, "c1")
	p.updateText("draft two", nil, "c1")
	p.run(func(done func(error)) { p.choose("c2", nil, done) })
	settle(h, tt)
	if len(f.drafts) != 2 || f.drafts[1]["draft"].(map[string]any)["text"] != "draft two" || p.activeID != "c2" {
		t.Fatalf("drafts %v active %q", f.drafts, p.activeID)
	}
	if f.called("chat.release") != 1 {
		t.Fatalf("release %d", f.called("chat.release"))
	}
}

func TestExportRetriesWithoutMediaWhenTooLarge(t *testing.T) {
	f, h, p, tt, _ := setup(t)
	h.Picked[i18n.T("chat.exportTitle")] = `C:\out\task.cshare`
	target := f.catalog.Conversations[0]
	p.ui.exportOf, p.ui.exportState = &target, exportState{}
	settle(h, tt)
	if err := tt.Click(i18n.T("chat.exportConfirm")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if !p.ui.exportState.excludeMedia || p.ui.exportOf == nil {
		t.Fatalf("export state %+v", p.ui.exportState)
	}
	if err := tt.Click(i18n.T("chat.exportWithoutMedia")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if f.export["excludeMedia"] != true || f.export["targetPath"] != `C:\out\task.cshare` || p.ui.exportOf != nil || p.notice == "" {
		t.Fatalf("export %v notice %q err %q", f.export, p.notice, p.ui.exportState.err)
	}
}

// TestShots draws the page's states in both themes, both languages and
// two widths when WINONE_SHOTS names a folder, as visual evidence.
func TestShots(t *testing.T) {
	if os.Getenv("WINONE_SHOTS") == "" {
		t.Skip("WINONE_SHOTS is not set")
	}
	defer func() { dark = false; i18n.SetLocale(i18n.Chinese) }()
	for _, theme := range []bool{false, true} {
		for _, locale := range []string{i18n.Chinese, i18n.English} {
			for _, width := range []int{1100, 640} {
				dark = theme
				_, h, p, tt, _ := setup(t)
				i18n.SetLocale(locale)
				tt.SetSize(width, 760)
				name := fmt.Sprintf("%s-%s-%d", map[bool]string{false: "light", true: "dark"}[theme], locale, width)
				settle(h, tt)
				shot(t, tt, "welcome-"+name)
				open(t, h, p, tt, "c1")
				p.ui.expanded = map[string]bool{"process:u1": true, "thinking:a1:0": true, "tool:t1": true}
				settle(h, tt)
				shot(t, tt, "conversation-"+name)
				p.view.Approvals = []model.Approval{{Kind: "question", ID: "q1", Questions: []model.Question{{Question: "Which shell?", Options: []string{"PowerShell", "cmd"}}}}}
				settle(h, tt)
				shot(t, tt, "question-"+name)
			}
		}
	}
}

func TestLinksFromOtherPagesOpenTheirConversationAndFile(t *testing.T) {
	_, h, p, tt, _ := setup(t)
	h.Env.Open("chat", core.Intent{Conversation: "c1", File: &core.IntentFile{Path: `E:\WinOne\notes.md`, Line: 3}})
	settle(h, tt)
	if p.activeID != "c1" || !p.ui.details || p.ui.panelRequest == nil || p.ui.panelRequest.File != `E:\WinOne\notes.md` || p.ui.panelRequest.Line != 3 {
		t.Fatalf("active %q details %v request %+v err %q", p.activeID, p.ui.details, p.ui.panelRequest, p.err)
	}
	h.Env.Open("chat", core.Intent{Conversation: "missing"})
	settle(h, tt)
	if p.activeID != "c1" || p.err != i18n.T("chat.requestedConversationMissing") {
		t.Fatalf("active %q err %q", p.activeID, p.err)
	}
	h.Env.Open("chat", core.Intent{ChatSettings: true})
	settle(h, tt)
	if !p.tools.settingsOpen {
		t.Fatal("Chat settings did not open")
	}
}

// An automation's error diagnosis opens a new conversation with its prompt
// after the unsent draft there, as Cindy prefills a new task; nothing is sent.
func TestADiagnosisPrefillsANewConversation(t *testing.T) {
	_, h, p, tt, _ := setup(t)
	h.Env.Open("chat", core.Intent{Conversation: "c1"})
	settle(h, tt)
	p.newDraft = "My own note"
	h.Env.Open("chat", core.Intent{NewChat: true, Prefill: "Diagnose this failure"})
	settle(h, tt)
	if p.activeID != "" || p.text != "My own note\n\nDiagnose this failure" || p.newDraft != p.text || !p.draftDirty {
		t.Fatalf("active %q text %q new %q dirty %v err %q", p.activeID, p.text, p.newDraft, p.draftDirty, p.err)
	}
}

// A subagent's own messages open from the agents menu: its conversation,
// an older page above it, and a follow-up that reads it again.
func TestSubagentMessagesReadPagesAndFollowUp(t *testing.T) {
	f := newFakeChat(t)
	f.snapshot.Subagents = []model.Subagent{{Name: "scout", Status: "completed", Model: "gpt-a"}}
	var reads, followUps []map[string]any
	h, p, tt := setupWith(t, f, func(method string, params json.RawMessage) (any, error) {
		if method != "chat.agents" {
			return f.handle(method, params)
		}
		var args map[string]any
		_ = json.Unmarshal(params, &args)
		if args["action"] == "follow_up" {
			followUps = append(followUps, args)
			return map[string]any{"accepted": true}, nil
		}
		reads = append(reads, args)
		if args["agentCursor"] != nil {
			return map[string]any{"entries": []any{map[string]any{"id": 1, "model": []any{map[string]any{"role": "user", "content": "Map the build scripts"}}}}, "cursor": nil}, nil
		}
		return map[string]any{"entries": []any{map[string]any{"id": 2, "model": []any{
			map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "toolCall", "id": "t1", "name": "grep", "arguments": map[string]any{}}}},
			map[string]any{"role": "assistant", "content": "Found **three** scripts."}}}}, "cursor": 7}, nil
	})
	open(t, h, p, tt, "c1")
	p.openAgent("scout")
	settle(h, tt)
	if len(reads) != 1 || !tt.HasText("Found three scripts.") || !tt.HasText(i18n.T("chat.tool")+" · grep") {
		t.Fatalf("reads %v texts %q", reads, tt.Texts())
	}
	if err := tt.Click(i18n.T("chat.loadOlder")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if len(reads) != 2 || reads[1]["agentCursor"] != 7.0 || !tt.HasText("Map the build scripts") || !tt.HasText("Found three scripts.") {
		t.Fatalf("older reads %v texts %q", reads, tt.Texts())
	}
	shot(t, tt, "chat-subagent")
	p.ui.agents.drafts["scout"] = "Also check the tests"
	settle(h, tt)
	if err := tt.Click(i18n.T("chat.agentFollowUp")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if len(followUps) != 1 || followUps[0]["text"] != "Also check the tests" || followUps[0]["name"] != "scout" || len(reads) != 3 || p.ui.agents.drafts["scout"] != "" {
		t.Fatalf("follow-ups %v reads %d draft %q", followUps, len(reads), p.ui.agents.drafts["scout"])
	}
}

// Chat settings open as a native dialog over Chat: the conversation list
// stays, the tabs switch, a change is saved with the automations the
// dialog does not edit, and malformed MCP JSON keeps it from saving.
// The scheduler is now a native view reached from Chat, even with no WebView
// service in the test window. Its actual Host list/history are rendered there.
func TestAutomationsOpenNativelyFromTheChatSidebar(t *testing.T) {
	f := newFakeChat(t)
	h, p, tt := setupWith(t, f, func(method string, params json.RawMessage) (any, error) {
		switch method {
		case "chat.automations.list":
			return []any{map[string]any{"id": "schedule-one", "name": "Native automation", "prompt": "Inspect", "agentKind": "pi", "kind": "cron", "cronExpr": "0 9 * * *", "timezone": "UTC", "status": "active"}}, nil
		case "chat.automations.runtime":
			return map[string]any{"slotsInUse": 0, "maxConcurrentRuns": 2, "timezone": "UTC"}, nil
		case "chat.automations.runs":
			return []any{map[string]any{"id": "run-one", "scheduleId": "schedule-one", "status": "success", "resultText": "Native run result"}}, nil
		case "chat.automations.channels":
			return map[string]any{"feishu": map[string]string{"status": "disconnected"}, "wecomGroup": map[string]bool{"configured": false, "enabled": false}}, nil
		case "chat.automations.capabilityStatus":
			return map[string]any{"statuses": []any{}}, nil
		case "chat.automations.project.pendingImports":
			return []any{}, nil
		case "chat.automations.events":
			return map[string]any{"cursor": 0, "events": []any{}}, nil
		}
		return f.handle(method, params)
	})
	if err := tt.Click(i18n.T("chat.automations")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if p.tools.automations == nil || !tt.HasText("Native automation") || !tt.HasText("Native run result") {
		t.Fatalf("automation did not render natively: %q", tt.Texts())
	}
	if err := tt.Click(i18n.T("nativeAutomation.create")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if err := tt.Click(i18n.T("nativeAutomation.name")); err != nil {
		t.Fatal(err)
	}
	tt.Type("Unsaved automation")
	settle(h, tt)
	if !p.tools.automations.Dirty() {
		t.Fatal("typing into native form did not create a protected draft")
	}
	var pending func()
	h.Env.RequestChange = func(action func()) { pending = action }
	p.leaveAutomation(func() {})
	if pending == nil || !p.ui.automations {
		t.Fatal("leaving bypassed the native unsaved guard")
	}
	p.tools.automations.Discard()
	pending()
	tt.Frame()
	if p.ui.automations {
		t.Fatal("confirmed navigation did not leave automations")
	}
}

func TestSettingsDialogIsNativeAndSavesTheDraft(t *testing.T) {
	f, h, p, tt, _ := setup(t)
	if err := tt.Click("Chat 设置"); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	shot(t, tt, "chat-settings")
	if !p.tools.settingsOpen || p.settings.value == nil || !tt.HasText("默认工作目录") || !tt.HasText("Fix the build") {
		t.Fatalf("settings did not open over Chat: %+v err %q", p.settings, p.settings.err)
	}
	// The larger type keeps this long form scrollable. Exercise the actual
	// scroll path before changing a field below the first viewport.
	tt.Scroll(700, 400, 0, 260)
	settle(h, tt)
	if err := tt.Click("自动重试"); err != nil {
		t.Fatal(err)
	}
	if !p.settings.dirty() || !tt.HasText("尚有未保存设置") {
		t.Fatal("a change is not reported as unsaved")
	}
	if err := tt.Click("MCP"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("自动化连接器") || !tt.HasText("jira") {
		t.Fatal("the MCP tab is missing its servers or connectors")
	}
	shot(t, tt, "chat-settings-mcp")
	p.settings.maps = map[int]map[string]string{0: {"env": "{"}}
	p.settings.advanced = map[int]bool{0: true}
	tt.Frame()
	if !p.settings.invalidMaps() || !tt.HasText(i18n.T("chat.mcpMapInvalid")) {
		t.Fatal("malformed JSON is not reported")
	}
	p.settings.maps = nil
	tt.Frame()
	if err := tt.Click("保存设置"); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if p.tools.settingsOpen || f.settings == nil || f.settings["autoRetry"] != false {
		t.Fatalf("saved %+v open %v err %q", f.settings, p.tools.settingsOpen, p.settings.err)
	}
	if automations, _ := f.settings["automations"].([]any); len(automations) != 1 {
		t.Fatalf("automations were not kept: %+v", f.settings["automations"])
	}
	if _, ok := f.settings["titleModel"]; ok {
		t.Fatal("following the session model saved an empty model")
	}
}
