package testhost

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/kit"
)

// ShotWidths are the window widths a page is looked at: a wide window
// and one narrow enough that rows put their controls below their text.
var ShotWidths = []int{1000, 560}

// Shots renders a page in WinOne's light and dark theme at ShotWidths and
// saves each picture as <name>-<theme>-<width>.png in the folder that
// WINONE_SHOTS names, for looking at. prepare runs on each new tester
// before it is saved, as clicking the tab to look at. Without
// WINONE_SHOTS it does nothing, so tests pass without it.
func (h *Harness) Shots(t *testing.T, name string, view func(c *ui.Context), height int, prepare func(tt *ui.Tester)) {
	t.Helper()
	dir := os.Getenv("WINONE_SHOTS")
	if dir == "" {
		return
	}
	for _, dark := range []bool{false, true} {
		theme := "light"
		if dark {
			theme = "dark"
		}
		for _, width := range ShotWidths {
			tt := ui.NewTester(func(c *ui.Context) {
				c.SetTheme(kit.Theme(c.Preferences(), dark))
				view(c)
			}, width, height)
			h.Settle(tt)
			if prepare != nil {
				prepare(tt)
				h.Settle(tt)
			}
			file, err := os.Create(filepath.Join(dir, fmt.Sprintf("%s-%s-%d.png", name, theme, width)))
			if err != nil {
				t.Fatal(err)
			}
			err = png.Encode(file, tt.Image())
			file.Close()
			if err != nil {
				t.Fatal(err)
			}
		}
	}
}
