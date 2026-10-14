package resultui

import (
	"github.com/egoist/mygo/ui"
	"testing"
)

func TestResultSelectionReadsTextWithoutChangingClipboard(t *testing.T) {
	p := NewPanel("ocr", "zh-CN", "light")
	p.Result = "Result selection 内容"
	c := &Controller{Panel: p}
	tt := ui.NewTester(c.View, 420, 500)
	tt.SetClipboard("user clipboard")
	if err := tt.Click(p.Result); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	if c.selectedText != p.Result {
		t.Fatalf("selected=%q", c.selectedText)
	}
	if tt.Clipboard() != "user clipboard" {
		t.Fatal("selection changed clipboard")
	}
	tt.ClickAt(1, 1)
	if c.selectedText != "" {
		t.Fatalf("stale selection=%q", c.selectedText)
	}
}
