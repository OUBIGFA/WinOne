package resultui

import (
	"math"
	"strings"

	"github.com/egoist/mygo/ui"
)

// promptHeight measures wrapped text with the same native font as the editor,
// including blank/trailing lines. Count native line boxes, then apply the
// editor's 1.5 line height; character-count estimates break for CJK and paths.
func (p *Panel) promptHeight(c *ui.Context, t *ui.Theme) float32 {
	width, _ := c.Size()
	width = max(1, min(width, readingMaxWidth+2*gutter)-2*composerInset-2*bleed-2)
	fontSize := em(t, sizeBody)
	_, line := c.MeasureText(0, ui.Span{Text: "Ag国", Size: fontSize})
	var lines float32
	for _, paragraph := range strings.Split(strings.ReplaceAll(p.Prompt, "\r\n", "\n"), "\n") {
		if paragraph == "" {
			lines++
			continue
		}
		_, height := c.MeasureText(width, ui.Span{Text: paragraph, Size: fontSize})
		lines += max(1, float32(math.Ceil(float64(height/line)-0.01)))
	}
	return lines * max(line, fontSize*1.5)
}

func (p *Panel) composer(c *ui.Context, t *ui.Theme) {
	// The region's width follows the reading column, so the input box's
	// face lines up with the result region's and its text with the replies'.
	// The top area owns all room between the divider and the box; never add
	// outer top padding, which would move the quick actions off center.
	ui.Column(c).Key("composer-region").Fill().MaxWidth(readingMaxWidth+2*gutter).AlignSelf(ui.Center).Padding(0, composerInset, composerInset, composerInset).Children(func() {
		top := ui.Column(c).Key("composer-top").FillWidth().MinHeight(composerInset).Shrink(0).Justify(ui.Center)
		if p.Source == "quick-assistant" {
			top.Height(quickToolbarHeight).Children(func() { p.quickToolbar(c, t) })
		}
		box := ui.Column(c).Key("composer").Grow(1).MinHeight(0).Label(p.T("消息输入框", "Message input box")).Padding(10, bleed, 8, bleed).Gap(8).Border(1, t.Border).Radius(12)
		box.Children(func() {
			input := ui.TextArea(c.Key("prompt"), &p.Prompt).Placeholder(p.T("围绕结果继续提问…", "Ask about this result…")).Label(p.T("继续提问", "Ask a follow-up")).
				FontSize(em(t, sizeBody)).LineHeight(1.5).Grow(1).MinHeight(24).Padding(0).Border(0, ui.Transparent).Background(ui.Transparent)
			if p.ContextReset {
				input.Placeholder(p.T("输入新一轮消息…", "Start a new conversation…"))
			}
			if p.Source == "quick-assistant" {
				input.Placeholder(p.T("输入消息…", "Message…")).Label(p.T("输入消息", "Message"))
			}
			if p.FocusComposer {
				input.Focus()
				p.FocusComposer = false
			}
			composing := input.Composing()
			if input.Changed() || composing {
				p.ShowComposerHint = false
				p.composerEdited = true
			}
			// Enter sends and Shift+Enter breaks the line. Keys an input
			// method composes with never reach this, so choosing a candidate
			// with Enter sends nothing. The draft stays until the request
			// starts; the controller clears it then.
			input.OnTextKey(func(k ui.TextKey) bool {
				if k.Key != ui.KeyEnter || k.Mods&ui.Shift != 0 {
					return false
				}
				// Also guard synthetic/accessibility key delivery, which can
				// bypass the platform IME's own key filtering.
				if composing {
					return true
				}
				if !p.Busy && strings.TrimSpace(k.Text()) != "" && p.Actions.Send != nil {
					p.Actions.Send(k.Text())
				}
				return true
			})
			if input.Focused() {
				if start, end := input.TextSelection(); start < end {
					runes := []rune(p.Prompt)
					p.selected = string(runes[min(start, len(runes)):min(end, len(runes))])
				}
			}
			if p.ShowComposerHint {
				p.composerHintVisibility.observe("composer")
				p.composerHintVisibility.update(c)
				p.ShowComposerHint = !p.composerHintVisibility.dismissed
			} else {
				p.composerHintVisibility.observe("")
			}
			if p.ShowComposerHint {
				ui.Row(c.Key("composer-hint")).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Text(c, p.T("Enter 发送，Shift+Enter 换行", "Enter to send, Shift+Enter for a new line")).Grow(1).MinWidth(0).FontSize(em(t, sizeMeta)).TextColor(t.TextMuted)
				})
			}
			ui.Row(c).Key("composer-actions").FillWidth().Height(iconButtonSize).AlignItems(ui.Center).Gap(4).Children(func() {
				p.modelPicker(c, t)
				ui.Box(c).Grow(1).MinWidth(0)
				if p.expandedReasoning() {
					if iconButton(c, "collapse", p.T("收起思考过程", "Collapse reasoning"), false).Clicked() {
						p.collapseReasoning()
					}
				}
				p.conversationControls(c, t)
				if p.Busy {
					if iconButton(c, "stop", p.T("停止", "Stop"), false).Clicked() && p.Actions.Cancel != nil {
						p.Actions.Cancel()
					}
					return
				}
				disabled := strings.TrimSpace(p.Prompt) == "" || composing
				send := ui.PrimaryButton(c, "").Label(p.T("发送", "Send")).Tooltip(p.T("发送", "Send")).Disabled(disabled).
					Size(iconButtonSize, iconButtonSize).Shrink(0).Padding(0).Radius(99).Justify(ui.Center).AlignItems(ui.Center).Cursor(ui.CursorPointer).
					Children(func() { ui.Icon(c, svg("send")).Size(16, 16) })
				if send.Clicked() && !disabled && p.Actions.Send != nil {
					p.Actions.Send(p.Prompt)
				}
			})
		})
		if box.FocusWithin() {
			box.Border(1, t.Focus)
		}
	})
}

// conversationControls are the two ways to start over. Both stay usable
// while a reply is being written: each cancels it.
func (p *Panel) conversationControls(c *ui.Context, t *ui.Theme) {
	restart := iconButton(c, "restart", p.T("重新开始", "Restart"), false).Tooltip(p.T("重新开始：清理上下文，保留旧结果供参考", "Restart with a fresh context; keep earlier results for reference"))
	if restart.Clicked() && p.Actions.Restart != nil {
		p.Actions.Restart()
	}
	clear := iconButton(c, "clear", p.T("清空全部", "Clear all"), false).Tooltip(p.T("清空全部：结果、输入及对话上下文", "Clear all results, input and conversation context"))
	if clear.Clicked() && p.Actions.ClearAll != nil {
		p.Actions.ClearAll()
	}
}

func (p *Panel) targetLanguage(c *ui.Context) {
	codes := []string{"auto", "zh-CN", "zh-TW", "en", "ja", "ko"}
	labels := []string{p.T("智能互译", "Smart translate"), p.T("简体中文", "Chinese"), p.T("繁体中文", "Traditional Chinese"), "English", "日本語", "한국어"}
	selected := labels[0]
	for index, code := range codes {
		if code == p.Target {
			selected = labels[index]
		}
	}
	t := c.Theme()
	if ui.Select(c.Key("target-language"), &selected, labels).Label(p.T("目标语言", "Target language")).Disabled(p.Busy).MinWidth(0).MaxWidth(160).
		FontSize(em(t, sizeMeta)).TextColor(t.TextMuted).Border(0, ui.Transparent).Background(ui.Transparent).Changed() {
		for index, label := range labels {
			if selected == label {
				p.Target = codes[index]
				if p.Actions.Target != nil {
					p.Actions.Target(p.Target)
				}
				break
			}
		}
	}
}
