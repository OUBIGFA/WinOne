package resultui

import (
	"github.com/egoist/mygo/ui"
	"image/color"
	"strings"
	"testing"
)

func TestResultSurfaceDoesNotPaintASecondOuterFrame(t *testing.T) {
	for _, theme := range []string{"light", "dark"} {
		for _, scene := range []string{"ocr", "translate", "selection-toolbar"} {
			p := NewPanel("translate", "zh-CN", theme)
			controller := &Controller{Panel: p, Toolbar: scene == "selection-toolbar"}
			width, height := 420, 420
			if controller.Toolbar {
				width, height = controller.ToolbarDimensions()
			}
			if scene == "ocr" {
				p.Mode = "ocr"
			}
			tt := ui.NewTester(controller.View, width, height)
			background := color.RGBA{255, 255, 255, 255}
			if theme == "dark" {
				background = color.RGBA{23, 23, 23, 255}
			}
			for _, point := range [][2]int{{width / 2, 0}, {0, height / 2}, {width - 1, height / 2}, {width / 2, height - 1}, {0, 0}, {width - 1, 0}, {0, height - 1}, {width - 1, height - 1}} {
				if got := color.RGBAModel.Convert(tt.Image().At(point[0], point[1])); got != background {
					t.Fatalf("%s %s paints an inner frame at %v: %v", theme, scene, point, got)
				}
			}
		}
	}
}

func TestResultSurfaceKeepsBreathingRoomAroundItsContent(t *testing.T) {
	for _, theme := range []string{"light", "dark"} {
		p := NewPanel("ocr", "zh-CN", theme)
		p.Original, p.Result = "识别内容", "识别内容"
		tt := ui.NewTester(p.View, 420, 600)
		body, ok := tt.Find(p.Result)
		if !ok {
			t.Fatal("result body missing")
		}
		// Sample the empty gap immediately above the body. The surface begins
		// where the pixels differ from the window gutter, independent of its color.
		y := int(body.Y) - 4
		background := tt.Image().RGBAAt(0, y)
		left := 0
		for left < 420 && tt.Image().RGBAAt(left, y) == background {
			left++
		}
		if body.X-float32(left) < 16 {
			t.Fatalf("%s result content is too close to its surface edge: content=%+v surfaceLeft=%d", theme, body, left)
		}
	}
}

func TestSourcePreviewReflowsOCRLinesAcrossTheReadingWidth(t *testing.T) {
	p := NewPanel("translate", "zh-CN", "light")
	p.Original = strings.Repeat("A short OCR line from the selected image.\n", 12)
	p.Result = "The complete translated paragraph."
	tt := ui.NewTester(p.View, 420, 600)
	tt.SetSize(1100, 600)
	preview := strings.Join(strings.Fields(p.Original), " ")
	if !tt.HasText(preview) {
		t.Fatal("two-line source preview kept hard OCR line breaks instead of reflowing")
	}
	source, ok := tt.Find(preview)
	body, bodyOK := tt.Find(p.Result)
	if !ok || !bodyOK || source.W != body.W || source.X != body.X || source.H > 43 {
		t.Fatalf("source and result differ: source=%+v body=%+v", source, body)
	}
	var copied string
	p.Actions.Copy = func(value string) { copied = value }
	if err := tt.Click("复制原文"); err != nil {
		t.Fatal(err)
	}
	if copied != p.Original {
		t.Fatal("reflow changed the copied source")
	}
}

func TestInitialResultNeverDisplaysReasoningAndChatCanCollapseFromComposer(t *testing.T) {
	p := NewPanel("translate", "zh-CN", "light")
	p.Result = "译文"
	p.ResultDetails = Message{Reasoning: "初始翻译思考", ReasoningOpen: true}
	p.Messages = []Message{{Role: "assistant", Text: "回答", Reasoning: strings.Repeat("长思考内容。", 100), ReasoningOpen: true}}
	tt := ui.NewTester(p.View, 420, 440)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true, TextScale: 1})
	if tt.HasText("初始翻译思考") {
		t.Fatal("translation exposed reasoning")
	}
	p.FollowEnd()
	tt.Frame()
	button, ok := tt.Find("收起思考过程")
	if !ok || button.Y < 0 || button.Y+button.H > 440 {
		t.Fatal("reasoning cannot be collapsed while reading its end")
	}
	if err := tt.Click("收起思考过程"); err != nil {
		t.Fatal(err)
	}
	if p.Messages[0].ReasoningOpen {
		t.Fatal("fixed collapse action did not close reasoning")
	}
}

func TestConversationBottomKeepsSpaceAboveInputDivider(t *testing.T) {
	p := NewPanel("ocr", "zh-CN", "light")
	answer := strings.Repeat("完整回答需要舒适的阅读间距。", 100)
	p.Messages = []Message{{Role: "assistant", Text: answer}}
	tt := ui.NewTester(p.View, 520, 440)
	p.FollowEnd()
	tt.Frame()
	text, ok := tt.Find(answer)
	divider, dividerOK := tt.Find("Divider")
	if !ok || !dividerOK || divider.Y-(text.Y+text.H) < 24 {
		t.Fatalf("reply touches input divider: reply=%+v divider=%+v", text, divider)
	}
}

func TestFailureExplanationStaysWithItsResultOrReply(t *testing.T) {
	for _, theme := range []string{"light", "dark"} {
		for _, chat := range []bool{false, true} {
			p := NewPanel("ocr", "zh-CN", theme)
			p.Error = "服务连接失败，请重试。"
			anchor := "重试"
			if chat {
				p.Original, p.Result = "识别文字", "识别文字"
				p.Messages = []Message{{Role: "user", Text: "解释这句话"}}
				p.Pending = &Message{Text: "已收到的回复", State: "failed"}
				anchor = "生成中断 · 内容未完成"
			}
			tt := ui.NewTester(p.View, 420, 700)
			explanation, found := tt.Find(p.Error)
			control, controlFound := tt.Find(anchor)
			gap := explanation.Y - control.Y - control.H
			if !found || !controlFound || gap < 0 || gap > 16 {
				t.Fatalf("%s chat=%v error detached from its context: gap=%v", theme, chat, gap)
			}
		}
	}
}
