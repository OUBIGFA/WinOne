package flashchat

import (
	"fmt"
	"math"
	"testing"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/testhost"
)

// Read the production welcome's first and last labels after its very first
// visible frame. Settling before this check would hide a measurement-based jump.
func TestLobbyIsCenteredOnTheFirstVisibleFrameAndAfterReturning(t *testing.T) {
	for _, locale := range []string{i18n.Chinese, i18n.English} {
		for _, size := range [][2]int{{1000, 760}, {560, 560}} {
			t.Run(fmt.Sprintf("%s/%dx%d", locale, size[0], size[1]), func(t *testing.T) {
				i18n.SetLocale(locale)
				host := &fakeHost{nickname: "Ada", tasks: map[string]map[string]any{}}
				h := testhost.New(t, host.handle)
				p := newPage(h.Env)
				t.Cleanup(p.Close)
				shown := false
				tt := ui.NewTester(func(c *ui.Context) {
					c.SetTheme(kit.Theme(c.Preferences(), false))
					ui.Column(c).Fill().Children(func() {
						if shown {
							p.View(c)
						} else {
							ui.Text(c, "Other page")
						}
					})
				}, size[0], size[1])
				centered := func() float32 {
					t.Helper()
					top, ok := tt.Find(tr("peerChat"))
					if !ok {
						t.Fatal("welcome title is not visible")
					}
					bottom, ok := tt.Find(tr("chatEncryption"))
					if !ok {
						t.Fatal("welcome footer is not visible")
					}
					return (top.Y + bottom.Y + bottom.H) / 2
				}
				shown = true
				tt.Frame()
				first := centered()
				if math.Abs(float64(first-float32(size[1])/2)) > 3 {
					t.Fatalf("first welcome frame is not centered: center=%g viewport=%d", first, size[1])
				}
				tt.Frame()
				if next := centered(); math.Abs(float64(next-first)) > 1 {
					t.Fatalf("welcome jumped after its first layout: %g -> %g", first, next)
				}
				shown = false
				tt.Frame()
				shown = true
				tt.Frame()
				if returned := centered(); math.Abs(float64(returned-first)) > 1 {
					t.Fatalf("returning changed the welcome position: %g -> %g", first, returned)
				}
			})
		}
	}
}
