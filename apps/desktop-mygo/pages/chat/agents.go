package chat

import (
	"encoding/json"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"

	"winone/desktopmygo/chat/model"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

// agentEntry is one entry of a subagent's own conversation: the model
// messages it holds, as Pi Durable stores them.
type agentEntry struct {
	ID    json.RawMessage `json:"id"`
	Model []model.Message `json:"model"`
}

// agentHistory is a page of a subagent's conversation, oldest first;
// Cursor reads the page before it.
type agentHistory struct {
	Entries []agentEntry    `json:"entries"`
	Cursor  json.RawMessage `json:"cursor"`
}

// agentDialog follows the subagent whose messages are open. React source:
// features/chat-agents.tsx.
type agentDialog struct {
	name    string
	history *agentHistory
	err     string
	loading bool
	sending bool
	// read discards a page an older read or another child returns late.
	read int
	// seen is the status and finish time the history was read at; a change
	// reads it again, as a fast follow-up may end between snapshots.
	seen   string
	drafts map[string]string
}

func (p *Chat) agent(name string) *model.Subagent {
	for i := range p.view.Subagents {
		if p.view.Subagents[i].Name == name {
			return &p.view.Subagents[i]
		}
	}
	return nil
}

func agentEdge(a *model.Subagent) string {
	if a == nil {
		return ""
	}
	edge := a.Status
	if a.FinishedAt != nil {
		edge += ":" + number(*a.FinishedAt)
	}
	return edge
}

// openAgent shows a subagent's messages.
func (p *Chat) openAgent(name string) {
	d := &p.ui.agents
	d.name, d.history, d.err, d.loading, d.seen = name, nil, "", false, ""
	d.read++
}

// readAgent reads the newest page of the open subagent, or the page
// before cursor, which goes above what is shown.
func (p *Chat) readAgent(cursor json.RawMessage) {
	d := &p.ui.agents
	d.read++
	token, name := d.read, d.name
	d.loading, d.err = true, ""
	params := map[string]any{"id": p.activeID, "action": "read", "name": name}
	older := len(cursor) > 0 && string(cursor) != "null"
	if older {
		params["agentCursor"] = cursor
	}
	chatCall(p, "agents", params, func(page agentHistory, err error) {
		if token != d.read {
			return
		}
		d.loading = false
		if err != nil {
			d.err = err.Error()
			return
		}
		if older && d.history != nil {
			page.Entries = append(page.Entries, d.history.Entries...)
		}
		d.history = &page
	})
}

func (p *Chat) agentDialogView(c *ui.Context) {
	d := &p.ui.agents
	if d.name == "" {
		return
	}
	agent, paused := p.agent(d.name), isPaused(p.view)
	// Read at the child's committed status edges, never while paused.
	if edge := agentEdge(agent); !paused && edge != d.seen {
		d.seen = edge
		p.readAgent(nil)
	}
	if d.drafts == nil {
		d.drafts = map[string]string{}
	}
	draft := d.drafts[d.name]
	open := true
	confirmed := kit.Dialog(c, &open, kit.DialogOptions{
		Title: i18n.T("chat.agentMessages") + " · " + d.name, Error: d.err, Width: 720,
		Cancel: i18n.T("chat.close"), Confirm: i18n.T("chat.agentFollowUp"), Busy: d.sending,
		ConfirmDisabled: p.busy || paused || strings.TrimSpace(draft) == "",
		Body: func() {
			t := c.Theme()
			if agent != nil && agent.Model != "" {
				status := "chat.agentStatus_" + agent.Status
				if paused {
					status = "chat.taskWaitingForResume"
				}
				kit.Help(c, agent.Model+" · "+i18n.T(status))
			}
			if kit.GhostButton(c, i18n.T("chat.agentRefresh"), icons.RefreshCw, false).AlignSelf(ui.Start).
				Disabled(p.busy || paused || d.loading || d.sending).Clicked() {
				p.readAgent(nil)
			}
			ui.Scroll(c.Key("agent-history")).FillWidth().MinHeight(128).MaxHeight(420).Children(func() {
				ui.Column(c).FillWidth().Gap(12).Padding(4, 2).Children(func() {
					if h := d.history; h != nil && len(h.Cursor) > 0 && string(h.Cursor) != "null" {
						if kit.GhostButton(c, i18n.T("chat.loadOlder"), icons.ChevronUp, false).
							Disabled(d.loading || paused).AlignSelf(ui.Center).Clicked() {
							p.readAgent(h.Cursor)
						}
					}
					if d.loading {
						ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
							kit.Loader(c)
							ui.Text(c, i18n.T("chat.loading")).TextColor(t.TextMuted)
						})
					}
					if d.history != nil {
						for _, entry := range d.history.Entries {
							for i, m := range entry.Model {
								p.agentMessage(c, string(entry.ID)+":"+number(float64(i)), m)
							}
						}
					}
				})
			})
			ui.TextArea(c, &draft).FillWidth().MinHeight(72).Label(i18n.T("chat.agentMessage")).
				Placeholder(i18n.T("chat.agentMessage")).Disabled(p.busy || paused || d.sending)
			if len([]rune(draft)) > 100000 {
				draft = string([]rune(draft)[:100000])
			}
			d.drafts[d.name] = draft
		},
	})
	if confirmed {
		p.followUpAgent()
	}
	if !open && !d.sending {
		d.name = ""
		d.read++
	}
}

// followUpAgent sends the draft to the open subagent, then reads its
// conversation again.
func (p *Chat) followUpAgent() {
	d := &p.ui.agents
	name, text := d.name, d.drafts[d.name]
	if strings.TrimSpace(text) == "" {
		return
	}
	d.sending, d.err = true, ""
	chatCall(p, "agents", map[string]any{"id": p.activeID, "action": "follow_up", "name": name, "text": text}, func(_ json.RawMessage, err error) {
		d.sending = false
		if err != nil {
			d.err = err.Error()
			return
		}
		if d.drafts[name] == text {
			d.drafts[name] = ""
		}
		if d.name == name {
			p.readAgent(nil)
		}
	})
}

// agentMessage shows one message of a subagent: what it was asked, its
// answers, and the tools it used, folded to their names.
func (p *Chat) agentMessage(c *ui.Context, key string, m model.Message) {
	t := c.Theme()
	text := strings.TrimSpace(m.Text())
	switch m.Role {
	case "user":
		if text == "" {
			return
		}
		ui.Column(c.Key("agent-user:"+key)).AlignSelf(ui.End).MaxWidthPercent(85).Padding(8, 12).Radius(kit.PanelRadius).
			Background(t.Surface).Children(func() {
			ui.Text(c, text).Selectable().MinWidth(0)
		})
	case "assistant":
		if text != "" {
			ui.Column(c.Key("agent-answer:" + key)).FillWidth().Children(func() {
				markdown.View(c, text, p.answerOptions("agent:"+key)).FillWidth()
			})
		}
		for _, part := range m.Content.Parts {
			if part.Type == "toolCall" && part.Name != "" {
				kit.Help(c, i18n.T("chat.tool")+" · "+part.Name)
			}
		}
		if m.ErrorMessage != "" {
			kit.InlineError(c, m.ErrorMessage)
		}
	case "toolResult":
		if m.IsError && text != "" {
			kit.InlineError(c, m.ToolName+" · "+truncate(text, 400))
		}
	}
}
