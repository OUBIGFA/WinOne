package resultui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

// blockSpacing is the one control point of the reading rhythm: the room
// above a block of kind next placed after one of kind prev. A reply sits
// close to the message it answers; the next question starts a new turn
// with more room, so a conversation reads in pairs.
func blockSpacing(prev, next string) float32 {
	switch {
	case prev == "":
		return 0
	case prev == "user" && next == "assistant":
		return 16
	case prev == "assistant" && next == "user":
		return 28
	case next == "status":
		return 12
	}
	return 24
}

// quickEmpty tells the Quick assistant's state before the first message.
func (p *Panel) quickEmpty() bool {
	return p.Source == "quick-assistant" && len(p.Messages) == 0 && p.Pending == nil && p.Study == nil
}

func (p *Panel) reading(c *ui.Context, t *ui.Theme, height float32) {
	const top, bottom float32 = 16, 24
	ui.Scroll(c.Key("results")).Grow(1).TrackScroll(&p.scroll).Padding(top, gutter, bottom, gutter).Children(func() {
		// Center the reading column, not the text. Narrow windows use all
		// available space; wide windows keep a comfortable line length.
		column := ui.Column(c).FillWidth().MaxWidth(readingMaxWidth).AlignSelf(ui.Center)
		if p.quickEmpty() {
			// The empty state stands in the middle of the reading area.
			column.MinHeight(max(0, height-top-bottom)).Justify(ui.Center)
		}
		column.Children(func() {
			previous := ""
			place := func(kind string, e ui.Element) {
				e.Margin(blockSpacing(previous, kind), 0, 0, 0)
				previous = kind
			}
			switch {
			case p.Source != "quick-assistant":
				// The initial result shares one region: it bleeds into the
				// gutter so its text keeps the reading column's edges.
				tn := toneFor(t, c.Preferences().HighContrast)
				region := ui.Column(c).Key("result-surface").Margin(0, -bleed).Padding(12, bleed, 14, bleed).Gap(12).Radius(10).Background(tn.region)
				region.Children(func() {
					if p.Source != "selection" && p.Mode == "translate" && p.Original != "" {
						p.original(c, t)
						ui.Divider(c)
					}
					p.result(c, t)
				})
				previous = "result"
			case p.quickEmpty():
				place("empty", ui.Column(c).Key("empty").FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, p.T("有什么可以帮你？", "How can I help?")).FontSize(em(t, sizeEmpty)).FontWeight(600).TextAlign(ui.Center)
					ui.Text(c, p.T("直接发送消息，或输入内容后选择翻译、解释等功能。", "Send a message, or enter content and choose Translate, Explain, or another action.")).
						FontSize(em(t, sizeLabel)).TextColor(t.TextMuted).TextAlign(ui.Center).LineHeight(1.5)
				}))
			}
			p.conversation(c, t, place)
			if p.Source == "quick-assistant" && p.Study != nil {
				place("assistant", ui.Column(c).FillWidth().Children(func() { p.studyView(c, t, p.Study) }))
			}
			if p.Busy && p.Pending == nil && p.ResultPending == nil {
				place("status", ui.Spinner(c).Size(16, 16).AlignSelf(ui.Start).Label(p.T("正在处理…", "Working…")))
			}
			if e := p.errorFeedback(c, t, "general"); e.Valid() {
				place("status", e)
			}
		})
	})
}

func (p *Panel) original(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Key("original-preview").FillWidth().Gap(4).Children(func() {
		ui.Row(c).FillWidth().MinHeight(iconButtonSize).AlignItems(ui.Center).Gap(4).Children(func() {
			ui.Text(c, p.T("原文", "Original")).FontSize(em(t, sizeMeta)).TextColor(t.TextMuted).Grow(1).MinWidth(0)
			p.speechButton(c, "original", p.Original, p.T("朗读原文", "Read original aloud"), false, false)
			if iconButton(c, "copy", p.T("复制原文", "Copy original"), false).Clicked() && p.Actions.Copy != nil {
				p.Actions.Copy(p.Original)
			}
		})
		// OCR hard wraps belong to the copied source, not to this two-line
		// preview. The source stays muted: the translation is the task.
		preview := strings.Join(strings.Fields(p.Original), " ")
		p.selectable(c, ui.Column(c).Key("original-body").FillWidth(), func() {
			ui.Text(c, preview).FillWidth().FontSize(em(t, sizeLabel)).TextColor(t.TextMuted).LineHeight(1.5).MaxLines(2).Ellipsis("…")
		})
		p.speechFeedback(c, t, "original")
	})
}

func (p *Panel) result(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Key("initial-result").FillWidth().Gap(8).Children(func() {
		if p.Study != nil {
			p.studyView(c, t, p.Study)
		}
		label, copyLabel, text := p.T("识别结果", "OCR result"), p.T("复制原文", "Copy original"), p.Original
		if p.Mode == "translate" {
			label, copyLabel, text = p.T("译文", "Translation"), p.T("复制译文", "Copy translation"), p.Result
			if p.Recognizing {
				label, copyLabel = p.T("识别原文", "Recognizing source"), p.T("复制原文", "Copy original")
			}
		}
		if text == "" && p.ResultPending != nil {
			text = p.ResultPending.Text
		}
		details := &p.ResultDetails
		if p.Source == "selection" {
			copyLabel, text = p.T("复制结果", "Copy result"), p.Result
			if p.ResultPending != nil {
				details = p.ResultPending
				if text == "" {
					text = details.Text
				}
			}
		}
		// The result's own actions share the right edge of every copy button.
		text = p.Study.displayAnalysis(text)
		ui.Row(c).Key("result-actions").FillWidth().MinHeight(iconButtonSize).AlignItems(ui.Center).Gap(4).Children(func() {
			switch {
			case p.Source == "selection" && p.Mode == "translate":
				p.translationModelPicker(c, t)
				ui.Spacer(c)
			case p.Source == "selection":
				p.messageIdentity(c, t, details).Grow(1)
			default:
				ui.Text(c, label).FontSize(em(t, sizeMeta)).TextColor(t.TextMuted).Grow(1).MinWidth(0)
			}
			if iconButton(c, "retry", p.T("重试", "Retry"), p.Busy).Clicked() && p.Actions.RetryResult != nil {
				p.Actions.RetryResult()
			}
			speechLabel := p.T("朗读原文", "Read original aloud")
			if p.Mode == "translate" {
				speechLabel = p.T("朗读译文", "Read translation aloud")
			}
			if p.Source == "selection" {
				speechLabel = p.T("朗读结果", "Read result aloud")
			}
			p.speechButton(c, "result", text, speechLabel, p.Mode == "translate" || p.Source == "selection", p.Busy && p.ResultPending != nil || p.Recognizing)
			if iconButton(c, "copy", copyLabel, text == "").Clicked() && p.Actions.Copy != nil {
				p.Actions.Copy(text)
			}
		})
		// How a capture is translated, its language and its model, sits
		// together below the label, quieter than the translation itself.
		if p.Source != "selection" && p.Mode == "translate" {
			ui.Row(c).Key("translation-options").FillWidth().Wrap().AlignItems(ui.Center).Gap(4).Margin(-4, 0, 0, -6).Children(func() {
				p.targetLanguage(c)
				p.translationModelPicker(c, t)
			})
		}
		if p.Result != "" {
			p.markdownView(c, "result-body", p.Study.displayAnalysis(p.Result), markdownResult, false)
		}
		if p.ResultPending != nil {
			if p.Result != "" {
				ui.Divider(c).Margin(4, 0)
				ui.Text(c, p.T("重新生成", "Regenerating")).FontSize(em(t, sizeMeta)).TextColor(t.TextMuted)
			}
			if p.ResultPending.Text != "" {
				if p.Recognizing && p.Mode == "translate" {
					p.selectable(c, ui.Column(c).Key("recognizing").FillWidth(), func() {
						ui.Text(c, strings.Join(strings.Fields(p.ResultPending.Text), " ")).FillWidth().FontSize(em(t, sizeLabel)).TextColor(t.TextMuted).LineHeight(1.5).MaxLines(2).Ellipsis("…")
					})
				} else {
					p.markdownView(c, "result-draft", p.Study.displayAnalysis(p.ResultPending.Text), markdownResult, p.Busy)
				}
			}
			p.draftStatus(c, t, p.ResultPending)
		}
		p.errorFeedback(c, t, "result")
		p.speechFeedback(c, t, "result")
	})
}

// errorFeedback keeps a request failure next to its draft or initial
// result. Errors without either context use the general area below the
// content. It returns the message, or no element in other areas.
func (p *Panel) errorFeedback(c *ui.Context, t *ui.Theme, area string) ui.Element {
	if p.Error == "" {
		return ui.Element{}
	}
	location := "general"
	if p.ResultPending != nil || len(p.Messages) == 0 && p.Pending == nil && p.Source != "quick-assistant" {
		location = "result"
	} else if p.Pending != nil {
		location = "reply"
	}
	if location != area || p.errorVisibility.dismissed {
		return ui.Element{}
	}
	return ui.Row(c.Key("error-" + area)).FillWidth().Gap(6).AlignItems(ui.Start).Role(ui.RoleStatus).Children(func() {
		p.selectable(c, ui.Column(c).Grow(1).MinWidth(0), func() {
			ui.Text(c, p.Error).FillWidth().FontSize(em(t, sizeLabel)).LineHeight(1.5).TextColor(t.Danger)
		})
	})
}

func (p *Panel) reasoning(c *ui.Context, t *ui.Theme, message *Message) {
	if message.Reasoning == "" || !p.reasoningVisible() {
		return
	}
	label := p.T("思考完成", "Thought")
	if message.ReasoningActive {
		label = p.T("正在思考…", "Thinking…") + " · " + thinkingDuration(time.Now().UnixMilli()-message.ReasoningStartedAt)
		c.After(500 * time.Millisecond)
	} else if message.State == "stopped" {
		label = p.T("思考已停止", "Thinking stopped")
	}
	if !message.ReasoningActive && message.ReasoningStartedAt > 0 {
		label = p.T("已思考 ", "Thought for ") + thinkingDuration(message.ReasoningDurationMs)
		if message.State == "stopped" {
			label = p.T("思考已停止 · ", "Thinking stopped · ") + thinkingDuration(message.ReasoningDurationMs)
		}
	}
	tn := toneFor(t, c.Preferences().HighContrast)
	ui.Column(c).Key("reasoning").Children(func() {
		parts := ui.CollapsibleBase(c.Key("reasoning-toggle"), &message.ReasoningOpen)
		// The trigger's face bleeds into the gutter so its text keeps the
		// answer's left edge; the column stretches it across both margins.
		trigger := parts.Trigger.Label(p.T("思考过程", "Reasoning")).MinHeight(26).AlignItems(ui.Center).Gap(6).Padding(2, 8).Margin(0, -8).Radius(6).TextColor(t.TextMuted).Cursor(ui.CursorPointer)
		if trigger.Hovered() {
			trigger.Background(tn.hover)
		}
		trigger.Children(func() {
			icon := "thinking-complete"
			if message.ReasoningActive || message.State == "stopped" {
				icon = "thinking"
			}
			ui.Icon(c, svg(icon)).Size(14, 14).Shrink(0)
			ui.Text(c, label).Grow(1).MinWidth(0).FontSize(em(t, sizeLabel))
			ui.Icon(c, svg("chevron")).Size(14, 14).Shrink(0).Rotate(-90 + 90*parts.Progress())
		})
		parts.Panel(func() {
			if message.thinkingInline == nil || message.thinkingInline.raw != message.Reasoning {
				message.thinkingInline = &thinkingDocument{message.Reasoning, thinkingTokens(message.Reasoning)}
			}
			width, height := c.Size()
			if message.ReasoningActive && message.thinkingScroll.Y >= message.thinkingScroll.MaxY-120 {
				message.thinkingScroll.Y = math.MaxFloat32
			}
			ui.Scroll(c.Key("thinking")).Label(p.T("思考内容", "Thinking content")).FillWidth().MaxHeight(min(float32(320), height*0.4)).TrackScroll(&message.thinkingScroll).Children(func() {
				ui.Row(c).FillWidth().Gap(12).Margin(4, 0, 0, 0).Padding(6, 0, 0, 0).Children(func() {
					ui.Box(c).Width(2).AlignSelf(ui.Stretch).Background(t.Border)
					p.selectable(c, ui.Column(c).Grow(1).MinWidth(0), func() {
						// Cindy's narrow presentation: only strong text and
						// inline code are formatted; the rest stays text.
						spans := make([]ui.Span, 0, len(message.thinkingInline.tokens))
						for _, token := range message.thinkingInline.tokens {
							span := ui.Span{Text: token.Value}
							if token.Kind == "code" {
								span.Font = monoFont
							} else {
								span.Italic = true
								if token.Kind == "strong" {
									span.Weight = 500
								}
							}
							spans = append(spans, span)
						}
						ui.RichText(c, spans...).FillWidth().FontSize(em(t, sizeLabel)).TextColor(t.TextMuted).LineHeight(1.6)
					})
				})
			})
			// One bounded follow-up sees the measured scroll range, including
			// systems with reduced motion where no animation supplies frames.
			if !message.thinkingMeasuredOpen || message.thinkingMeasuredText != message.Reasoning || message.thinkingMeasuredWidth != width || message.thinkingMeasuredHeight != height || message.thinkingMeasuredScale != c.Preferences().TextScale {
				message.thinkingMeasuredText = message.Reasoning
				message.thinkingMeasuredWidth = width
				message.thinkingMeasuredHeight = height
				message.thinkingMeasuredScale = c.Preferences().TextScale
				message.thinkingMeasuredOpen = true
				c.After(time.Millisecond)
			}
		})
		if !message.ReasoningOpen {
			message.thinkingMeasuredOpen = false
		}
	})
}

// This content stack owns the complete thinking-to-answer gap. Neither the
// thinking body nor the surrounding identity/action layout adds to it.
func (p *Panel) replyContent(c *ui.Context, t *ui.Theme, message *Message, key string, working bool) {
	ui.Column(c).Key("content").FillWidth().Gap(16).Children(func() {
		if message.Study != nil {
			p.studyView(c, t, message.Study)
		}
		p.reasoning(c, t, message)
		if message.Text != "" {
			p.markdownView(c, key, message.Study.displayAnalysis(message.Text), markdownReply, working)
		}
	})
}

func (p *Panel) draftStatus(c *ui.Context, t *ui.Theme, message *Message) {
	if message.State == "running" || message.State == "" {
		ui.Spinner(c).Size(16, 16).AlignSelf(ui.Start).Label(p.T("正在生成…", "Generating…"))
		return
	}
	label := p.T("正在生成…", "Generating…")
	if message.State == "stopped" {
		label = p.T("已停止 · 内容未完成", "Stopped · Incomplete")
	}
	if message.State == "failed" {
		label = p.T("生成中断 · 内容未完成", "Interrupted · Incomplete")
	}
	ui.Text(c, label).FontSize(em(t, sizeMeta)).TextColor(t.TextMuted)
}

func (p *Panel) replyActions(c *ui.Context, t *ui.Theme, message *Message, key string, retry bool) {
	// Copy and speech must use the same learning content the reader sees.
	text := message.Study.displayAnalysis(message.Text)
	// Keep actions after their answer and on a shared right edge. They remain
	// visible for keyboard users and each callback owns this exact message.
	ui.Row(c).Key("actions").FillWidth().Gap(4).Children(func() {
		ui.Spacer(c)
		if retry {
			if iconButton(c, "retry", p.T("重试回复", "Retry reply"), false).Clicked() && p.Actions.Retry != nil {
				p.Actions.Retry()
			}
		}
		p.speechButton(c, key, text, p.T("朗读回复", "Read reply aloud"), true, message.State == "running" || key == "pending-reply" && p.Busy)
		if iconButton(c, "copy", p.T("复制回复", "Copy reply"), text == "").Clicked() && p.Actions.Copy != nil {
			p.Actions.Copy(text)
		}
	})
	p.speechFeedback(c, t, key)
}

func (p *Panel) messageIdentity(c *ui.Context, t *ui.Theme, message *Message) ui.Element {
	// Callers choose horizontal growth. Grow on a direct Column child would
	// consume vertical space and could overlap the following answer.
	return ui.Row(c).Key("identity").MinWidth(0).MinHeight(24).AlignItems(ui.Center).Gap(6).Children(func() {
		if message.Model != "" || message.ProviderName != "" {
			p.providerImage(c, t, p.messageProviderIcon(message), 16)
		}
		if message.ProviderName != "" {
			ui.Text(c, message.ProviderName).SingleLine().Ellipsis("…").Tooltip(message.ProviderName).MinWidth(0).MaxWidthPercent(40).FontSize(em(t, sizeMeta)).TextColor(t.TextMuted)
			if message.Model != "" {
				ui.Text(c, "·").FontSize(em(t, sizeMeta)).TextColor(t.TextMuted).Shrink(0)
			}
		}
		ui.Text(c, message.Model).SingleLine().Ellipsis("…").Tooltip(message.Model).MinWidth(0).FontSize(em(t, sizeMeta)).TextColor(t.Text).Grow(1)
	})
}

func (p *Panel) conversation(c *ui.Context, t *ui.Theme, place func(string, ui.Element)) {
	tn := toneFor(t, c.Preferences().HighContrast)
	for index := range p.Messages {
		message := &p.Messages[index]
		if message.Role == "user" {
			place("user", ui.Column(c).Key(index).Label(p.T("用户", "User")).AlignSelf(ui.End).MaxWidthPercent(85).Gap(4).Children(func() {
				// The action belongs to this bubble; its marker follows the bubble's
				// left edge even when a short message is aligned to the right.
				if message.ActionIcon != "" {
					color := t.TextMuted
					if !c.Preferences().HighContrast {
						color = color.Alpha(0.65)
					}
					ui.Icon(c, svg(message.ActionIcon)).AlignSelf(ui.Start).Size(14, 14).TextColor(color).Label(p.T("快捷操作：", "Quick action: ") + message.ActionLabel).Tooltip(message.ActionLabel)
				}
				p.selectable(c, ui.Column(c).Label(p.T("用户消息气泡", "User message bubble")).Padding(10, 14).Background(tn.bubble).Radius(12), func() {
					ui.Text(c, message.Text).FontSize(em(t, sizeBody)).LineHeight(1.5)
				})
			}))
			continue
		}
		place("assistant", ui.Column(c).Key(index).FillWidth().Gap(8).Children(func() {
			p.messageIdentity(c, t, message).FillWidth().Margin(0, 0, 4, 0)
			p.replyContent(c, t, message, fmt.Sprintf("reply-%d", index), false)
			p.replyActions(c, t, message, fmt.Sprintf("reply-%d", index), index == len(p.Messages)-1 && !p.Busy && p.Pending == nil)
		}))
	}
	if p.Pending != nil {
		place("assistant", ui.Column(c).Key("pending-reply").FillWidth().Gap(8).Children(func() {
			p.messageIdentity(c, t, p.Pending).FillWidth().Margin(0, 0, 4, 0)
			p.replyContent(c, t, p.Pending, "pending-reply", p.Busy)
			p.replyActions(c, t, p.Pending, "pending-reply", !p.Busy)
			p.draftStatus(c, t, p.Pending)
			p.errorFeedback(c, t, "reply")
		}))
	}
	if len(p.Messages) > 0 && p.Messages[len(p.Messages)-1].Role == "user" && !p.Busy && p.Pending == nil {
		retry := iconButton(c, "retry", p.T("重试回复", "Retry reply"), false).AlignSelf(ui.Start)
		place("status", retry)
		if retry.Clicked() && p.Actions.Retry != nil {
			p.Actions.Retry()
		}
	}
}

func (p *Panel) expandedReasoning() bool {
	if !p.reasoningVisible() {
		return false
	}
	if p.Pending != nil && p.Pending.ReasoningOpen && p.Pending.thinkingScroll.MaxY > 1 {
		return true
	}
	for _, message := range p.Messages {
		if message.ReasoningOpen && message.thinkingScroll.MaxY > 1 {
			return true
		}
	}
	return false
}
func (p *Panel) collapseReasoning() {
	if p.Pending != nil {
		p.Pending.ReasoningOpen = false
	}
	for i := range p.Messages {
		p.Messages[i].ReasoningOpen = false
	}
}
