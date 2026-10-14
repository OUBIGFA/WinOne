package resultui

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestQuickActionShowsSeparateIconWithoutAddingActionNameToUserText(t *testing.T) {
	for _, theme := range []string{"light", "dark"} {
		for _, width := range []int{320, 520} {
			panel := NewPanel("chat", "zh-CN", "light")
			panel.Theme = theme
			panel.Source = "quick-assistant"
			panel.Messages = []Message{{Role: "user", Text: "A paragraph to translate.", ActionIcon: "translate", ActionLabel: "翻译"}}
			tt := ui.NewTester(func(c *ui.Context) {
				ui.Column(c).Fill().Background(panel.palette(c).Background).Children(func() { panel.conversation(c, panel.palette(c)) })
			}, width, 160)
			icon, ok := tt.Find("快捷操作：翻译")
			user, _ := tt.Find("用户消息气泡")
			if !ok || icon.X != user.X || icon.Y+icon.H > user.Y {
				t.Fatal("action marker must sit above the matching bubble at its left edge", icon, user)
			}
			if tt.HasText("翻译\nA paragraph to translate.") || !tt.HasText("A paragraph to translate.") {
				t.Fatal("action label replaced or prefixed user text")
			}
			if directory := os.Getenv("WINONE_QUICK_LAYOUT_DIR"); directory != "" {
				if err := os.MkdirAll(directory, 0700); err != nil {
					t.Fatal(err)
				}
				file, err := os.Create(filepath.Join(directory, fmt.Sprintf("marker-%s-%d.png", theme, width)))
				if err != nil {
					t.Fatal(err)
				}
				if err = png.Encode(file, tt.Image()); err != nil {
					t.Fatal(err)
				}
				if err = file.Close(); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}
