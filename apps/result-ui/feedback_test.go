package resultui

import (
	"github.com/egoist/mygo/ui"
	"testing"
)

func TestResultFeedbackClosesOnlyTheNoticeAndKeepsDraftFailureAndWindow(t *testing.T) {
	for _, language := range []string{"zh-CN", "en-US"} {
		for _, theme := range []string{"light", "dark"} {
			p := NewPanel("ocr", language, theme)
			p.Source = "capture"
			p.Error = "Request failed"
			p.Prompt = "Unsent owner draft"
			p.Busy = true
			closed, retried := 0, 0
			p.Actions.Close = func() { closed++ }
			p.Actions.Retry = func() { retried++ }
			tt := ui.NewTester(p.View, 420, 700)
			if err := tt.Click(p.T("关闭错误提示", "Dismiss error")); err != nil {
				t.Fatal(err)
			}
			for range 4 {
				tt.Frame()
			}
			if tt.HasText(p.Error) || p.Error != "Request failed" || p.Prompt != "Unsent owner draft" || !p.Busy || closed != 0 || retried != 0 {
				t.Fatal("dismissal changed a request, draft, or window instead of feedback")
			}
			p.Error = "A different failure"
			tt.Frame()
			if !tt.HasText(p.Error) {
				t.Fatal("new error stayed dismissed")
			}
			p.setNotice(p.T("已复制", "Copied"))
			tt.Frame()
			if err := tt.Click(p.T("关闭提示", "Dismiss notice")); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			if tt.HasText(p.Notice) || closed != 0 {
				t.Fatal("notice did not close independently of window")
			}
			p.setNotice(p.T("已复制", "Copied"))
			tt.Frame()
			if !tt.HasText(p.Notice) {
				t.Fatal("new repeated action inherited the old notice dismissal")
			}
		}
	}
}

func TestResultComposerAndSpeechHintsHaveIndependentCloseControls(t *testing.T) {
	p := NewPanel("ocr", "en-US", "light")
	p.Prompt = "Keep the owner text"
	p.ShowComposerHint = true
	p.SpeechErrorKey, p.SpeechError = "result", "Speech failed"
	tt := ui.NewTester(p.View, 520, 700)
	if err := tt.Click("Dismiss composer hint"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if p.ShowComposerHint || p.Prompt != "Keep the owner text" || !tt.HasText("Speech failed") {
		t.Fatal("closing a composer hint changed its draft or unrelated speech feedback")
	}
	if err := tt.Click("Dismiss speech error"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tt.HasText("Speech failed") || p.SpeechError != "Speech failed" || p.SpeechErrorKey != "result" {
		t.Fatal("closing speech feedback changed the underlying failed operation")
	}
}
