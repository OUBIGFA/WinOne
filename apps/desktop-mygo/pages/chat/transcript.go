package chat

import (
	"encoding/json"
	"fmt"
	"strings"

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
		row := ui.Column(c.Key(item.Kind + ":" + item.Key)).FillWidth().Gap(8)
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
}

// userMessage is the user's bubble: its text, attachments, and actions
// to copy, edit (the latest), retry and fork.
func (p *Chat) userMessage(c *ui.Context, m model.Message, latest bool) {
	t, pal := c.Theme(), kit.Colors(c)
	ui.Column(c).AlignSelf(ui.End).MaxWidthPercent(85).Gap(6).AlignItems(ui.End).Children(func() {
		bubble := ui.Column(c).Padding(10, 14).Radius(14).Background(pal.Selected).Gap(6)
		if m.Pending {
			bubble.Opacity(0.7)
		}
		bubble.Children(func() {
			text := m.Text()
			if text != "" {
				ui.Text(c, text).LineHeight(1.5).Selectable().MinWidth(0)
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
		ui.Row(c).Gap(2).Children(func() {
			if kit.IconButton(c, icons.Copy, i18n.T("chat.copy")).Clicked() {
				p.copy(m.Text())
			}
			if latest && m.ID != "" && kit.IconButton(c, icons.Pencil, i18n.T("chat.editMessage")).Disabled(p.busy || isPaused(p.view)).Clicked() {
				p.beginEdit(m)
			}
			if m.ID != "" && kit.IconButton(c, icons.RotateCcw, i18n.T("chat.retry")).Disabled(p.busy || p.view.Running || isPaused(p.view)).Clicked() {
				id := m.ID
				p.run(func(done func(error)) { p.retryMessage(id, done) })
			}
			if m.ID != "" && kit.IconButton(c, icons.GitBranch, i18n.T("chat.fork")).Disabled(p.busy || p.view.Running).Clicked() {
				id := m.ID
				p.run(func(done func(error)) { p.fork(id, done) })
			}
		})
		_ = t
	})
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

// responseView is a turn's reply: its work process, folded once the
// turn ends, its plan, and its answer with actions and the files it names.
func (p *Chat) responseView(c *ui.Context, r *model.Response, last bool) {
	// Plan calls are not action rows: the turn's plan shows once, as a
	// card at its first call, updated from the newest (Cindy's
	// workGroupsForResponse).
	var entries []model.Entry
	for _, e := range r.Entries {
		if e.Kind == "thinking" && !p.ai.reasoning() || e.Kind == "tool" && model.WorkflowFromTool(e.Tool) != nil {
			continue
		}
		entries = append(entries, e)
	}
	if len(entries) > 0 || r.Running && r.Answer == nil {
		p.workProcess(c, r, entries)
	}
	if plan := model.PlanOf(r); plan != nil {
		var card *ui.Handle
		if latest := p.ui.items.plan; latest != nil && latest.Anchor == plan.Anchor {
			p.ui.items.planAnchor = p.activeID + ":" + r.Key + ":" + plan.Anchor
			card = &p.ui.items.planCard
		}
		p.planCard(c, plan.Workflow, card)
	}
	if r.Answer != nil {
		p.answerView(c, *r.Answer, r.Running, r.Key)
		if !r.Running {
			ui.Row(c).Gap(2).Children(func() {
				if kit.IconButton(c, icons.Copy, i18n.T("chat.copy")).Clicked() {
					p.copy(r.Answer.Text())
				}
				if last && kit.IconButton(c, icons.RotateCcw, i18n.T("chat.retry")).Disabled(p.busy || p.view.Running || isPaused(p.view)).Clicked() {
					p.run(func(done func(error)) { p.retryMessage("", done) })
				}
				if r.Answer.ID != "" && kit.IconButton(c, icons.GitBranch, i18n.T("chat.fork")).Disabled(p.busy || p.view.Running).Clicked() {
					id := r.Answer.ID
					p.run(func(done func(error)) { p.fork(id, done) })
				}
				if label := modelSource(*r.Answer); label != "" {
					ui.Text(c, label).FontSize(kit.CaptionSize(c)).TextColor(c.Theme().TextMuted).Padding(0, 6).SingleLine().Ellipsis("…").MinWidth(0)
				}
			})
		}
	}
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
	text := m.Text()
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
	ui.Column(c.Key("answer:" + key)).FillWidth().Children(func() {
		o := p.answerOptions(key)
		var root ui.Element
		if streaming {
			root = markdown.Stream(c, text, o)
		} else {
			root = markdown.View(c, text, o)
		}
		root.FillWidth()
	})
	if m.ErrorMessage != "" {
		kit.InlineError(c, m.ErrorMessage)
	}
	if !streaming {
		p.answerFiles(c, key, text)
	}
}

// workProcess is the turn's work: thinking, commentary, tools and
// operations, folded under a summary with its time, open while running.
func (p *Chat) workProcess(c *ui.Context, r *model.Response, entries []model.Entry) {
	t, pal := c.Theme(), kit.Colors(c)
	if p.ui.expanded == nil {
		p.ui.expanded = map[string]bool{}
	}
	open, known := p.ui.expanded["process:"+r.Key]
	if !known {
		open = r.Running
	}
	summary := i18n.T("chat.processWorking")
	if !r.Running {
		summary = i18n.T("chat.processWorked", "duration", duration(r.DurationMs))
	}
	ui.Column(c).FillWidth().Gap(4).Children(func() {
		head := ui.ButtonBase(c).AlignSelf(ui.Start).Gap(6).AlignItems(ui.Center).Padding(4, 6).Radius(t.Radius).Label(summary).Expanded(open)
		head.Children(func() {
			if r.Running {
				ui.Spinner(c).Size(14, 14)
			}
			ui.Text(c, summary).FontSize(kit.HelpSize(c)).TextColor(t.TextMuted)
			chevron := icons.ChevronRight
			if open {
				chevron = icons.ChevronDown
			}
			ui.Icon(c, chevron).FontSize(14).TextColor(t.TextMuted)
		})
		if head.Clicked() {
			p.ui.expanded["process:"+r.Key] = !open
		}
		if !open {
			return
		}
		if p.ui.processBodies == nil {
			p.ui.processBodies = map[string]*ui.Handle{}
		}
		body := p.ui.processBodies[r.Key]
		if body == nil {
			body = &ui.Handle{}
			p.ui.processBodies[r.Key] = body
		}
		p.ui.openProcesses = append(p.ui.openProcesses, r.Key)
		ui.Column(c).FillWidth().Gap(6).Padding(4, 40, 4, 12).BorderWidth(0, 0, 0, 2).BorderColor(pal.Hairline).Bind(body).Children(func() {
			for _, e := range entries {
				e := e
				ui.Column(c.Key("entry:" + e.Key)).FillWidth().Children(func() { p.entryView(c, e, r.Running) })
			}
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
		p.disclosure(c, "thinking:"+e.Key, icons.Brain, i18n.T("chat.thinking"), "", func() {
			ui.Text(c, e.Part.Thinking).FontSize(kit.HelpSize(c)).TextColor(t.TextMuted).LineHeight(1.5).Selectable()
		})
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
	if head.Hovered() {
		head.Background(t.SurfaceHover)
	}
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
			ui.Text(c, i18n.T("chat.planTitle")).FontWeight(600).Grow(1)
			ui.Text(c, i18n.T("chat.planProgress", "completed", done, "total", len(w.Items))).FontSize(kit.HelpSize(c)).TextColor(t.TextMuted)
		})
		if w.Explanation != "" {
			kit.Help(c, w.Explanation)
		}
		planSteps(c, w.Items, p.view.Running && !isPaused(p.view))
	})
}
