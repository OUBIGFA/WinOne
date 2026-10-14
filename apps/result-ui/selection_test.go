package resultui

import (
	"github.com/egoist/mygo/ui"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"winone/internal/resultwindow"
)

func TestCustomToolbarActionsUseSavedOrderAndVisibility(t *testing.T) {
	p := NewPanel("translate", "zh-CN", "light")
	c := &Controller{Panel: p, Toolbar: true, Initial: resultwindow.Initial{Actions: []resultwindow.SelectionAction{
		{ID: "custom-reply", Name: "邮件回复", Icon: "conversation", Kind: "ai", Prompt: "写回复", Enabled: true},
		{ID: "copy", Name: "复制", Icon: "copy", Kind: "copy", Enabled: true},
		{ID: "translate", Name: "不显示的翻译", Icon: "translate", Kind: "translate", Enabled: false},
	}}}
	w, h := c.ToolbarDimensions()
	tt := ui.NewTester(c.View, w, h)
	first, ok := tt.Find("邮件回复")
	second, found := tt.Find("复制")
	if !ok || !found || first.X >= second.X {
		t.Fatal("saved toolbar order was not used")
	}
	if _, found = tt.Find("不显示的翻译"); found {
		t.Fatal("disabled action remained visible")
	}
}

func TestSelectionToolbarUsesCompactActionsAndSharedResult(t *testing.T) {
	for _, theme := range []string{"light", "dark"} {
		p := NewPanel("translate", "zh-CN", theme)
		p.Source, p.Action, p.Original, p.Result = "selection", "code", "hidden selected source", "这段代码按顺序处理请求。\n\n```go\nfor _, request := range requests {\n    handle(request)\n}\n```"
		p.ResultDetails = Message{ProviderName: "示例服务", ProviderIcon: "deepseek", Model: "示例模型"}
		c := &Controller{Panel: p, Toolbar: true}
		width, height := c.ToolbarDimensions()
		tt := ui.NewTester(c.View, width, height)
		for _, label := range []string{"复制", "翻译", "解释", "总结", "润色", "代码"} {
			box, ok := tt.Find(label)
			if !ok || box.X < 0 || box.Y < 0 || box.X+box.W > float32(width) || box.Y+box.H > float32(height) {
				t.Fatalf("toolbar action unavailable: %s %+v", label, box)
			}
		}
		if _, ok := tt.Find("hidden selected source"); ok {
			t.Fatal("toolbar exposed selected text")
		}
		output := os.Getenv("WINONE_SELECTION_LAYOUT_DIR")
		save := func(name string) {
			if output == "" {
				return
			}
			if err := os.MkdirAll(output, 0700); err != nil {
				t.Fatal(err)
			}
			file, err := os.Create(filepath.Join(output, theme+"-"+name+".png"))
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			if err = png.Encode(file, tt.Image()); err != nil {
				t.Fatal(err)
			}
		}
		save("toolbar")
		before, _ := tt.Find("复制")
		tt.Move(before.X+before.W/2, before.Y+before.H/2)
		after, _ := tt.Find("复制")
		if before != after {
			t.Fatal("hover changed the toolbar action bounds")
		}
		if tt.Cursor() != ui.CursorPointer {
			t.Fatal("toolbar action lost pointer cursor")
		}
		save("toolbar-hover")
		c.Toolbar = false
		p.Messages = []Message{{Role: "user", Text: "如何处理失败的请求？"}, {Role: "assistant", Text: "记录失败原因，并让用户选择是否重试。", ProviderName: "示例服务", ProviderIcon: "deepseek", Model: "示例模型"}}
		tt.SetSize(600, 650)
		tt.Frame()
		save("result")
	}
}

func TestSelectionUsesSharedResultAndConversationWithoutSourceBlocks(t *testing.T) {
	p := NewPanel("translate", "zh-CN", "light")
	p.Source, p.Action = "selection", "translate"
	p.Original, p.Result = "private selected source", "对应的翻译结果"
	p.ResultDetails = Message{ProviderName: "服务名称", Model: "模型名称", ProviderIcon: "openai"}
	p.Messages = []Message{{Role: "user", Text: "继续解释"}, {Role: "assistant", Text: "进一步的说明", Model: "对话模型", ProviderName: "对话服务"}}
	var copied string
	p.Actions.Copy = func(value string) { copied = value }
	tt := ui.NewTester(p.View, 700, 800)
	for _, hidden := range []string{"原文", "译文", "智能互译", "private selected source", "复制原文", "复制译文"} {
		if _, found := tt.Find(hidden); found {
			t.Fatalf("selection result exposed capture-only content: %s", hidden)
		}
	}
	for _, visible := range []string{"对应的翻译结果", "服务名称", "模型名称", "继续解释", "进一步的说明", "继续提问"} {
		if _, found := tt.Find(visible); !found {
			t.Fatalf("shared result content missing: %s", visible)
		}
	}
	tt.Click("复制结果")
	if copied != p.Result {
		t.Fatal("result action copied hidden source")
	}
}
