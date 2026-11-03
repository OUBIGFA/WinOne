package chat

import (
	"encoding/json"
	"fmt"
	"image/png"
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
}

func newFakeChat(t *testing.T) *fakeChat {
	return &fakeChat{t: t, catalog: Catalog{
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
