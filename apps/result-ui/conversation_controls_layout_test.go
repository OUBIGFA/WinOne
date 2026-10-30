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

func TestConversationActionsFloatAboveComposer(t *testing.T) {
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
							if !clearOK || !restartOK || math.Abs(float64(clear.X+clear.W/2-restart.X-restart.W/2)) > 1 || clear.Y+clear.H >= restart.Y || restart.Y+restart.H >= input.Y {
								t.Fatalf("actions must form a vertical stack above the input: input=%+v clear=%+v restart=%+v", input, clear, restart)
							}
							divider, ok := tt.Find("Divider")
							if !ok || restart.Y+restart.H > divider.Y-8 {
								t.Fatalf("conversation actions cross into the composer pane: divider=%+v restart=%+v", divider, restart)
							}
							box, ok := tt.Find(p.T("消息输入框", "Message input box"))
							if !ok || box.Y-divider.Y-divider.H < 12 {
								t.Fatalf("input box touches the resize divider: divider=%+v box=%+v", divider, box)
							}
							if restart.X+restart.W < input.X+input.W-20 {
								t.Fatalf("actions are not at the input's right edge: input=%+v restart=%+v", input, restart)
							}
							for _, rect := range []ui.Rect{clear, restart, input} {
								if rect.X < 0 || rect.Y < 40 || rect.X+rect.W > float32(width) || rect.Y+rect.H > float32(height) {
									t.Fatalf("composer action left the usable window: %+v", rect)
								}
							}
							model, _ := tt.Find(p.T("切换供应商和模型", "Change provider and model"))
							send, _ := tt.Find(p.T("发送", "Send"))
							if model.X+model.W > send.X {
								t.Fatalf("model picker overlaps send: model=%+v send=%+v", model, send)
							}
							if source == "quick-assistant" {
								bar, ok := tt.Find(p.T("快捷功能", "Quick actions"))
								if !ok || restart.Y+restart.H >= bar.Y {
									t.Fatalf("floating actions overlap the quick-action strip: bar=%+v restart=%+v", bar, restart)
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
