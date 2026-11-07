package webui

import (
	"crypto/rand"
	"errors"
	"net/url"
	"regexp"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
)

// HTMLPreview renders only a Host-issued HTML document, with the same sandbox
// response as the former iframe. It has neither the App bridge nor trusted
// frontend storage, permission, downloads or a new-window handler.
type HTMLPreview struct {
	view   *mygo.WebView
	label  string
	off    func()
	closed bool
}

var pluginTaskSessionID = regexp.MustCompile(`^[A-Za-z0-9-]{1,64}$`)

func validPreviewQuery(u *url.URL) bool {
	if u.RawQuery == "" { return true }
	values, err := url.ParseQuery(u.RawQuery)
	return err == nil && strings.HasSuffix(u.Path, "/__page_task__") && len(values) == 1 && len(values["session"]) == 1 && pluginTaskSessionID.MatchString(values.Get("session"))
}

func htmlPreviewURL(address string) (*url.URL, error) {
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Port() == "" || u.User != nil || !validPreviewQuery(u) {
		return nil, errors.New(i18n.T("chat.fileReadFailed"))
	}
	parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || strings.ContainsAny(u.Path, "\\\x00") {
		return nil, errors.New(i18n.T("chat.fileReadFailed"))
	}
	return u, nil
}

func NewHTMLPreview(env *core.Env, address, label string) (*HTMLPreview, error) {
	u, err := htmlPreviewURL(address)
	if err != nil {
		return nil, err
	}
	if env.WebViews == nil {
		return nil, errors.New(i18n.T("web.unavailable"))
	}
	view, err := env.WebViews(mygo.WebViewOptions{
		URL: u.String(), BackgroundColor: "light-dark(#fcfcfc, #191919)",
		Page: mygo.PageOptions{Untrusted: true, Session: mygo.NewSession("winone-html-preview-" + rand.Text())},
	})
	if err != nil {
		return nil, err
	}
	p := &HTMLPreview{view: view, label: label}
	p.off = view.Page().OnWillNavigate(func(e *mygo.NavigateEvent) {
		// Fragment navigation stays in this document. Link/resource origins
		// cannot convert a preview grant into another top-level document.
		next, err := url.Parse(e.URL)
		if err != nil || next.Scheme != u.Scheme || next.Host != u.Host || next.Path != u.Path || next.RawQuery != u.RawQuery {
			e.PreventDefault()
		}
	})
	return p, nil
}

func (p *HTMLPreview) View(c *ui.Context) {
	if !p.closed {
		ui.WebView(c, p.view).Fill().MinWidth(0).MinHeight(0).Label(p.label)
	}
}
func (p *HTMLPreview) Close() {
	if p.closed {
		return
	}
	p.closed = true
	if p.off != nil {
		p.off()
	}
	p.view.Destroy()
}
