package chat

import (
	"encoding/json"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/chat/model"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/webui"
	"winone/desktopmygo/widgets"
)

// The side panel, the automations page and Chat settings stay React (the
// user's hybrid plan): package webui shows chat-embed.html in web views
// and this file is their native side, as chat.tsx was their parent: the
// props it gave them, the callbacks they called, the panel's dock.

// tools are the page's web views, made the first time each shows.
type tools struct {
	panel, automations, settings *webui.Surface
	browser                      *webui.Browser
	// settingsOpen shows Chat settings over the page.
	settingsOpen bool
	// panelURLHandled drops a page address the panel already opened.
	panelURLHandled int
}

// embedHost is what Cindy's hooks read in the page (publishCindyHost): the
// catalog, the AI services as the Host returned them, and the model of a
// new conversation.
type embedHost struct {
	Catalog              *Catalog             `json:"catalog"`
	Providers            json.RawMessage      `json:"providers"`
	DefaultModel         *widgets.AISelection `json:"defaultModel"`
	DefaultThinkingLevel string               `json:"defaultThinkingLevel,omitempty"`
}

// embedState is what a view of the page is told (chat-embed-entry.tsx).
type embedState struct {
	Language string         `json:"language"`
	Theme    string         `json:"theme"`
	Host     embedHost      `json:"host"`
	Panel    *panelProps    `json:"panel,omitempty"`
	Settings *settingsProps `json:"settings,omitempty"`
}

// panelProps are ChatSidePanel's props, as chat.tsx gave them.
type panelProps struct {
	Key                  string       `json:"key"`
	ID                   string       `json:"id"`
	Cwd                  string       `json:"cwd"`
	InitialTab           string       `json:"initialTab"`
	InitialTabRequest    int          `json:"initialTabRequest"`
	InitialDirectory     string       `json:"initialDirectory,omitempty"`
	InitialURL           string       `json:"initialURL,omitempty"`
	InitialFile          string       `json:"initialFile,omitempty"`
	InitialLine          int          `json:"initialLine,omitempty"`
	InitialReview        *reviewProps `json:"initialReview,omitempty"`
	Dock                 string       `json:"dock"`
	Visible              bool         `json:"visible"`
	Running              bool         `json:"running"`
	LastTurnPaths        []string     `json:"lastTurnPaths"`
	CommentMarkerNumbers []float64    `json:"commentMarkerNumbers"`
	ComposerLocked       bool         `json:"composerLocked"`
}

type reviewProps struct {
	TurnID string `json:"turnId"`
	Path   string `json:"path,omitempty"`
}

// settingsProps are ChatSettings' props.
type settingsProps struct {
	Providers     json.RawMessage     `json:"providers"`
	DefaultModel  widgets.AISelection `json:"defaultModel"`
	ModelsLoading bool                `json:"modelsLoading"`
	ModelsError   string              `json:"modelsError"`
}

func (p *Chat) embedCommon(c *ui.Context) embedState {
	theme := "light"
	if c.Theme().Dark {
		theme = "dark"
	}
	host := embedHost{Providers: p.providersJSON(), DefaultThinkingLevel: p.newThinking}
	if p.catalogLoaded {
		host.Catalog = &p.catalog
	}
	if p.newModel.Model != "" {
		model := p.newModel
		host.DefaultModel = &model
	}
	return embedState{Language: i18n.Locale(), Theme: theme, Host: host}
}

// providersJSON is the AI services' providers as the Host returned them:
// Cindy reads fields this window does not decode.
func (p *Chat) providersJSON() json.RawMessage {
	if len(p.providersRaw) == 0 {
		return json.RawMessage("null")
	}
	return p.providersRaw
}

// panelProps are the panel's props now.
func (p *Chat) panelProps() *panelProps {
	conversation := p.conversation()
	cwd := ""
	if conversation != nil {
		cwd = conversation.Directory
	}
	props := &panelProps{
		Key: p.activeID + ":" + cwd, ID: p.activeID, Cwd: cwd,
		InitialTab: first(p.ui.detailsTab, "home"), InitialTabRequest: p.ui.panelRequests,
		Dock: p.prefs.Dock, Visible: p.ui.details, Running: p.view.Running,
		LastTurnPaths:  p.lastTurnPaths(),
		ComposerLocked: p.busy || p.opening || p.draftLoading,
	}
	if r := p.ui.panelRequest; r != nil {
		props.InitialDirectory, props.InitialFile, props.InitialLine = r.Directory, r.File, r.Line
		if p.tools.panelURLHandled != p.ui.panelRequests {
			props.InitialURL = r.URL
		}
		if r.Review != nil {
			props.InitialReview = &reviewProps{TurnID: r.Review.TurnID, Path: r.Review.Path}
		}
	}
	props.CommentMarkerNumbers = []float64{}
	for _, f := range p.files {
		var a struct {
			MarkerNumber float64 `json:"markerNumber"`
		}
		if len(f.Annotation) > 0 && json.Unmarshal(f.Annotation, &a) == nil {
			props.CommentMarkerNumbers = append(props.CommentMarkerNumbers, a.MarkerNumber)
		}
	}
	return props
}

// lastTurnPaths are the files the latest answer's finished, successful
// tools wrote, which the review compares (chat.tsx latestReviewPaths).
func (p *Chat) lastTurnPaths() []string {
	paths := []string{}
	items := model.Project(p.view.Messages, p.view.Tools, p.view.Running, p.view.Operations)
	for i := len(items) - 1; i >= 0; i-- {
		item := items[i]
		if item.Kind == "user" {
			break
		}
		if item.Kind != "response" || item.Response == nil {
			continue
		}
		for _, entry := range item.Response.Entries {
			tool := entry.Tool
			if entry.Kind != "tool" || tool.Running || tool.IsError {
				continue
			}
			if tool.Files != nil {
				for _, f := range tool.Files {
					paths = append(paths, f.Path)
				}
			} else if path, ok := tool.Args["path"].(string); ok && (tool.Name == "write" || tool.Name == "edit") {
				paths = append(paths, path)
			}
		}
		break
	}
	return paths
}

// panelView is the side panel, with the browser's pages over it.
func (p *Chat) panelView(c *ui.Context) ui.Element {
	if p.tools.panel == nil {
		p.tools.browser = webui.NewBrowser(p.env)
		p.tools.panel = webui.New(p.env, webui.Options{View: "panel", Label: i18n.T("chat.details"),
			Receive: p.receivePanel, Browser: p.tools.browser})
	}
	state := p.embedCommon(c)
	state.Panel = p.panelProps()
	return p.tools.panel.View(c, state).Children(func() { p.tools.browser.Overlay(c) })
}

// receivePanel answers the panel's callbacks, as chat.tsx's handlers did.
func (p *Chat) receivePanel(kind string, payload json.RawMessage, reply func(any, error)) {
	switch kind {
	case "close":
		p.ui.details = false
	case "dirty":
		var dirty bool
		_ = json.Unmarshal(payload, &dirty)
		p.ui.filesDirty = dirty
	case "dock":
		if p.prefs.Dock == "bottom" {
			p.prefs.Dock = "right"
		} else {
			p.prefs.Dock = "bottom"
		}
		p.prefs.save()
	case "urlHandled":
		p.tools.panelURLHandled = p.ui.panelRequests
	case "reference":
		var path string
		if json.Unmarshal(payload, &path) == nil && path != "" && !p.busy && !p.opening && !p.draftLoading {
			p.updateText(joinNonEmpty("\n", p.text, "@"+path), p.files, p.activeID)
		}
	case "reviewComment":
		var text string
		if json.Unmarshal(payload, &text) != nil || text == "" {
			break
		}
		if p.busy || p.opening || p.draftLoading {
			reply(nil, core.Errorf(i18n.T("chat.browserDraftBusy")))
			return
		}
		p.updateText(joinNonEmpty("\n\n", p.text, text), p.files, p.activeID)
	case "comment":
		p.addBrowserComment(payload, reply)
		return
	case "drop":
		var paths []string
		if json.Unmarshal(payload, &paths) == nil && len(paths) > 0 && !p.busy {
			p.addPaths(paths, func(err error) {
				if err != nil {
					p.err = err.Error()
				}
			})
		}
	case "navigate":
		webui.Navigate(p.env, payload)
	default:
		reply(nil, core.Errorf(i18n.T("chat.unsupportedOperation")))
		return
	}
	reply(nil, nil)
}

// addBrowserComment attaches a browser comment's screenshot with the
// comment, as chat.tsx did: through the Host for a conversation, as is
// for the chat not yet created.
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
		case p.selection != id:
			reply(nil, core.Errorf(i18n.T("chat.browserDraftBusy")))
		default:
			add(added[0])
		}
	})
}

func formatMarker(n float64) string {
	data, _ := json.Marshal(n)
	return string(data)
}

// automationsView is the automations page.
func (p *Chat) automationsView(c *ui.Context) {
	if p.tools.automations == nil {
		p.tools.automations = webui.New(p.env, webui.Options{View: "automations", Label: i18n.T("chat.automations"),
			Receive: p.receiveTools})
	}
	p.tools.automations.View(c, p.embedCommon(c)).Grow(1).AlignSelf(ui.Stretch)
}

// settingsView shows Chat settings over the page while open.
func (p *Chat) settingsView(c *ui.Context) {
	if !p.tools.settingsOpen {
		return
	}
	if p.tools.settings == nil {
		p.tools.settings = webui.New(p.env, webui.Options{View: "settings", Label: i18n.T("chat.settings"),
			Receive: p.receiveTools})
	}
	state := p.embedCommon(c)
	state.Settings = &settingsProps{Providers: p.providersJSON(), DefaultModel: p.newModel,
		ModelsLoading: !p.catalogLoaded && p.err == "", ModelsError: p.view.ModelError}
	ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Children(func() {
		p.tools.settings.View(c, state).Fill()
	})
}

// receiveTools answers the automations page and Chat settings.
func (p *Chat) receiveTools(kind string, payload json.RawMessage, reply func(any, error)) {
	switch kind {
	case "close":
		p.tools.settingsOpen = false
		// The dialog opens afresh next time, as React remounted it.
		if p.tools.settings != nil {
			p.tools.settings.Close()
			p.tools.settings = nil
		}
	case "saved":
		p.refresh(func(err error) {
			if err != nil {
				p.err = err.Error()
			}
		})
	case "navigate":
		webui.Navigate(p.env, payload)
	default:
		reply(nil, core.Errorf(i18n.T("chat.unsupportedOperation")))
		return
	}
	reply(nil, nil)
}

// closeTools destroys the page's web views.
func (p *Chat) closeTools() {
	for _, s := range []*webui.Surface{p.tools.panel, p.tools.automations, p.tools.settings} {
		if s != nil {
			s.Close()
		}
	}
	if p.tools.browser != nil {
		p.tools.browser.Close()
	}
	p.tools = tools{}
}

// dockedView lays out the conversation and the side panel docked right
// or below, the panel as large as the user last left it.
func (p *Chat) dockedView(c *ui.Context, main func()) {
	if !p.ui.details {
		main()
		return
	}
	bottom := p.prefs.Dock == "bottom"
	t, pal := c.Theme(), kit.Colors(c)
	container := ui.Row(c)
	if bottom {
		container = ui.Column(c)
	}
	container.Grow(1).Basis(0).MinWidth(0).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
		ui.Column(c).Grow(1).Basis(0).MinWidth(0).MinHeight(0).Children(main)
		size := &p.prefs.Right
		if bottom {
			size = &p.prefs.Bottom
		}
		handle := ui.Box(c).Shrink(0).Focusable().Role(ui.RoleSplitter).Label(i18n.T("chat.resizeDetails")).Tooltip(i18n.T("chat.resizeDetails"))
		if bottom {
			handle.FillWidth().Height(5).Cursor(ui.CursorResizeNS)
		} else {
			handle.FillHeight().Width(5).Cursor(ui.CursorResizeEW)
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
		// The panel grows as the divider moves toward the conversation.
		move := func(delta float32) {
			*size = min(max(*size-delta, 240), 2000)
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
		panel := ui.Column(c).Shrink(1).MinWidth(0).MinHeight(0)
		if bottom {
			panel.Height(*size).FillWidth()
		} else {
			panel.Width(*size).FillHeight()
		}
		panel.Children(func() { p.panelView(c).Fill() })
	})
}

// takeIntents follows links of other pages into Chat (env.Open): Chat
// settings open at once; a conversation opens once the catalog is read
// and the page is idle, as chat.tsx's requestedConversationId did.
func (p *Chat) takeIntents() {
	for _, intent := range p.env.TakeIntents("chat") {
		if intent.ChatSettings {
			p.tools.settingsOpen = true
		}
		if intent.Conversation != "" {
			request := intent
			p.requested = &request
		}
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

// busy reports a change the React tools asked of the Host and that is not
// answered yet: an editor's save, a review's commit.
func (t *tools) busy() bool {
	for _, s := range []*webui.Surface{t.panel, t.automations, t.settings} {
		if s != nil && s.Busy() {
			return true
		}
	}
	return false
}
