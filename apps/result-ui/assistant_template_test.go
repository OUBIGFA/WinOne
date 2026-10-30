package resultui

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
	"winone/internal/resultwindow"
)

// The quick toolbar is optional content above the editor, not a second editor
// template. Both assistants must keep the same input and footer geometry.
func TestAssistantComposerUsesSharedTemplate(t *testing.T) {
	for _, theme := range []string{"light", "dark"} {
		for _, language := range []string{"zh-CN", "en-US"} {
			for _, size := range [][2]int{{320, 300}, {520, 400}, {1000, 700}} {
				for _, hint := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/%s/%dx%d/hint=%t", theme, language, size[0], size[1], hint), func(t *testing.T) {
						var reference []ui.Rect
						for _, scenario := range []struct{ source, mode string }{{"selection", "chat"}, {"quick-assistant", "chat"}, {"capture", "ocr"}, {"capture", "translate"}} {
							source := scenario.source
							controller := NewController(nil, resultwindow.Initial{Source: source, Mode: scenario.mode, Language: language, Theme: theme})
							p := controller.Panel
							p.Busy, p.Prompt, p.ShowComposerHint = false, "draft", hint
							tt := ui.NewTester(p.View, size[0], size[1])
							inputLabel := p.T("继续提问", "Ask a follow-up")
							if source == "quick-assistant" {
								inputLabel = p.T("输入消息", "Message")
							}
							for index, label := range []string{inputLabel, p.T("切换供应商和模型", "Change provider and model"), p.T("发送", "Send")} {
								box, ok := tt.Find(label)
								if !ok {
									t.Fatalf("%s is missing %s", source, label)
								}
								if source == "selection" {
									reference = append(reference, box)
								} else if box != reference[index] {
									t.Fatalf("%s uses a different composer layout: selection=%+v %s-%s=%+v", label, reference[index], source, scenario.mode, box)
								}
							}
						}
					})
				}
			}
		}
	}
}
