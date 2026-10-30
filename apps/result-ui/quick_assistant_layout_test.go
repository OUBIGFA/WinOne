package resultui

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
	"winone/internal/ai"
	"winone/internal/resultwindow"
)

func TestQuickAssistantChatActionsAndLayouts(t *testing.T) {
	for _, theme := range []string{"light", "dark"} {
		for _, lang := range []string{"zh-CN", "en-US"} {
			c := NewController(nil, resultwindow.Initial{Mode: "chat", Source: "quick-assistant", Language: lang, Theme: theme})
			p := c.Panel
			p.Busy = false
			p.Configuration = ai.Configuration{Providers: []ai.Provider{{ID: "demo", Name: "示例服务", Icon: "openai", Models: []string{"示例模型"}}}, Tasks: map[string]ai.Selection{"default": {Provider: "demo", Model: "示例模型"}}}
			var sent string
			var inputs []string
			p.Actions.Send = func(text string) { sent = text }
			p.Actions.Quick = func(a resultwindow.SelectionAction) { inputs = append(inputs, a.ID+":"+p.Prompt) }
			tt := ui.NewTester(p.View, 520, 600)
			assertCentered := func() {
				t.Helper()
				bar, ok := tt.Find(p.T("快捷功能", "Quick actions"))
				if !ok {
					t.Fatal("quick action region missing")
				}
				first, _ := tt.Find(p.T("翻译", "Translate"))
				last, _ := tt.Find(p.T("代码", "Code"))
				if math.Abs(float64(first.Y+first.H/2-bar.Y-bar.H/2)) > 1 || math.Abs(float64((first.X+last.X+last.W)/2-bar.X-bar.W/2)) > 1 {
					t.Fatalf("actions not centered in region: region=%+v first=%+v last=%+v", bar, first, last)
				}
				inputBox, ok := tt.Find(p.T("消息输入框", "Message input box"))
				divider, dividerOK := tt.Find("Divider")
				if !ok || !dividerOK || math.Abs(float64(first.Y+first.H/2-(divider.Y+divider.H+inputBox.Y)/2)) > 1 {
					t.Fatalf("quick icons must be centered between the divider and input border: divider=%+v input=%+v icon=%+v", divider, inputBox, first)
				}
			}
			assertCentered()
			if _, ok := tt.Find(p.T("复制", "Copy")); ok {
				t.Fatal("quick toolbar must not contain Copy")
			}
			save := func(state string) {
				if directory := os.Getenv("WINONE_QUICK_LAYOUT_DIR"); directory != "" {
					if err := os.MkdirAll(directory, 0700); err != nil {
						t.Fatal(err)
					}
					f, err := os.Create(filepath.Join(directory, theme+"-"+lang+"-"+state+".png"))
					if err != nil {
						t.Fatal(err)
					}
					if err = png.Encode(f, tt.Image()); err != nil {
						t.Fatal(err)
					}
					if err = f.Close(); err != nil {
						t.Fatal(err)
					}
				}
			}
			save("empty")
			tt.Type("测试内容")
			tt.Key(0, ui.KeyEnter)
			if sent != "测试内容" {
				t.Fatalf("initial composer did not receive keyboard input: %q", sent)
			}
			if err := tt.Click(p.T("翻译", "Translate")); err != nil {
				t.Fatal(err)
			}
			if err := tt.Click(p.T("解释", "Explain")); err != nil {
				t.Fatal(err)
			}
			if len(inputs) != 2 || inputs[0] != "translate:测试内容" || inputs[1] != "explain:测试内容" {
				t.Fatal(inputs)
			}
			p.Messages = []Message{{Role: "user", Text: "请解释闭包。"}, {Role: "assistant", Text: "闭包是能访问外部作用域变量的函数。\n\n它可以在外层函数返回后继续保存这些变量。", ProviderName: "示例服务", ProviderIcon: "openai", Model: "示例模型"}}
			tt.Frame()
			save("conversation")
			p.Pending = &Message{Role: "assistant", Text: "正在整理一个示例…", Reasoning: "检查变量作用域。", ProviderName: "示例服务", Model: "示例模型"}
			p.Busy = true
			tt.Frame()
			save("streaming")
			if _, ok := tt.Find(p.T("停止", "Stop")); !ok {
				t.Fatal("streaming cannot be stopped")
			}
			p.Busy = false
			p.Pending.State = "failed"
			p.Error = p.T("连接失败，请检查 AI 服务后重试。", "Connection failed. Check AI services and retry.")
			tt.Frame()
			save("error")
			p.Pending = nil
			p.Error = ""
			for _, size := range [][2]int{{320, 300}, {520, 600}, {1000, 700}} {
				tt.SetSize(size[0], size[1])
				tt.Frame()
				assertCentered()
				for _, label := range []string{p.T("输入消息", "Message"), p.T("发送", "Send"), p.T("翻译", "Translate"), p.T("切换供应商和模型", "Change provider and model")} {
					box, ok := tt.Find(label)
					if !ok || box.X < 0 || box.Y < 0 || box.X+box.W > float32(size[0])+1 || box.Y+box.H > float32(size[1])+1 {
						t.Fatalf("%s %v inaccessible %+v", label, size, box)
					}
				}
				save(fmt.Sprintf("width-%d", size[0]))
			}
			for n := 0; n < 18; n++ {
				p.QuickActions = append(p.QuickActions, resultwindow.SelectionAction{ID: fmt.Sprintf("custom-%d", n), Name: fmt.Sprintf("自定义 %d", n), Icon: "polish", Kind: "ai", Prompt: "Summarize", Enabled: true})
			}
			tt.SetSize(320, 400)
			if err := tt.Click(p.T("更多功能", "More actions")); err != nil {
				t.Fatal(err)
			}
			save("more")
		}
	}
}
func TestQuickChatUsesGlobalAIAndKeepsHistoryAcrossFollowups(t *testing.T) {
	c := NewController(nil, resultwindow.Initial{Source: "quick-assistant", Mode: "chat"})
	c.Panel.Configuration = ai.Configuration{Tasks: map[string]ai.Selection{"default": {Provider: "ai", Model: "chat"}, "translate": {Provider: "google", Model: "translate"}}}
	c.quickInput = "A separate action input"
	c.history = []ai.Message{{Role: "user", Content: "Question"}, {Role: "assistant", Content: "Answer"}}
	params := c.chatParameters(c.seed())
	if params["selection"] != (ai.Selection{Provider: "ai", Model: "chat"}) {
		t.Fatal("chat used capture translation route", params)
	}
	messages := params["messages"].([]ai.Message)
	if len(messages) != 2 || messages[0].Content != "Question" || messages[1].Content != "Answer" {
		t.Fatal(messages)
	}
	c.newChat()
	if len(c.seed()) != 0 || c.quickInput != "" || !c.Panel.FocusComposer {
		t.Fatal("new chat retained old context")
	}
}
