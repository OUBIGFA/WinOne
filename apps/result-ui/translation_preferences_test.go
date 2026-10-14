package resultui

import (
	"github.com/egoist/mygo/ui"
	"testing"
	"winone/internal/ai"
	"winone/internal/resultwindow"
)

func TestTranslationModelChangeRespectsAutomaticPreference(t *testing.T) {
	for _, automatic := range []bool{true, false} {
		p := pickerPanel()
		p.Source = "selection"
		p.Action = "translate"
		p.Configuration.AutoRetranslate = &automatic
		calls := 0
		p.Actions.RetryResult = func() { calls++ }
		p.chooseTranslationModel(&ai.Selection{Provider: "two", Model: "deepseek-chat"})
		want := 0
		if automatic {
			want = 1
		}
		if calls != want {
			t.Fatalf("automatic=%v calls=%d", automatic, calls)
		}
		p.chooseTranslationModel(&ai.Selection{Provider: "two", Model: "deepseek-chat"})
		if calls != want {
			t.Fatal("same model retriggered translation")
		}
		if p.Selection != nil {
			t.Fatal("translation changed the conversation override")
		}
	}
}
func TestTranslationOverrideDoesNotAffectOCRorConversation(t *testing.T) {
	c := NewController(nil, resultwindow.Initial{ID: "session", Mode: "translate"})
	c.Panel.TranslationSelection = &ai.Selection{Provider: "translation", Model: "layout"}
	c.selection = &ai.Selection{Provider: "chat", Model: "reply"}
	if c.translationParameters(map[string]any{})["selection"] != *c.Panel.TranslationSelection {
		t.Fatal("translation override lost")
	}
	if c.parameters(map[string]any{})["selection"] != *c.selection {
		t.Fatal("translation changed chat/OCR")
	}
}
func TestQuickReasoningIsIndependentAndStartsCollapsed(t *testing.T) {
	p := NewPanel("chat", "zh-CN", "light")
	p.Source = "quick-assistant"
	disabled := false
	p.Configuration.ShowReasoning = &disabled
	p.Messages = []Message{{Role: "assistant", Text: "答案", Reasoning: "思考内容"}}
	tt := ui.NewTester(p.View, 520, 600)
	if !tt.HasText("思考过程") || tt.HasText("思考内容") {
		t.Fatal("quick reasoning must be independent and collapsed")
	}
	p.QuickReasoning = &disabled
	tt.Frame()
	if tt.HasText("思考过程") || !tt.HasText("答案") {
		t.Fatal("quick reasoning preference did not preserve answer")
	}
}
func TestPinAndDragHandleUseRequestedTitlebarPositions(t *testing.T) {
	for _, source := range []string{"capture", "selection", "quick-assistant"} {
		p := NewPanel("translate", "zh-CN", "light")
		p.Source = source
		tt := ui.NewTester(p.View, 520, 600)
		for _, size := range [][2]int{{520, 600}, {320, 300}, {900, 720}} {
			tt.SetSize(size[0], size[1])
			pin, _ := tt.Find("固定结果窗")
			grip, ok := tt.Find("拖动结果窗")
			close, _ := tt.Find("关闭")
			if !ok || pin.X >= 40 || pin.X >= grip.X || grip.X >= close.X {
				t.Fatal("pin or grip is misplaced", pin, grip)
			}
			title := "翻译"
			if source == "selection" {
				title = p.selectionTitle()
			}
			if source == "quick-assistant" {
				title = "快捷助手"
			}
			label, found := tt.Find(title)
			if !found || label.X < grip.X+grip.W || (grip.X+label.X+label.W)/2 != float32(size[0])/2 || grip.Y+grip.H/2 != resultHeaderHeight/2 {
				t.Fatalf("%s: drag/title group is not centered: grip=%+v title=%+v", source, grip, label)
			}
		}
	}
}

func TestResultActionsDoNotMoveOnHover(t *testing.T) {
	p := pickerPanel()
	tt := ui.NewTester(p.View, 520, 650)
	for _, label := range []string{"固定结果窗", "关闭", "重试", "复制译文", "朗读译文", "切换翻译模型", "切换供应商和模型"} {
		before, ok := tt.Find(label)
		if !ok {
			t.Fatalf("missing action: %s", label)
		}
		tt.Move(before.X+before.W/2, before.Y+before.H/2)
		after, _ := tt.Find(label)
		if before != after {
			t.Fatalf("%s moved on hover: %+v -> %+v", label, before, after)
		}
		tt.Move(0, 0)
	}
}

func TestTranslationModelEntryFitsNarrowResult(t *testing.T) {
	for _, source := range []string{"capture", "selection"} {
		p := pickerPanel()
		p.Source = source
		p.Action = "translate"
		p.ResultDetails = Message{ProviderName: "A long provider name for translation", Model: "A long translation model name"}
		tt := ui.NewTester(p.View, 320, 600)
		for _, label := range []string{"切换翻译模型", "重试", p.T("复制译文", "Copy translation")} {
			if source == "selection" && label == "复制译文" {
				label = "复制结果"
			}
			box, ok := tt.Find(label)
			if !ok || box.X < 0 || box.X+box.W > 320 {
				t.Fatalf("%s %s outside narrow window: %+v", source, label, box)
			}
		}
	}
}
