package markdown

import (
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown/parser"
	"testing"
)

// WinOne's math worker answers after the paragraph's source stopped streaming.
// Rebuild shaping when the formula arrives, without replacing the document or
// changing the copied source/selection. Ordinary frames still reuse shaping.
func TestWinOneInlineMathAsyncRevision(t *testing.T) {
	var revision uint64
	calls := 0
	ready := false
	src := "Let $x_i$ remain selectable."
	o := &Options{Extensions: parser.Default | parser.ExtDollarMath, InlineMathRevision: func() uint64 { return revision }, InlineMath: func(string, float32) (InlineObject, bool) {
		calls++
		return InlineObject{Width: 30, Height: 20, Depth: 3, Draw: func(*ui.Painter, ui.Rect) {}}, ready
	}}
	tt, get := rvView(&src, o)
	original := get().doc
	if !tt.HasText("Let x_i remain selectable.") {
		t.Fatalf("missing initial fallback: %q", tt.Texts())
	}
	before := calls
	ready = true
	tt.Frame()
	if calls != before {
		t.Fatal("unchanged revision did not reuse paragraph shaping")
	}
	revision++
	tt.Frame()
	if get().doc != original {
		t.Fatal("formula completion replaced the Markdown document")
	}
	r, ok := tt.Find("Let \uFFFC remain selectable.")
	if !ok || calls <= before {
		t.Fatalf("ready math was not shaped: %q", tt.Texts())
	}
	tt.ClickAt(r.X+2, r.Y+r.H/2)
	tt.Command("selectAll")
	tt.Command("copy")
	if got := tt.Clipboard(); got != src {
		t.Fatalf("math changed copy source: %q", got)
	}
}
