package chat

import (
	"math"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"

	"winone/desktopmygo/chat/model"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
	"winone/desktopmygo/widgets"
)

// uiState is what the page's views keep between frames.
type uiState struct {
	closed bool
	// sidebar shows the conversation list; details, the side panel.
	sidebar, details bool
	// automations shows the scheduled tasks in place of the conversation.
	automations bool
	// filesDirty marks an unsaved file in the side panel, which blocks
	// leaving the conversation.
	filesDirty bool
	search     string

	transcript ui.ScrollState
	// follow keeps the transcript at its end as replies stream, until
	// the reader scrolls up; lastY is where the reader was last frame.
	follow  bool
	lastY   float32
	toTop   bool
	pinNext bool

	// archived lists archived conversations instead of the others.
	archived bool

	dialog      dialogState
	move        *Conversation
	moveTo      *string
	exportOf    *Conversation
	exportState exportState
	confirmFull bool
	edit        *editState
	usageOpen   bool
	// agents is the subagent whose messages are open.
	agents agentDialog
	// The model picker: open, its search, the providers it folds, and
	// whether it has shown the chosen model yet.
	models      widgets.ModelMenuState
	composerBox ui.Handle
	// welcomeArea is the new chat's frame, whose height centers it.
	welcomeArea ui.Handle
	compact     bool
	// expanded remembers which work processes and steps are open.
	expanded map[string]bool
	// answers are the question card's answers, for the question asked.
	answers    []string
	answersFor string
	// focusedApproval is the approval card that took the focus.
	focusedApproval string
	detailsTab      string
	// panelRequest is the side panel's latest request, numbered by
	// panelRequests so that the panel sees each once (panel.go).
	panelRequest  *panelRequest
	panelRequests int

	services ui.Services
	markdown *markdown.Options

	// items is what the transcript's file chips, change card, tool
	// dialog, scroll controls, plan reminder and paste keep (files.go).
	items itemsState
}

// resetDetails closes what a conversation opened in the side panel.
func (u *uiState) resetDetails() { u.details = false }

// followLatest scrolls to the end and follows new content.
func (u *uiState) followLatest() { u.follow, u.pinNext = true, true }

// contentChanged keeps the end in view as content arrives, if followed.
func (u *uiState) contentChanged() {
	if u.follow {
		u.pinNext = true
	}
}

// keepPosition shows the earlier page just loaded, from its top, and
// stops following so an arriving event cannot move it.
func (u *uiState) keepPosition() { u.follow, u.toTop = false, true }

func (u *uiState) close() {}

// View draws the page.
func (p *Chat) View(c *ui.Context) {
	p.ui.services = c.Services()
	p.takeIntents()
	p.env.Activity.Report("chat", p.draftDirty || p.ui.filesDirty || p.settings.dirty(),
		p.busy || p.saving || p.opening || p.transferBusy || p.reconnecting || p.tools.busy() || p.settings.saving, nil)
	if p.ui.sidebar {
		p.pollActivity()
	}
	t, pal := c.Theme(), kit.Colors(c)
	ui.Row(c).Fill().AlignItems(ui.Stretch).Background(t.Background).Children(func() {
		if p.ui.sidebar {
			ui.Column(c).Width(p.prefs.Sidebar).Shrink(0).MinWidth(0).Background(pal.Sidebar).
				BorderWidth(0, 1, 0, 0).BorderColor(pal.Hairline).Children(func() { p.sidebarView(c) })
			p.sidebarHandle(c)
		}
		ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
			if p.ui.automations {
				p.automationsView(c)
				return
			}
			p.dockedView(c, func() { p.mainView(c) })
		})
		p.settingsView(c)
	})
	p.dialogs(c)
}

// mainView is the conversation: header, notices, transcript or welcome,
// and the input area.
func (p *Chat) mainView(c *ui.Context) {
	t, pal := c.Theme(), kit.Colors(c)
	conversation := p.conversation()
	ui.Row(c).FillWidth().Padding(10, 16).Gap(8).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(pal.Hairline).Shrink(0).Children(func() {
		if !p.ui.sidebar {
			if kit.IconButton(c, icons.PanelLeftOpen, i18n.T("chat.showSidebar")).Clicked() {
				p.setSidebar(true)
			}
		}
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			title := i18n.T("chat.newChat")
			place := i18n.T("chat.noProject")
			if conversation != nil {
				title, place = conversation.Title, conversation.Directory
			} else if pr := p.catalog.project(p.newProjectID); pr != nil {
				place = pr.Name
			}
			ui.Text(c, title).FontWeight(600).SingleLine().Ellipsis("…").MinWidth(0)
			ui.Text(c, place).FontSize(kit.HelpSize(c)).TextColor(t.TextMuted).SingleLine().Ellipsis("…").MinWidth(0).Tooltip(place)
		})
		if conversation != nil {
			p.conversationMenu(c, *conversation, true, false)
		} else if kit.IconButton(c, icons.FolderOpen, i18n.T("chat.saveLocation")).Clicked() {
			p.openLocations()
		}
		// The side panel (files, resources, review, terminal, browser)
		// opens from here, as the React header's button did.
		if !p.ui.details && !p.ui.automations && kit.IconButton(c, icons.PanelRightOpen, i18n.T("chat.showDetails")).Clicked() {
			p.openTools("home")
		}
	})
	p.feedback(c)
	ui.Column(c).Grow(1).Basis(0).MinHeight(0).FillWidth().Children(func() {
		if p.activeID == "" && !p.opening && p.submitting == nil {
			p.welcome(c)
			return
		}
		p.transcriptView(c)
		if p.activeID != "" {
			p.inputArea(c)
		}
	})
}

// feedback shows the page's notice, its error with a retry, and the
// model's error with a way to the AI services.
func (p *Chat) feedback(c *ui.Context) {
	notice := first(p.notice, p.view.Notice)
	if notice == "" && p.err == "" && p.view.ModelError == "" {
		return
	}
	ui.Column(c).FillWidth().Padding(8, 16, 0, 16).Gap(8).Shrink(0).Children(func() {
		if notice != "" {
			kit.Notice(c, kit.Info, "", notice, nil)
		}
		if p.err != "" && p.ui.dialog.kind != dialogProject {
			kit.Notice(c, kit.Failure, "", p.err, func() {
				if kit.GhostButton(c, i18n.T("chat.retry"), icons.RefreshCw, false).Clicked() {
					p.run(func(done func(error)) {
						p.refresh(func(err error) {
							if err == nil && p.activeID != "" {
								p.open(false)
							}
							done(err)
						})
					})
				}
			})
		}
		if p.view.ModelError != "" {
			kit.Notice(c, kit.Failure, "", p.view.ModelError, func() {
				if kit.Button(c, i18n.T("chat.openAI"), nil, false).Clicked() {
					p.env.Navigate("ai-services", nil)
				}
			})
		}
	})
}

// welcome is the new chat: WinOne's mark, the project and the composer.
func (p *Chat) welcome(c *ui.Context) {
	// A scroll view does not stretch its content: the column takes the
	// frame's height from the last layout, so the mark and the composer
	// are centered as a whole and scroll when the window is too low.
	height := p.ui.welcomeArea.Bounds(c).H
	ui.Scroll(c).Fill().Bind(&p.ui.welcomeArea).Children(func() {
		ui.Column(c).FillWidth().MinHeight(height).Justify(ui.Center).AlignItems(ui.Center).Padding(32, 24).Gap(24).Children(func() {
			ui.Image(c, welcomeLogo()).Size(160, 160).Fit(ui.Contain).Label("")
			ui.Column(c).FillWidth().MaxWidth(readingWidth).AlignSelf(ui.Center).Gap(8).Children(func() {
				p.projectContext(c, p.newProjectID, func(id string) { p.newProjectID = id })
				p.composer(c)
			})
		})
	})
}

// sidebarHandle widens or narrows the conversation list by dragging the
// edge between it and the conversation, or with the Left and Right keys
// once focused, down to the narrowest readable list and up to half the
// window, as React's resizable panel kept it; the width is remembered.
func (p *Chat) sidebarHandle(c *ui.Context) {
	t := c.Theme()
	handle := ui.Box(c).FillHeight().Width(5).Margin(0, 0, 0, -3).Shrink(0).Cursor(ui.CursorResizeEW).Focusable().
		Role(ui.RoleSplitter).Label(i18n.T("chat.resizeSidebar")).Tooltip(i18n.T("chat.resizeSidebar"))
	line := ui.Color{}
	if handle.Hovered() || handle.Pressed() {
		line = t.Border
	}
	focus := handle.FocusVisible()
	handle.Draw(func(painter *ui.Painter, r ui.Rect) {
		if line != (ui.Color{}) {
			painter.Fill(ui.Rect{X: r.X + 2, Y: r.Y, W: 1, H: r.H}, line, 0)
		}
		if focus {
			painter.FocusRing(r, [4]float32{})
		}
	})
	move := func(delta float32) {
		p.prefs.Sidebar = min(max(p.prefs.Sidebar+delta, minimumSidebar), maximumSidebar)
	}
	if dx, _, ok := handle.Dragged(); ok {
		move(dx)
	}
	if _, _, ended := handle.DragEnded(); ended {
		p.prefs.save()
	}
	if handle.Shortcut(0, ui.KeyLeft) {
		move(-16)
		p.prefs.save()
	}
	if handle.Shortcut(0, ui.KeyRight) {
		move(16)
		p.prefs.save()
	}
}

// readingWidth is the transcript's and input's widest column.
const readingWidth = 820

// setSidebar shows or hides the conversation list; hiding remembers its
// width.
func (p *Chat) setSidebar(open bool) {
	p.ui.sidebar = open
	if open {
		p.pollActivity()
	}
}

// transcriptView shows the conversation's turns and keeps the end in view
// while followed, with the scroll controls at the reading area's right.
func (p *Chat) transcriptView(c *ui.Context) {
	t := c.Theme()
	s := &p.ui.transcript
	// committed is where the container showed the content last frame,
	// within its range: comparing with an end set as MaxFloat32 would read
	// every frame after a follow as the reader scrolling up.
	committed := max(0, min(s.Y, s.MaxY))
	// The reader scrolling up stops following; returning near the end
	// (80 DIPs, as Cindy's resolveNearBottomOnScroll) follows again. A
	// jump of the scroll controls on its way decides once it lands.
	gliding := p.startJump()
	switch {
	case gliding:
	case committed < p.ui.lastY-1 && !p.ui.pinNext:
		p.ui.follow = false
	case s.MaxY-committed <= 80:
		p.ui.follow = true
	}
	if p.ui.toTop {
		s.Y, p.ui.toTop = 0, false
	} else if (p.ui.follow || p.ui.pinNext) && !gliding {
		s.Y = math.MaxFloat32
	}
	p.ui.pinNext = false
	// A turn that ended may have written or removed files.
	items := &p.ui.items
	if items.wasRunning && !p.view.Running {
		p.forgetChecks()
	}
	items.wasRunning = p.view.Running
	items.controls = p.measureRounds(c, items.content)
	padding := p.readingPadding(c)
	ui.Column(c.Key("transcript-area")).Grow(1).Basis(0).MinHeight(0).FillWidth().Children(func() {
		ui.Scroll(c.Key("transcript")).Grow(1).Basis(0).MinHeight(0).FillWidth().TrackScroll(s).Children(func() {
			ui.Column(c).FillWidth().MaxWidth(readingWidth+48).AlignSelf(ui.Center).Padding(16, padding, 24, 24).Gap(16).Bind(&items.content).Children(func() {
				if p.opening && p.submitting == nil {
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						kit.Loader(c)
						ui.Text(c, i18n.T("chat.loading")).TextColor(t.TextMuted)
					})
					return
				}
				if p.view.HasOlder {
					label := i18n.T("chat.loadOlder")
					if p.historyLoading {
						label = i18n.T("chat.loadingOlder")
					}
					if kit.GhostButton(c, label, icons.ChevronUp, p.historyLoading).Disabled(p.busy).AlignSelf(ui.Center).Clicked() {
						p.loadEarlier()
					}
				}
				p.turns(c)
			})
		})
		if p.activeID != "" {
			p.scrollNav(c)
		}
	})
	p.flushChecks()
	p.toolDialogView(c)
	p.agentDialogView(c)
	p.ui.lastY = committed
}

// visibleMessages are the conversation's messages with the one being
// sent, until the Host has it.
func (p *Chat) visibleMessages() []model.Message {
	if p.submitting == nil {
		return p.view.Messages
	}
	for _, m := range p.view.Messages {
		if m.RequestID == p.submitting.RequestID || m.ID == p.submitting.ID {
			return p.view.Messages
		}
	}
	return append(append([]model.Message(nil), p.view.Messages...), *p.submitting)
}

// inputHint is the line under the input.
func (p *Chat) inputHint() string {
	queued := 0
	steer := false
	for _, m := range p.view.Messages {
		if m.Pending && m.Queued && !m.HiddenInput {
			queued++
		}
		if m.Pending && m.DeliveryMode == model.DeliverySteer {
			steer = true
		}
	}
	switch {
	case queued > 0:
		return i18n.T("chat.queuedCount", "count", queued)
	case steer:
		return i18n.T("chat.steerSubmitted")
	case p.view.Running:
		return i18n.T("chat.runningInputHint")
	}
	return i18n.T("chat.inputHint")
}
