package chat

import (
	"fmt"
	"math"
	"testing"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
)

// Use the production welcome itself, measuring its first committed layout.
// A settled-only screenshot cannot catch previous-frame centering regressions.
func TestWelcomeComposerHasItsFinalPositionOnFirstFrame(t *testing.T) {
	_, h, p, _, _ := setup(t)
	for _, locale := range []string{i18n.Chinese, i18n.English} {
		i18n.SetLocale(locale)
		h.Shots(t, fmt.Sprintf("chat-welcome-%s", locale), p.welcome, 900, nil)
	}
	i18n.SetLocale(i18n.Chinese)
	for _, size := range [][2]int{{1000, 900}, {560, 900}, {560, 420}} {
		for _, reduced := range []bool{false, true} {
			var first, settled ui.Rect
			captured := false
			tt := ui.NewTesterWith(func(c *ui.Context) {
				p.welcome(c)
				// Tester settles pending frames internally. Record during the
				// first paint, not after NewTester/Frame has hidden the jump.
				c.Resolve(p.ui.composerBox).Draw(func(_ *ui.Painter, box ui.Rect) {
					settled = box
					if !captured {
						first, captured = box, true
					}
				})
			}, size[0], size[1], ui.TesterOptions{Preferences: ui.Preferences{ReduceMotion: reduced}})
			if !captured || first.H == 0 {
				t.Fatal("first paint has no composer")
			}
			h.Settle(tt)
			logo, ok := tt.Find("WinOne")
			if !ok || logo.W != 160 || logo.H != 160 {
				t.Fatalf("welcome logo lost its original size: %+v", logo)
			}
			if size[1] == 900 {
				center := (logo.Y + settled.Y + settled.H) / 2
				if math.Abs(float64(center)-float64(size[1])/2) > 2 || math.Abs(float64(logo.X+logo.W/2-settled.X-settled.W/2)) > 1 {
					t.Fatalf("logo and composer are not centered together: logo=%+v composer=%+v", logo, settled)
				}
			}
			if math.Abs(float64(first.Y-settled.Y)) > 0.5 || math.Abs(float64(first.X-settled.X)) > 0.5 {
				t.Fatalf("welcome jumped at %v: %+v -> %+v", size, first, settled)
			}
			if size[1] == 900 && first.Y < 300 {
				t.Fatalf("welcome composer remained at the top: %+v", first)
			}
		}
	}
}

// The control follows layout immediately; it never glides over prose.
func TestProcessCollapseHasNoMotion(t *testing.T) {
	for _, reduced := range []bool{false, true} {
		p := &Chat{}
		p.ui.openProcesses = []string{"group"}
		body := &ui.Handle{}
		p.ui.processBodies = map[string]*ui.Handle{"group": body}
		height := float32(300)
		tt := ui.NewTester(func(c *ui.Context) {
			ui.Column(c).Fill().Children(func() {
				ui.Scroll(c).Fill().TrackScroll(&p.ui.transcript).Children(func() {
					ui.Box(c).Width(400).Height(height).Bind(body)
				})
				p.processCollapseControls(c)
			})
		}, 500, 600)
		tt.SetPreferences(ui.Preferences{ReduceMotion: reduced})
		tt.Frame()
		tt.Frame()
		_, ok := tt.Find(i18n.T("chat.collapseProcess"))
		if !ok {
			t.Fatal("missing collapse control")
		}
		height = 400
		tt.Frame()
		tt.Frame()
		middle, _ := tt.Find(i18n.T("chat.collapseProcess"))
		if middle.Y != 364 {
			t.Fatalf("collapse control still animates: %+v", middle)
		}
		height = 260
		tt.Frame()
		tt.Frame()
		reversed, _ := tt.Find(i18n.T("chat.collapseProcess"))
		if reversed.Y < 8 || reversed.Y+reversed.H > height {
			t.Fatalf("motion escaped process: %+v", reversed)
		}
	}
}
