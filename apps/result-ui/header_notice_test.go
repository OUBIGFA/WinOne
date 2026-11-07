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
					naturalWidth, _ = c.MeasureText(0, ui.Span{Text: p.Notice, Size: em(c.Theme(), sizeMeta)})
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
							dismiss, dismissFound := tt.Find(p.T("关闭提示", "Dismiss notice"))
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
							// The new feedback close control also occupies this half of
							// the title bar. Count its actual measured width and preserve
							// full text whenever that real control budget leaves room.
							available := close.X + close.W - (grip.X + grip.W + 8) - dismiss.W - close.W - 12
							hasRoom := naturalWidth <= available
							if !ok || !dismissFound || (hasRoom && status.W+0.5 < naturalWidth) || status.X < pin.X+pin.W || status.X+status.W > dismiss.X || dismiss.X+dismiss.W > close.X || close.X+close.W > float32(width) {
								t.Fatalf("%s %s %s width=%d scale=%g: short notice %q clipped: natural=%g status=%+v pin=%+v close=%+v", source, language, theme, width, scale, notice, naturalWidth, status, pin, close)
							}
							if math.Abs(float64(grip.Y+grip.H/2-resultHeaderHeight/2)) > 0.5 {
								t.Fatal("notice moved the drag handle vertically")
							}
							if status.X < grip.X+grip.W {
								t.Fatal("notice overlaps the centered handle")
							}
						}
					}
				}
			}
		}
	}
}
