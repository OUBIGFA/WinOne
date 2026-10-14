package resultui

import (
	"context"
	"encoding/json"
	"github.com/egoist/mygo/ui"
	"testing"
	"winone/internal/ai"
	"winone/internal/resultwindow"
)

func TestQuickWindowNeverRestoresClosedContent(t *testing.T) {
	old := json.RawMessage(`{"Version":1,"Prompt":"old draft","Input":"old selection","History":[{"role":"user","content":"old question"}],"Messages":[{"Role":"assistant","Text":"old reply"}]}`)
	c := NewController(nil, resultwindow.Initial{Mode: "chat", Source: "quick-assistant", State: old, Text: "new selection"})
	if c.Panel.Prompt != "new selection" || c.quickInput != "new selection" || len(c.history) != 0 || len(c.Panel.Messages) != 0 || len(c.Initial.State) != 0 {
		t.Fatal("closed content was restored")
	}
	c.Panel.Original = "old source"
	c.Panel.Result = "old result"
	c.Panel.Prompt = "unsent"
	c.Panel.Messages = []Message{{Role: "assistant", Text: "old reply"}}
	c.history = []ai.Message{{Role: "user", Content: "old question"}}
	c.Panel.Pending = &Message{Text: "partial"}
	c.Panel.diagrams = map[string]*diagramEntry{"old": {state: "ready"}}
	c.CloseWindow()
	if c.Panel.Original != "" || c.Panel.Result != "" || c.Panel.Prompt != "" || len(c.Panel.Messages) != 0 || len(c.seed()) != 0 || c.quickInput != "" || c.quickRequest != nil || c.Panel.Pending != nil || len(c.Panel.diagrams) != 0 || c.Initial.Text != "" {
		t.Fatal("Quick close retained content")
	}
}

func TestRestartKeepsVisibleReferenceButRemovesProviderHistory(t *testing.T) {
	for _, source := range []string{"selection", "quick-assistant"} {
		c := NewController(nil, resultwindow.Initial{Mode: "chat", Source: source, Text: "source text"})
		c.Panel.Busy = false
		c.Panel.Original = "old source"
		c.Panel.Result = "old result"
		c.Panel.Prompt = "old draft"
		c.Panel.Messages = []Message{{Role: "assistant", Text: "old reply"}}
		c.history = []ai.Message{{Role: "user", Content: "old question"}, {Role: "assistant", Content: "old reply"}}
		running, epoch := c.begin("chat")
		c.restartConversation()
		if running.Err() != context.Canceled || c.Panel.Busy || c.Panel.Result != "old result" || len(c.Panel.Messages) != 1 || c.Panel.Prompt != "" || len(c.seed()) != 0 {
			t.Fatalf("%s restart kept request context or lost reference", source)
		}
		messages := append(c.seed(), ai.Message{Role: "user", Content: "new question"})
		params := c.chatParameters(messages)
		sent := params["messages"].([]ai.Message)
		if len(sent) != 1 || sent[0].Content != "new question" {
			t.Fatal("provider payload reused old context", sent)
		}
		c.completeRequest(epoch, func() { t.Fatal("stale reply crossed restart boundary") }, nil)
		c.clearAll()
		if c.Panel.Result != "" || c.Panel.Original != "" || len(c.Panel.Messages) != 0 {
			t.Fatal("clear all kept visible reference")
		}
	}
}

func TestResetButtonsAreVisibleAndCanCancelAnActiveRequest(t *testing.T) {
	for _, source := range []string{"selection", "quick-assistant"} {
		c := NewController(nil, resultwindow.Initial{Mode: "chat", Source: source, Language: "zh-CN"})
		c.Panel.Result = "old result"
		c.Panel.Prompt = "draft"
		running, _ := c.begin("chat")
		tt := ui.NewTester(c.Panel.View, 520, 600)
		if _, ok := tt.Find("重新开始"); !ok {
			t.Fatal("restart button missing", source)
		}
		if err := tt.Click("清空全部"); err != nil {
			t.Fatal(err)
		}
		if running.Err() != context.Canceled || c.Panel.Result != "" || c.Panel.Prompt != "" {
			t.Fatal("clear button failed during request", source)
		}
	}
}

func TestComposerHintDisappearsWhenTypingAndNeverReturnsLocally(t *testing.T) {
	p := NewPanel("chat", "zh-CN", "light")
	p.ShowComposerHint = true
	tt := ui.NewTester(p.View, 520, 600)
	if _, ok := tt.Find("Enter 发送，Shift+Enter 换行"); !ok {
		t.Fatal("first-use hint missing")
	}
	if err := tt.Click("继续提问"); err != nil {
		t.Fatal(err)
	}
	tt.Type("开始")
	if p.ShowComposerHint || !p.composerEdited {
		t.Fatal("typing did not dismiss the first-use hint")
	}
	p.Prompt = ""
	tt.Frame()
	if _, ok := tt.Find("Enter 发送，Shift+Enter 换行"); ok {
		t.Fatal("hint returned after input cleared")
	}
	if NewPanel("chat", "zh-CN", "light").ShowComposerHint {
		t.Fatal("another window guessed first-use status")
	}
}

func TestQuickClearCannotUndoBackIntoPreviousDraft(t *testing.T) {
	c := NewController(nil, resultwindow.Initial{Source: "quick-assistant", Mode: "chat", Language: "zh-CN"})
	c.Panel.Busy = false
	tt := ui.NewTester(c.Panel.View, 520, 600)
	tt.Type("private unsent content")
	if c.Panel.Prompt == "" {
		t.Fatal("fixture did not type a draft")
	}
	if err := tt.Click("清空全部"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("输入消息"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Ctrl, ui.KeyZ)
	if c.Panel.Prompt != "" {
		t.Fatal("editor undo resurrected a cleared draft")
	}
}

func TestQuickFirstUseHintFitsMinimumWindow(t *testing.T) {
	for _, language := range []string{"zh-CN", "en-US"} {
		c := NewController(nil, resultwindow.Initial{Source: "quick-assistant", Mode: "chat", Language: language})
		p := c.Panel
		p.Busy = false
		p.ShowComposerHint = true
		tt := ui.NewTester(p.View, 320, 300)
		for _, label := range []string{p.T("输入消息", "Message"), p.T("清空全部", "Clear all"), p.T("重新开始", "Restart"), p.T("发送", "Send"), p.T("Enter 发送，Shift+Enter 换行", "Enter to send, Shift+Enter for a new line")} {
			box, ok := tt.Find(label)
			if !ok || box.X < 0 || box.Y < 0 || box.X+box.W > 321 || box.Y+box.H > 301 {
				t.Fatalf("first-use control %q does not fit: %+v", label, box)
			}
		}
	}
}
