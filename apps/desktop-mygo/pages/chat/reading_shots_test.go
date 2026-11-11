package chat

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"winone/desktopmygo/chat/model"
	"winone/desktopmygo/i18n"
)

// readingTurn is a conversation as people actually have it: a question in
// Chinese, thinking, several tools, a note between them, and an answer with
// headings, lists, code, a table and a quote. The thin fixtures of the other
// tests cannot show how such content reads.
func readingTurn(t *testing.T) *itemsHost {
	f := workedTurn(t)
	answer := strings.Join([]string{
		"构建失败的原因是图标生成步骤没有运行，`kit/icons` 里缺少 `layers-plus.svg`。",
		"## 原因",
		"页面引用了新的图标，但生成文件没有更新。编译时 `embed` 找不到对应文件，于是在启动阶段报错，而不是在编译阶段。",
		"## 修复步骤",
		"1. 运行图标生成脚本。\n2. 重新编译并运行测试。\n3. 提交生成的文件。",
		"```powershell\nnode build/generate-mygo-icons.mjs\ngo test ./pages/... -count=1\n```",
		"### 涉及的文件",
		"| 文件 | 变化 | 说明 |\n| --- | --- | --- |\n| `kit/icons/lucide_gen.go` | 修改 | 新增一个图标声明 |\n| `kit/icons/lucide/layers-plus.svg` | 新增 | 图标本身 |",
		"> 注意：生成文件不要手动修改，改动会在下次生成时丢失。",
		"- 已验证：全部测试通过\n- 未验证：真实窗口中的显示\n  - 需要在本机打开一次确认",
		"之后如果再新增图标，只需要重新运行第一步。",
	}, "\n\n")
	f.snapshot = model.Snapshot{Messages: []model.Message{
		{ID: "u1", Role: "user", Content: model.TextContent("构建为什么失败了？帮我查一下原因并修好，改完之后告诉我改了哪些文件。"), TurnID: "u1"},
		{ID: "a1", Role: "assistant", TurnID: "u1", Content: model.Content{Parts: []model.Part{
			{Type: "thinking", Thinking: "先看构建日志，确认是编译错误还是运行时错误。如果是 embed 找不到文件，说明生成步骤没有跑。"},
			{Type: "toolCall", ID: "t1", Name: "read", Arguments: map[string]any{"path": "build.log"}},
			{Type: "toolCall", ID: "t2", Name: "bash", Arguments: map[string]any{"command": "node build/generate-mygo-icons.mjs"}},
		}}},
		{ID: "r1", Role: "toolResult", TurnID: "u1", ToolCallID: "t1", ToolName: "read", Content: model.TextContent("panic: open lucide/layers-plus.svg: file does not exist")},
		{ID: "r2", Role: "toolResult", TurnID: "u1", ToolCallID: "t2", ToolName: "bash", Content: model.TextContent("icons written: 209")},
		{ID: "a2", Role: "assistant", TurnID: "u1", Content: model.Content{Parts: []model.Part{
			{Type: "text", Text: "图标已经重新生成，接下来运行测试确认。"},
			{Type: "toolCall", ID: "t3", Name: "bash", Arguments: map[string]any{"command": "go test ./pages/... -count=1"}},
			{Type: "toolCall", ID: "t4", Name: "write", Arguments: map[string]any{"path": "notes.md", "content": "# Notes\n"}},
		}}},
		{ID: "r3", Role: "toolResult", TurnID: "u1", ToolCallID: "t3", ToolName: "bash", Content: model.TextContent("ok  \twinone/desktopmygo/pages/settings\t5.058s\nok  \twinone/desktopmygo/pages/chat\t7.365s")},
		{ID: "r4", Role: "toolResult", TurnID: "u1", ToolCallID: "t4", ToolName: "write", Content: model.TextContent("Wrote notes.md")},
		{ID: "a3", Role: "assistant", TurnID: "u1", Content: model.TextContent(answer)},
		{ID: "u2", Role: "user", Content: model.TextContent("好的，谢谢。"), TurnID: "u2"},
		{ID: "a4", Role: "assistant", TurnID: "u2", Content: model.TextContent("不客气。如果之后构建再出问题，把日志发给我就行。")},
	}}
	return f
}

// TestReadingShots draws that conversation from its top and from its end in
// both themes and two widths when WINONE_SHOTS names a folder.
func TestReadingShots(t *testing.T) {
	if os.Getenv("WINONE_SHOTS") == "" {
		t.Skip("WINONE_SHOTS is not set")
	}
	defer func() { dark = false; i18n.SetLocale(i18n.Chinese) }()
	for _, theme := range []bool{false, true} {
		for _, width := range []int{1280, 640} {
			dark = theme
			h, p, tt := setupItems(t, readingTurn(t))
			i18n.SetLocale(i18n.Chinese)
			tt.SetSize(width, 1400)
			name := fmt.Sprintf("%s-%d", map[bool]string{false: "light", true: "dark"}[theme], width)
			p.ui.expanded = map[string]bool{}
			p.ui.keepPosition()
			p.ui.toTop = true
			settle(h, tt)
			settle(h, tt)
			shot(t, tt, "reading-top-"+name)
			p.ui.expanded["tool:t3"] = true
			p.ui.expanded["thinking:a1:0"] = true
			p.ui.followLatest()
			settle(h, tt)
			settle(h, tt)
			shot(t, tt, "reading-end-"+name)
		}
	}
}
