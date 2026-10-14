package resultui

import (
	"context"
	"errors"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSpeechButtonsAppearBesideEachContentAction(t *testing.T) {
	for _, theme := range []string{"light", "dark"} {
		for _, language := range []string{"zh-CN", "en-US"} {
			p := NewPanel("translate", language, theme)
			p.Original, p.Result = "Original text", "译文内容"
			p.Messages = []Message{{Role: "assistant", Text: "First answer"}, {Role: "user", Text: "Question"}, {Role: "assistant", Text: "Second answer"}}
			tt := ui.NewTester(p.View, 520, 900)
			for _, label := range []string{p.T("朗读原文", "Read original aloud"), p.T("朗读译文", "Read translation aloud"), p.T("朗读回复", "Read reply aloud")} {
				if _, ok := tt.Find(label); !ok {
					t.Fatalf("%s %s: missing %s", theme, language, label)
				}
			}
		}
	}
}

func TestSpeechActionsOwnTheirTextAndKeepCopyAtRightEdge(t *testing.T) {
	p := NewPanel("translate", "zh-CN", "light")
	p.Original, p.Result = "Full source\nincluding hidden preview lines", "**完整译文**"
	var gotKey, gotText string
	var gotMarkdown bool
	p.Actions.Speak = func(key, text string, markdown bool) { gotKey, gotText, gotMarkdown = key, text, markdown }
	tt := ui.NewTester(p.View, 520, 650)
	for _, tc := range []struct {
		label, copy, key, text string
		markdown               bool
	}{
		{"朗读原文", "复制原文", "original", p.Original, false},
		{"朗读译文", "复制译文", "result", p.Result, true},
	} {
		if err := tt.Click(tc.label); err != nil {
			t.Fatal(err)
		}
		if gotKey != tc.key || gotText != tc.text || gotMarkdown != tc.markdown {
			t.Fatalf("wrong speech target: %s %q %v", gotKey, gotText, gotMarkdown)
		}
		for _, width := range []int{320, 520, 1000} {
			tt.SetSize(width, 650)
			speak, found := tt.Find(tc.label)
			copy, copyFound := tt.Find(tc.copy)
			if !found || !copyFound || speak.X+speak.W > copy.X || copy.X+copy.W > float32(width) || speak.Y != copy.Y {
				t.Fatalf("actions overlap or leave the window at %d: speech=%+v copy=%+v", width, speak, copy)
			}
		}
	}
}

func TestSpeechReplyButtonsSwitchBetweenExactMessages(t *testing.T) {
	p := NewPanel("chat", "zh-CN", "dark")
	p.Source = "quick-assistant"
	p.Messages = []Message{{Role: "assistant", Text: "Answer one"}, {Role: "user", Text: "Question"}, {Role: "assistant", Text: "Answer two"}}
	var calls []string
	p.Actions.Speak = func(key, text string, markdown bool) {
		if !markdown {
			t.Fatal("reply must use rendered text")
		}
		calls = append(calls, key+":"+text)
		p.SpeechKey = key
	}
	tt := ui.NewTester(p.View, 520, 900)
	for range 2 {
		if err := tt.Click("朗读回复"); err != nil {
			t.Fatal(err)
		}
	}
	if strings.Join(calls, "|") != "reply-0:Answer one|reply-2:Answer two" {
		t.Fatal(calls)
	}
	if _, ok := tt.Find("停止朗读"); !ok {
		t.Fatal("active reply cannot be stopped")
	}
}

func TestSpeechStreamingAndEmptyContentAreDisabled(t *testing.T) {
	p := NewPanel("translate", "zh-CN", "light")
	calls := 0
	p.Actions.Speak = func(string, string, bool) { calls++ }
	tt := ui.NewTester(p.View, 520, 650)
	if err := tt.Click("朗读译文"); err != nil {
		t.Fatal(err)
	}
	p.ResultPending = &Message{Text: "Partial translation", State: "running"}
	p.Pending = &Message{Role: "assistant", Text: "Partial answer", State: "running"}
	p.Busy = true
	tt.Frame()
	if err := tt.Click("朗读译文"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("朗读回复"); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatal("empty/streaming text was spoken")
	}
	p.Pending.State, p.Busy = "stopped", false
	tt.Frame()
	if err := tt.Click("朗读回复"); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal("stopped draft cannot be read")
	}
}

func TestSpeechSwitchCancelAndStaleCompletionAcrossWindows(t *testing.T) {
	a := &Controller{Panel: NewPanel("ocr", "zh-CN", "light"), ctx: context.Background()}
	b := &Controller{Panel: NewPanel("translate", "zh-CN", "light"), ctx: context.Background()}
	defer a.stopSpeech()
	defer b.stopSpeech()
	first, firstEpoch := a.beginSpeech("result")
	second, secondEpoch := b.beginSpeech("original")
	if first.Err() != context.Canceled || second.Err() != nil || a.Panel.SpeechKey != "" {
		t.Fatal("switch did not cancel previous window")
	}
	a.completeSpeech(firstEpoch, errors.New("late error"))
	if b.Panel.SpeechKey != "original" || a.Panel.SpeechError != "" {
		t.Fatal("old completion altered current playback")
	}
	third, thirdEpoch := b.beginSpeech("result")
	b.completeSpeech(secondEpoch, nil)
	if second.Err() != context.Canceled || third.Err() != nil || b.Panel.SpeechKey != "result" {
		t.Fatal("same-window stale completion altered current playback")
	}
	b.completeSpeech(thirdEpoch, errors.New("network failure"))
	if third.Err() != context.Canceled || b.Panel.SpeechErrorKey != "result" || b.Panel.SpeechError == "" || speakingController != nil {
		t.Fatal("failure did not release playback or show feedback")
	}
	current, _ := b.beginSpeech("result")
	b.toggleSpeech("result", "text", false)
	if current.Err() != context.Canceled || b.Panel.SpeechKey != "" {
		t.Fatal("clicking active button did not stop")
	}
	current, _ = b.beginSpeech("reply-0")
	b.newChat()
	if current.Err() != context.Canceled {
		t.Fatal("new chat retained speech")
	}
}

func TestSpeechTextReadsContentWithoutMarkdownDestinations(t *testing.T) {
	raw := "# Heading\n\nHello **world** &amp; [link](https://example.com/private-token).\n\n- `value`\n\n```go\nx < y\n```\n\n```mermaid\ngraph TD; A-->B\n```"
	got := speechText(raw, true)
	if got != "Heading Hello world & link. value x < y" {
		t.Fatalf("spoken text: %q", got)
	}
	literal := "# Heading *literal* [name](address)\x00\n第二行"
	if got := speechText(literal, false); got != "# Heading *literal* [name](address) 第二行" {
		t.Fatalf("source was parsed as markup: %q", got)
	}
}

func TestSpeechShutdownWaitsForCancelledPlaybackCleanup(t *testing.T) {
	c := &Controller{Panel: NewPanel("ocr", "zh-CN", "light"), ctx: context.Background()}
	ctx, _ := c.beginSpeech("result")
	cleaned := make(chan struct{})
	speechWorkers.Add(1)
	go func() {
		defer speechWorkers.Done()
		<-ctx.Done()
		close(cleaned)
	}()
	ShutdownSpeech()
	select {
	case <-cleaned:
	default:
		t.Fatal("host shutdown returned before cancelled playback cleaned up")
	}
	if c.Panel.SpeechKey != "" || speakingController != nil {
		t.Fatal("shutdown retained speech ownership")
	}
}

// Optional images use the production view with public fixtures, including
// preparing/stopping/error states. No network or audio output in normal tests.
func TestSpeechVisualStates(t *testing.T) {
	for _, theme := range []string{"light", "dark"} {
		p := NewPanel("translate", "zh-CN", theme)
		p.Original, p.Result = "Read the complete original text aloud.", "点击朗读按钮，即可听取这一段译文。"
		p.Messages = []Message{{Role: "assistant", Text: "每条 AI 回复都有独立的朗读按钮。", Model: "示例模型"}}
		tt := ui.NewTester(p.View, 520, 660)
		for _, state := range []string{"idle", "loading", "playing", "error"} {
			p.SpeechKey, p.SpeechLoading = "", false
			if state == "loading" || state == "playing" {
				p.SpeechKey = "result"
				p.SpeechLoading = state == "loading"
			}
			if state == "error" {
				p.SpeechErrorKey, p.SpeechError = "result", "朗读失败，请检查网络和音频输出后重试。"
			}
			tt.Frame()
			if state == "loading" || state == "playing" {
				if _, ok := tt.Find("停止朗读"); !ok {
					t.Fatal("active speech has no stop action")
				}
			}
			if state == "loading" {
				button, _ := tt.Find("停止朗读")
				spinner, ok := tt.Find("正在准备语音")
				if !ok || spinner.X < button.X || spinner.X+spinner.W > button.X+button.W || spinner.Y < button.Y || spinner.Y+spinner.H > button.Y+button.H {
					t.Fatalf("loading indicator escaped its button: %+v %+v", button, spinner)
				}
			}
			if dir := os.Getenv("WINONE_SPEECH_LAYOUT_DIR"); dir != "" {
				if err := os.MkdirAll(dir, 0700); err != nil {
					t.Fatal(err)
				}
				f, err := os.Create(filepath.Join(dir, theme+"-"+state+".png"))
				if err != nil {
					t.Fatal(err)
				}
				err = png.Encode(f, tt.Image())
				if closeErr := f.Close(); err != nil || closeErr != nil {
					t.Fatal(err, closeErr)
				}
			}
		}
	}
}
