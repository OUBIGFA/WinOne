// Package pluginpage hosts only a plugin's own untrusted HTML. Native page
// lifecycle and embedded focus go to the authenticated plugin Host; the guest
// receives no Go bridge or permission API.
package pluginpage

import (
	"crypto/rand"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"sync/atomic"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
)

var (
	privateScope = regexp.MustCompile(`^[0-9a-f]{64}$`)
	taskID       = regexp.MustCompile(`^[A-Za-z0-9-]{1,64}$`)
)

// issuedPageURL admits only the exact local page capability issued by the
// plugin sandbox. An arbitrary preview/file URL cannot acquire this lifecycle.
func issuedPageURL(address string) (*url.URL, error) {
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Port() == "" || u.User != nil || strings.ContainsAny(u.Path, "\\\x00") {
		return nil, errors.New(i18n.T("chat.fileReadFailed"))
	}
	parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	if len(parts) != 2 || !privateScope.MatchString(parts[0]) {
		return nil, errors.New(i18n.T("chat.fileReadFailed"))
	}
	switch parts[1] {
	case "__page_settings__", "__page_panel__", "__page_main__":
		if u.RawQuery != "" {
			return nil, errors.New(i18n.T("chat.fileReadFailed"))
		}
	case "__page_task__":
		query, err := url.ParseQuery(u.RawQuery)
		if err != nil || len(query) != 1 || len(query["session"]) != 1 || !taskID.MatchString(query.Get("session")) {
			return nil, errors.New(i18n.T("chat.fileReadFailed"))
		}
	default:
		return nil, errors.New(i18n.T("chat.fileReadFailed"))
	}
	return u, nil
}

func samePluginDocument(initial, next *url.URL) bool {
	prefix := initial.Path[:strings.LastIndex(initial.Path, "/")+1]
	return next.Scheme == initial.Scheme && next.Host == initial.Host && next.User == nil && strings.HasPrefix(next.Path, prefix) && !strings.ContainsAny(next.Path, "\\\x00")
}

type Page struct {
	env        *core.Env
	view       *mygo.WebView
	focus      interface{ IsFocused() bool }
	label      string
	id         string
	off        func()
	closed     atomic.Bool
	visible    bool
	gestureOff func()
	ready      chan struct{}
	attachErr  error
}

// New opens a Host-issued plugin page without exposing WinOne's frontend
// bridge. External navigation is always stopped in the guest and handed to the
// upstream Host gate, whose confirmation is owned by WinOne's native window.
func New(env *core.Env, address, label, pluginID string) (*Page, error) {
	u, err := issuedPageURL(address)
	if err != nil {
		return nil, err
	}
	if env.WebViews == nil {
		return nil, errors.New(i18n.T("web.unavailable"))
	}
	view, err := env.WebViews(mygo.WebViewOptions{
		URL: u.String(), BackgroundColor: "light-dark(#fcfcfc, #191919)",
		Page: mygo.PageOptions{Untrusted: true, Session: mygo.NewSession("winone-plugin-page-" + rand.Text())},
	})
	if err != nil {
		return nil, err
	}
	focus, supported := any(view).(interface{ IsFocused() bool })
	if !supported {
		// Upstream v0.7.4 has no public embedded-focus query. The verified
		// WinOne source-cache hook is required; window focus is not a fallback.
		view.Destroy()
		return nil, errors.New(i18n.T("web.unavailable"))
	}
	p := &Page{env: env, view: view, focus: focus, label: label, id: rand.Text(), ready: make(chan struct{}), visible: true}
	core.Do(env, func() error {
		p.attachErr = env.Host.CallInto("chat.ghosts.page-attach", map[string]string{"pageId": p.id, "pluginId": pluginID, "language": i18n.Locale()}, nil)
		close(p.ready)
		return p.attachErr
	}, p.report)
	p.off = view.Page().OnWillNavigate(func(e *mygo.NavigateEvent) {
		next, err := url.Parse(e.URL)
		previewPath := err == nil && strings.HasPrefix(next.Path, u.Path[:strings.LastIndex(u.Path, "/")+1]+"preview/")
		if err == nil && samePluginDocument(u, next) && !previewPath {
			return
		}
		e.PreventDefault()
		address := e.URL
		core.Do(env, func() error {
			<-p.ready
			if p.closed.Load() || !p.focused() {
				return nil
			}
			if p.attachErr != nil {
				return p.attachErr
			}
			return env.Host.CallInto("chat.ghosts.page-navigation", map[string]any{"pageId": p.id, "url": address, "focused": true, "language": i18n.Locale()}, nil)
		}, p.report)
	})
	p.gestureOff = view.OnUserGesture(func() {
		// Loading/focus/DOM events do not authorize anything. A click before
		// the native attachment is ready is not cached for later replay.
		select {
		case <-p.ready:
		default:
			return
		}
		if p.closed.Load() || p.attachErr != nil || !p.focused() {
			return
		}
		core.Do(env, func() error {
			return env.Host.CallInto("chat.ghosts.page-gesture", map[string]any{"pageId": p.id, "focused": true}, nil)
		}, p.report)
	})
	return p, nil
}

// Native handles stay on the native side; no untrusted document receives them.
func (p *Page) WebView() *mygo.WebView { return p.view }
func (p *Page) ID() string             { return p.id }
func (p *Page) IsFocused() bool        { return p.focused() }
func (p *Page) SetVisible(value bool)  { p.visible = value }
func (p *Page) Focus() {
	if !p.closed.Load() && p.visible {
		p.view.Focus()
	}
}

func (p *Page) focused() bool {
	// The host window being focused is not enough: a native control, a dialog
	// or a different embedded page may own its keyboard. This is native focus,
	// never guest-controlled document.hasFocus() or a page message.
	return !p.closed.Load() && p.visible && !p.view.IsDestroyed() && p.view.Window().IsFocused() && p.focus.IsFocused()
}

func (p *Page) report(err error) {
	if !p.closed.Load() && err != nil && p.env.Toast != nil {
		p.env.Toast(err.Error())
	}
}

func (p *Page) View(c *ui.Context) {
	if !p.closed.Load() && p.visible {
		ui.WebView(c, p.view).Fill().MinWidth(0).MinHeight(0).Label(p.label)
	}
}

func (p *Page) Close() {
	if !p.closed.CompareAndSwap(false, true) {
		return
	}
	if p.off != nil {
		p.off()
	}
	if p.gestureOff != nil {
		p.gestureOff()
	}
	p.view.Destroy()
	// Keep detach ordered after attach even when a page closes during startup.
	// The request still runs if the UI has already discarded its callback.
	core.Do(p.env, func() error {
		<-p.ready
		return p.env.Host.CallInto("chat.ghosts.page-detach", map[string]string{"pageId": p.id}, nil)
	}, func(error) {})
}
