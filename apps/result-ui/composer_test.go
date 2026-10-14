package resultui

import (
	"github.com/egoist/mygo/ui"
	"testing"
)

func TestComposerCanBeDraggedUpWithoutLosingDraft(t *testing.T) {
	p := NewPanel("translate", "zh-CN", "light")
	p.Original = "Original"
	p.Result = "Translation"
	p.Prompt = "尚未发送的内容"
	tt := ui.NewTester(p.View, 520, 600)
	input, ok := tt.Find("继续提问")
	if !ok {
		t.Fatal("composer missing")
	}
	divider, ok := tt.Find("Divider")
	if !ok {
		t.Fatal("input resize divider missing")
	}
	tt.Press(divider.X+divider.W/2, divider.Y)
	tt.Move(divider.X+divider.W/2, divider.Y-80)
	tt.Release(divider.X+divider.W/2, divider.Y-80)
	enlarged, _ := tt.Find("继续提问")
	if enlarged.H < input.H+60 || p.Prompt != "尚未发送的内容" {
		t.Fatalf("drag did not enlarge input safely: before=%+v after=%+v", input, enlarged)
	}
	tt.SetSize(320, 300)
	send, _ := tt.Find("发送")
	if send.Y+send.H > 300 || send.X+send.W > 320 {
		t.Fatal("small window hid send action")
	}
}

func TestMultilineComposerUsesShiftEnterAndEnterToSend(t *testing.T) {
	p := NewPanel("ocr", "zh-CN", "light")
	var sent string
	p.Actions.Send = func(text string) { sent = text }
	tt := ui.NewTester(p.View, 520, 600)
	if err := tt.Click("继续提问"); err != nil {
		t.Fatal(err)
	}
	tt.Type("第一行")
	tt.Key(ui.Shift, ui.KeyEnter)
	tt.Type("第二行")
	if p.Prompt != "第一行\n第二行" || sent != "" {
		t.Fatalf("Shift+Enter failed: %q", p.Prompt)
	}
	tt.Key(0, ui.KeyEnter)
	if sent != "第一行\n第二行" {
		t.Fatalf("Enter did not send multiline draft: %q", sent)
	}
}
