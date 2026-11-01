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
// process and answer, then the plan and the conversation's files.
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
	for index, item := range items {
		item := item
		ui.Column(c.Key(item.Kind + ":" + item.Key)).FillWidth().Gap(8).Children(func() {
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
	if p.view.Workflow != nil && len(p.view.Workflow.Items) > 0 {
		p.planCard(c, *p.view.Workflow)
	}
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
// turn ends, and its answer with actions.
func (p *Chat) responseView(c *ui.Context, r *model.Response, last bool) {
	entries := r.Entries
	if !p.ai.reasoning() {
		var kept []model.Entry
		for _, e := range entries {
			if e.Kind != "thinking" {
				kept = append(kept, e)
			}
		}
		entries = kept
	}
	if len(entries) > 0 || r.Running && r.Answer == nil {
		p.workProcess(c, r, entries)
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

// answerView is a reply's text as Markdown, streamed while its turn runs.
func (p *Chat) answerView(c *ui.Context, m model.Message, streaming bool, key string) {
	text := m.Text()
	if m.ErrorMessage != "" && strings.TrimSpace(text) == "" {
		kit.Notice(c, kit.Failure, "", m.ErrorMessage, nil)
		return
	}
	ui.Column(c.Key("answer:" + key)).FillWidth().Children(func() {
		o := p.markdownOptions(c)
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
}

// markdownOptions are the replies' Markdown: code wraps rather than
// scrolling sideways, and links open only web addresses.
func (p *Chat) markdownOptions(c *ui.Context) *markdown.Options {
	if p.ui.markdown == nil {
		p.ui.markdown = &markdown.Options{WrapCode: true, OnLink: func(destination string) {
			if safeLink(destination) {
				p.ui.services.OpenURL(destination)
			}
		}}
	}
	return p.ui.markdown
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
		if head.Hovered() {
			head.Background(t.SurfaceHover)
		}
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
		ui.Column(c).FillWidth().Gap(6).Padding(4, 0, 4, 12).BorderWidth(0, 0, 0, 2).BorderColor(pal.Hairline).Children(func() {
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
		root := markdown.View(c, e.Text, p.markdownOptions(c))
		root.FillWidth()
	case "tool":
		tool := e.Tool
		label := tool.Name
		detail := toolSummary(tool)
		icon := icons.Wrench
		if tool.IsError {
			icon = icons.CircleAlert
		}
		p.disclosure(c, "tool:"+e.Key, icon, label, detail, func() {
			if len(tool.Args) > 0 {
				ui.Text(c, i18n.T("chat.toolInput")).FontSize(kit.CaptionSize(c)).TextColor(t.TextMuted)
				data, _ := json.MarshalIndent(tool.Args, "", "  ")
				kit.Mono(c, string(data)).FontSize(kit.HelpSize(c)).Selectable().MaxHeight(240)
			}
			if result := resultText(tool.Result); result != "" {
				ui.Text(c, i18n.T("chat.toolOutput")).FontSize(kit.CaptionSize(c)).TextColor(t.TextMuted)
				out := kit.Mono(c, truncate(result, 20000)).FontSize(kit.HelpSize(c)).Selectable()
				if tool.IsError {
					out.TextColor(t.Danger)
				}
			}
		}, tool.Running)
	case "operation":
		op := e.Operation
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icons.Activity).FontSize(14).TextColor(t.TextMuted)
			ui.Text(c, i18n.T(operationLabels[op.Kind])).FontSize(kit.HelpSize(c)).TextColor(t.TextMuted)
			ui.Text(c, i18n.T("chat.operationStatus_"+op.Status)).FontSize(kit.CaptionSize(c)).TextColor(t.TextMuted)
		})
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

// toolSummary is the argument a tool row names: its command, path or
// query.
func toolSummary(tool model.Tool) string {
	for _, key := range []string{"command", "path", "file_path", "pattern", "query", "url"} {
		if v, ok := tool.Args[key].(string); ok && v != "" {
			return strings.Join(strings.Fields(v), " ")
		}
	}
	return ""
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

// planCard is the agent's plan: its steps and their state.
func (p *Chat) planCard(c *ui.Context, w model.Workflow) {
	t, pal := c.Theme(), kit.Colors(c)
	done := 0
	for _, item := range w.Items {
		if item.Status == "completed" {
			done++
		}
	}
	ui.Column(c).FillWidth().Padding(12, 14).Gap(8).Radius(10).Border(1, pal.Hairline).Background(pal.Panel).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icons.ListTodo).FontSize(15)
			ui.Text(c, i18n.T("chat.planTitle")).FontWeight(600).Grow(1)
			ui.Text(c, i18n.T("chat.planProgress", "completed", done, "total", len(w.Items))).FontSize(kit.HelpSize(c)).TextColor(t.TextMuted)
		})
		if w.Explanation != "" {
			kit.Help(c, w.Explanation)
		}
		for i, item := range w.Items {
			ui.Row(c.Key(fmt.Sprint("step-", i))).Gap(8).AlignItems(ui.Start).Children(func() {
				icon, color := icons.Circle, t.TextMuted
				switch item.Status {
				case "completed":
					icon = icons.CircleCheck
				case "in_progress":
					icon, color = icons.CircleDot, t.Text
				}
				ui.Icon(c, icon).FontSize(15).TextColor(color).Shrink(0).Margin(2, 0, 0, 0)
				text := ui.Text(c, item.Step).LineHeight(1.45).MinWidth(0)
				if item.Status == "completed" {
					text.TextColor(t.TextMuted)
				}
			})
		}
	})
}
