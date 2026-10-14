package resultui

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"math"
	"strings"
	"testing"
	"winone/internal/ai"
)

// A wide window should add balanced gutters, rather than leaving a narrow
// reading area at the left or stretching paragraphs across the entire screen.
func TestReadingColumnStaysCenteredAcrossResize(t *testing.T) {
	for _, mode := range []string{"ocr", "translate"} {
		p := NewPanel(mode, "zh-CN", "light")
		p.Original = "Original text"
		p.Result = "Readable result"
		p.Messages = []Message{{Role: "user", Text: "Question"}, {Role: "assistant", Text: "Answer", Model: "Model"}}
		tt := ui.NewTester(p.View, 420, 700)
		for _, width := range []int{420, 1200, 780, 320, 1200} {
			tt.SetSize(width, 700)
			result, ok := tt.Find(p.Result)
			if !ok || math.Abs(float64(result.X+result.W/2-float32(width)/2)) > 1 {
				t.Fatalf("%s at %d: reading area is not centered: %+v", mode, width, result)
			}
			if width == 1200 && (result.W < 720 || result.W > 920) {
				t.Fatalf("%s: wide-window reading area needs comfortable width and gutters: %+v", mode, result)
			}
			if width <= 780 && result.W < float32(width)-64 {
				t.Fatalf("%s at %d: reading area became too narrow: %+v", mode, width, result)
			}
			answer, _ := tt.Find("Answer")
			copy, _ := tt.Find("复制回复")
			if answer.X != result.X || math.Abs(float64(copy.X+copy.W-result.X-result.W)) > 1 {
				t.Fatalf("%s at %d: replies or actions left the reading column", mode, width)
			}
			input, _ := tt.Find("继续提问")
			send, _ := tt.Find("发送")
			if input.X < result.X-8 || send.X+send.W > result.X+result.W+8 {
				t.Fatalf("%s at %d: composer is detached from the reading column", mode, width)
			}
		}
	}
}

func TestOnlyTranslationOriginalIsLimitedToTwoLines(t *testing.T) {
	p := NewPanel("translate", "zh-CN", "light")
	p.Original = strings.Repeat("Original text with several words. ", 20)
	p.Result = strings.Repeat("Translated text continues completely. ", 20)
	tt := ui.NewTester(p.View, 400, 800)
	box, ok := tt.Find(strings.Join(strings.Fields(p.Original), " "))
	if !ok || box.H > 43 {
		t.Fatalf("original preview: %+v %v", box, ok)
	}
	result, ok := tt.Find(strings.TrimSpace(p.Result))
	if !ok || result.H <= box.H {
		t.Fatal("translation was truncated")
	}
	p.Mode = "ocr"
	p.Result = p.Original
	tt.Frame()
	ocr, ok := tt.Find(strings.TrimSpace(p.Result))
	if !ok || ocr.H <= 43 {
		t.Fatal("OCR result was truncated")
	}
}

func TestCopyActionsShareTheRightEdge(t *testing.T) {
	p := NewPanel("translate", "zh-CN", "light")
	p.Original = "Original"
	p.Result = "Translation"
	p.Messages = []Message{{Role: "user", Text: "Question"}, {Role: "assistant", Text: "Answer", Model: "model"}}
	tt := ui.NewTester(p.View, 420, 650)
	source, ok := tt.Find("复制原文")
	if !ok {
		t.Fatal("original copy missing")
	}
	for _, name := range []string{"复制译文", "复制回复"} {
		button, found := tt.Find(name)
		if !found || button.X+button.W != source.X+source.W {
			t.Fatalf("%s is not aligned with source copy", name)
		}
		tt.Move(button.X+button.W/2, button.Y+button.H/2)
		if tt.Cursor() != ui.CursorPointer {
			t.Fatalf("%s has the wrong pointer", name)
		}
	}
}

func TestLongProviderAndModelDoNotCoverReplyActions(t *testing.T) {
	p := NewPanel("ocr", "zh-CN", "light")
	p.Messages = []Message{{Role: "assistant", Text: "Answer", ProviderName: strings.Repeat("Long provider ", 10), Model: strings.Repeat("long-model-", 20)}}
	tt := ui.NewTester(p.View, 320, 500)
	provider, providerOK := tt.Find(p.Messages[0].ProviderName)
	model, modelOK := tt.Find(p.Messages[0].Model)
	retry, retryOK := tt.Find("重试回复")
	copy, copyOK := tt.Find("复制回复")
	answer, answerOK := tt.Find("Answer")
	if !providerOK || !modelOK || !retryOK || !copyOK || !answerOK || provider.W < 30 || model.W < 30 || provider.X+provider.W > model.X || model.X+model.W > 304 || retry.Y < answer.Y+answer.H || retry.X+retry.W > copy.X || copy.X+copy.W > 320 {
		t.Fatalf("long attribution overlaps actions: provider=%+v model=%+v retry=%+v copy=%+v", provider, model, retry, copy)
	}
	if answer.Y-max(provider.Y+provider.H, model.Y+model.H) < 14 {
		t.Fatalf("provider/model attribution touches the answer: provider=%+v model=%+v answer=%+v", provider, model, answer)
	}
}

// A layout move must keep each copy button bound to its own answer, including
// a partial streamed reply. Clicking a later answer must never copy the first.
func TestEveryReplyCopiesItsOwnAnswerBelowTheBody(t *testing.T) {
	p := NewPanel("translate", "zh-CN", "light")
	p.Original, p.Result = "Source", "Initial translation"
	p.Messages = []Message{
		{Role: "assistant", Text: "First complete answer", Model: "first"},
		{Role: "user", Text: "Please explain"},
		{Role: "assistant", Text: "Second complete answer", Model: "second"},
	}
	p.Busy = true
	p.Pending = &Message{Role: "assistant", Text: "Partial answer", Model: "third", State: "running"}
	var copied string
	p.Actions.Copy = func(value string) { copied = value }
	tt := ui.NewTester(p.View, 520, 1000)
	first, _ := tt.Find("First complete answer")
	button, ok := tt.Find("复制回复")
	gap := button.Y - first.Y - first.H
	if !ok || gap < 4 || gap > 16 {
		t.Fatalf("reply copy should follow the body: body=%+v copy=%+v", first, button)
	}
	for _, answer := range []string{"First complete answer", "Second complete answer", "Partial answer"} {
		body, found := tt.Find(answer)
		if !found {
			t.Fatalf("answer not rendered: %s", answer)
		}
		copied = ""
		tt.ClickAt(button.X+button.W/2, body.Y+body.H+gap+button.H/2)
		if copied != answer {
			t.Fatalf("copy for %q returned %q", answer, copied)
		}
	}
}

func TestDragHandleRemainsAvailableWhileTranslating(t *testing.T) {
	p := NewPanel("translate", "zh-CN", "light")
	p.Busy = true
	p.Original = "Waiting for translation"
	tt := ui.NewTester(p.View, 420, 420)
	grip, ok := tt.Find("拖动结果窗")
	if !ok {
		t.Fatal("drag handle disappeared while translating")
	}
	tt.Move(grip.X+grip.W/2, grip.Y+grip.H/2)
	if tt.Cursor() != ui.CursorGrab {
		t.Fatal("busy state disabled the drag handle")
	}
}

func TestManyModelsCanBeFilteredAndSelected(t *testing.T) {
	p := NewPanel("ocr", "zh-CN", "light")
	models := []string{}
	for i := 0; i < 100; i++ {
		models = append(models, fmt.Sprintf("model-%03d", i))
	}
	p.Configuration = ai.Configuration{Providers: []ai.Provider{{ID: "one", Name: "服务", Models: models}}, Tasks: map[string]ai.Selection{"default": {Provider: "one", Model: models[0]}}}
	tt := ui.NewTester(p.View, 420, 420)
	if err := tt.Click("切换供应商和模型"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("搜索供应商或模型"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("model-099")
	tt.Key(0, ui.KeyEnter)
	if p.Selection == nil || p.Selection.Model != "model-099" || p.ModelPickerOpen {
		t.Fatal("long model list could not be searched and selected")
	}
}

func TestWindowSelectionChangesOnlyTheTemporaryRoute(t *testing.T) {
	p := NewPanel("ocr", "zh-CN", "light")
	p.Configuration = ai.Configuration{Providers: []ai.Provider{{ID: "first", Name: "服务一", Models: []string{"fast", "precise"}}, {ID: "second", Name: "服务二", Models: []string{"default"}}}, Tasks: map[string]ai.Selection{"default": {Provider: "second", Model: "default"}}}
	tt := ui.NewTester(p.View, 500, 500)
	if err := tt.Click("切换供应商和模型"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("precise"); err != nil {
		t.Fatal(err)
	}
	if p.Selection.Model != "precise" || p.Configuration.Tasks["default"].Model != "default" || p.ModelPickerOpen {
		t.Fatal("model choice changed global defaults")
	}
}

func TestCopyOriginalRemainsIndependentFromChat(t *testing.T) {
	p := NewPanel("translate", "zh-CN", "light")
	p.Original = "原文，逐字保留。"
	p.Result = "Original text."
	var copied string
	p.Actions.Copy = func(text string) { copied = text }
	tt := ui.NewTester(p.View, 400, 280)
	if err := tt.Click("复制原文"); err != nil {
		t.Fatal(err)
	}
	if copied != p.Original {
		t.Fatal("original copy changed")
	}
	p.Messages = append(p.Messages, Message{Role: "assistant", Text: "A new answer"})
	tt.Frame()
	if err := tt.Click("复制原文"); err != nil {
		t.Fatal(err)
	}
	if copied != "原文，逐字保留。" || !tt.HasText("A new answer") {
		t.Fatal("chat replaced original or copy closed the window")
	}
}

func TestInitialResultCopyAndRetryStayIndependentOfConversation(t *testing.T) {
	for _, mode := range []string{"ocr", "translate"} {
		t.Run(mode, func(t *testing.T) {
			p := NewPanel(mode, "zh-CN", "light")
			p.Original, p.Result = "Original with\nexact line breaks", "Complete translation"
			if mode == "ocr" {
				p.Result = p.Original
			}
			p.Messages = []Message{{Role: "assistant", Text: "Later answer"}}
			var copied string
			resultRetries, answerRetries := 0, 0
			p.Actions.Copy = func(value string) { copied = value }
			p.Actions.RetryResult = func() { resultRetries++ }
			p.Actions.Retry = func() { answerRetries++ }
			tt := ui.NewTester(p.View, 520, 750)
			if err := tt.Click("复制原文"); err != nil {
				t.Fatal(err)
			}
			if copied != "Original with\nexact line breaks" {
				t.Fatal("source copy changed")
			}
			if mode == "translate" {
				if err := tt.Click("复制译文"); err != nil {
					t.Fatal(err)
				}
				if copied != "Complete translation" {
					t.Fatal("translation copied another message")
				}
			}
			if err := tt.Click("重试"); err != nil {
				t.Fatal(err)
			}
			if resultRetries != 1 || answerRetries != 0 {
				t.Fatal("result retry was routed to the conversation")
			}
			if err := tt.Click("复制回复"); err != nil {
				t.Fatal(err)
			}
			if copied != "Later answer" {
				t.Fatal("reply copy changed the result")
			}
		})
	}
}

func TestDraftAndIMEArePreservedAcrossResize(t *testing.T) {
	p := NewPanel("ocr", "zh-CN", "light")
	p.Original = "识别原文"
	p.Result = p.Original
	sent := ""
	p.Actions.Send = func(text string) { sent = text }
	tt := ui.NewTester(p.View, 400, 280)
	if err := tt.Click("继续提问"); err != nil {
		t.Fatal(err)
	}
	tt.Type("请解释")
	tt.Compose("这句话", 3)
	tt.Key(0, ui.KeyEnter)
	if sent != "" {
		t.Fatal("IME confirmation sent the message")
	}
	tt.Type("这句话")
	tt.Compose("", 0)
	tt.SetSize(640, 480)
	if p.Prompt != "请解释这句话" {
		t.Fatalf("draft changed: %q", p.Prompt)
	}
	if err := tt.Click("发送"); err != nil {
		t.Fatal(err)
	}
	if sent != p.Prompt {
		t.Fatal("message was not submitted")
	}
	for _, size := range [][2]int{{320, 260}, {800, 600}, {400, 280}} {
		tt.SetSize(size[0], size[1])
		if _, ok := tt.Find("复制原文"); !ok {
			t.Fatal("original-copy button disappeared")
		}
	}
}

func TestConversationRolesRemainVisuallySeparated(t *testing.T) {
	p := NewPanel("ocr", "zh-CN", "light")
	p.Original, p.Result = "Source", "Source"
	p.Messages = []Message{{Role: "user", Text: "请解释"}, {Role: "assistant", Text: "这是回答"}}
	tt := ui.NewTester(p.View, 420, 500)
	for _, width := range []int{420, 320, 700} {
		tt.SetSize(width, 500)
		user, userOK := tt.Find("请解释")
		assistant, assistantOK := tt.Find("这是回答")
		if !userOK || !assistantOK || user.X <= assistant.X+20 || user.Y >= assistant.Y {
			t.Fatal("conversation roles lost their distinct alignment and order")
		}
	}
}
