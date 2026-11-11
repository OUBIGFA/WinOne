package workspace

import (
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

// DocumentBody is a native work-panel document's lifecycle; actual web pages
// are only delegated for HTML and browser content, never for native controls.
type DocumentBody interface {
	View(*ui.Context)
	Close()
}
type DocumentOptions struct {
	Prefix, SessionID, Path string
	Line                    int
	OnLink                  func(string)
	Media                   func(FilePreview) (DocumentBody, error)
	HTML                    func(FilePreview) (DocumentBody, error)
	// Alternate preview authorities reuse this renderer without impersonating
	// a Chat session. Each source owns its read and native-file action checks.
	Preview    func() (FilePreview, error)
	FileAction func(string) error
}
type DocumentPane struct {
	env             *core.Env
	options         DocumentOptions
	preview         FilePreview
	body            DocumentBody
	text            *TextDocument
	html            DocumentBody
	htmlMode        bool
	loading, closed bool
	visible         bool
	err             string
}

func NewDocumentPane(env *core.Env, options DocumentOptions) *DocumentPane {
	p := &DocumentPane{env: env, options: options, loading: true, visible: true}
	core.Go(env, func() (FilePreview, error) {
		if options.Preview != nil {
			return options.Preview()
		}
		var preview FilePreview
		err := env.Host.CallInto(options.Prefix+"file.preview", map[string]string{"id": options.SessionID, "path": options.Path}, &preview)
		return preview, err
	}, func(preview FilePreview, err error) {
		if p.closed {
			return
		}
		p.loading = false
		if err != nil {
			p.err = err.Error()
			return
		}
		p.preview = preview
		switch preview.Kind {
		case "text":
			p.text = NewTextDocument(env, TextDocumentOptions{Prefix: options.Prefix, SessionID: options.SessionID, Path: options.Path, Line: options.Line, OnLink: options.OnLink, InitialPreview: &preview, Preview: options.Preview, FileAction: options.FileAction})
			p.body = p.text
			// This pane's header shows the text's tools beside the name.
			p.text.hosted = true
			extension := strings.ToLower(filepath.Ext(options.Path))
			p.htmlMode = options.Line == 0 && (extension == ".htm" || extension == ".html")
		case "image":
			p.body = NewImagePane(env, preview)
		case "pdf":
			p.body = NewPDFPane(env, PDFOptions{Prefix: options.Prefix, SessionID: options.SessionID, Path: options.Path, OnLink: options.OnLink, InitialPreview: &preview, Preview: options.Preview})
		case "audio", "video":
			if options.Media != nil {
				p.body, err = options.Media(preview)
			} else {
				// Normal Chat panels need no override to use the native engine.
				// An optional factory remains for alternate owned preview hosts.
				p.body, err = NewMediaPane(env, preview)
			}
		}
		if err != nil {
			p.err = err.Error()
		}
		// Preview construction finishes asynchronously; use the most recent
		// visibility rather than the state at the time the file was opened.
		p.SetVisible(p.visible)
	})
	return p
}
func (p *DocumentPane) Dirty() bool { return p.text != nil && p.text.Dirty() }
func (p *DocumentPane) Busy() bool {
	if p.body != nil {
		if busy, ok := p.body.(interface{ Busy() bool }); ok {
			return busy.Busy()
		}
	}
	return p.loading
}
func (p *DocumentPane) Save(done func(error)) {
	if p.text != nil {
		p.text.Save(done)
	} else if done != nil {
		done(nil)
	}
}
func (p *DocumentPane) Close() {
	if p.closed {
		return
	}
	p.closed = true
	if p.body != nil {
		p.body.Close()
	}
	if p.html != nil {
		p.html.Close()
	}
}
func (p *DocumentPane) SetVisible(visible bool) {
	p.visible = visible
	for _, body := range []DocumentBody{p.body, p.html} {
		if setter, ok := body.(interface{ SetVisible(bool) }); ok {
			setter.SetVisible(visible)
		}
	}
}
func (p *DocumentPane) View(c *ui.Context) {
	if p.closed {
		return
	}
	ui.Column(c).Fill().MinWidth(0).Children(func() {
		// One line: the file's name, then its tools as icons.
		ui.Row(c).FillWidth().Shrink(0).MinHeight(44).Padding(4, 8, 4, 12).Gap(2).AlignItems(ui.Center).Children(func() {
			ui.Text(c, firstText(p.preview.Name, filepath.Base(p.options.Path))).SingleLine().Ellipsis("…").MinWidth(0).Grow(1).Tooltip(p.options.Path)
			html := strings.EqualFold(filepath.Ext(p.options.Path), ".html") || strings.EqualFold(filepath.Ext(p.options.Path), ".htm")
			if html && p.options.HTML != nil {
				label, icon := i18n.T("chat.fileRendered"), icons.Eye
				if p.htmlMode {
					label, icon = i18n.T("chat.fileSource"), icons.Code
				}
				if toolButton(c, icon, label).Clicked() {
					p.htmlMode = !p.htmlMode
				}
			}
			if p.text != nil {
				p.text.Tools(c)
			}
			toolButton(c, icons.Ellipsis, i18n.T("chat.more")).Menu(func(m *ui.Menu) {
				if m.Item(i18n.T("chat.copyPath")).Chosen() {
					c.Services().WriteClipboard(p.options.Path)
				}
				if m.Item(i18n.T("chat.revealFile")).Chosen() {
					p.fileAction("file.reveal")
				}
				if m.Item(i18n.T("chat.openSystem")).Chosen() {
					p.fileAction("file.open")
				}
			})
		})
		if p.err != "" {
			ui.Column(c).FillWidth().Padding(0, 12, 8, 12).Children(func() { kit.InlineError(c, p.err) })
		}
		if p.loading {
			ui.Row(c).Padding(8, 12).Children(func() { kit.Loader(c) })
		}
		if !p.loading && p.preview.Kind == "text" && p.htmlMode && p.options.HTML != nil {
			if p.html == nil {
				var err error
				p.html, err = p.options.HTML(p.preview)
				if err != nil {
					p.err = err.Error()
				}
			}
			if p.html != nil {
				p.html.View(c)
			}
		} else if p.body != nil {
			p.body.View(c)
		} else if !p.loading && p.err == "" {
			message := i18n.T("chat.previewUnsupported")
			if p.preview.Kind == "oversize" {
				message = i18n.T("chat.previewOversize")
			}
			ui.Column(c).Fill().MinWidth(0).Padding(24).Gap(12).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
				kit.Help(c, message).TextAlign(ui.Center)
				if kit.Button(c, i18n.T("chat.openSystem"), icons.ExternalLink, false).Clicked() {
					p.fileAction("file.open")
				}
			})
		}
	})
}

// toolButton is a pane's tool: an icon of 30 DIPs, named by its tooltip, so
// a narrow pane keeps its tools on one line.
func toolButton(c *ui.Context, icon *ui.SVG, label string) ui.Element {
	return kit.IconButton(c, icon, label).Size(30, 30)
}
func firstText(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
func (p *DocumentPane) fileAction(method string) {
	core.Do(p.env, func() error {
		if p.options.FileAction != nil {
			return p.options.FileAction(method)
		}
		return p.env.Host.CallInto(p.options.Prefix+method, map[string]string{"id": p.options.SessionID, "path": p.options.Path}, nil)
	}, func(err error) {
		if !p.closed && err != nil {
			p.err = err.Error()
		}
	})
}
