package resultui

import (
	"strings"

	"github.com/egoist/mygo/ui"

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
// quickToolbar shows the Quick assistant's actions as MyGo's toolbar,
// centered between the divider and the input box (user requirement). The
// toolbar measures its buttons and moves those without room, from the
// last, into its "More" menu; Left and Right move among them.
func (p *Panel) quickToolbar(c *ui.Context, t *ui.Theme) {
	var actions []resultwindow.SelectionAction
	for _, action := range p.QuickActions {
		if action.Enabled {
			actions = append(actions, action)
		}
	}
	ui.Toolbar(c.Key("quick-actions"), func() {
		for _, action := range actions {
			label := p.quickActionLabel(action)
			if selectionButton(c, action.Icon, label, p.Busy).Tooltip(label).Clicked() && p.Actions.Quick != nil {
				p.Actions.Quick(action)
			}
		}
	}).FillWidth().Height(quickToolbarHeight).Padding(0).Gap(4).Justify(ui.Center).Label(p.T("快捷功能", "Quick actions"))
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
		var target resultwindow.WindowTarget
		err := c.Client.Call(ctx, "quick-assistant.fromSelection", input, &target)
		c.update(func() {
			// This result process owns the user's click, including when its
			// selection toolbar was shown inactive. Do not rely on Host focus.
			if err == nil {
				err = target.Activate()
			}
			c.completeRequest(epoch, func() { c.Window.Close() }, err)
			if epoch == c.sequence && c.Panel.Error != "" && c.tooltip != nil {
				c.Panel.Error += "\n" + c.Panel.T("请检查快捷助手状态后重试。", "Check Quick assistant settings and try again.")
				c.tooltip.showMessage(c.quickButtonID, c.Panel.Error)
			}
		})
	}()
}
