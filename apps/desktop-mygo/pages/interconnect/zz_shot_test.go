package interconnect

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
)

// Temporary: renders the two transfer tabs for a visual check.
func TestZZShots(t *testing.T) {
	dir := os.Getenv("WINONE_SHOTS")
	if dir == "" {
		t.Skip()
	}
	i18n.SetLocale(i18n.Chinese)
	for _, width := range []int{1100, 520} {
		var calls []call
		h := interconnectHost(t, general{Notify: true}, &calls)
		p := newPage(h.Env)
		tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(kit.Theme(c.Preferences(), false)); p.View(c) }, width, 900)
		h.Settle(tt)
		for _, tab := range []string{"文件互传", "云端分享"} {
			if err := tt.Click(tab); err != nil {
				t.Fatal(err)
			}
			h.Settle(tt)
			f, _ := os.Create(filepath.Join(dir, tab+"-"+string(rune('0'+width/100))+".png"))
			png.Encode(f, tt.Image())
			f.Close()
		}
	}
}
