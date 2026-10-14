package resultui

import (
	"github.com/egoist/mygo/ui"
	"testing"
	"winone/internal/resultwindow"
)

func TestReasoningIsCollapsedAndUserCanExpandIt(t *testing.T) {
	p := NewPanel("ocr", "zh-CN", "light")
	p.Messages = []Message{{Role: "assistant", Text: "正式回答", Reasoning: "模型提供的思考", ProviderName: "服务", Model: "模型"}}
	tt := ui.NewTester(p.View, 420, 550)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true, TextScale: 1})
	if tt.HasText("模型提供的思考") || !tt.HasText("正式回答") {
		t.Fatal("reasoning was expanded by default or replaced answer")
	}
	if err := tt.Click("思考过程"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("模型提供的思考") {
		t.Fatal("user could not expand reasoning")
	}
	visible := false
	p.Configuration.ShowReasoning = &visible
	tt.Frame()
	if tt.HasText("思考过程") || tt.HasText("模型提供的思考") || !tt.HasText("正式回答") {
		t.Fatal("global visibility switch did not hide reasoning independently")
	}
}

func TestLateStreamCannotOverwriteStoppedOrNewRequest(t *testing.T) {
	c := NewController(nil, resultwindow.Initial{Mode: "ocr", Language: "zh-CN", Theme: "light"})
	_, old := c.begin("chat")
	c.applyProgress(old, Outcome{Text: "partial", Reasoning: "thinking", Model: "m"}, true)
	c.cancel()
	c.applyProgress(old, Outcome{Text: "late"}, true)
	if c.Panel.Pending == nil || c.Panel.Pending.Text != "partial" || c.Panel.Pending.State != "stopped" {
		t.Fatal("stop lost partial output or accepted late text")
	}
	_, current := c.begin("chat")
	c.applyProgress(old, Outcome{Text: "stale"}, true)
	c.applyProgress(current, Outcome{Text: "new"}, true)
	if c.Panel.Pending.Text != "new" {
		t.Fatal("new request did not own its draft")
	}
	c.Panel.Pending.ReasoningOpen = true
	c.applyProgress(current, Outcome{Text: "new text", Reasoning: "more reasoning"}, true)
	if !c.Panel.Pending.ReasoningOpen {
		t.Fatal("incoming delta collapsed user's open reasoning")
	}
	c.acceptReply(Outcome{Text: "final", Reasoning: "more reasoning"}, false)
	if !c.Panel.Messages[0].ReasoningOpen || len(c.history) != 1 || c.history[0].Content != "final" {
		t.Fatal("completion lost fold state or sent reasoning as answer history")
	}
}
