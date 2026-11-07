package chat

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"winone/desktopmygo/chat/model"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/widgets"
)

// Use real wheel notches, not Tester.Scroll (which models a touchpad and
// cannot reproduce canceling MyGo's 150ms wheel animation).
func TestWheelCanLeaveFollowedTranscript(t *testing.T) {
	f := workedTurn(t)
	f.snapshot.Messages = []model.Message{
		{ID: "u1", Role: "user", Content: model.TextContent("Question")},
		{ID: "a1", Role: "assistant", TurnID: "u1", Content: model.TextContent(strings.Repeat("A paragraph in the answer.\n\n", 80))},
	}
	h, p, tt := setupItems(t, f)
	p.ui.followLatest()
	settle(h, tt)
	start := p.ui.transcript.Y
	b, _ := tt.Find(i18n.T("chat.readingArea"))
	if start < 400 {
		t.Fatalf("not at a long transcript's end: %v", start)
	}
	tt.Wheel(b.X+100, b.Y+100, 0, -120)
	tt.Advance(50 * time.Millisecond)
	middle := p.ui.transcript.Y
	if middle >= start || middle <= start-120 {
		t.Fatalf("wheel did not glide: %v -> %v", start, middle)
	}
	tt.Wheel(b.X+100, b.Y+100, 0, -120)
	tt.Advance(time.Second)
	if got := p.ui.transcript.Y; got > start-239 || got < start-241 || p.ui.follow {
		t.Fatalf("notches canceled or snapped back: start=%v got=%v follow=%v", start, got, p.ui.follow)
	}
	p.ui.contentChanged()
	settle(h, tt)
	if p.ui.transcript.Y > start-239 {
		t.Fatal("content update stole the reader's position")
	}
}

func TestLongProcessHasVisibleCollapseControl(t *testing.T) {
	f := workedTurn(t)
	f.snapshot.Messages[1].Content.Parts = []model.Part{{Type: "thinking", Thinking: strings.Repeat("Working thought.\n\n", 70)}, {Type: "toolCall", ID: "t1", Name: "read", Arguments: map[string]any{"path": "build.log"}}}
	h, p, tt := setupItems(t, f)
	// Public commentary is a reply, not long work. Expand a real thought to
	// exercise the process-owned collapse control rather than hiding prose.
	p.ui.expanded["thinking:a1:0"] = true
	p.ui.follow, p.ui.pinNext = false, false
	p.ui.transcript.Y = 240
	settle(h, tt)
	button, ok := tt.Find(i18n.T("chat.collapseProcess"))
	viewport, _ := tt.Find(i18n.T("chat.readingArea"))
	if !ok || button.Y < viewport.Y || button.Y+button.H > viewport.Y+viewport.H {
		t.Fatalf("collapse is not visible: %+v in %+v", button, viewport)
	}
	shot(t, tt, "long-process-collapse")
	if err := tt.Click(i18n.T("chat.collapseProcess")); err != nil {
		t.Fatal(err)
	}
	settle(h, tt)
	if p.ui.expanded["process:u1"] {
		t.Fatal("process did not collapse")
	}
	if p.ui.transcript.Y >= 240 || p.ui.follow {
		t.Fatal("collapse left the reader below the folded process")
	}
}

func TestModelFallbackValidatesDefaultAndEmptyCatalog(t *testing.T) {
	cfg := &AIConfiguration{Providers: []widgets.AIProvider{{ID: "empty"}, {ID: "first", Models: []string{"a", "b"}}, {ID: "default", Models: []string{"c"}}}, Tasks: map[string]*widgets.AISelection{"default": {Provider: "default", Model: "c"}, "chat": {Provider: "removed", Model: "old"}}}
	p := &Chat{ai: cfg}
	p.newModel = aiSelection("removed", "old")
	if got := p.currentModel(); got != aiSelection("default", "c") {
		t.Fatalf("default: %+v", got)
	}
	cfg.Tasks["default"].Model = "deleted"
	if got := p.currentModel(); got != aiSelection("first", "a") {
		t.Fatalf("first: %+v", got)
	}
	p.newModel = aiSelection("first", "b")
	if got := p.currentModel(); got != p.newModel {
		t.Fatalf("valid choice replaced: %+v", got)
	}
	cfg.Providers = nil
	if got := p.currentModel(); got != (widgets.AISelection{}) {
		t.Fatalf("retained unavailable model: %+v", got)
	}
}

func TestSendingWithDeletedModelConfiguresFallbackBeforePrompt(t *testing.T) {
	f := newFakeChat(t)
	f.catalog.Conversations[0].Model = aiSelection("removed", "gone")
	configured := false
	handler := func(method string, params json.RawMessage) (any, error) {
		if method == "chat.configure" {
			var request struct {
				Model widgets.AISelection `json:"model"`
			}
			if err := json.Unmarshal(params, &request); err != nil {
				return nil, err
			}
			if request.Model != aiSelection("one", "gpt-a") {
				t.Errorf("configured %+v", request.Model)
			}
			f.mu.Lock()
			f.catalog.Conversations[0].Model = request.Model
			updated := f.catalog.Conversations[0]
			configured = true
			f.mu.Unlock()
			return updated, nil
		}
		if method == "chat.prompt" {
			f.mu.Lock()
			before := configured
			f.mu.Unlock()
			if !before {
				t.Error("prompt reached the deleted runtime model")
			}
		}
		return f.handle(method, params)
	}
	h, p, tt := setupWith(t, f, handler)
	open(t, h, p, tt, "c1")
	p.text = "Use the available model"
	p.requestSend("")
	settle(h, tt)
	if len(f.prompts) != 1 || !configured || p.conversation().Model != aiSelection("one", "gpt-a") {
		t.Fatalf("fallback did not reach send: configured=%v prompts=%v model=%+v error=%s", configured, f.prompts, p.conversation().Model, p.err)
	}
}
