package chat

import (
	"encoding/json"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/chat/automations"
	"winone/desktopmygo/chat/model"
	"winone/desktopmygo/chat/pluginhost"
	"winone/desktopmygo/chat/pluginhost/globalpages"
	"winone/desktopmygo/chat/workspace"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/webui"
)

// Native tools own their buffers and terminal lifetimes. Only browser pages
// and HTML document content use untrusted WebViews; no React shell is created.
type tools struct {
	panel        *workspace.Panel
	panelKey     string
	panelRequest int
	automations  *automations.Page
	settingsOpen bool
}

func (p *Chat) appendReference(text, separator string) error {
	if p.busy || p.opening || p.draftLoading {
		return core.Errorf(i18n.T("chat.browserDraftBusy"))
	}
	p.updateText(joinNonEmpty(separator, p.text, text), p.files, p.activeID)
	return nil
}

func (p *Chat) commentMarkers() []float64 {
	markers := []float64{}
	for _, file := range p.files {
		var annotation struct {
			MarkerNumber float64 `json:"markerNumber"`
		}
		if len(file.Annotation) > 0 && json.Unmarshal(file.Annotation, &annotation) == nil {
			markers = append(markers, annotation.MarkerNumber)
		}
	}
	return markers
}

func (p *Chat) nativePanel() *workspace.Panel {
	cwd := ""
	if conversation := p.conversation(); conversation != nil {
		cwd = conversation.Directory
	}
	key := p.activeID + ":" + cwd
	if p.tools.panel != nil && p.tools.panelKey == key {
		return p.tools.panel
	}
	if p.tools.panel != nil {
		p.tools.panel.Close()
	}
	options := workspace.PanelOptions{
		Prefix: p.method(""), SessionID: p.activeID, Directory: cwd,
		Running:         func() bool { return p.view.Running },
		OnReference:     func(path string) error { return p.appendReference("@"+path, "\n") },
		OnReviewComment: func(text string) error { return p.appendReference(text, "\n\n") },
		OnClose:         func() { p.ui.details = false },
		OnDock: func() {
			if p.prefs.Dock == "bottom" {
				p.prefs.Dock = "right"
			} else {
				p.prefs.Dock = "bottom"
			}
			p.prefs.save()
		},
		Plugin: func(tab workspace.PanelTab) workspace.DocumentBody {
			return pluginhost.NewTaskPane(p.env, tab.Path, p.activeID)
		},
		PluginAvailable: func(tab workspace.PanelTab) bool { return globalpages.For(p.env).Available(tab.Path, "task") },
		HTML: func(preview workspace.FilePreview) (workspace.DocumentBody, error) {
			return webui.NewHTMLPreview(p.env, preview.URL, preview.Name)
		},
	}
	options.Browser = func(tab workspace.PanelTab, onTitle func(string, string), onClose func()) workspace.DocumentBody {
		return webui.NewNativeBrowserPane(p.env, webui.NativeBrowserOptions{
			SessionID: first(p.activeID, "new-chat"), TabID: tab.ID, URL: tab.URL,
			OnTitle: onTitle, OnClose: onClose, OnNewTab: p.tools.panel.OpenBrowser,
			MarkerNumbers: p.commentMarkers,
			OnComment: func(annotation json.RawMessage, done func(error)) {
				p.addBrowserComment(annotation, func(_ any, err error) { done(err) })
			},
		})
	}
	p.tools.panel = workspace.NewPanel(p.env, options)
	p.tools.panelKey, p.tools.panelRequest = key, 0
	return p.tools.panel
}

func (p *Chat) panelView(c *ui.Context) ui.Element {
	panel := p.nativePanel()
	if p.tools.panelRequest != p.ui.panelRequests {
		p.tools.panelRequest = p.ui.panelRequests
		if request := p.ui.panelRequest; request != nil {
			switch {
			case request.File != "":
				panel.OpenFile(request.File, request.Line)
			case request.URL != "":
				panel.OpenBrowser(request.URL)
			case request.Review != nil:
				panel.OpenReview(request.Review.TurnID, request.Review.Path)
			case request.Tab == "files" || request.Tab == "resources":
				panel.OpenLibrary(request.Tab, request.Directory)
			case request.Tab != "home":
				panel.OpenTool(request.Tab)
			}
		}
	}
	return ui.Column(c).Fill().MinWidth(0).MinHeight(0).Children(func() { panel.View(c) })
}

// Browser comments keep the original Host attachment import receipt. The
// browser commits its marker only after the screenshot actually joined this
// conversation's draft; navigation cannot turn a late import into success.
func (p *Chat) addBrowserComment(payload json.RawMessage, reply func(any, error)) {
	if p.busy || p.opening || p.draftLoading {
		reply(nil, core.Errorf(i18n.T("chat.browserDraftBusy")))
		return
	}
	var comment model.BrowserComment
	if err := json.Unmarshal(payload, &comment); err != nil {
		reply(nil, err)
		return
	}
	id := p.activeID
	add := func(file model.Attachment) {
		file.Annotation = payload
		p.updateText(p.text, append(append([]model.Attachment(nil), p.files...), file), id)
		reply(nil, nil)
	}
	if id == "" {
		add(model.Attachment{Name: "#" + formatMarker(comment.MarkerNumber) + ".png", Path: comment.ScreenshotPath, MimeType: "image/png"})
		return
	}
	core.Go(p.env, func() ([]model.Attachment, error) {
		var added struct {
			Attachments []model.Attachment `json:"attachments"`
		}
		err := p.env.Host.CallInto(p.method("attachments.add"), map[string]any{"id": id, "paths": []string{comment.ScreenshotPath}}, &added)
		return added.Attachments, err
	}, func(added []model.Attachment, err error) {
		switch {
		case err != nil:
			reply(nil, err)
		case len(added) == 0:
			reply(nil, core.Errorf(i18n.T("chat.browserCommentFailed")))
		case p.ui.closed || p.selection != id:
			reply(nil, core.Errorf(i18n.T("chat.browserDraftBusy")))
		default:
			add(added[0])
		}
	})
}

func formatMarker(n float64) string { data, _ := json.Marshal(n); return string(data) }

func (p *Chat) automationsView(c *ui.Context) {
	if p.tools.automations == nil {
		p.tools.automations = automations.New(p.env, automations.Options{
			ConfigureConnections: func() { p.settings.tab = 2; p.openSettings() },
			Back:                 func() { p.ui.automations = false },
			OpenSession:          func(id string) { p.env.Open("chat", core.Intent{Requested: true, Conversation: id}) },
		})
	}
	if len(p.automationDraft) > 0 {
		p.tools.automations.OpenPluginDraft(p.automationDraft)
		p.automationDraft = nil
	}
	p.tools.automations.View(c)
}

func (p *Chat) leaveAutomation(action func()) {
	leave := func() { p.ui.automations = false; action() }
	if p.ui.automations && p.tools.automations != nil && (p.tools.automations.Dirty() || p.tools.automations.Busy()) {
		p.env.RequestChange(leave)
	} else {
		leave()
	}
}

func (p *Chat) closeTools() {
	if p.tools.automations != nil {
		p.tools.automations.Close()
	}
	if p.tools.panel != nil {
		p.tools.panel.Close()
	}
	p.tools = tools{}
}

const (
	minimumConversationWidth = 340
	// Reserve room for the header, a wrapped composer toolbar and readable
	// messages when the workspace moves below the conversation.
	minimumConversationHeight = 360
	minimumWorkspaceWidth     = 340
	minimumWorkspaceHeight    = 120
	detailsDividerSize        = 5
)

// dockedView keeps the resizable layout and preferred dimensions. When the
// actual content width cannot hold both panes, the workspace docks below the
// conversation temporarily; widening restores the saved dock without closing
// documents, terminals or browser tabs.
func (p *Chat) dockedView(c *ui.Context, main func()) {
	if !p.ui.details {
		main()
		return
	}
	bounds := p.ui.workspaceBox.Bounds(c)
	bottom := p.prefs.Dock == "bottom" || bounds.W > 0 && bounds.W < minimumConversationWidth+minimumWorkspaceWidth+detailsDividerSize
	size := &p.prefs.Right
	minimum, maximum := float32(minimumWorkspaceWidth), float32(2000)
	if bottom {
		size, minimum = &p.prefs.Bottom, minimumWorkspaceHeight
		if bounds.H > 0 {
			maximum = max(minimum, bounds.H-minimumConversationHeight-detailsDividerSize)
		}
	} else if bounds.W > 0 {
		maximum = max(minimum, bounds.W-minimumConversationWidth-detailsDividerSize)
	}
	displayedSize := min(max(*size, minimum), maximum)
	t, pal := c.Theme(), kit.Colors(c)
	container := ui.Row(c)
	if bottom {
		container = ui.Column(c)
	}
	container.Grow(1).Basis(0).MinWidth(0).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
		ui.Column(c).Grow(1).Basis(0).MinWidth(0).MinHeight(0).Children(main)
		handle := ui.Box(c).Shrink(0).Focusable().Role(ui.RoleSplitter).Label(i18n.T("chat.resizeDetails")).Tooltip(i18n.T("chat.resizeDetails"))
		if bottom {
			handle.FillWidth().Height(detailsDividerSize).Cursor(ui.CursorResizeNS)
		} else {
			handle.FillHeight().Width(detailsDividerSize).Cursor(ui.CursorResizeEW)
		}
		line := pal.Hairline
		if handle.Hovered() || handle.Pressed() {
			line = t.Border
		}
		focus := handle.FocusVisible()
		handle.Draw(func(painter *ui.Painter, r ui.Rect) {
			if bottom {
				painter.Fill(ui.Rect{X: r.X, Y: r.Y + 2, W: r.W, H: 1}, line, 0)
			} else {
				painter.Fill(ui.Rect{X: r.X + 2, Y: r.Y, W: 1, H: r.H}, line, 0)
			}
			if focus {
				painter.FocusRing(r, [4]float32{})
			}
		})
		move := func(delta float32) {
			// Start at the displayed edge even if an older saved size exceeded
			// the current bounds, so the first drag/arrow press responds.
			*size = min(max(displayedSize-delta, minimum), maximum)
			displayedSize = *size
		}
		if dx, dy, ok := handle.Dragged(); ok {
			if bottom {
				move(dy)
			} else {
				move(dx)
			}
		}
		if _, _, ended := handle.DragEnded(); ended {
			p.prefs.save()
		}
		back, forward := ui.KeyLeft, ui.KeyRight
		if bottom {
			back, forward = ui.KeyUp, ui.KeyDown
		}
		if handle.Shortcut(0, back) {
			move(-16)
			p.prefs.save()
		}
		if handle.Shortcut(0, forward) {
			move(16)
			p.prefs.save()
		}
		panel := ui.Column(c).Shrink(0).MinWidth(0).MinHeight(0)
		if bottom {
			panel.Height(displayedSize).FillWidth()
		} else {
			panel.Width(displayedSize).FillHeight()
		}
		panel.Children(func() { p.panelView(c).Fill() })
	})
}

// takeIntents opens links only after catalog reads and draft saves complete.
func (p *Chat) takeIntents() {
	for _, intent := range p.env.TakeIntents(p.intentPage) {
		if len(intent.AutomationDraft) > 0 && p.automationDraft == nil {
			p.automationDraft = append(json.RawMessage(nil), intent.AutomationDraft...)
			p.ui.automations = true
		}
		if intent.ChatSettings {
			p.tools.settingsOpen = true
		}
		if intent.Conversation != "" {
			request := intent
			p.requested = &request
		}
		if intent.NewChat {
			request := intent
			p.newChat = &request
		}
	}
	if p.newChat != nil && p.catalogLoaded && !p.busy {
		p.openNewChat()
	}
	if p.requested == nil || !p.catalogLoaded || p.busy {
		return
	}
	request := p.requested
	p.requested = nil
	current := func() bool { return p.requested == nil }
	p.run(func(done func(error)) {
		open := func() {
			target := p.catalog.conversation(request.Conversation)
			if target == nil {
				done(core.Errorf(i18n.T("chat.requestedConversationMissing")))
				return
			}
			archived := target.Archived
			p.choose(request.Conversation, current, func(err error) {
				if err == nil && current() {
					p.ui.archived, p.ui.automations, p.tools.settingsOpen = archived, false, false
					p.ui.dialog = dialogState{}
					if f := request.File; f != nil {
						if f.Directory {
							p.openPanel(panelRequest{Tab: "files", Directory: f.Path})
						} else {
							p.openFile(f.Path, f.Line)
						}
					}
				}
				done(err)
			})
		}
		if p.catalog.conversation(request.Conversation) != nil {
			open()
			return
		}
		p.refresh(func(err error) {
			if err != nil {
				done(err)
				return
			}
			open()
		})
	})
}

func (p *Chat) openNewChat() {
	request := p.newChat
	p.newChat = nil
	p.run(func(done func(error)) {
		p.choose("", nil, func(err error) {
			if err == nil {
				p.ui.archived, p.ui.automations, p.tools.settingsOpen = false, false, false
				p.ui.dialog = dialogState{}
				if request.Prefill != "" {
					p.updateText(joinNonEmpty("\n\n", p.newDraft, request.Prefill), p.newFiles, "")
				}
			}
			done(err)
		})
	})
}

func (t *tools) busy() bool {
	return t.automations != nil && t.automations.Busy() || t.panel != nil && t.panel.Busy()
}
