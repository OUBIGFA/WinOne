package kit

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSplitDividerMovesByDragAndKeysWithinItsLimits(t *testing.T) {
	state := SplitState{Percent: 40}
	view := func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() {
			Split(c, &state, SplitOptions{Label: "resize", MinFirst: 28, MinSecond: 30},
				func() { ui.Text(c, "list") }, func() { ui.Text(c, "details") }).Height(400)
		})
	}
	tt := ui.NewTester(view, 1000, 500)
	tt.Frame()
	tt.Frame()
	divider, ok := tt.Find("resize")
	if !ok {
		t.Fatalf("no divider: %q", tt.Texts())
	}
	// Dragged 100 DIPs right across a 1000-DIP split: 10 points more.
	x, y := divider.X+divider.W/2, divider.Y+divider.H/2
	tt.Press(x, y)
	for step := float32(1); step <= 4; step++ {
		tt.Move(x+25*step, y)
	}
	tt.Release(x+100, y)
	tt.Frame()
	if state.Percent < 49 || state.Percent > 51 {
		t.Fatalf("after dragging %v", state.Percent)
	}
	// The keys move it 16 DIPs, and never past the second pane's limit.
	tt.Key(0, ui.KeyLeft)
	tt.Frame()
	if state.Percent < 47.9 || state.Percent > 48.9 {
		t.Fatalf("after Left %v", state.Percent)
	}
	for range 30 {
		tt.Key(0, ui.KeyRight)
	}
	tt.Frame()
	if state.Percent != 70 {
		t.Fatalf("past the limit %v", state.Percent)
	}
	// A share saved outside the limits is brought back within them.
	state.Percent = 5
	tt.Frame()
	if state.Percent != 28 {
		t.Fatalf("below the limit %v", state.Percent)
	}
}
