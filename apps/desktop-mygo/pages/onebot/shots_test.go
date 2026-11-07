package onebot

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
)

// Reuse production pages and dialogs for the global visual review matrix.
func TestShots(t *testing.T) {
	dir := os.Getenv("WINONE_SHOTS")
	if dir == "" {
		t.Skip("WINONE_SHOTS is not set")
	}
	defer i18n.SetLocale(i18n.Chinese)
	for _, locale := range []string{i18n.Chinese, i18n.English} {
		for _, dark := range []bool{false, true} {
			for _, width := range []int{560, 1000} {
				f := &fakeOneBot{profiles: []map[string]any{oneBot("self", "小柴")}}
				h, p, _ := setup(t, f)
				i18n.SetLocale(locale)
				tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(kit.Theme(c.Preferences(), dark)); p.View(c) }, width, 900)
				for _, section := range []string{"home", "roster", "groups", "settings-home", "profile", "personality", "model", "advanced"} {
					p.section = "home"
					if p.settings != nil {
						p.settings.Close()
						p.settings = nil
					}
					switch section {
					case "home", "roster", "groups":
						p.section = section
					case "settings-home":
						p.settings = newProfileSettings(p, p.profiles[0], "home")
					default:
						p.settings = newProfileSettings(p, p.profiles[0], section)
					}
					for range 3 {
						h.Settle(tt)
					}
					file, err := os.Create(filepath.Join(dir, fmt.Sprintf("onebot-%s-%s-%t-%d.png", section, locale, dark, width)))
					if err != nil {
						t.Fatal(err)
					}
					err = png.Encode(file, tt.Image())
					closeErr := file.Close()
					if err != nil {
						t.Fatal(err)
					}
					if closeErr != nil {
						t.Fatal(closeErr)
					}
				}
				p.Close()
			}
		}
	}
}
