package workspace

import (
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

type PanelTab struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Path   string `json:"path,omitempty"`
	Title  string `json:"title,omitempty"`
	URL    string `json:"url,omitempty"`
	TurnID string `json:"turnId,omitempty"`
	Line   int    `json:"line,omitempty"`
	Pinned bool   `json:"pinned,omitempty"`
}
type PanelOptions struct {
	Prefix, SessionID, Directory string
	Running                      func() bool
	OnReference                  func(string) error
	OnReviewComment              func(string) error
	OnClose, OnDock              func()
	Plugin                       func(PanelTab) DocumentBody
	// Availability projects legacy plugin tabs without deleting their saved
	// owner/session or selection when the plugin is temporarily asleep.
	PluginAvailable func(PanelTab) bool
	Browser         func(tab PanelTab, onTitle func(string, string), onClose func()) DocumentBody
	Media           func(FilePreview) (DocumentBody, error)
	HTML            func(FilePreview) (DocumentBody, error)
}
type panelPreferences struct {
	Tabs                       []PanelTab `json:"tabs"`
	Selected, Section, Library string
	Directory                  string
}
type panelDrag struct {
	panel *Panel
	id    string
}
type panelGuard struct {
	id           string
	switchTo     string
	switchLine   int
	stopTerminal bool
	saving       bool
	error        string
	leave        func()
	cancel       func()
}

// Panel ports ChatSidePanel's tabs/library lifecycle. Closing or switching
// never destroys an unsaved buffer or running terminal without a decision.
type Panel struct {
	env                        *core.Env
	options                    PanelOptions
	tabs                       []PanelTab
	selected, section, library string
	bodies                     map[string]DocumentBody
	fileLibrary                *FileLibrary
	resources                  *ResourcePane
	libraryDocument            *DocumentPane
	libraryPath                string
	librarySplit               kit.SplitState
	closedTabs                 []PanelTab
	guard                      *panelGuard
	closeQueue                 []string
	closed, visible            bool
	err                        string
	newFileOpen                bool
	newFileName                string
	creating                   bool
	handle                     ui.Handle
	expand                     bool
}

func NewPanel(env *core.Env, options PanelOptions) *Panel {
	p := &Panel{env: env, options: options, bodies: map[string]DocumentBody{}, section: "library", library: "files", visible: true, librarySplit: kit.SplitState{Percent: 38}, newFileName: "Untitled.md"}
	p.loadPreferences()
	return p
}
func (p *Panel) Dirty() bool {
	if p.libraryDocument != nil && p.libraryDocument.Dirty() {
		return true
	}
	for _, body := range p.bodies {
		if d, ok := body.(interface{ Dirty() bool }); ok && d.Dirty() {
			return true
		}
	}
	return false
}
func (p *Panel) Busy() bool {
	if p.creating || p.guard != nil && p.guard.saving {
		return true
	}
	for _, body := range p.bodies {
		if d, ok := body.(interface{ Busy() bool }); ok && d.Busy() {
			return true
		}
	}
	return false
}
func (p *Panel) HasTerminal() bool {
	for _, body := range p.bodies {
		if term, ok := body.(*TerminalPane); ok && (term.Running() || term.Busy()) {
			return true
		}
	}
	return false
}
func (p *Panel) SetVisible(visible bool) {
	p.visible = visible
	p.syncVisibility()
	if p.fileLibrary != nil {
		p.fileLibrary.SetVisible(visible && p.section == "library" && p.library == "files")
	}
}
func (p *Panel) visibleTabs() []PanelTab {
	if p.options.PluginAvailable == nil {
		return p.tabs
	}
	var tabs []PanelTab
	for _, tab := range p.tabs {
		if tab.Kind != "plugin" || p.options.PluginAvailable(tab) {
			tabs = append(tabs, tab)
		}
	}
	return tabs
}
func (p *Panel) visibleSelection(tabs []PanelTab) string {
	for _, tab := range tabs {
		if tab.ID == p.selected {
			return tab.ID
		}
	}
	if len(tabs) > 0 {
		return tabs[0].ID
	}
	return ""
}

func (p *Panel) syncVisibility() {
	selected := p.visibleSelection(p.visibleTabs())
	for id, body := range p.bodies {
		if setter, ok := body.(interface{ SetVisible(bool) }); ok {
			setter.SetVisible(p.visible && p.section == "tabs" && selected == id && p.guard == nil)
		}
	}
	if p.libraryDocument != nil {
		p.libraryDocument.SetVisible(p.visible && p.section == "library" && p.library == "files" && p.guard == nil)
	}
}
func (p *Panel) Close() {
	if p.closed {
		return
	}
	p.closed = true
	p.savePreferences()
	for _, body := range p.bodies {
		body.Close()
	}
	clear(p.bodies)
	if p.fileLibrary != nil {
		p.fileLibrary.Close()
	}
	if p.resources != nil {
		p.resources.Close()
	}
	if p.libraryDocument != nil {
		p.libraryDocument.Close()
	}
}
func (p *Panel) Discard() {
	for _, body := range p.bodies {
		body.Close()
	}
	clear(p.bodies)
	if p.libraryDocument != nil {
		p.libraryDocument.Close()
		p.libraryDocument = nil
		p.libraryPath = ""
	}
	p.guard = nil
	p.closeQueue = nil
}
func (p *Panel) OpenLibrary(kind, directory string) {
	p.section, p.library = "library", kind
	if directory != "" && directory != p.options.Directory {
		p.options.Directory = directory
		if p.fileLibrary != nil {
			p.fileLibrary.Close()
			p.fileLibrary = nil
		}
	}
	p.syncVisibility()
	p.savePreferences()
}
func (p *Panel) OpenFile(path string, line int) {
	p.open(PanelTab{ID: "file:" + path, Kind: "file", Path: path, Line: line, Title: filepath.Base(path)})
}
func (p *Panel) OpenReview(turn, path string) {
	p.open(PanelTab{ID: "review:" + turn, Kind: "review", TurnID: turn, Path: path, Title: i18n.T("chat.review")})
}
func (p *Panel) OpenPlugin(instance string) {
	p.open(PanelTab{ID: "plugin:" + instance, Kind: "plugin", Path: instance, Title: i18n.T("pluginHost.taskView")})
}
func (p *Panel) OpenBrowser(address string) {
	p.open(PanelTab{ID: "browser:" + rand.Text(), Kind: "browser", URL: address, Title: i18n.T("chat.browser")})
}
func (p *Panel) open(tab PanelTab) {
	found := -1
	for i := range p.tabs {
		if p.tabs[i].ID == tab.ID {
			found = i
			break
		}
	}
	if found < 0 {
		if len(p.tabs) >= 32 {
			p.err = wt("tooManyTabs")
			return
		}
		p.tabs = append(p.tabs, tab)
	} else {
		old := p.tabs[found]
		tab.Pinned = old.Pinned
		if tab.Kind == "file" && tab.Line != old.Line {
			if document, ok := p.bodies[tab.ID].(*DocumentPane); ok && document.text != nil {
				document.text.options.Line = tab.Line
				if tab.Line > 0 {
					document.text.rendered = false
					document.text.state.GoTo(editor.Pos{Line: tab.Line - 1})
				}
			}
		}
		p.tabs[found] = tab
	}
	p.section = "tabs"
	p.selected = tab.ID
	p.syncVisibility()
	p.savePreferences()
}

// OpenTool selects the same native document types as the composer's tools menu.
func (p *Panel) OpenTool(kind string) { p.newTab(kind) }

// RequestLeave uses the panel's own save/terminal decision rather than the
// shell's whole-page discard, so leaving a conversation never discards its
// autosaved message draft. Cancellation completes the caller's busy receipt.
func (p *Panel) RequestLeave(leave, cancel func()) {
	if p.guard != nil {
		if cancel != nil {
			cancel()
		}
		return
	}
	if p.Dirty() || p.HasTerminal() {
		p.guard = &panelGuard{id: "panel", stopTerminal: p.HasTerminal(), leave: leave, cancel: cancel}
		p.syncVisibility()
		return
	}
	leave()
}

// Dialogs keeps a pending leave decision visible even when the panel is hidden.
func (p *Panel) Dialogs(c *ui.Context) { p.guardView(c) }

func (p *Panel) newTab(kind string) {
	switch kind {
	case "files", "resources":
		p.OpenLibrary(kind, "")
	case "file":
		p.newFileOpen = true
	case "browser":
		p.OpenBrowser("about:blank")
	default:
		p.open(PanelTab{ID: kind + ":" + rand.Text(), Kind: kind, Title: i18n.T("chat." + kind)})
	}
}
func (p *Panel) body(tab PanelTab) DocumentBody {
	if body := p.bodies[tab.ID]; body != nil {
		return body
	}
	var body DocumentBody
	switch tab.Kind {
	case "file":
		body = NewDocumentPane(p.env, DocumentOptions{Prefix: p.options.Prefix, SessionID: p.options.SessionID, Path: tab.Path, Line: tab.Line, OnLink: p.OpenBrowser, Media: p.options.Media, HTML: p.options.HTML})
	case "terminal":
		body = NewTerminalPane(p.env, TerminalOptions{Prefix: p.options.Prefix, SessionID: p.options.SessionID, TabID: tab.ID, OnLink: p.OpenBrowser})
	case "review":
		body = NewReviewPane(p.env, ReviewOptions{Prefix: p.options.Prefix, SessionID: p.options.SessionID, TurnID: tab.TurnID, InitialPath: tab.Path, Running: p.options.Running, OnOpenFile: p.OpenFile, OnComment: p.options.OnReviewComment})
	case "plugin":
		if p.options.Plugin != nil {
			body = p.options.Plugin(tab)
		}
	case "browser":
		if p.options.Browser != nil {
			body = p.options.Browser(tab, func(title, address string) {
				for i := range p.tabs {
					if p.tabs[i].ID == tab.ID {
						p.tabs[i].Title, p.tabs[i].URL = title, address
					}
				}
			}, func() { p.requestClose(tab.ID) })
		}
	}
	if body != nil {
		p.bodies[tab.ID] = body
	}
	return body
}
func (p *Panel) remove(id string) {
	if id == "panel" {
		if p.options.OnClose != nil {
			p.options.OnClose()
		}
		return
	}
	index := -1
	for i, tab := range p.tabs {
		if tab.ID == id {
			index = i
			p.closedTabs = append(p.closedTabs, tab)
			break
		}
	}
	if index < 0 {
		return
	}
	if body := p.bodies[id]; body != nil {
		body.Close()
		delete(p.bodies, id)
	}
	p.tabs = append(p.tabs[:index], p.tabs[index+1:]...)
	if p.selected == id {
		p.selected = ""
		if len(p.tabs) > 0 {
			p.selected = p.tabs[min(index, len(p.tabs)-1)].ID
		}
	}
	p.syncVisibility()
	p.savePreferences()
	if len(p.closeQueue) > 0 {
		next := p.closeQueue[0]
		p.closeQueue = p.closeQueue[1:]
		p.requestClose(next)
	}
}
func (p *Panel) requestClose(id string) {
	if p.guard != nil {
		return
	}
	terminal := false
	dirty := false
	if id == "panel" {
		terminal = p.HasTerminal()
		dirty = p.Dirty()
	} else {
		body := p.bodies[id]
		if d, ok := body.(interface{ Dirty() bool }); ok {
			dirty = d.Dirty()
		}
		if term, ok := body.(*TerminalPane); ok {
			terminal = term.Running() || term.Busy()
		}
	}
	if terminal || dirty {
		p.guard = &panelGuard{id: id, stopTerminal: terminal}
		p.syncVisibility()
		return
	}
	p.remove(id)
}
func (p *Panel) closeMany(ids []string) {
	if p.guard != nil {
		return
	}
	p.closeQueue = append([]string(nil), ids...)
	if len(p.closeQueue) > 0 {
		next := p.closeQueue[0]
		p.closeQueue = p.closeQueue[1:]
		p.requestClose(next)
	}
}
func (p *Panel) saveGuard() {
	guard := p.guard
	if guard == nil || guard.saving {
		return
	}
	docs := []*DocumentPane{}
	if guard.switchTo != "" && p.libraryDocument != nil {
		docs = append(docs, p.libraryDocument)
	} else if guard.id == "panel" {
		if p.libraryDocument != nil && p.libraryDocument.Dirty() {
			docs = append(docs, p.libraryDocument)
		}
		for _, body := range p.bodies {
			if doc, ok := body.(*DocumentPane); ok && doc.Dirty() {
				docs = append(docs, doc)
			}
		}
	} else if doc, ok := p.bodies[guard.id].(*DocumentPane); ok {
		docs = append(docs, doc)
	}
	guard.saving = true
	var next func(int)
	next = func(index int) {
		if p.closed || p.guard != guard {
			return
		}
		if index == len(docs) {
			guard.saving = false
			p.finishGuard()
			return
		}
		docs[index].Save(func(err error) {
			if p.closed || p.guard != guard {
				return
			}
			if err != nil {
				guard.saving = false
				guard.error = err.Error()
				return
			}
			next(index + 1)
		})
	}
	next(0)
}
func (p *Panel) finishGuard() {
	guard := p.guard
	if guard == nil {
		return
	}
	p.guard = nil
	if guard.leave != nil {
		guard.leave()
		return
	}
	if guard.switchTo != "" {
		p.openLibraryDocument(guard.switchTo, guard.switchLine)
		return
	}
	p.remove(guard.id)
}
func (p *Panel) chooseLibraryFile(path string, line int, newTab bool) {
	if newTab {
		p.OpenFile(path, line)
		return
	}
	if p.libraryDocument != nil && path != p.libraryPath && p.libraryDocument.Dirty() {
		p.guard = &panelGuard{switchTo: path, switchLine: line}
		p.syncVisibility()
		return
	}
	p.openLibraryDocument(path, line)
}
func (p *Panel) openLibraryDocument(path string, line int) {
	if p.libraryDocument != nil && p.libraryPath == path {
		if line > 0 && p.libraryDocument.text != nil {
			p.libraryDocument.text.rendered = false
			p.libraryDocument.text.state.GoTo(editor.Pos{Line: line - 1})
		}
		return
	}
	if p.libraryDocument != nil {
		p.libraryDocument.Close()
	}
	p.libraryPath = path
	p.libraryDocument = NewDocumentPane(p.env, DocumentOptions{Prefix: p.options.Prefix, SessionID: p.options.SessionID, Path: path, Line: line, OnLink: p.OpenBrowser, Media: p.options.Media, HTML: p.options.HTML})
	p.syncVisibility()
}
func (p *Panel) View(c *ui.Context) {
	if p.closed {
		return
	}
	p.syncVisibility()
	pal := kit.Colors(c)
	ui.Column(c).Fill().MinWidth(0).Bind(&p.handle).Children(func() {
		// As tall as the conversation's header beside it, on the same hairline.
		ui.Row(c).FillWidth().MinHeight(panelHeaderHeight).Shrink(0).Padding(8, 8, 8, 12).Gap(4).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(pal.Hairline).Children(func() {
			section := p.section
			fillSwitch(c, &section, []kit.SelectOption{{Value: "library", Label: i18n.T("chat.panelLibrary")}, {Value: "tabs", Label: i18n.T("chat.panelDocuments")}}, i18n.T("chat.panelSections"), false)
			if section != p.section {
				p.section = section
				p.syncVisibility()
				p.savePreferences()
			}
			kit.IconButton(c, icons.Ellipsis, i18n.T("chat.panelActions")).Menu(func(m *ui.Menu) {
				for _, kind := range []string{"file", "browser", "terminal", "review", "files", "resources"} {
					if m.Item(i18n.T("chat." + kind)).Chosen() {
						p.newTab(kind)
					}
				}
				if m.Item(i18n.T("chat.reopenTab")).Disabled(len(p.closedTabs) == 0).Chosen() {
					tab := p.closedTabs[len(p.closedTabs)-1]
					p.closedTabs = p.closedTabs[:len(p.closedTabs)-1]
					p.open(tab)
				}
				if m.Item(i18n.T("chat.dockBottom")).Chosen() && p.options.OnDock != nil {
					p.options.OnDock()
				}
			})
			if kit.IconButton(c, icons.PanelRightClose, i18n.T("chat.hideDetails")).Clicked() && p.options.OnClose != nil {
				p.options.OnClose()
			}
		})
		if p.err != "" {
			ui.Column(c).FillWidth().Padding(8, 12, 0, 12).Children(func() { kit.InlineError(c, p.err) })
		}
		if p.section == "library" {
			p.libraryView(c)
		} else {
			p.tabsView(c)
		}
	})
	p.guardView(c)
	p.createFileView(c)
}

// panelHeaderHeight matches the Chat page's header.
const panelHeaderHeight = 56

// fillSwitch chooses one of a few views with segments of equal width across
// the panel, their names centered (docs/working-rules/ui.md): the panel's
// two sections in a squared frame, a section's views in a pill.
func fillSwitch(c *ui.Context, selected *string, options []kit.SelectOption, label string, pill bool) bool {
	t, pal := c.Theme(), kit.Colors(c)
	index := 0
	for i, option := range options {
		if option.Value == *selected {
			index = i
		}
	}
	before := index
	outer, inner := t.Radius, t.Radius-2
	if pill {
		outer, inner = 999, 999
	}
	seg := ui.SegmentedBase(c, &index, len(options))
	seg.Track.Grow(1).Basis(0).MinWidth(0).Padding(3).Gap(2).Radius(outer).Border(1, pal.Hairline).Label(label).Children(func() {
		for i, option := range options {
			item := seg.Segment(i).Grow(1).Basis(0).MinWidth(0).MinHeight(28).Padding(0, 8).Radius(inner).Justify(ui.Center).AlignItems(ui.Center).Label(option.Label)
			if i == index {
				item.Background(pal.Selected)
			} else if item.Hovered() {
				item.Background(t.SurfaceHover)
			}
			item.Children(func() {
				text := ui.Text(c, option.Label).SingleLine().Ellipsis("…").MinWidth(0)
				if i == index {
					text.FontWeight(500)
				} else {
					text.TextColor(t.TextMuted)
				}
			})
		}
	})
	if index != before && index < len(options) {
		*selected = options[index].Value
		return true
	}
	return false
}

func (p *Panel) libraryView(c *ui.Context) {
	ui.Column(c).Fill().MinWidth(0).Children(func() {
		ui.Row(c).FillWidth().Shrink(0).Padding(8, 12).Children(func() {
			if fillSwitch(c, &p.library, []kit.SelectOption{{Value: "files", Label: i18n.T("chat.files")}, {Value: "resources", Label: i18n.T("chat.resources")}}, i18n.T("chat.panelLibraryViews"), true) {
				p.syncVisibility()
				p.savePreferences()
			}
		})
		if p.library == "resources" {
			if p.resources == nil {
				p.resources = NewResourcePane(p.env, ResourceOptions{Prefix: p.options.Prefix, SessionID: p.options.SessionID, OnOpenFile: func(path string) { p.OpenFile(path, 0) }, OnReference: func(path string) {
					if p.options.OnReference != nil {
						if err := p.options.OnReference(path); err != nil {
							p.err = err.Error()
						}
					}
				}})
			}
			p.resources.View(c)
			return
		}
		if p.fileLibrary == nil {
			p.fileLibrary = NewFileLibrary(p.env, LibraryOptions{Prefix: p.options.Prefix, SessionID: p.options.SessionID, Directory: p.options.Directory, OnOpen: p.chooseLibraryFile, OnReference: func(path string) {
				if p.options.OnReference != nil {
					if err := p.options.OnReference(path); err != nil {
						p.err = err.Error()
					}
				}
			}})
		}
		p.fileLibrary.SetVisible(p.visible)
		kit.Split(c, &p.librarySplit, kit.SplitOptions{Label: i18n.T("chat.resizeFileTree"), MinFirst: 25, MinSecond: 35}, func() { p.fileLibrary.View(c) }, func() {
			if p.libraryDocument != nil {
				p.libraryDocument.View(c)
			} else {
				emptyHint(c, icons.FileText, wt("chooseDocument"))
			}
		}).Grow(1).MinHeight(0)
	})
}

// emptyHint fills a pane that shows nothing yet: a quiet icon over a
// sentence, centered.
func emptyHint(c *ui.Context, icon *ui.SVG, text string) ui.Element {
	t := c.Theme()
	return ui.Column(c).Fill().MinWidth(0).Padding(24).Gap(10).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
		ui.Icon(c, icon).FontSize(24).TextColor(t.TextMuted.Alpha(0.6))
		kit.Help(c, text).TextAlign(ui.Center)
	})
}

// tabIcons picture a tab's kind before its title.
var tabIcons = map[string]*ui.SVG{"file": icons.FileText, "browser": icons.Globe, "terminal": icons.SquareTerminal, "review": icons.FileDiff, "plugin": icons.Puzzle}

func (p *Panel) tabsView(c *ui.Context) {
	t, pal := c.Theme(), kit.Colors(c)
	tabs := p.visibleTabs()
	selected := p.visibleSelection(tabs)
	ui.Column(c).Fill().MinWidth(0).Children(func() {
		if len(tabs) == 0 {
			ui.Column(c).Fill().MinWidth(0).Padding(24).Gap(16).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
				kit.Help(c, wt("emptyTabs")).TextAlign(ui.Center)
				ui.Row(c).MaxWidth(320).Gap(8).Wrap().Justify(ui.Center).Children(func() {
					for _, kind := range []string{"file", "browser", "terminal", "review"} {
						if kit.Button(c, i18n.T("chat."+kind), tabIcons[kind], false).Clicked() {
							p.newTab(kind)
						}
					}
				})
			})
			return
		}
		// The strip wraps instead of scrolling sideways; a tab is as wide
		// as its share of a line, within limits.
		ui.Row(c).FillWidth().Shrink(0).Gap(2).Wrap().AlignItems(ui.Center).Padding(6, 8).BorderWidth(0, 0, 1, 0).BorderColor(pal.Hairline).Children(func() {
			for _, tab := range tabs {
				row := ui.Row(c.Key(tab.ID)).MinWidth(0).Grow(1).Basis(110).MaxWidth(220).Height(32).Radius(t.Radius).AlignItems(ui.Center).Drag(panelDrag{p, tab.ID})
				chosen := tab.ID == selected
				hovered := row.Hovered()
				if chosen {
					row.Background(pal.Selected)
				} else if hovered {
					row.Background(t.SurfaceHover)
				}
				row.Children(func() {
					title := tab.Title
					if title == "" {
						title = i18n.T("chat." + tab.Kind)
					}
					if document, ok := p.bodies[tab.ID].(*DocumentPane); ok && document.Dirty() {
						title += " ·"
					}
					button := ui.ButtonBase(c).Grow(1).MinWidth(0).FillHeight().Padding(0, 2, 0, 10).Gap(6).Justify(ui.Start).AlignItems(ui.Center).Label(title).Tooltip(title)
					button.Children(func() {
						icon := tabIcons[tab.Kind]
						if icon == nil {
							icon = icons.FileText
						}
						ui.Icon(c, icon).FontSize(14).TextColor(t.TextMuted).Shrink(0)
						text := ui.Text(c, title).SingleLine().Ellipsis("…").MinWidth(0)
						if !chosen {
							text.TextColor(t.TextMuted)
						}
					})
					if button.Clicked() {
						p.selected = tab.ID
						p.syncVisibility()
						p.savePreferences()
					}
					// Closing keeps its place and keyboard focus on every tab,
					// and shows on the chosen one and under the pointer.
					closing := kit.IconButton(c, icons.X, i18n.T("chat.closeTab", "name", title)).Size(24, 24).Margin(0, 4, 0, 0)
					if !chosen && !hovered && !closing.FocusVisible() {
						closing.Opacity(0)
					}
					if closing.Clicked() {
						p.requestClose(tab.ID)
					}
				})
				if dragged, ok := ui.Drop[panelDrag](row); ok && dragged.panel == p {
					p.reorder(dragged.id, tab.ID)
				}
				row.ContextMenu(func(m *ui.Menu) {
					if m.Item(i18n.T("chat.pinTab")).Chosen() {
						for i := range p.tabs {
							if p.tabs[i].ID == tab.ID {
								p.tabs[i].Pinned = !p.tabs[i].Pinned
							}
						}
						slices.SortStableFunc(p.tabs, func(a, b PanelTab) int {
							if a.Pinned == b.Pinned {
								return 0
							}
							if a.Pinned {
								return -1
							}
							return 1
						})
						p.savePreferences()
					}
					if m.Item(wt("closeOthers")).Chosen() {
						ids := []string{}
						for _, other := range p.tabs {
							if other.ID != tab.ID {
								ids = append(ids, other.ID)
							}
						}
						p.closeMany(ids)
					}
					if m.Item(wt("closeAll")).Chosen() {
						ids := []string{}
						for _, other := range p.tabs {
							ids = append(ids, other.ID)
						}
						p.closeMany(ids)
					}
				})
			}
			if kit.IconButton(c, icons.Plus, i18n.T("chat.paneNewTab")).Size(32, 32).Clicked() {
				p.newTab("browser")
			}
		})
		for _, tab := range tabs {
			if tab.ID == selected {
				if body := p.body(tab); body != nil {
					body.View(c)
				}
			}
		}
	})
}
func (p *Panel) reorder(id, before string) {
	if id == before {
		return
	}
	index := slices.IndexFunc(p.tabs, func(tab PanelTab) bool { return tab.ID == id })
	if index < 0 {
		return
	}
	tab := p.tabs[index]
	p.tabs = slices.Delete(p.tabs, index, index+1)
	target := slices.IndexFunc(p.tabs, func(tab PanelTab) bool { return tab.ID == before })
	if target < 0 {
		target = len(p.tabs)
	}
	p.tabs = slices.Insert(p.tabs, target, tab)
	p.savePreferences()
}
func (p *Panel) guardDirty(guard *panelGuard) bool {
	if guard.switchTo != "" {
		return p.libraryDocument != nil && p.libraryDocument.Dirty()
	}
	if guard.id == "panel" {
		return p.Dirty()
	}
	if document, ok := p.bodies[guard.id].(*DocumentPane); ok {
		return document.Dirty()
	}
	return false
}

func (p *Panel) guardView(c *ui.Context) {
	guard := p.guard
	if guard == nil {
		return
	}
	open := true
	title, message, confirm := i18n.T("chat.unsavedFile"), i18n.T("chat.unsavedFileClose"), i18n.T("chat.saveFile")
	if guard.stopTerminal {
		title, message, confirm = i18n.T("chat.terminalClose"), i18n.T("chat.terminalCloseHint"), i18n.T("chat.terminalClose")
	}
	if kit.Dialog(c, &open, kit.DialogOptions{Title: title, Message: message, Confirm: confirm, Cancel: i18n.T("chat.cancel"), Busy: guard.saving, Error: guard.error, Actions: func() {
		if !guard.stopTerminal && kit.Button(c, i18n.T("chat.discardFile"), icons.Trash2, false).Disabled(guard.saving).Clicked() {
			p.finishGuard()
		}
	}}) {
		if guard.stopTerminal {
			guard.stopTerminal = false
			if !p.guardDirty(guard) {
				p.finishGuard()
			}
		} else {
			p.saveGuard()
		}
	}
	if !open {
		p.guard = nil
		p.closeQueue = nil
		p.syncVisibility()
		if guard.cancel != nil {
			guard.cancel()
		}
	}
}
func (p *Panel) createFileView(c *ui.Context) {
	if !p.newFileOpen {
		return
	}
	if kit.Dialog(c, &p.newFileOpen, kit.DialogOptions{Title: i18n.T("chat.paneNewPage"), Confirm: i18n.T("chat.create"), Cancel: i18n.T("chat.cancel"), Busy: p.creating, Error: p.err, ConfirmDisabled: p.options.SessionID == "", Body: func() { kit.TextInput(c, &p.newFileName).FillWidth().Label(i18n.T("chat.panePageName")) }}) {
		if p.creating {
			return
		}
		p.creating = true
		path := p.newFileName
		core.Go(p.env, func() (FilePreview, error) {
			var result FilePreview
			err := p.env.Host.CallInto(p.options.Prefix+"file.create", map[string]string{"id": p.options.SessionID, "path": path}, &result)
			return result, err
		}, func(result FilePreview, err error) {
			if p.closed {
				return
			}
			p.creating = false
			if err != nil {
				p.err = err.Error()
				return
			}
			p.newFileOpen = false
			p.OpenFile(result.Path, 0)
			if p.fileLibrary != nil {
				p.fileLibrary.Refresh()
			}
		})
	}
}
func (p *Panel) prefsPath() string {
	return filepath.Join(p.env.Directory, "desktop-work-panel-"+p.options.SessionID+".json")
}
func (p *Panel) loadPreferences() {
	if p.env.Directory == "" || p.options.SessionID == "" {
		return
	}
	data, err := os.ReadFile(p.prefsPath())
	if err != nil || len(data) > 128<<10 {
		return
	}
	var saved panelPreferences
	if json.Unmarshal(data, &saved) != nil {
		return
	}
	for _, tab := range saved.Tabs {
		if len(p.tabs) >= 32 {
			break
		}
		switch tab.Kind {
		case "file", "browser", "review", "terminal", "plugin":
			p.tabs = append(p.tabs, tab)
		}
	}
	p.selected = saved.Selected
	if !slices.ContainsFunc(p.tabs, func(tab PanelTab) bool { return tab.ID == p.selected }) && len(p.tabs) > 0 {
		p.selected = p.tabs[0].ID
	}
	if saved.Section == "library" || saved.Section == "tabs" {
		p.section = saved.Section
	}
	if saved.Library == "files" || saved.Library == "resources" {
		p.library = saved.Library
	}
}
func (p *Panel) savePreferences() {
	if p.env.Directory == "" || p.options.SessionID == "" {
		return
	}
	data, err := json.Marshal(panelPreferences{Tabs: p.tabs, Selected: p.selected, Section: p.section, Library: p.library})
	if err != nil {
		return
	}
	file, err := os.CreateTemp(p.env.Directory, ".work-panel-*.tmp")
	if err != nil {
		p.err = err.Error()
		return
	}
	name := file.Name()
	defer os.Remove(name)
	_, err = file.Write(data)
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(name, p.prefsPath())
	}
	if err != nil {
		p.err = err.Error()
	}
}
