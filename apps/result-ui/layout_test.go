package resultui

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"winone/internal/ai"
)

// Optional artifacts render the real component with deterministic public text.
// Ordinary tests still verify that the composer stays reachable at small sizes.
func TestResultReadingAndComposerLayout(t *testing.T) {
	output := os.Getenv("WINONE_RESULT_LAYOUT_DIR")
	if output != "" {
		if _, err := os.Stat(output); !os.IsNotExist(err) {
			t.Fatal("choose a new layout directory")
		}
		if err := os.MkdirAll(output, 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, mode := range []string{"ocr", "translate"} {
		for _, theme := range []string{"light", "dark"} {
			for _, language := range []string{"zh-CN", "en-US"} {
				p := NewPanel(mode, language, theme)
				p.Original = "The draft was adopted as-is, including the section on issue fixes and the two-line version comparison table."
				p.Result = "草案已按原样采纳，其中包含问题修复章节，以及两行版本对照表。"
				if mode == "ocr" {
					p.Result = p.Original
				}
				p.Configuration = ai.Configuration{Providers: []ai.Provider{{ID: "demo", Name: "示例服务", Icon: "openai", Models: []string{"示例模型"}}}, Tasks: map[string]ai.Selection{"default": {Provider: "demo", Model: "示例模型"}}}
				tt := ui.NewTester(p.View, 420, 420)
				tt.SetPreferences(ui.Preferences{ReduceMotion: true, TextScale: 1})
				save := func(name string) {
					if output == "" {
						return
					}
					file, err := os.Create(filepath.Join(output, mode+"-"+theme+"-"+language+"-"+name+".png"))
					if err != nil {
						t.Fatal(err)
					}
					err = png.Encode(file, tt.Image())
					closeErr := file.Close()
					if err != nil || closeErr != nil {
						t.Fatal(err, closeErr)
					}
				}
				save("result")
				p.Messages = []Message{{Role: "user", Text: "请把译文改得更自然一些。"}, {Role: "assistant", ProviderName: "示例服务", ProviderIcon: "openai", Model: "示例模型", Text: "草案已原样通过，包含问题修复章节和两行版本对照表。"}}
				p.FollowEnd()
				tt.Frame()
				save("conversation")
				tt.SetSize(1200, 700)
				save("wide-conversation")
				p.Messages[1].Reasoning = "先根据上下文检查语气，再保持原文含义并整理句子。"
				tt.SetSize(520, 600)
				save("reasoning-collapsed")
				p.Messages[1].ReasoningOpen = true
				tt.Frame()
				save("reasoning-expanded")
				p.Messages[1].ReasoningOpen = false
				for _, size := range [][2]int{{320, 300}, {420, 420}, {780, 600}} {
					tt.SetSize(size[0], size[1])
					p.FollowEnd()
					tt.Frame()
					for _, label := range []string{p.T("继续提问", "Ask a follow-up"), p.T("切换供应商和模型", "Change provider and model")} {
						box, ok := tt.Find(label)
						if !ok || box.X < 0 || box.Y < 0 || box.X+box.W > float32(size[0]) || box.Y+box.H > float32(size[1]) {
							t.Fatalf("%s %s %s inaccessible at %v: %+v", mode, theme, label, size, box)
						}
					}
					p.Result = strings.Repeat("完整识别和翻译结果需要保留。", 150)
					tt.Frame()
					save(fmt.Sprintf("long-%d", size[0]))
				}
			}
		}
	}
}
