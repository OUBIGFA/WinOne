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
	ui.Column(c).Fill().MinWidth(0).Gap(6).Children(func() {
		ui.Row(c).FillWidth().Padding(6).Gap(4).Wrap().Children(func() {
			ui.Text(c, firstText(p.preview.Name, filepath.Base(p.options.Path))).SingleLine().Ellipsis("…").MinWidth(0).Grow(1).Tooltip(p.options.Path)
			html := strings.EqualFold(filepath.Ext(p.options.Path), ".html") || strings.EqualFold(filepath.Ext(p.options.Path), ".htm")
			if html && p.options.HTML != nil {
				label := i18n.T("chat.fileRendered")
				if p.htmlMode {
					label = i18n.T("chat.fileSource")
				}
				if kit.Button(c, label, icons.Code, false).Clicked() {
					p.htmlMode = !p.htmlMode
				}
			}
			kit.IconButton(c, icons.Ellipsis, i18n.T("chat.more")).Menu(func(m *ui.Menu) {
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
		kit.InlineError(c, p.err)
		if p.loading {
			kit.Loader(c)
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
			kit.Help(c, message)
			if kit.Button(c, i18n.T("chat.openSystem"), icons.ExternalLink, false).Clicked() {
				p.fileAction("file.open")
			}
		}
	})
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
