package resultui

import (
	"fmt"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"winone/internal/resultwindow"
)

func TestConversationActionsStayInComposerWithoutCoveringReading(t *testing.T) {
	for _, scenario := range []struct{ source, mode string }{{"selection", "chat"}, {"quick-assistant", "chat"}, {"capture", "ocr"}, {"capture", "translate"}} {
		source, mode := scenario.source, scenario.mode
		for _, theme := range []string{"light", "dark"} {
			for _, language := range []string{"zh-CN", "en-US"} {
				for _, size := range [][2]int{{320, 300}, {520, 400}, {1000, 700}} {
					width, height := size[0], size[1]
					t.Run(fmt.Sprintf("%s-%s/%s/%s/%dx%d", source, mode, theme, language, width, height), func(t *testing.T) {
						controller := NewController(nil, resultwindow.Initial{Source: source, Mode: mode, Language: language, Theme: theme})
						p := controller.Panel
						p.Busy, p.Prompt = false, "draft"
						p.Result = "Earlier result remains available while entering another message."
						tt := ui.NewTester(p.View, width, height)
						inputLabel := p.T("继续提问", "Ask a follow-up")
						if source == "quick-assistant" {
							inputLabel = p.T("输入消息", "Message")
						}
						for _, composerHeight := range []float32{0, 210} {
							p.composerHeight = composerHeight
							tt.Frame()
							input, _ := tt.Find(inputLabel)
							clear, clearOK := tt.Find(p.T("清空全部", "Clear all"))
							restart, restartOK := tt.Find(p.T("重新开始", "Restart"))
							if !clearOK || !restartOK || math.Abs(float64(clear.Y-restart.Y)) > 1 || restart.X+restart.W > clear.X || input.Y+input.H > restart.Y {
								t.Fatalf("actions must share a row below the input: input=%+v clear=%+v restart=%+v", input, clear, restart)
							}
							divider, ok := tt.Find("Divider")
							if !ok || restart.Y < divider.Y+divider.H {
								t.Fatalf("conversation actions cover reading: divider=%+v restart=%+v", divider, restart)
							}
							box, ok := tt.Find(p.T("消息输入框", "Message input box"))
							if !ok || box.Y-divider.Y-divider.H < 12 {
								t.Fatalf("input box touches the resize divider: divider=%+v box=%+v", divider, box)
							}
							if clear.X+clear.W > input.X+input.W {
								t.Fatalf("actions exceed the input's right edge: input=%+v clear=%+v", input, clear)
							}
							for _, rect := range []ui.Rect{clear, restart, input} {
								if rect.X < 0 || rect.Y < 40 || rect.X+rect.W > float32(width) || rect.Y+rect.H > float32(height) {
									t.Fatalf("composer action left the usable window: %+v", rect)
								}
							}
							model, _ := tt.Find(p.T("切换供应商和模型", "Change provider and model"))
							send, _ := tt.Find(p.T("发送", "Send"))
							if model.X+model.W > restart.X || clear.X+clear.W > send.X || send.Y != clear.Y {
								t.Fatalf("composer controls overlap: model=%+v restart=%+v clear=%+v send=%+v", model, restart, clear, send)
							}
							if source == "quick-assistant" {
								bar, ok := tt.Find(p.T("快捷功能", "Quick actions"))
								if !ok || bar.Y+bar.H > input.Y {
									t.Fatalf("quick-action strip overlaps input: bar=%+v input=%+v", bar, input)
								}
							}
						}
						if directory := os.Getenv("WINONE_RESULT_ACTION_LAYOUT_DIR"); directory != "" && width == 520 && language == "zh-CN" {
							if err := os.MkdirAll(directory, 0700); err != nil {
								t.Fatal(err)
							}
							file, err := os.Create(filepath.Join(directory, source+"-"+mode+"-"+theme+".png"))
							if err != nil {
								t.Fatal(err)
							}
							encodeErr := png.Encode(file, tt.Image())
							closeErr := file.Close()
							if encodeErr != nil || closeErr != nil {
								t.Fatal(encodeErr, closeErr)
							}
						}
					})
				}
			}
		}
	}
}
