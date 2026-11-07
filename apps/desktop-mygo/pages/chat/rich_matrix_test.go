package chat

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"winone/desktopmygo/chat/model"
	"winone/desktopmygo/i18n"
)

// These are model-authored cindy-ui operations, not a second view. Both the
// component checks and the explicit native-window matrix render them with the
// production Chat/RichAnswer/Markdown implementations.
type richProductionCase struct {
	kind, source string
	labels       []string
}

func richProductionCases(locale, directory string) []richProductionCase {
	text := func(zh, en string) string {
		if locale == i18n.Chinese {
			return zh
		}
		return en
	}
	label := func(kind string) string { return text("完整能力 ", "Full capability ") + kind }
	inline := func(id, kind string, props map[string]any) map[string]any {
		return map[string]any{"id": id, "kind": kind, "props": props}
	}
	child := text("独立子项：内容完整且可换行", "Independent child: complete content wraps")
	markdown := text("# 正式回复\n\n**加重**、*强调*、~~旧值~~ 和 `inline-code`。\n\n> 引用保持独立层级。\n\n- 第一项\n- 第二项\n\n1. 有序步骤\n2. 下一步骤\n\n", "# Delivered answer\n\n**Strong**, *emphasis*, ~~old value~~ and `inline-code`.\n\n> A quote keeps its own hierarchy.\n\n- First item\n- Second item\n\n1. Ordered step\n2. Next step\n\n") +
		"| " + text("程序名称", "Application name") + " | " + text("完整路径", "Complete path") + " | " + text("结果", "Result") + " |\n|:---|:---|---:|\n| WinOne | `C:\\workspace\\folder with spaces\\very-long-resource-name.md` | 42 |\n| " + text("末项可达", "Last row remains reachable") + " | " + text("长内容随窄窗换行，不以隐藏横滚条掩盖", "Long content wraps in a narrow window rather than concealing overflow") + " | -8 |\n\n" +
		text("[已核对链接](https://example.invalid/reference) 与脚注[^proof]。\n\n[^proof]: 完整脚注内容。", "[Checked link](https://example.invalid/reference) and a footnote[^proof].\n\n[^proof]: Complete footnote content.")
	kinds := []string{"text", "markdown", "heading", "icon", "stack", "row", "grid", "group", "divider", "image", "video", "audio", "code", "math", "diagram", "stat", "chart", "svg", "range", "input", "check", "select", "button", "form", "details", "tabs"}
	var cases []richProductionCase
	for _, kind := range kinds {
		props := map[string]any{"label": label(kind)}
		labels := []string{label(kind)}
		switch kind {
		case "text", "heading":
			props["text"] = label(kind) + " — " + child
		case "markdown":
			props["text"] = markdown
			labels = []string{text("正式回复", "Delivered answer"), text("末项可达", "Last row remains reachable"), text("完整脚注内容", "Complete footnote content")}
		case "icon":
			props["name"], props["text"] = "mail", child
		case "stack", "row", "grid", "group":
			props["children"] = []any{inline("first", "text", map[string]any{"text": label(kind)}), inline("second", "text", map[string]any{"text": child})}
			props["columns"], props["mobileColumns"] = 2, 1
			if kind == "grid" {
				props["weights"] = []any{1, 2}
			}
			// Authors' fixed dimensions must remain bounded by the actual
			// Chat reading area, including simultaneous v0.8 max constraints.
			props["style"] = map[string]any{"width": 900, "minWidth": 680, "maxWidth": "100%"}
		case "divider":
			labels = nil
		case "image", "video", "audio":
			ext := map[string]string{"image": "png", "video": "mp4", "audio": "wav"}[kind]
			props["src"] = filepath.Join(directory, "native-rich."+ext)
			props["alt"], props["caption"] = label(kind), label(kind)
		case "code":
			props["language"], props["text"] = "go", "// "+label(kind)+"\nfmt.Println(\""+strings.Repeat("long-readable-code-", 12)+"\")"
		case "math":
			props["text"], props["caption"] = `\frac{a+b}{2}=3`, label(kind)
		case "diagram":
			props["text"], props["caption"] = "graph LR\n A["+text("输入", "Input")+"] --> B["+text("已验证输出", "Verified output")+"]", label(kind)
		case "stat":
			props["value"], props["prefix"], props["suffix"] = map[string]any{"op": "mul", "args": []any{6, 18}}, "¥", text(" / 月", " / month")
			labels = append(labels, "¥108")
		case "chart":
			props["data"] = []any{map[string]any{"label": text("正值", "Positive"), "value": 42}, map[string]any{"label": text("负值", "Negative"), "value": -8}, map[string]any{"label": text("零", "Zero"), "value": 0}}
			props["colors"] = []any{"success", "secondary", "warning"}
			labels = append(labels, "42", "-8")
		case "svg":
			props["viewBox"] = []any{0, 0, 320, 120}
			props["shapes"] = []any{map[string]any{"type": "rect", "x": 10, "y": 10, "width": 280, "height": 90, "fill": "secondary"}, map[string]any{"type": "circle", "cx": 55, "cy": 55, "r": 20, "fill": "success"}, map[string]any{"type": "path", "d": "M 100 90 Q 180 5 260 90", "fill": "none", "stroke": "primary", "strokeWidth": 2}}
		case "range":
			props["bind"], props["min"], props["max"], props["default"], props["step"] = "amount", 1, 10, 3, 1
		case "input":
			props["bind"], props["default"] = "entry", "Ada"
		case "check":
			props["bind"], props["default"] = "approved", false
		case "select":
			props["bind"], props["default"] = "format", "text"
			props["options"] = []any{map[string]any{"value": "text", "label": text("文字", "Text")}, map[string]any{"value": "table", "label": text("表格", "Table")}}
		case "button":
			props["set"] = map[string]any{"entry": text("由原生按钮更新", "Updated by the native button")}
		case "form":
			props["children"] = []any{inline("field", "input", map[string]any{"label": text("表单草稿", "Form draft"), "bind": "entry", "default": "Ada"})}
			props["set"] = map[string]any{"approved": true}
		case "details":
			props["children"] = []any{inline("inside", "text", map[string]any{"text": child})}
		case "tabs":
			props["children"] = []any{inline("first", "text", map[string]any{"label": text("第一标签", "First tab"), "text": text("第一标签正文", "First tab body")}), inline("second", "text", map[string]any{"label": text("第二标签", "Second tab"), "text": text("第二标签正文", "Second tab body")})}
			labels = []string{text("第一标签", "First tab"), text("第二标签", "Second tab")}
		}
		op, _ := json.Marshal(map[string]any{"version": 1, "seq": 1, "op": "create", "id": "sample", "kind": kind, "props": props})
		cases = append(cases, richProductionCase{kind: kind, source: "```cindy-ui\n" + string(op) + "\n```", labels: labels})
		if kind == "chart" {
			props["variant"], props["showValues"], props["height"] = "line", true, 200
			op, _ = json.Marshal(map[string]any{"version": 1, "seq": 1, "op": "create", "id": "sample", "kind": kind, "props": props})
			cases = append(cases, richProductionCase{kind: "chart-line", source: "```cindy-ui\n" + string(op) + "\n```", labels: []string{label(kind)}})
		}
	}
	return cases
}

func TestNativeRichProductionKindsUseTheActualAnswerRenderer(t *testing.T) {
	f := newFakeChat(t)
	h, p, tt := setupWith(t, f, f.handle)
	open(t, h, p, tt, "c1")
	p.setSidebar(false)
	for _, width := range []int{1100, 560} {
		tt.SetSize(width, 900)
		for _, sample := range richProductionCases(i18n.Chinese, h.Env.Directory) {
			id := fmt.Sprintf("matrix-%d-%s", width, sample.kind)
			p.view.Messages = []model.Message{{ID: id, Role: "assistant", Content: model.TextContent(sample.source)}}
			p.ui.followLatest()
			settle(h, tt)
			for _, label := range sample.labels {
				if !tt.HasText(label) {
					t.Fatalf("%s at %d lost real component content %q: %q", sample.kind, width, label, tt.Texts())
				}
			}
		}
	}
}
