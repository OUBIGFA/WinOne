package resultui

import (
	"github.com/egoist/mygo/ui"
	"strings"
	"winone/internal/ai"
	"winone/internal/resultwindow"
)

type quickActionRequest struct {
	action   resultwindow.SelectionAction
	text     string
	accepted bool
}

func (c *Controller) quickAction(action resultwindow.SelectionAction) {
	if c.Panel.Busy {
		return
	}
	text := c.Panel.Prompt
	if strings.TrimSpace(text) == "" {
		text = c.quickInput
	}
	if strings.TrimSpace(text) == "" {
		c.Panel.Error = c.Panel.T("请先输入内容，再选择功能。", "Enter content before choosing an action.")
		return
	}
	if action.Kind == "copy" {
		c.copy(text)
		return
	}
	c.quickInput = text
	request := quickActionRequest{action: action, text: text}
	c.quickRequest = &request
	c.runQuickAction(request, false)
}
func (c *Controller) runQuickAction(request quickActionRequest, retry bool) {
	method, values, err := resultwindow.ActionRequest(request.action, request.text, c.Panel.Target, c.Panel.Language)
	if err != nil {
		c.Panel.Error = err.Error()
		return
	}
	c.Panel.ActionSelection = request.action.Selection
	c.Panel.ResultSelection = ai.Selection{}
	question := request.text
	if retry && request.accepted {
		question = ""
	}
	// Keep the input for switching Translate -> Explain on the original content.
	c.runChat(method, c.parameters(values), question, retry && request.accepted, true)
}
func (c *Controller) newChat() {
	c.clearAll()
}

// The request history is independent from visible replies. Restart keeps those
// replies as reference but the next provider request contains only new messages.
func (c *Controller) restartConversation() { c.resetConversation(false) }
func (c *Controller) clearAll()            { c.resetConversation(true) }
func (c *Controller) resetConversation(clearVisible bool) {
	c.KeepOpen()
	c.stopSpeech()
	c.cancel()
	c.history = nil
	c.quickInput = ""
	c.quickRequest = nil
	c.last = ""
	c.Initial.Text = ""
	c.Initial.State = nil
	c.Panel.ContextReset = true
	if clearVisible {
		c.Panel.Messages = nil
		c.Panel.Original = ""
		c.Panel.Result = ""
		c.Panel.ResultDetails = Message{}
		c.Panel.diagrams = nil
		c.Panel.SpeechKey = ""
		c.Panel.SpeechErrorKey = ""
		c.Panel.SpeechError = ""
		c.selectedText = ""
		c.selectionRead = ""
	}
	c.Panel.Pending = nil
	c.Panel.ResultPending = nil
	c.Panel.Prompt = ""
	c.Panel.Error = ""
	c.Panel.Notice = ""
	if !clearVisible {
		c.Panel.Notice = c.Panel.T("已重新开始，旧结果仅供参考", "Restarted; earlier results are reference only")
	}
	c.Panel.ActionSelection = ai.Selection{}
	c.Panel.ResultSelection = ai.Selection{}
	c.Panel.FocusComposer = true
}
func (p *Panel) quickActionLabel(action resultwindow.SelectionAction) string {
	if p.Language == "en-US" && action.EnglishName != "" {
		return action.EnglishName
	}
	return action.Name
}
func (p *Panel) quickToolbar(c *ui.Context, t *ui.Theme) {
	var actions []resultwindow.SelectionAction
	for _, action := range p.QuickActions {
		if action.Enabled {
			actions = append(actions, action)
		}
	}
	width, _ := c.Size()
	// Conversation controls float above this strip. Use its complete width for
	// quick actions so narrow windows do not reserve two empty side regions.
	count := max(1, int((min(width, 872)-24+4)/34))
	if len(actions) > count {
		count--
	}
	ui.Row(c).FillWidth().Height(42).Shrink(0).AlignItems(ui.Center).Label(p.T("快捷功能", "Quick actions")).Children(func() {
		ui.Row(c).Grow(1).MinWidth(0).AlignItems(ui.Center).Justify(ui.Center).Gap(4).Children(func() {
			for i, action := range actions {
				if i >= count {
					break
				}
				label := p.quickActionLabel(action)
				if selectionButton(c, action.Icon, label, p.Busy).Tooltip(label).Clicked() && p.Actions.Quick != nil {
					p.Actions.Quick(action)
				}
			}
			if len(actions) > count {
				more := iconButton(c, "chevron", p.T("更多功能", "More actions")).Disabled(p.Busy)
				if more.Clicked() {
					p.quickMore = !p.quickMore
				}
				ui.PopoverBase(c, more, &p.quickMore, func(panel *ui.Element) {
					panel.MinWidth(180).MaxWidth(max(180, width-48)).MaxHeight(280).Children(func() {
						ui.Scroll(c).Children(func() {
							ui.Column(c).Gap(2).Children(func() {
								for _, action := range actions[count:] {
									if ui.Button(c, p.quickActionLabel(action)).FillWidth().Disabled(p.Busy).Clicked() && p.Actions.Quick != nil {
										p.quickMore = false
										p.Actions.Quick(action)
									}
								}
							})
						})
					})
				})
			}
		})
	})
}

// ReceiveQuickInput fills a new chat or appends to an unsent draft. It never
// sends a message or discards the conversation on a selection handoff.
func (c *Controller) ReceiveQuickInput(text string) {
	if strings.TrimSpace(text) != "" {
		if strings.TrimSpace(c.Panel.Prompt) == "" {
			c.Panel.Prompt = text
		} else if c.Panel.Prompt != text {
			c.Panel.Prompt += "\n\n" + text
		}
		c.quickInput = text
	}
	c.Panel.FocusComposer = true
}
func (c *Controller) openQuickFromSelection(text string) {
	if c.Panel.Busy {
		return
	}
	ctx, epoch := c.begin("quick-assistant")
	input := resultwindow.SelectionHandoff{Session: c.Initial.ID, Text: text}
	go func() {
		err := c.Client.Call(ctx, "quick-assistant.fromSelection", input, nil)
		c.update(func() {
			c.completeRequest(epoch, func() { c.Window.Close() }, err)
			if epoch == c.sequence && c.Panel.Error != "" && c.tooltip != nil {
				c.Panel.Error += "\n" + c.Panel.T("请检查快捷助手状态后重试。", "Check Quick assistant settings and try again.")
				c.tooltip.showMessage(c.quickButtonID, c.Panel.Error)
			}
		})
	}()
}
