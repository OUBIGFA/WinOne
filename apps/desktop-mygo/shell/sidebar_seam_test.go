package shell

import (
	"encoding/json"
	"testing"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/testhost"
)

func TestConversationBeginsAtSidebarEdge(t *testing.T) {
	h := testhost.New(t, func(method string, _ json.RawMessage) (any, error) {
		if method == "chat.ghosts.pages" {
			return []any{}, nil
		}
		return map[string]any{}, nil
	})
	s := New(h.Env, &feedbackWindow{}, Options{Directory: h.Env.Directory})
	s.loaded, s.page = true, "chat"
	s.instance = core.PageFunc(func(c *ui.Context) { ui.Box(c).Fill().Label("conversation boundary") })
	t.Cleanup(s.Discarded)
	for _, width := range []float32{52, 240, 360} {
		s.sidebarWidth = width
		tt := ui.NewTester(s.View, 1280, 800)
		h.Settle(tt)
		r, ok := tt.Find("conversation boundary")
		if !ok || r.X != width {
			t.Fatalf("sidebar %.0f: content starts at %.1f, leaving a seam", width, r.X)
		}
	}
}
