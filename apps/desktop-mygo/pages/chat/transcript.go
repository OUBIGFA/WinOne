package chat

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"winone/desktopmygo/chat/pluginhost"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"

	"winone/desktopmygo/chat/model"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

// turns shows the transcript rows: user messages, each turn's work
// process, plan and answer, then the plan when no turn shows one, and the
// conversation's files.
func (p *Chat) turns(c *ui.Context) {
	var messages []model.Message
	for _, m := range p.visibleMessages() {
		if !(m.Pending && m.Queued) {
			messages = append(messages, m)
		}
	}
	items := model.Project(messages, p.view.Tools, p.view.Running, p.view.Operations)
	if p.bot != nil {
		if p.botMode.learningAsked != p.activeID {
			p.botMode.learningAsked = p.activeID
			p.loadBotLearning()
		}
		p.botMode.timeline = buildBotTimeline(botTimelineRows(items), p.bot.UnreadBoundaryAt)
		p.botAcknowledge()
	}
	lastUser := ""
	for i := len(p.view.Messages) - 1; i >= 0; i-- {
		if p.view.Messages[i].Role == "user" {
			lastUser = p.view.Messages[i].ID
			break
		}
	}
	hasSavedUser := false
	for _, m := range p.view.Messages {
		if m.Role == "user" && m.ID != "" && !m.Pending {
			hasSavedUser = true
			break
		}
	}
	s := &p.ui.items
	p.ui.openProcesses = p.ui.openProcesses[:0]
	s.turnOrder = s.turnOrder[:0]
	if s.turnRows == nil {
		s.turnRows = map[string]*ui.Handle{}
	}
	// The latest turn's plan is the one the reminder floats; any turn's
	// plan replaces the conversation's plan card at the end.
	s.plan, s.planAnchor = currentPlan(items), ""
	inlinePlan := false
	for _, item := range items {
		inlinePlan = inlinePlan || item.Kind == "response" && model.PlanOf(item.Response) != nil
	}
	for index, item := range items {
		item := item
		row := ui.Column(c.Key(item.Kind + ":" + item.Key)).FillWidth().Gap(stepGap)
		if item.Kind == "user" && index > 0 {
			// A user message opens a round: more room above it than between
			// the parts of the round it belongs to.
			row.Margin(turnGap-stepGap, 0, 0, 0)
		}
		if item.Kind == "user" {
			// Each user message starts a round the scroll controls stop at.
			h := s.turnRows[item.Key]
			if h == nil {
				h = &ui.Handle{}
				s.turnRows[item.Key] = h
			}
			row.Bind(h)
			s.turnOrder = append(s.turnOrder, item.Key)
		}
		row.Children(func() {
			if p.bot != nil && item.Kind != "response" {
				p.botTimelineMarkers(c, item.Kind+":"+item.Key)
			}
			switch item.Kind {
			case "presentation":
				p.answerView(c, *item.Message, false, "")
			case "user":
				p.userMessage(c, *item.Message, item.Message.ID != "" && item.Message.ID == lastUser)
			default:
				p.responseView(c, item.Response, index == len(items)-1 && hasSavedUser)
			}
		})
	}
	if !inlinePlan && p.view.Workflow != nil && len(p.view.Workflow.Items) > 0 {
		p.planCard(c, *p.view.Workflow, nil)
	}
	p.conversationFiles(c, items)
	p.advanceMessageJump()
}

// userMessage is the user's bubble: its text, attachments, and actions
// to copy, edit (the latest), retry and fork.
func (p *Chat) userMessage(c *ui.Context, m model.Message, latest bool) {
	if p.botCardFor(c, m) {
		return
	}
	if e := p.ui.edit; e != nil && e.entry == m.ID {
		ui.Column(c).FillWidth().Children(func() { p.editInPlace(c) })
		return
	}
	root := ui.Column(c).AlignSelf(ui.End).MaxWidthPercent(85).Gap(2).AlignItems(ui.End)
	root.Children(func() {
		bubble := ui.Column(c).Padding(10, 16).Radius(16).Background(messageBackground(c)).Gap(6)
		if m.ID != "" {
			if p.ui.userWidths == nil {
				p.ui.userWidths = map[string]float32{}
			}
			// Measure the bubble itself, never its wider action row. The inline
			// editor keeps this width until the owner's text changes.
			p.ui.userWidths[m.ID] = bubble.Bounds().W
		}
		if m.Pending {
			bubble.Opacity(0.7)
		}
		bubble.Children(func() {
			if p.bot != nil && len(m.ReplyQuote) > 0 {
				var quote botReplyQuote
				if json.Unmarshal(m.ReplyQuote, &quote) == nil && quote.ClientID != "" {
					p.botQuoteBlock(c, quote, true)
				}
			}
			text := m.Text()
			if text != "" {
				ui.Text(c, text).FontSize(c.Theme().Rem(readingSize)).LineHeight(1.6).Selectable().MinWidth(0)
			}
			if len(m.Attachments) > 0 {
				ui.Row(c).Wrap().Gap(6).Children(func() {
					for _, f := range m.Attachments {
						attachmentChip(c, f, nil)
					}
				})
			}
		})
		if m.Pending {
			return
		}
		// Retain the row's space and keyboard focus while its icons are hidden.
		// Hovering the bubble or the row reveals the same stable hit targets.
		actions := ui.Row(c).Gap(2)
		if !root.Hovered() && !root.FocusWithin() {
			actions.Opacity(0)
		}
		actions.Children(func() {
			if messageAction(c, icons.Copy, i18n.T("chat.copy")).Clicked() {
				p.copy(m.Text())
			}
			if latest && m.ID != "" && messageAction(c, icons.Pencil, i18n.T("chat.editMessage")).Disabled(p.busy || isPaused(p.view)).Clicked() {
				p.beginEdit(m)
			}
			if m.ID != "" && messageAction(c, icons.RotateCcw, i18n.T("chat.retry")).Disabled(p.busy || p.view.Running || isPaused(p.view)).Clicked() {
				id := m.ID
				p.run(func(done func(error)) { p.retryMessage(id, done) })
			}
			if p.bot == nil && m.ID != "" && messageAction(c, icons.GitBranch, i18n.T("chat.fork")).Disabled(p.busy || p.view.Running).Clicked() {
				id := m.ID
				p.run(func(done func(error)) { p.fork(id, done) })
			}
			if p.bot != nil {
				p.botReplyButton(c, m)
				p.botThreadButton(c, m)
			}
		})
		if p.bot != nil {
			p.botThreadPreview(c, m)
		}
	})
	p.revealMessage(root, m)
}

func isPaused(s model.State) bool { return s.Paused != nil && *s.Paused }

// attachmentChip is a file of a message or of the draft; remove, when
// given, takes it off the draft.
func attachmentChip(c *ui.Context, f model.Attachment, remove func()) {
	t, pal := c.Theme(), kit.Colors(c)
	ui.Row(c.Key("file-"+f.Path)).Gap(6).AlignItems(ui.Center).Padding(4, 8).Radius(t.Radius).Border(1, pal.Hairline).Background(pal.Panel).MaxWidth(240).Children(func() {
		icon := icons.FileText
		if strings.HasPrefix(f.MimeType, "image/") {
			icon = icons.Image
		}
		ui.Icon(c, icon).FontSize(14).TextColor(t.TextMuted).Shrink(0)
		ui.Text(c, f.Name).FontSize(kit.HelpSize(c)).SingleLine().Ellipsis("…").MinWidth(0).Tooltip(f.Path)
		if remove != nil && kit.IconButton(c, icons.X, i18n.T("chat.removeAttachment")).Size(20, 20).Clicked() {
			remove()
		}
	})
}

// responseView preserves public prose between independent activity disclosures.
// Only the trailing activity run may be live; retries cannot alter its identity.
func (p *Chat) responseView(c *ui.Context, r *model.Response, last bool) {
	for _, segment := range workSegments(r, p.ai.reasoning()) {
		ui.Column(c.Key(segment.key)).FillWidth().Gap(4).Children(func() {
			switch {
			case segment.prose != nil:
				p.replyView(c, *segment.prose, false, r.Key+":"+segment.key, false, true)
			case segment.plan != nil:
				plan := segment.plan
				var card *ui.Handle
				if latest := p.ui.items.plan; latest != nil && latest.Anchor == plan.Anchor {
					p.ui.items.planAnchor = p.activeID + ":" + r.Key + ":" + plan.Anchor
					card = &p.ui.items.planCard
				}
				p.planCard(c, plan.Workflow, card)
			default:
				if p.bot != nil {
					p.botReply(c, false, func() { p.workProcess(c, segment.response, segment.entries) })
				} else {
					p.workProcess(c, segment.response, segment.entries)
				}
			}
		})
	}
	if r.Answer != nil {
		ui.Column(c.Key("final")).FillWidth().Gap(4).Children(func() { p.replyView(c, *r.Answer, r.Running, r.Key, last, false) })
	}
}

// A delivered intermediate reply uses exactly the final reply's Markdown,
// source, copy and fork controls, outside the work rail and folding state.
// quiet is a reply between two work processes: its controls keep their
// place and keyboard focus but show only while the pointer or the focus is
// on the reply, so a long round is not broken up by rows of icons.
func (p *Chat) replyView(c *ui.Context, message model.Message, running bool, key string, retry, quiet bool) {
	if p.bot != nil {
		p.botAnswer(c, message, running, key, retry)
		return
	}
	if !quiet {
		p.answerView(c, message, running, key)
		if !running {
			p.replyActions(c, ui.Row(c).Gap(2).AlignItems(ui.Center), message, retry)
		}
		return
	}
	// The controls hang below the reply's right end, out of the flow: the
	// next work process follows the text directly, and its summary, at the
	// left, stays clear of them.
	body := ui.Column(c.Key("reply-body")).FillWidth()
	body.Children(func() {
		p.answerView(c, message, running, key)
		if running {
			return
		}
		actions := ui.Row(c).Absolute().Right(0).Bottom(-24).Gap(2).AlignItems(ui.Center)
		if !body.Hovered() && !actions.Hovered() && !actions.FocusWithin() {
			actions.Opacity(0)
		}
		p.replyActions(c, actions, message, retry)
	})
}

// replyActions fills a reply's row of controls: copy, retry (the latest),
// fork, and the model that wrote it.
func (p *Chat) replyActions(c *ui.Context, actions ui.Element, message model.Message, retry bool) {
	actions.Children(func() {
		if messageAction(c, icons.Copy, i18n.T("chat.copy")).Clicked() {
			p.copy(message.Text())
		}
		if retry && messageAction(c, icons.RotateCcw, i18n.T("chat.retry")).Disabled(p.busy || p.view.Running || isPaused(p.view)).Clicked() {
			p.run(func(done func(error)) { p.retryMessage("", done) })
		}
		if message.ID != "" && messageAction(c, icons.GitBranch, i18n.T("chat.fork")).Disabled(p.busy || p.view.Running).Clicked() {
			id := message.ID
			p.run(func(done func(error)) { p.fork(id, done) })
		}
		if label := modelSource(message); label != "" {
			ui.Text(c, label).FontSize(kit.CaptionSize(c)).TextColor(c.Theme().TextMuted).Padding(0, 6).SingleLine().Ellipsis("…").MinWidth(0)
		}
	})
}

// modelSource names the provider and model that wrote a reply.
func modelSource(m model.Message) string {
	name := first(m.ModelName, m.Model)
	provider := first(m.ProviderName, m.Provider)
	switch {
	case provider != "" && name != "":
		return provider + " · " + name
	default:
		return name
	}
}

// answerView is a reply's text as Markdown, streamed while its turn runs,
// then, once written, the files it names.
func (p *Chat) answerView(c *ui.Context, m model.Message, streaming bool, key string) {
	if p.botCardFor(c, m) {
		return
	}
	if p.bot != nil && key == "" {
		p.botAnswer(c, m, streaming, "", false)
		return
	}
	text := m.Text()
	plugins := pluginhost.For(p.env)
	if !streaming {
		if plugins.RenderOverride(c, p.activeID, m.ID, text) {
			return
		}
		if value := plugins.Reply(p.activeID, m.ID, text); value != nil && value.Action == "rewrite" {
			text = value.Text
		}
	}
	p.answerBody(c, m, text, streaming, key)
}

// lastAnswer says m is the conversation's latest assistant message.
func (p *Chat) lastAnswer(m model.Message) bool {
	for index := len(p.view.Messages) - 1; index >= 0; index-- {
		if p.view.Messages[index].Role == "assistant" {
			return p.view.Messages[index].ID == m.ID
		}
	}
	return false
}

// answerBody is a reply's text; a teammate's has its starter list taken out.
func (p *Chat) answerBody(c *ui.Context, m model.Message, text string, streaming bool, key string) {
	// A retrying transport attempt is not a terminal reply. Keep its error in
	// state, but show it only after the real run settles (as Cindy does).
	if streaming {
		m.ErrorMessage = ""
	}
	if m.ErrorMessage != "" && strings.TrimSpace(text) == "" {
		kit.Notice(c, kit.Failure, "", m.ErrorMessage, nil)
		return
	}
	if key == "" {
		key = "presentation:" + m.ID
		if m.ID == "" && m.Timestamp != nil {
			key += number(*m.Timestamp)
		}
	}
	bodyKey := key
	if m.Timestamp != nil {
		bodyKey = "message:" + number(*m.Timestamp)
	} else if m.ID != "" {
		bodyKey = "message:" + m.ID
	}
	text = quoteSpacedImagePaths(text)
	root := ui.Column(c.Key("answer:" + bodyKey)).FillWidth().Children(func() {
		// A reply becoming delivered prose in a later tool step keeps the same
		// Markdown/rich-form identity and the owner's local values.
		o := p.answerOptions(bodyKey)
		var root ui.Element
		if streaming {
			root = markdown.Stream(c, text, o)
		} else {
			root = markdown.View(c, text, o)
		}
		root.FillWidth()
		kit.InlineError(c, p.ui.items.errors["image:"+bodyKey])
	})
	p.revealMessage(root, m)
	if m.ErrorMessage != "" {
		kit.InlineError(c, m.ErrorMessage)
	}
	if !streaming {
		p.answerFiles(c, key, text)
	}
}

// workProcess keeps the latest four activities visible, live or completed.
// Expanding reveals history; collecting it never hides the entire process.
const processPreviewCount = 4

func (p *Chat) workProcess(c *ui.Context, r *model.Response, entries []model.Entry) {
	if len(entries) == 0 {
		return
	}
	t := c.Theme()
	if p.ui.expanded == nil {
		p.ui.expanded = map[string]bool{}
	}
	open := p.ui.expanded["process:"+r.Key]
	expandedStep := false
	for _, entry := range entries {
		expandedStep = expandedStep || p.ui.expanded[entry.Kind+":"+entry.Key]
	}
	canToggle := len(entries) > processPreviewCount || open || expandedStep
	shown := entries
	if !open && len(shown) > processPreviewCount {
		shown = shown[len(shown)-processPreviewCount:]
	}
	summary := i18n.T("chat.processWorking")
	if !r.Running {
		summary = i18n.T("chat.processWorked", "duration", duration(r.DurationMs))
	}
	if p.bot != nil {
		if label := teammateWorkLabel(entries); label != "" {
			summary = label
		}
	}
	process := ui.Column(c.Key("process:" + r.Key)).FillWidth().Gap(4)
	process.Children(func() {
		head := ui.ButtonBase(c).AlignSelf(ui.Start).Gap(6).AlignItems(ui.Center).Padding(4, 6).Radius(t.Radius).Label(summary)
		if canToggle {
			// Without more to show it is a caption: no state and no pointer.
			// The work process has no hover effects (docs/working-rules/ui.md).
			head.Expanded(open || expandedStep).Cursor(ui.CursorPointer)
		}
		head.Children(func() {
			if r.Running {
				ui.Spinner(c).Size(14, 14)
			} else {
				ui.Icon(c, icons.Layers).FontSize(14).TextColor(t.TextMuted)
			}
			ui.Text(c, summary).FontSize(kit.HelpSize(c)).TextColor(t.TextMuted)
			if canToggle {
				chevron := icons.ChevronRight
				if open {
					chevron = icons.ChevronDown
				}
				ui.Icon(c, chevron).FontSize(14).TextColor(t.TextMuted)
			}
		})
		if canToggle && head.Clicked() {
			if open || expandedStep {
				p.collapseProcess(r.Key)
			} else {
				p.ui.expanded["process:"+r.Key] = true
			}
		}
		if p.ui.processEntries == nil {
			p.ui.processEntries = map[string][]model.Entry{}
		}
		p.ui.processEntries[r.Key] = entries
		if p.ui.processBodies == nil {
			p.ui.processBodies = map[string]*ui.Handle{}
		}
		body := p.ui.processBodies[r.Key]
		if body == nil {
			body = &ui.Handle{}
			p.ui.processBodies[r.Key] = body
		}
		process.Bind(body)
		if open || expandedStep {
			p.ui.openProcesses = append(p.ui.openProcesses, r.Key)
		}
		// The right gutter is the floating collapse control's: it never
		// covers a step (process_collapse.go).
		// The indent is the wrapper's padding: a margin on a full-width
		// column would push its right edge into that gutter.
		ui.Column(c).FillWidth().Padding(0, 0, 0, 12).Children(func() {
			ui.Column(c).FillWidth().Gap(2).Padding(2, 40, 2, 8).BorderWidth(0, 0, 0, 1).BorderColor(t.Border).Children(func() {
				for _, entry := range shown {
					ui.Column(c.Key("entry:" + entry.Key)).FillWidth().Children(func() { p.entryView(c, entry, r.Running) })
				}
			})
		})
	})
}

// duration formats milliseconds as the work summary shows them.
func duration(ms *float64) string {
	if ms == nil {
		return "0s"
	}
	seconds := int(*ms / 1000)
	switch {
	case seconds < 60:
		return fmt.Sprintf("%ds", seconds)
	case seconds < 3600:
		return fmt.Sprintf("%dm %ds", seconds/60, seconds%60)
	}
	return fmt.Sprintf("%dh %dm", seconds/3600, seconds/60%60)
}

// entryView is one step of a work process.
func (p *Chat) entryView(c *ui.Context, e model.Entry, running bool) {
	t := c.Theme()
	switch e.Kind {
	case "thinking":
		key := "thinking:" + e.Key
		open := p.ui.expanded[key]
		row := ui.ButtonBase(c).FillWidth().Gap(6).AlignItems(ui.Start).Padding(4, 8).Radius(t.Radius).Label(i18n.T("chat.thinking")).Expanded(open)
		row.Children(func() {
			icon := icons.Sparkles
			if !running {
				icon = icons.Brain
			}
			ui.Row(c).Size(16, 21).Shrink(0).Justify(ui.Center).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, icon).FontSize(13).TextColor(t.TextMuted)
			})
			text := e.Part.Thinking
			if !open {
				text = strings.Join(strings.Fields(text), " ")
			}
			content := ui.Text(c, text).FontSize(kit.HelpSize(c)).TextColor(t.TextMuted).LineHeight(1.5).MinWidth(0).Grow(1)
			if !open {
				content.SingleLine().Ellipsis("…").Tooltip(text)
			}
			chevron := icons.ChevronRight
			if open {
				chevron = icons.ChevronDown
			}
			ui.Row(c).Size(13, 21).Shrink(0).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, chevron).FontSize(13).TextColor(t.TextMuted)
			})
		})
		if row.Clicked() {
			p.ui.expanded[key] = !open
		}
	case "commentary":
		key := "commentary:" + e.Key
		markdown.View(c, e.Text, p.answerOptions(key)).FillWidth()
		if !running {
			p.answerFiles(c, key, e.Text)
		}
	case "tool":
		p.toolRow(c, e)
	case "operation":
		p.operationRow(c, e.Operation)
	}
}

// operationLabels name the operations a work process lists.
var operationLabels = map[string]string{"compaction": "chat.operationCompaction", "review": "chat.operationReview",
	"approval": "chat.operationApproval", "retry": "chat.operationRetry"}

// disclosure is a step that opens to its details.
func (p *Chat) disclosure(c *ui.Context, key string, icon *ui.SVG, label, detail string, body func(), running ...bool) {
	t := c.Theme()
	open := p.ui.expanded[key]
	head := ui.ButtonBase(c).FillWidth().Justify(ui.Start).Gap(6).AlignItems(ui.Center).Padding(3, 6).Radius(t.Radius).Label(label).Expanded(open)
	head.Children(func() {
		if len(running) > 0 && running[0] {
			ui.Spinner(c).Size(14, 14)
		} else {
			ui.Icon(c, icon).FontSize(14).TextColor(t.TextMuted).Shrink(0)
		}
		ui.Text(c, label).FontSize(kit.HelpSize(c)).Shrink(0)
		if detail != "" {
			ui.Text(c, detail).FontSize(kit.HelpSize(c)).TextColor(t.TextMuted).SingleLine().Ellipsis("…").MinWidth(0).Shrink(1)
		}
	})
	if head.Clicked() {
		p.ui.expanded[key] = !open
	}
	if open {
		ui.Column(c).FillWidth().Gap(4).Padding(2, 6, 6, 26).Children(body)
	}
}

func resultText(result any) string {
	switch v := result.(type) {
	case nil:
		return ""
	case string:
		return v
	}
	data, _ := json.MarshalIndent(result, "", "  ")
	return string(data)
}

func truncate(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "\n…"
}

// planCard is the agent's plan: its steps and their state. bind, when
// given, is the card the plan reminder measures.
func (p *Chat) planCard(c *ui.Context, w model.Workflow, bind *ui.Handle) {
	t, pal := c.Theme(), kit.Colors(c)
	done := 0
	for _, item := range w.Items {
		if item.Status == "completed" {
			done++
		}
	}
	card := ui.Column(c).FillWidth().Padding(12, 14).Gap(8).Radius(10).Border(1, pal.Hairline).Background(pal.Panel).Label(i18n.T("chat.planTitle"))
	if bind != nil {
		card.Bind(bind)
	}
	card.Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icons.ListTodo).FontSize(15)
			ui.Text(c, i18n.T("chat.planTitle")).FontWeight(500).Grow(1)
			ui.Text(c, i18n.T("chat.planProgress", "completed", done, "total", len(w.Items))).FontSize(kit.HelpSize(c)).TextColor(t.TextMuted)
		})
		if w.Explanation != "" {
			kit.Help(c, w.Explanation)
		}
		planSteps(c, w.Items, p.view.Running && !isPaused(p.view))
	})
}

var spacedImagePath = regexp.MustCompile("!\\[([^\\]\\n]*)\\]\\(((?:[A-Za-z]:[\\\\/]|\\.{0,2}[\\\\/]|[^\\s()<>:]+[\\\\/])[^()<>\\n]* [^()<>\\n]*\\.(?:png|jpe?g|gif|webp|bmp|svg))\\)")
var windowsImagePath = regexp.MustCompile("(!\\[[^\\]\\n]*\\]\\()<?([A-Za-z]:[\\\\/][^()<>\\n]+)>?(\\))")

// quoteSpacedImagePaths brackets a local picture path holding spaces, as models
// often write one: CommonMark reads such a link only in its bracketed form.
func quoteSpacedImagePaths(text string) string {
	if !strings.Contains(text, "![") {
		return text
	}
	text = windowsImagePath.ReplaceAllStringFunc(text, func(image string) string {
		parts := windowsImagePath.FindStringSubmatch(image)
		// Backslashes are Markdown escapes even inside <dest>. Use the
		// equivalent Windows slash form before parsing, not a guessed path
		// after CommonMark has already consumed a separator such as \\_.
		return parts[1] + "<" + strings.ReplaceAll(parts[2], `\`, "/") + ">" + parts[3]
	})
	return spacedImagePath.ReplaceAllString(text, "![$1](<$2>)")
}
