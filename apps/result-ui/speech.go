package resultui

import (
	"context"
	"errors"
	"strings"
	"sync"
	"unicode"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown/parser"
)

// All result windows share one main thread and one cancellable playback lane.
// The lane is held until native playback has stopped, including on cancellation.
var speakingController *Controller
var speechLane = make(chan struct{}, 1)
var speechWorkers sync.WaitGroup

// ShutdownSpeech runs on the main thread after the event loop ends. Waiting
// here lets the last window's cancelled player close its device and audio file
// before process exit; switching/closing individual windows stays nonblocking.
func ShutdownSpeech() {
	if speakingController != nil {
		speakingController.stopSpeech()
	}
	speechWorkers.Wait()
}

func (p *Panel) speechButton(c *ui.Context, key, text, label string, markdown, working bool) {
	icon, tip := "speak", p.T("在线朗读，将这段文字发送给微软。", "Read online. This text is sent to Microsoft.")
	active := p.SpeechKey == key
	if active {
		icon, label, tip = "stop", p.T("停止朗读", "Stop reading"), p.T("停止朗读", "Stop reading")
		if p.SpeechLoading {
			tip = p.T("正在准备语音，点击停止。", "Preparing speech. Click to stop.")
		}
	}
	disabled := !active && (working || strings.TrimSpace(text) == "")
	button := contentIconButton(c.Key("speech-"+key), label, disabled, func() {
		if active && p.SpeechLoading {
			ui.Spinner(c).Size(16, 16).Label(p.T("正在准备语音", "Preparing speech"))
		} else {
			ui.Icon(c, svg(icon)).Size(18, 18)
		}
	}).Tooltip(tip)
	if button.Clicked() && p.Actions.Speak != nil {
		p.Actions.Speak(key, text, markdown)
	}
}

func (p *Panel) speechFeedback(c *ui.Context, t *ui.Theme, key string) {
	if p.SpeechErrorKey == key && p.SpeechError != "" && !p.speechVisibility.dismissed {
		ui.Row(c.Key("speech-error-" + key)).FillWidth().Gap(6).AlignItems(ui.Start).Role(ui.RoleStatus).Children(func() {
			ui.Text(c, p.SpeechError).Grow(1).MinWidth(0).FontSize(em(t, sizeLabel)).LineHeight(1.5).TextColor(t.Danger)
			p.closeFeedback(c.Key("speech-error-close"), &p.speechVisibility, p.T("关闭朗读提示", "Dismiss speech error"))
		})
	}
}

func (c *Controller) beginSpeech(key string) (context.Context, uint64) {
	if speakingController != nil {
		previous := speakingController
		previous.stopSpeech()
		if previous != c && previous.Window != nil {
			previous.Window.Update(func() {})
		}
	}
	c.stopSpeech()
	ctx, cancel := context.WithCancel(c.ctx)
	c.speechCancel = cancel
	c.Panel.SpeechKey, c.Panel.SpeechLoading = key, true
	c.Panel.SpeechError, c.Panel.SpeechErrorKey = "", ""
	speakingController = c
	return ctx, c.speechEpoch
}

func (c *Controller) stopSpeech() {
	c.speechEpoch++
	if c.speechCancel != nil {
		c.speechCancel()
		c.speechCancel = nil
	}
	c.Panel.SpeechKey, c.Panel.SpeechLoading = "", false
	c.Panel.SpeechError, c.Panel.SpeechErrorKey = "", ""
	c.Panel.speechVisibility = feedbackVisibility{}
	if speakingController == c {
		speakingController = nil
	}
}

func (c *Controller) completeSpeech(epoch uint64, err error) {
	if epoch != c.speechEpoch {
		return
	}
	key := c.Panel.SpeechKey
	c.stopSpeech()
	if err != nil && !errors.Is(err, context.Canceled) {
		c.Panel.SpeechErrorKey = key
		c.Panel.SpeechError = c.Panel.T("朗读失败，请检查网络和音频输出后重试。", "Reading failed. Check your network and audio output, then retry.")
		if errors.Is(err, errSpeechPlayback) {
			c.Panel.SpeechError = c.Panel.T("无法播放语音，请检查音频输出后重试。", "Could not play speech. Check your audio output, then retry.")
		}
	}
}

// SpeechText is the shared Markdown-to-spoken-text projection; callers supply
// their own synthesis route and never inherit the result window's provider.
func SpeechText(raw string, markdown bool) string { return speechText(raw, markdown) }

func (c *Controller) toggleSpeech(key, raw string, markdown bool) {
	if c.Panel.SpeechKey == key {
		c.stopSpeech()
		return
	}
	text := speechText(raw, markdown)
	if text == "" {
		c.Panel.SpeechErrorKey, c.Panel.SpeechError = key, c.Panel.T("没有可朗读的文字。", "There is no text to read.")
		return
	}
	ctx, epoch := c.beginSpeech(key)
	speechWorkers.Add(1)
	go func() {
		defer speechWorkers.Done()
		err := readSpeech(ctx, text, func(loading bool) {
			c.update(func() {
				if c.speechEpoch == epoch {
					c.Panel.SpeechLoading = loading
				}
			})
		})
		c.update(func() { c.completeSpeech(epoch, err) })
	}()
}

// Read visible prose and code, not Markdown punctuation, link destinations or
// diagram source. Source/OCR text bypasses Markdown so literal symbols survive.
// The same parser as the Markdown view reads the text, so what is spoken is
// what the window shows.
func speechText(raw string, markdown bool) string {
	if markdown {
		document := parser.Parse(raw)
		var out strings.Builder
		var walk func(parent int)
		walk = func(parent int) {
			for i := range document.Children(parent) {
				switch block := document.Block(i); block.Kind {
				case parser.CodeBlock:
					if !strings.EqualFold(block.Language(), "mermaid") {
						out.WriteString(document.Lines(i))
					}
				case parser.HTMLBlock, parser.MathBlock:
					out.WriteString(document.Lines(i))
				case parser.Paragraph, parser.Heading, parser.TableCell:
					out.WriteString(parser.PlainText(document.Inlines(i)))
				default:
					walk(i)
				}
				out.WriteByte('\n')
			}
		}
		walk(0)
		raw = out.String()
	}
	// XML 1.0 rejects control characters that can occur in clipboard/OCR text.
	raw = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && !unicode.IsSpace(r) || r == '\uFFFE' || r == '\uFFFF' {
			return -1
		}
		return r
	}, raw)
	return strings.Join(strings.Fields(raw), " ")
}
