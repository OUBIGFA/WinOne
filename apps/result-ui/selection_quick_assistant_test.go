package resultui

import (
	"github.com/egoist/mygo/ui"
	"math"
	"testing"
	"winone/internal/ai"
	"winone/internal/resultwindow"
)

func TestSelectionQuickEntryMirrorsLogoAndPassesSelectedText(t *testing.T) {
	c := NewController(nil, resultwindow.Initial{Source: "selection", Mode: "translate", Toolbar: true, Text: "选中的原文", Language: "zh-CN", Theme: "light"})
	defer c.close()
	c.Panel.Busy = false
	var received string
	c.Panel.Actions.OpenQuick = func(text string) { received = text }
	width, height := c.ToolbarDimensions()
	tt := ui.NewTester(c.View, width, height)
	logo, ok := tt.Find("拖动工具栏")
	if !ok {
		t.Fatal("logo slot missing")
	}
	slot, ok := tt.Find("快捷助手入口")
	if !ok {
		t.Fatal("quick assistant slot missing")
	}
	if logo.W != slot.W || logo.H != slot.H || math.Abs(float64(logo.X+logo.W/2+slot.X+slot.W/2-float32(width))) > 1 || logo.Y != slot.Y {
		t.Fatalf("side slots are not mirrored: logo=%+v quick=%+v width=%d", logo, slot, width)
	}
	if err := tt.Click("在快捷助手中打开"); err != nil {
		t.Fatal(err)
	}
	if received != "选中的原文" {
		t.Fatal("selected text was not handed to the chat input", received)
	}
}
func TestQuickHandoffFillsInputWithoutSendingOrDiscardingDraft(t *testing.T) {
	c := NewController(nil, resultwindow.Initial{Source: "quick-assistant", Mode: "chat", Text: "首次选中"})
	defer c.close()
	if c.Panel.Prompt != "首次选中" {
		t.Fatal("new quick window did not receive selection")
	}
	c.history = []ai.Message{{Role: "user", Content: "原来的问题"}, {Role: "assistant", Content: "原来的回复"}}
	c.Panel.Prompt = "未发送的草稿"
	c.ReceiveQuickInput("新选中的内容")
	if c.Panel.Prompt != "未发送的草稿\n\n新选中的内容" || len(c.history) != 2 || !c.Panel.FocusComposer {
		t.Fatal("handoff lost draft or conversation")
	}
	c.ReceiveQuickInput("")
	if c.Panel.Prompt != "未发送的草稿\n\n新选中的内容" {
		t.Fatal("plain activation cleared input")
	}
	c.Panel.Prompt = ""
	c.ReceiveQuickInput("新的原文")
	c.ReceiveQuickInput("新的原文")
	if c.Panel.Prompt != "新的原文" {
		t.Fatal("duplicate activation duplicated the input")
	}
}
