package resultui

import (
	"github.com/egoist/mygo/ui"
	"testing"
	"time"
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
			if _, found := tt.Find(p.T("关闭错误提示", "Dismiss error")); found {
				t.Fatal("error hint has a close button")
			}
			tt.Advance(5 * time.Second)
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
			if _, found := tt.Find(p.T("关闭提示", "Dismiss notice")); found {
				t.Fatal("transient notices must not have a close button")
			}
			tt.Advance(4 * time.Second)
			if !tt.HasText(p.T("已复制", "Copied")) {
				t.Fatal("notice disappeared before five seconds")
			}
			p.setNotice(p.T("已复制", "Copied"))
			tt.Frame()
			tt.Advance(4 * time.Second)
			if !tt.HasText(p.Notice) {
				t.Fatal("repeated action inherited the old notice deadline")
			}
			tt.Advance(time.Second)
			if tt.HasText(p.T("已复制", "Copied")) || closed != 0 || p.Prompt != "Unsent owner draft" || !p.Busy || p.Error != "A different failure" {
				t.Fatal("notice expiry failed or changed the window, draft or request")
			}
		}
	}
}

func TestResultComposerAndSpeechHintsExpireWithoutChangingDraftOrFailure(t *testing.T) {
	p := NewPanel("ocr", "en-US", "light")
	p.Prompt = "Keep the owner text"
	p.ShowComposerHint = true
	p.SpeechErrorKey, p.SpeechError = "result", "Speech failed"
	tt := ui.NewTester(p.View, 520, 700)
	for _, label := range []string{"Dismiss composer hint", "Dismiss speech error"} {
		if _, found := tt.Find(label); found {
			t.Fatal("hint has a close button")
		}
	}
	tt.Advance(4 * time.Second)
	if !p.ShowComposerHint || !tt.HasText("Speech failed") {
		t.Fatal("hint expired early")
	}
	tt.Advance(time.Second)
	if p.ShowComposerHint || p.Prompt != "Keep the owner text" || tt.HasText("Speech failed") || p.SpeechError != "Speech failed" || p.SpeechErrorKey != "result" {
		t.Fatal("expiry changed draft or failed operation")
	}
}
