package chat

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"

	"winone/desktopmygo/chat/model"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/widgets"
)

// thinkingLevels are the levels the composer offers; "" follows the
// default.
var thinkingLevels = []string{"", "off", "minimal", "low", "medium", "high", "xhigh", "max"}

// inputArea is the footer of an open conversation: the plan reminder
// floating above it, the queue, the queue edit state, the retry state,
// the project, then the question, the approval or the composer, and the
// hint.
func (p *Chat) inputArea(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).FillWidth().MaxWidth(readingWidth+48).AlignSelf(ui.Center).Padding(0, 24, 12, 24).Gap(8).Shrink(0).Children(func() {
		p.planReminder(c, p.ui.items.plan)
		p.queuePanel(c)
		if p.queueEdit != nil {
			ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, i18n.T("chat.queue_editing")).FontSize(kit.HelpSize(c)).Grow(1)
				if kit.Button(c, i18n.T("chat.queue_cancelEdit"), nil, false).Disabled(p.busy).Clicked() {
					p.run(func(done func(error)) { p.finishQueueEdit(false, false, done) })
				}
			})
		}
		p.retryStatus(c)
		conversation := p.conversation()
		projectID := ""
		if conversation != nil {
			projectID = conversation.ProjectID
		}
		// A teammate's chat has no project of its own to switch (Cindy keeps it on
		// the teammate's settings).
		if p.bot == nil {
			p.projectContext(c, projectID, func(id string) {
				if id == "" || conversation == nil || conversation.ProjectID == id {
					return
				}
				target := conversation.ID
				p.run(func(done func(error)) { p.moveConversation(target, id, done) })
			})
		}
		p.botQuotePreview(c)
		if p.bot != nil {
			if p.bot.VoiceBlocked != nil {
				blocked := p.opening
				for _, approval := range p.view.Approvals {
					blocked = blocked || approval.Kind != "question"
				}
				p.bot.VoiceBlocked(blocked)
			}
			if p.bot.VoicePanel != nil {
				p.bot.VoicePanel(c)
			}
		}
		switch {
		case len(p.view.Approvals) > 0 && p.view.Approvals[0].Kind == "question":
			p.questionCard(c, p.view.Approvals[0])
		case len(p.view.Approvals) > 0:
			p.approvalCard(c, p.view.Approvals[0])
		default:
			p.composer(c)
		}
		hint := p.inputHint()
		ui.Text(c, hint).FontSize(kit.CaptionSize(c)).TextColor(t.TextMuted).SingleLine().Ellipsis("…").MinWidth(0).AlignSelf(ui.Center).Tooltip(hint)
	})
}

// permission is the open (or new) conversation's permission: its
// project's when it has one.
func (p *Chat) permission() model.Permission {
	conversation := p.conversation()
	if conversation != nil && conversation.OrcaRole == "worker" {
		return conversation.Permission
	}
	projectID := p.newProjectID
	if conversation != nil {
		projectID = conversation.ProjectID
	}
	if pr := p.catalog.project(projectID); pr != nil && pr.Permission != "" {
		return pr.Permission
	}
	if conversation != nil {
		return conversation.Permission
	}
	return p.newPermission
}

// currentModel is the open (or new) conversation's model.
func (p *Chat) currentModel() widgets.AISelection {
	selection := p.newModel
	if c := p.conversation(); c != nil {
		selection = c.Model
	}
	return resolveModel(p.ai, selection)
}

// composer is the message input and its tools, as the React ChatComposer.
func (p *Chat) composer(c *ui.Context) {
	t, pal := c.Theme(), kit.Colors(c)
	running := p.view.Running
	paused := isPaused(p.view)
	queueEditing := p.queueEdit != nil
	inputLocked := p.busy && !p.sendCleared || p.opening || p.draftLoading || p.reconnecting || p.transferBusy
	busy := p.busy || p.stopping || p.opening || p.draftLoading || p.reconnecting || p.transferBusy
	canSend := !isBlank(p.text) || len(p.files) > 0
	selection := p.currentModel()
	var provider *widgets.AIProvider
	if p.ai != nil {
		for i := range p.ai.Providers {
			pr := &p.ai.Providers[i]
			if pr.ID == selection.Provider && contains(pr.Models, selection.Model) {
				provider = pr
			}
		}
	}
	canSubmit := canSend && provider != nil
	mainIsStop := !queueEditing && running && (busy || !canSend)
	secondaryStop := running && canSend && !busy
	locked := busy || paused || inputLocked
	p.releasePasted()

	caret := p.botMode.mentionCaret
	mentionQuery, mentionRows := p.botMentionRows(caret)
	p.botMentionPanel(c, mentionQuery, caret, mentionRows)
	box := ui.Column(c.Key("composer")).FillWidth().Padding(16, 16, 12, 16).Gap(12).Radius(16).Border(1, t.Border).Background(pal.Panel).Bind(&p.ui.composerBox)
	// In a narrow composer the pills show their icons only, their names
	// as tooltips, so the tools keep one line.
	// Before the first layout use the compact form. All actions remain
	// present, and a narrow first frame cannot wrap then jump on repaint.
	p.ui.compact = p.ui.composerBox.Bounds(c).W < 600
	box.Children(func() {
		if len(p.files) > 0 {
			ui.Row(c).FillWidth().Wrap().Gap(6).Children(func() {
				for _, f := range p.files {
					path := f.Path
					var remove func()
					if !busy && !paused {
						remove = func() { p.updateText(p.text, model.RemoveAttachment(p.files, path), p.activeID) }
					}
					attachmentChip(c, f, remove)
				}
			})
		}
		placeholder := i18n.T("chat.placeholder")
		if p.bot != nil {
			placeholder = botComposerPlaceholder(p.bot.Name)
		}
		input := ui.TextArea(c.Key("message"), &p.text).Placeholder(placeholder).Label(i18n.T("chat.message")).
			LineHeight(1.5).FillWidth().MinHeight(48).MaxHeight(320).Padding(0).Border(0, ui.Transparent).Background(ui.Transparent).
			Disabled(inputLocked)
		if input.Changed() {
			p.updateText(p.text, p.files, p.activeID)
		}
		if p.bot != nil {
			if p.botMode.mentionMoved {
				input.SetTextSelection(p.botMode.mentionCaret, p.botMode.mentionCaret)
				p.botMode.mentionMoved = false
			} else if _, at := input.TextSelection(); at != p.botMode.mentionCaret {
				p.botMode.mentionCaret = at
				if query := activeBotMention(p.text, at); query == nil || query.query != p.botMode.mentionHidden {
					p.botMode.mentionHidden = ""
				}
			}
		}
		// Files and pictures pasted attach, as dropped ones do (paste.go).
		p.onPaste(input, locked)
		composing := input.Composing()
		input.OnTextKey(func(k ui.TextKey) bool {
			if !composing && p.botMentionKey(k.Key, mentionQuery, caret, mentionRows) {
				return true
			}
			// Enter sends, Shift+Enter (and Alt+Enter) break the line,
			// Ctrl+Enter steers into a running turn (Cindy's
			// resolveComposerEnterIntent). Keys an input method composes
			// with never send.
			if k.Key != ui.KeyEnter || k.Mods&(ui.Shift|ui.Alt) != 0 {
				return false
			}
			if composing {
				return true
			}
			p.text = k.Text()
			if canSendText(p.text, p.files) && provider != nil && !paused && !busy {
				if k.Mods&ui.Ctrl != 0 && running {
					p.requestSend(model.DeliverySteer)
				} else {
					p.requestSend("")
				}
			}
			return true
		})
		// A native URI/custom transfer is resolved by the plugin ledger; an
		// ordinary Explorer transfer still goes through the same addPaths path.
		if !p.pluginMediaDrop(c, box, locked) {
			widgets.DropArea(c, p.env, box, widgets.DropAny, true, locked, func(paths []string, err error) {
				if err != nil {
					p.err = err.Error()
					return
				}
				p.run(func(done func(error)) { p.addPaths(paths, done) })
			})
		}
		// Tools wrap within their side; the model and send keep theirs.
		ui.Row(c).FillWidth().AlignItems(ui.End).Gap(4).Children(func() {
			ui.Row(c).Gap(2).AlignItems(ui.Center).Grow(1).Basis(0).MinWidth(0).Wrap().Children(func() {
				plus := kit.IconButton(c, icons.Plus, i18n.T("chat.attachments")).Size(32, 32).Disabled(busy || paused)
				plus.Menu(func(m *ui.Menu) {
					if m.Item(i18n.T("chat.uploadAttachment")).Chosen() {
						p.run(p.pickFiles)
					}
					if m.Item(i18n.T("chat.insertFile")).Chosen() {
						p.openTools("files")
					}
					if m.Item(i18n.T("chat.toolsAvailable")).Chosen() {
						p.openTools("resources")
					}
				})
				p.permissionPill(c, busy || running || paused || queueEditing)
				p.thinkingPill(c, busy || running || paused || queueEditing)
				p.sessionTools(c, busy, running, paused, queueEditing)
				if paused && p.activeID != "" && kit.GhostButton(c, i18n.T("chat.resumeTask"), icons.Play, false).Disabled(p.busy).Clicked() {
					p.run(func(done func(error)) { p.simple("resume", map[string]any{"id": p.activeID}, done) })
				}
				p.subagents(c)
				if p.bot != nil && p.bot.StartVoice != nil && kit.IconButton(c, icons.AudioLines, botT("bots.voiceMode.startConversation")).Disabled(busy || paused).Clicked() {
					p.bot.StartVoice()
				}
			})
			ui.Row(c).Gap(4).AlignItems(ui.Center).Shrink(0).Children(func() {
				p.usageButton(c, busy)
				p.modelPicker(c, provider, selection, busy || running || paused || queueEditing)
				if secondaryStop && !paused {
					if stopButton(c).Disabled(p.stopping).Clicked() {
						p.stopCurrent()
					}
				}
				if mainIsStop {
					if stopButton(c).Disabled(p.stopping).Clicked() {
						p.stopCurrent()
					}
				} else {
					label := i18n.T("chat.send")
					if queueEditing {
						label = i18n.T("chat.queue_save")
					}
					send := ui.PrimaryButton(c, "").Label(label).Tooltip(label).Size(32, 32).Padding(0).Radius(99).Justify(ui.Center).AlignItems(ui.Center).
						Disabled(busy || paused || !canSubmit).Children(func() { ui.Icon(c, icons.ArrowUp).FontSize(16) })
					if send.Clicked() {
						p.requestSend("")
					}
				}
			})
		})
	})
	if box.FocusWithin() {
		box.Border(1, t.Focus)
	}
	if p.activeID == "" && p.saveLocation != "" {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icons.FolderOpen).FontSize(13).TextColor(t.TextMuted)
			ui.Text(c, p.saveLocation).FontSize(kit.CaptionSize(c)).TextColor(t.TextMuted).SingleLine().Ellipsis("…").MinWidth(0).Tooltip(p.saveLocation)
		})
	}
}

// pill is a composer choice: its icon and current value, the icon alone
// in a narrow composer.
func (p *Chat) pill(c *ui.Context, value string, icon *ui.SVG, label string) ui.Element {
	if p.ui.compact {
		return kit.IconButton(c, icon, label)
	}
	return kit.GhostButton(c, value, icon, false).Label(label).Tooltip(label)
}

func canSendText(text string, files []model.Attachment) bool { return !isBlank(text) || len(files) > 0 }

func contains(list []string, value string) bool {
	for _, v := range list {
		if v == value {
			return true
		}
	}
	return false
}

func stopButton(c *ui.Context) ui.Element {
	label := i18n.T("chat.stop")
	return ui.Button(c, "").Label(label).Tooltip(label).Size(32, 32).Padding(0).Radius(99).Justify(ui.Center).AlignItems(ui.Center).
		Children(func() { ui.Icon(c, icons.Square).FontSize(14) })
}

// permissionPill chooses how tools are approved; full access asks first.
func (p *Chat) permissionPill(c *ui.Context, disabled bool) {
	current := p.permission()
	labels := map[model.Permission]string{model.PermissionAsk: "chat.ask", model.PermissionAuto: "chat.auto", model.PermissionBypass: "chat.full", model.Permission("acceptEdits"): "chat.acceptEdits"}
	// The former React PermissionSelector used a distinct Lucide icon for
	// each mode; keep the same meaning in the native pill.
	modeIcons := map[model.Permission]*ui.SVG{model.PermissionAsk: icons.ShieldUser, model.PermissionAuto: icons.Sparkles, model.PermissionBypass: icons.TriangleAlert, model.Permission("acceptEdits"): icons.Pencil}
	label, icon := string(current), icons.ShieldUser
	if key := labels[current]; key != "" {
		label, icon = i18n.T(key), modeIcons[current]
	}
	pill := p.pill(c, label, icon, i18n.T("chat.permissions")+" · "+label).Disabled(disabled)
	pill.Menu(func(m *ui.Menu) {
		if current != model.PermissionAsk && current != model.PermissionAuto && current != model.PermissionBypass {
			m.Item(label).Checked(true).Disabled(true)
			m.Separator()
		}
		for _, mode := range []model.Permission{model.PermissionAsk, model.PermissionAuto, model.PermissionBypass} {
			if m.Item(i18n.T(labels[mode])).Checked(mode == current).Chosen() && mode != current {
				if mode == model.PermissionBypass {
					p.ui.confirmFull = true
					continue
				}
				chosen := mode
				p.run(func(done func(error)) { p.configure(nil, chosen, done) })
			}
		}
		if conversation := p.conversation(); conversation != nil && conversation.PluginID != "" {
			m.Separator()
			if m.Item(i18n.T("chat.retryPluginWriteAccess")).Chosen() {
				id := conversation.ID
				p.run(func(done func(error)) {
					chatCall(p, "ghosts.write-access-retry", map[string]string{"taskId": id}, func(value struct {
						Granted bool `json:"granted"`
					}, err error) {
						if err != nil || !value.Granted {
							done(err)
							return
						}
						p.refresh(func(err error) {
							if err == nil && p.activeID == id {
								p.reconnect()
							}
							done(err)
						})
					})
				})
			}
		}
	})
}

// thinkingPill chooses the thinking level and fast mode.
func (p *Chat) thinkingPill(c *ui.Context, disabled bool) {
	level, fast := p.newThinking, p.newFastMode
	if conv := p.conversation(); conv != nil {
		level, fast = conv.ThinkingLevel, conv.FastMode
	}
	key := "chat.thinkingAuto"
	if level != "" {
		key = "chat.thinking_" + level
	}
	pill := p.pill(c, i18n.T(key), icons.Brain, i18n.T("chat.thinkingLevel")+" · "+i18n.T(key)).Disabled(disabled)
	pill.Menu(func(m *ui.Menu) {
		for _, l := range thinkingLevels {
			name := "chat.thinkingAuto"
			if l != "" {
				name = "chat.thinking_" + l
			}
			if m.Item(i18n.T(name)).Checked(l == level).Chosen() && l != level {
				chosen := l
				p.run(func(done func(error)) { p.setThinking(chosen, done) })
			}
		}
		m.Separator()
		if m.Item(i18n.T("chat.fastMode")).Checked(fast).Chosen() {
			p.run(func(done func(error)) { p.setFast(!fast, done) })
		}
	})
}

// sessionTools are compaction, the chime, the task graph and pause.
func (p *Chat) sessionTools(c *ui.Context, busy, running, paused, queueEditing bool) {
	graph := p.view.TaskGraph
	canCompact := p.activeID != "" && len(p.view.Messages) > 0
	if !canCompact && (graph == nil || graph.Total == 0) && !running && !paused {
		// Only the chime: still offered, as the React page's tools were.
	}
	tools := kit.IconButton(c, icons.Ellipsis, i18n.T("chat.sessionTools"))
	tools.Menu(func(m *ui.Menu) {
		if p.activeID != "" && m.Item(i18n.T("chat.workdirPlugins")).Disabled(busy).Chosen() {
			p.openWorkdirPlugins()
		}
		if canCompact && m.Item(i18n.T("chat.compact")).Disabled(busy || running || paused || queueEditing).Chosen() {
			p.run(func(done func(error)) { p.simple("compact", map[string]any{"id": p.activeID}, done) })
		}
		sound := "chat.enableSound"
		if p.prefs.Sound {
			sound = "chat.disableSound"
		}
		if m.Item(i18n.T(sound)).Disabled(busy).Chosen() {
			p.prefs.Sound = !p.prefs.Sound
			p.prefs.save()
		}
		if p.activeID != "" && (running || paused || graph != nil && graph.Total > 0) {
			m.Separator()
			if graph != nil {
				for _, task := range graph.Tasks {
					state := task.State
					if key := "chat.taskState_" + state; i18n.Has(key) {
						state = i18n.T(key)
					}
					m.Item(fmt.Sprintf("#%v %s · %s", task.ID, task.Kind, state)).Disabled(true)
				}
			}
			if !paused && m.Item(i18n.T("chat.pauseTask")).Disabled(busy || !running).Chosen() {
				p.run(func(done func(error)) { p.simple("pause", map[string]any{"id": p.activeID}, done) })
			}
		}
	})
}

// subagents lists the conversation's agents with a way to stop each.
func (p *Chat) subagents(c *ui.Context) {
	if len(p.view.Subagents) == 0 {
		return
	}
	running := 0
	for _, a := range p.view.Subagents {
		if a.Status == "running" {
			running++
		}
	}
	label := i18n.T("chat.agents", "count", len(p.view.Subagents))
	b := kit.GhostButton(c, label, icons.Users, running > 0)
	b.Menu(func(m *ui.Menu) {
		paused := isPaused(p.view)
		for _, a := range p.view.Subagents {
			name := a.Name
			status := "chat.agentStatus_" + a.Status
			if paused {
				status = "chat.taskWaitingForResume"
			}
			text := name + " · " + i18n.T(status)
			// Each child opens its own messages; a running one can also stop.
			if m.Item(text + " · " + i18n.T("chat.agentMessages")).Disabled(p.busy || paused).Chosen() {
				p.openAgent(name)
			}
			if a.Status == "running" {
				if m.Item(name + " · " + i18n.T("chat.stopAgent")).Disabled(p.busy || paused).Chosen() {
					p.run(func(done func(error)) {
						p.simple("agents", map[string]any{"id": p.activeID, "action": "stop", "name": name}, done)
					})
				}
			}
		}
	})
}

// usageButton shows the context's use and the conversation's tokens.
func (p *Chat) usageButton(c *ui.Context, busy bool) {
	u, ctx := p.view.Usage, p.view.ContextUsage
	if u == nil || ctx == nil || ctx.ContextWindow <= 0 {
		return
	}
	percent := ctx.Tokens / ctx.ContextWindow * 100
	if ctx.Percent != nil {
		percent = *ctx.Percent
	}
	percent = min(100, max(0, percent))
	b := kit.IconButton(c, icons.Gauge, fmt.Sprintf("%s · %d%%", i18n.T("chat.contextUsage"), int(percent+0.5))).Disabled(busy)
	if b.Clicked() {
		p.ui.usageOpen = !p.ui.usageOpen
	}
	ui.Popover(c, b, &p.ui.usageOpen, func() {
		cost := i18n.T("chat.usageCostUnknown")
		if u.CostKnown {
			cost = fmt.Sprintf("%.4f", u.Cost)
		}
		kit.Facts(c, []kit.Fact{
			{Label: i18n.T("chat.contextUsage"), Value: compactCount(ctx.Tokens) + " / " + compactCount(ctx.ContextWindow)},
			{Label: i18n.T("chat.usageInput"), Value: compactCount(u.Input)},
			{Label: i18n.T("chat.usageOutput"), Value: compactCount(u.Output)},
			{Label: i18n.T("chat.usageCacheRead"), Value: compactCount(u.CacheRead)},
			{Label: i18n.T("chat.usageCacheWrite"), Value: compactCount(u.CacheWrite)},
			{Label: i18n.T("chat.usageTotal"), Value: compactCount(u.TotalTokens)},
			{Label: i18n.T("chat.usageCost"), Value: cost},
		})
	})
}

func compactCount(v float64) string {
	switch {
	case v >= 1_000_000:
		return fmt.Sprintf("%.1fM", v/1_000_000)
	case v >= 1_000:
		return fmt.Sprintf("%.1fk", v/1_000)
	}
	return fmt.Sprint(v)
}

// approvalCard asks to allow a tool: deny (Esc), allow once (Enter),
// allow for the conversation (Ctrl+Enter, when it has a scope).
func (p *Chat) approvalCard(c *ui.Context, a model.Approval) {
	t, pal := c.Theme(), kit.Colors(c)
	card := ui.Column(c.Key("approval-"+a.ID)).FillWidth().Padding(14).Gap(10).Radius(14).Border(1, t.Border).Background(pal.Panel).
		Label(i18n.T("chat.permissionTitle")).Focusable()
	// The card takes the focus as it appears, so its keys answer it
	// without a click, as the React card listened on the window.
	if p.ui.focusedApproval != a.ID {
		p.ui.focusedApproval = a.ID
		card.Focus()
	}
	card.Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
			ui.Text(c, i18n.T("chat.permissionTitle")).FontWeight(600).Grow(1)
			if stopButton(c).Disabled(p.busy).Clicked() {
				p.run(func(done func(error)) { p.simple("stop", map[string]any{"id": p.activeID}, done) })
			}
		})
		if a.AutoReviewUnavailable {
			kit.Notice(c, kit.Caution, "", i18n.T("chat.unavailable"), nil)
		}
		if a.Reason != "" {
			ui.Text(c, a.Reason).LineHeight(1.45)
		}
		command, _ := a.Args["command"].(string)
		if command == "" {
			data, _ := jsonIndent(a.Args)
			command = data
		}
		ui.Column(c).FillWidth().Gap(4).Children(func() {
			ui.Text(c, a.Tool).FontWeight(600)
			kit.Mono(c, command).FontSize(kit.HelpSize(c)).Selectable().MaxHeight(200)
		})
		if a.ScopeLabel != "" {
			p.disclosure(c, "scope:"+a.ID, icons.Info, i18n.T("chat.scope"), "", func() { kit.Mono(c, a.ScopeLabel).FontSize(kit.HelpSize(c)) })
		}
		ui.Row(c).FillWidth().Gap(8).Justify(ui.End).Wrap().Children(func() {
			if kit.Button(c, i18n.T("chat.deny")+"  Esc", nil, false).Disabled(p.busy).Clicked() {
				p.respondApproval(false, "once")
			}
			if a.ScopeLabel != "" && kit.Button(c, i18n.T("chat.allowSession")+"  Ctrl ↵", nil, false).Disabled(p.busy).Clicked() {
				p.respondApproval(true, "session")
			}
			if kit.PrimaryButton(c, i18n.T("chat.allowOnce")+"  ↵", nil, p.busy).Clicked() {
				p.respondApproval(true, "once")
			}
		})
	})
	if !p.busy {
		if card.Shortcut(0, ui.KeyEscape) {
			p.respondApproval(false, "once")
		} else if a.ScopeLabel != "" && card.Shortcut(ui.Ctrl, ui.KeyEnter) {
			p.respondApproval(true, "session")
		} else if card.Shortcut(0, ui.KeyEnter) {
			p.respondApproval(true, "once")
		}
	}
}

// questionCard asks the agent's questions: each answered by an option or
// in the user's words.
func (p *Chat) questionCard(c *ui.Context, a model.Approval) {
	t, pal := c.Theme(), kit.Colors(c)
	if p.ui.answers == nil || p.ui.answersFor != a.ID {
		p.ui.answers, p.ui.answersFor = make([]string, len(a.Questions)), a.ID
	}
	ui.Column(c.Key("question-"+a.ID)).FillWidth().Padding(14).Gap(12).Radius(14).Border(1, t.Border).Background(pal.Panel).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
			ui.Text(c, i18n.T("chat.questionTitle")).FontWeight(600).Grow(1)
			if stopButton(c).Disabled(p.busy).Clicked() {
				p.run(func(done func(error)) { p.simple("stop", map[string]any{"id": p.activeID}, done) })
			}
		})
		for i, q := range a.Questions {
			i := i
			ui.Column(c.Key(fmt.Sprint("q", i))).FillWidth().Gap(6).Children(func() {
				markdown.View(c, q.Question, p.answerOptions("question:"+a.ID+":"+fmt.Sprint(i))).FillWidth()
				if len(q.Options) > 0 {
					ui.Row(c).Wrap().Gap(6).Children(func() {
						for _, option := range q.Options {
							option := option
							b := kit.Button(c, option, nil, false).Disabled(p.busy)
							if p.ui.answers[i] == option {
								b.Background(pal.Selected)
							}
							if b.Clicked() {
								p.ui.answers[i] = option
							}
						}
					})
				}
				ui.TextArea(c, &p.ui.answers[i]).FillWidth().MinHeight(56).MaxHeight(160).Placeholder(i18n.T("chat.questionHint")).Label(q.Question).Disabled(p.busy)
			})
		}
		ui.Row(c).FillWidth().Gap(8).Justify(ui.End).Children(func() {
			if kit.Button(c, i18n.T("chat.questionCancel"), nil, false).Disabled(p.busy).Clicked() {
				p.respondQuestion(nil)
			}
			complete := true
			for _, answer := range p.ui.answers {
				complete = complete && strings.TrimSpace(answer) != ""
			}
			if kit.PrimaryButton(c, i18n.T("chat.questionSubmit"), nil, p.busy).Disabled(!complete).Clicked() {
				answers := append([]string(nil), p.ui.answers...)
				p.respondQuestion(answers)
			}
		})
	})
}

// VoiceQuestionPending reports whether the agent's question waits for an answer.
func (p *Chat) VoiceQuestionPending() bool {
	return len(p.view.Approvals) > 0 && p.view.Approvals[0].Kind == "question"
}

// ReceiveVoiceAnswer applies a spoken utterance to the first unanswered question,
// by Cindy's whole-utterance grammar (parseVoiceQuestionSelection). "Confirm"
// submits a complete card and "cancel" clears the choices; anything else stays
// with the caller as a draft, so speech never guesses an answer.
func (p *Chat) ReceiveVoiceAnswer(text string) bool {
	if !p.VoiceQuestionPending() {
		return false
	}
	a := p.view.Approvals[0]
	if p.ui.answers == nil || p.ui.answersFor != a.ID {
		p.ui.answers, p.ui.answersFor = make([]string, len(a.Questions)), a.ID
	}
	index := 0
	for index < len(p.ui.answers)-1 && strings.TrimSpace(p.ui.answers[index]) != "" {
		index++
	}
	if len(a.Questions) == 0 {
		return false
	}
	selection := parseVoiceQuestionSelection(text, a.Questions[index].Options)
	switch selection.kind {
	case "confirm":
		for _, answer := range p.ui.answers {
			if strings.TrimSpace(answer) == "" {
				return false
			}
		}
		p.respondQuestion(append([]string(nil), p.ui.answers...))
	case "cancel":
		clear(p.ui.answers)
	case "select":
		var labels []string
		for _, i := range selection.indices {
			labels = append(labels, a.Questions[index].Options[i])
		}
		p.ui.answers[index] = strings.Join(labels, ", ")
	default:
		return false
	}
	return true
}

// queuePanel lists the messages waiting for the running turn: edit,
// steer, cancel, reorder; resume when the queue is paused.
func (p *Chat) queuePanel(c *ui.Context) {
	t, pal := c.Theme(), kit.Colors(c)
	var queue []model.Message
	for _, m := range p.view.Messages {
		if m.Pending && m.Queued && !m.HiddenInput {
			queue = append(queue, m)
		}
	}
	if len(queue) == 0 {
		return
	}
	shown := queue
	if !p.queueOpen && len(queue) > 3 {
		shown = queue[:3]
	}
	paused := p.view.QueuePaused != nil && *p.view.QueuePaused
	if p.ui.queue.rows == nil {
		p.ui.queue.rows = map[string]ui.Rect{}
	}
	p.ui.queue.order = nil
	for _, message := range queue {
		p.ui.queue.order = append(p.ui.queue.order, first(message.RequestID, message.ID))
	}
	ui.Column(c).FillWidth().Padding(8).Gap(4).Radius(10).Border(1, pal.Hairline).Background(pal.Panel).Label(i18n.T("chat.queue_regionAria")).Children(func() {
		ui.Scroll(c).FillWidth().MaxHeight(196).TrackScroll(&p.ui.queue.scroll).Children(func() {
			for i, m := range shown {
				i, m := i, m
				entry := first(m.RequestID, m.ID)
				editing := p.queueEdit != nil && p.queueEdit.id == entry
				row := ui.Row(c.Key("queue-" + entry)).FillWidth().Gap(6).AlignItems(ui.Center).Focusable().Role(ui.RoleListItem)
				p.ui.queue.rows[entry] = row.Bounds()
				if drag := p.ui.queue.drag; drag != nil && drag.target == i {
					row.BorderWidth(1, 0, 0, 0).BorderColor(t.Focus)
				}
				row.Children(func() {
					handle := kit.IconButton(c, icons.GripVertical, i18n.T("chat.queue_dragAria", "index", i+1)).Disabled(p.busy || editing || p.queueWorking[entry]).HandleInput(p.queueDragInput(entry, i))
					p.queueReorderKey(handle, entry, i, len(queue))
					text := strings.TrimSpace(m.Text())
					if text == "" {
						text = i18n.T("chat.queue_noTextContent")
					}
					label := ui.Text(c, text).Grow(1).SingleLine().Ellipsis("…").MinWidth(0).Tooltip(text)
					if editing {
						label.FontWeight(600)
					}
					if n := len(m.Attachments); n > 0 {
						ui.Text(c, i18n.T("chat.queue_attachments", "count", n)).FontSize(kit.CaptionSize(c)).TextColor(t.TextMuted)
					}
					if editing {
						return
					} // The locked row's only edit actions belong to the composer.
					if kit.IconButton(c, icons.Pencil, i18n.T("chat.queue_editAria", "index", i+1)).Disabled(p.busy || p.queueEdit != nil || p.queueWorking[entry]).Clicked() {
						p.run(func(done func(error)) { p.beginQueueEdit(entry, done) })
					}
					steerLabel := i18n.T("chat.queue_steerAria", "index", i+1)
					if p.queueWorking[entry] {
						steerLabel = i18n.T("chat.queue_steeringAria", "index", i+1)
					}
					if kit.IconButton(c, icons.CornerDownRight, steerLabel).Disabled(p.busy || p.queueWorking[entry] || !p.view.Running).Clicked() {
						p.steerQueued(entry)
					}
					if kit.IconButton(c, icons.X, i18n.T("chat.queue_removeAria", "index", i+1)).Disabled(p.busy || p.queueWorking[entry]).Clicked() {
						p.queueAction("cancel", entry, nil, p.showError)
					}
				})
				if row.Shortcut(ui.Ctrl, ui.KeyEnter) && !editing && !p.busy && !p.queueWorking[entry] {
					p.steerQueued(entry)
				}
			}
		})
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			if len(queue) > 3 {
				label := i18n.T("chat.queue_showMore", "count", len(queue)-3)
				if p.queueOpen {
					label = i18n.T("chat.queue_showLess")
				}
				if kit.GhostButton(c, label, nil, false).Clicked() {
					p.queueOpen = !p.queueOpen
				}
			}
			ui.Spacer(c)
			if paused {
				ui.Text(c, i18n.T("chat.queue_pausedFooter")).FontSize(kit.CaptionSize(c)).TextColor(t.TextMuted)
				if kit.Button(c, i18n.T("chat.queue_resumeAction"), icons.Play, false).Label(i18n.T("chat.queue_resumeAria")).Disabled(p.busy).Clicked() {
					p.queueAction("resume", "", nil, p.showError)
				}
			}
		})
	})
}

func (p *Chat) showError(err error) {
	if err != nil {
		p.err = err.Error()
	}
}

// retryStatus shows an automatic retry in progress, with Stop.
func (p *Chat) retryStatus(c *ui.Context) {
	if !p.view.Running {
		return
	}
	var op *model.Operation
	for i := len(p.view.Operations) - 1; i >= 0; i-- {
		if o := p.view.Operations[i]; o.Kind == "retry" && o.Status == "running" {
			op = &p.view.Operations[i]
			break
		}
	}
	if op == nil {
		return
	}
	text := i18n.T("chat.retryConnecting")
	if op.Attempt != nil && op.MaxAttempts != nil {
		text += fmt.Sprintf(" · %v / %v", *op.Attempt, *op.MaxAttempts)
	}
	// Dismissing transport feedback only hides this notice. The generation
	// and its ordinary composer Stop control keep their original lifecycle.
	kit.Notice(c, kit.Info, "", text, func() {
		ui.Spinner(c).Size(14, 14)
		if kit.Button(c, i18n.T("chat.stop"), nil, false).Disabled(p.stopping).Clicked() {
			p.stopCurrent()
		}
	})
}

// projectContext chooses the conversation's project; choose gets the
// project chosen ("" for none).
func (p *Chat) projectContext(c *ui.Context, projectID string, choose func(string)) {
	t := c.Theme()
	name := i18n.T("chat.noProject")
	if pr := p.catalog.project(projectID); pr != nil {
		name = pr.Name
	}
	b := kit.GhostButton(c, name, icons.FolderOpen, false).Disabled(p.busy).Label(i18n.T("chat.projectContextLabel", "name", name)).TextColor(t.TextMuted).AlignSelf(ui.Start)
	b.Menu(func(m *ui.Menu) {
		if m.Item(i18n.T("chat.noProject")).Checked(projectID == "").Chosen() {
			choose("")
		}
		for _, pr := range p.catalog.Projects {
			if m.Item(pr.Name).Checked(pr.ID == projectID).Chosen() {
				choose(pr.ID)
			}
		}
		m.Separator()
		if m.Item(i18n.T("chat.openOrCreateProject")).Chosen() {
			p.openProjectDialog()
		}
	})
}
