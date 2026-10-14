package resultui

import (
	"fmt"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// Measure the actual rendered status rather than a hard-coded translation
// width: the same title bar serves three result sources and system text scales.
func TestShortHeaderNoticesHaveTheirMeasuredWidth(t *testing.T) {
	for _, language := range []string{"zh-CN", "en-US"} {
		for _, theme := range []string{"light", "dark"} {
			for _, source := range []string{"capture", "selection", "quick-assistant"} {
				p := NewPanel("translate", language, theme)
				p.Source, p.Result = source, "Readable result"
				var naturalWidth float32
				tt := ui.NewTester(func(c *ui.Context) {
					p.View(c)
					naturalWidth, _ = c.MeasureText(0, ui.Span{Text: p.Notice})
				}, 520, 600)
				for _, scale := range []float32{1, 1.5} {
					tt.SetPreferences(ui.Preferences{ReduceMotion: true, TextScale: scale})
					for _, width := range []int{320, 520, 900} {
						tt.SetSize(width, 600)
						for _, notice := range []string{p.T("已复制", "Copied"), p.T("已停止", "Stopped"), p.T("收到新回复", "New reply")} {
							p.Notice = notice
							tt.Frame()
							status, ok := tt.Find(notice)
							pin, _ := tt.Find(p.T("固定结果窗", "Pin result window"))
							close, _ := tt.Find(p.T("关闭", "Close"))
							grip, _ := tt.Find(p.T("拖动结果窗", "Drag result window"))
							if directory := os.Getenv("WINONE_HEADER_LAYOUT_DIR"); directory != "" && source == "capture" && scale == 1 && notice == p.T("已复制", "Copied") {
								if err := os.MkdirAll(directory, 0700); err != nil {
									t.Fatal(err)
								}
								file, err := os.Create(filepath.Join(directory, fmt.Sprintf("%s-%s-%d.png", language, theme, width)))
								if err != nil {
									t.Fatal(err)
								}
								encodeErr, closeErr := png.Encode(file, tt.Image()), file.Close()
								if encodeErr != nil || closeErr != nil {
									t.Fatal(encodeErr, closeErr)
								}
							}
							// A long notice at 150% text scale can exceed half a 320-DIP
							// window. Require full text when pin/drag/gutters leave room;
							// always require actionable buttons and no overlap.
							hasRoom := naturalWidth+pin.W+grip.W+24 <= float32(width)/2
							if !ok || (hasRoom && status.W+0.5 < naturalWidth) || status.X < pin.X+pin.W || status.X+status.W > close.X || close.X+close.W > float32(width) {
								t.Fatalf("%s %s %s width=%d scale=%g: short notice %q clipped: natural=%g status=%+v pin=%+v close=%+v", source, language, theme, width, scale, notice, naturalWidth, status, pin, close)
							}
							if math.Abs(float64(grip.Y+grip.H/2-resultHeaderHeight/2)) > 0.5 {
								t.Fatal("notice moved the drag handle vertically")
							}
							if status.X+status.W > grip.X {
								t.Fatal("notice overlaps the centered title group")
							}
						}
					}
				}
			}
		}
	}
}
