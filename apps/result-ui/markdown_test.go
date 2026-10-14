package resultui

import (
	"github.com/egoist/mygo/ui"
	"image"
	"testing"
)

func TestMermaidWaitsForClosedFenceAndDoesNotRenderOnEveryFrame(t *testing.T) {
	p := NewPanel("ocr", "zh-CN", "light")
	p.Busy = true
	p.Pending = &Message{Text: "```mermaid\nflowchart LR\n A", State: "running"}
	calls := 0
	var source string
	p.Actions.RenderDiagram = func(text string, dark bool, done func(*ui.Bitmap, error)) {
		calls++
		source = text
		done(ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 120, 80))), nil)
	}
	tt := ui.NewTester(p.View, 520, 600)
	if calls != 0 {
		t.Fatal("unfinished diagram was rendered")
	}
	p.Pending.Text += "-->B\n```"
	tt.Frame()
	tt.Frame()
	if calls != 1 || source != "flowchart LR\n A-->B\n" {
		t.Fatalf("diagram dispatch: %d %q", calls, source)
	}
}

func TestMarkdownRendersHeadingsListsTablesAndCode(t *testing.T) {
	p := NewPanel("ocr", "zh-CN", "light")
	p.Source, p.Action = "selection", "code"
	p.Result = "# 标题\n\n**重点**与普通内容。\n\n- 项目甲\n- 项目乙\n\n|名称|结果|\n|---|---|\n|测试|通过|\n\n```go\nfmt.Println(\"hello\")\n```"
	tt := ui.NewTester(p.View, 760, 1000)
	for _, text := range []string{"标题", "重点", "项目甲", "项目乙", "名称", "测试", "通过", "fmt.Println(\"hello\")"} {
		if !tt.HasText(text) {
			t.Errorf("Markdown content missing: %s", text)
		}
	}
	if tt.HasText("# 标题") || tt.HasText("**重点**") || tt.HasText("|名称|结果|") {
		t.Fatal("Markdown syntax remained in rendered content")
	}
	var copied string
	p.Actions.Copy = func(text string) { copied = text }
	if err := tt.Click("复制代码"); err != nil {
		t.Fatal(err)
	}
	if copied != "fmt.Println(\"hello\")\n" {
		t.Fatal("code-only copy included surrounding result")
	}
	if err := tt.Click("复制结果"); err != nil {
		t.Fatal(err)
	}
	if copied != p.Result {
		t.Fatalf("complete result copy changed: %q", copied)
	}
}

func TestMarkdownDoesNotExecuteHTMLOrUnsafeLinks(t *testing.T) {
	p := NewPanel("ocr", "zh-CN", "light")
	p.Result = "[文档](https://example.invalid/guide) [危险](javascript:alert(1))\n\n<script>alert(1)</script>"
	tt := ui.NewTester(p.View, 700, 600)
	if err := tt.Click("文档"); err != nil {
		t.Fatal(err)
	}
	urls := tt.OpenedURLs()
	if len(urls) != 1 || urls[0] != "https://example.invalid/guide" {
		t.Fatalf("valid link failed: %v", urls)
	}
	_ = tt.Click("危险")
	if len(tt.OpenedURLs()) != 1 {
		t.Fatal("unsafe link executed")
	}
}
