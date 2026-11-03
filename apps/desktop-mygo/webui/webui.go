// Package webui shows the Chat page's React tools in web views of the
// window: the side panel (file library, editors, PDF, review, terminal,
// browser), the automations page and Chat settings, which the user chose
// to keep in React when Chat moved to native UI (the hybrid plan, see
// docs/mygo-parity/chat.md). The page is chat-embed.html of
// apps/desktop/frontend, which `npm run build:mygo` builds into dist/ here
// and the webui build tag embeds as the window's frontend.
//
// The React code reaches its host as it did in the Rust window: through
// window.go.main.App and window.runtime.EventsOn. A script injected before
// the page's own maps those onto MyGo's bound service App (this package's
// app type) and its events, so the components keep their bridge. What the
// page asks of the native Chat page (close the panel, add a reference, a
// browser comment) goes through App.Embed to the Surface that shows it.
package webui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"sync"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
)

// files is the built page, set by the webui build tag (embed_on.go); nil
// when the window was built without it.
var files fs.FS

// Available reports whether the window carries the page.
func Available() bool {
	if files == nil {
		return false
	}
	_, err := fs.Stat(files, "chat-embed.html")
	return err == nil
}

var registered sync.Once

// Register makes the page the window's frontend and binds App. Call it
// before the app runs: MyGo binds services and frontends only then.
func Register() {
	registered.Do(func() {
		if files != nil {
			mygo.SetFrontend(files)
		}
		mygo.BindAs("App", &app{})
	})
}

// dispatch carries a window event to a page: the script below fires it as
// a CustomEvent named name, or calls the runtime.EventsOn listeners of
// name when runtime is set, as the Rust window's events did.
type dispatch struct {
	Name    string          `json:"name"`
	Detail  json.RawMessage `json:"detail,omitempty"`
	Runtime bool            `json:"runtime,omitempty"`
}

var dispatched = mygo.NewEvent[dispatch]("winone:dispatch")

// Options configure a Surface.
type Options struct {
	// View is the page's view: "panel", "automations" or "settings".
	View string
	// Label names the web view for assistive technology.
	Label string
	// Receive takes the page's requests of the native page (App.Embed) on
	// the UI thread: kind names the request, payload is its JSON value.
	// It calls reply once, on the UI thread, now or when work it started
	// ends; the result goes back to the page as JSON.
	Receive func(kind string, payload json.RawMessage, reply func(any, error))
	// Browser hosts the side panel's web pages (App.ChatBrowser); nil
	// refuses them.
	Browser *Browser
}

// Surface is the page in a web view of the window. Make it when the page
// first shows it, and Close it when the page closes.
type Surface struct {
	env  *core.Env
	o    Options
	view *mygo.WebView
	page *mygo.Page
	err  string

	// mu guards state, which the page's ready request reads from App's
	// goroutine, and loaded.
	mu     sync.Mutex
	state  []byte
	sent   []byte
	loaded bool
	closed bool
	offs   []func()
}

// surfaces finds the Surface of a page that calls App.
var surfaces = struct {
	sync.Mutex
	byPage map[*mygo.Page]*Surface
}{byPage: map[*mygo.Page]*Surface{}}

// New prepares a surface; its web view is made the first time it shows.
func New(env *core.Env, o Options) *Surface { return &Surface{env: env, o: o} }

// View shows the page in an element the caller sizes, and sends it state
// when it changed: the look and data every view reads, and the view's own
// props (see chat-embed-entry.tsx).
func (s *Surface) View(c *ui.Context, state any) ui.Element {
	s.open()
	if s.err != "" {
		return kit.Notice(c, kit.Failure, "", s.err, nil)
	}
	if data, err := json.Marshal(state); err == nil {
		s.mu.Lock()
		s.state = data
		send := s.loaded && string(data) != string(s.sent)
		if send {
			s.sent = data
		}
		s.mu.Unlock()
		if send {
			s.emit(dispatch{Name: "winone:embed", Detail: data})
		}
	}
	return ui.WebView(c, s.view).Label(s.o.Label)
}

// open makes the web view, once.
func (s *Surface) open() {
	if s.view != nil || s.err != "" || s.closed {
		return
	}
	if !Available() {
		s.err = i18n.T("chat.toolsUnavailable")
		return
	}
	if s.env.WebViews == nil {
		s.err = i18n.T("web.unavailable")
		return
	}
	view, err := s.env.WebViews(mygo.WebViewOptions{
		URL:             "/chat-embed.html?view=" + url.QueryEscape(s.o.View),
		BackgroundColor: "light-dark(#fcfcfc, #191919)",
		Page:            mygo.PageOptions{PreloadScript: preload},
	})
	if err != nil {
		s.err = err.Error()
		return
	}
	s.view, s.page = view, view.Page()
	surfaces.Lock()
	surfaces.byPage[s.page] = s
	surfaces.Unlock()
	s.offs = append(s.offs,
		s.page.OnDidFinishLoad(func() {
			// A new document asks for its state again with ready.
			s.mu.Lock()
			s.loaded, s.sent = false, nil
			s.mu.Unlock()
		}),
		// The page's own links stay in it; others open in the browser,
		// as from the React window.
		s.page.OnWillNavigate(func(e *mygo.NavigateEvent) {
			if u, err := url.Parse(e.URL); err == nil && u.Path == "/chat-embed.html" {
				return
			}
			e.PreventDefault()
			if e.UserInitiated && ui.SafeToOpen(e.URL) {
				openURL(e.URL)
			}
		}),
	)
	s.page.SetWindowOpenHandler(func(req mygo.WindowOpenRequest) *mygo.WindowOptions {
		if req.UserInitiated && ui.SafeToOpen(req.URL) {
			openURL(req.URL)
		}
		return nil
	})
}

// openURL opens a link in the system's browser, off the UI thread.
var openURL = func(u string) { go mygo.Shell.OpenExternal(u) }

// Dispatch fires a window event named name in the page, with detail.
func (s *Surface) Dispatch(name string, detail any) {
	data, err := json.Marshal(detail)
	if err != nil {
		return
	}
	s.emit(dispatch{Name: name, Detail: data})
}

// Event calls the page's runtime.EventsOn listeners of name.
func (s *Surface) Event(name string) { s.emit(dispatch{Name: name, Runtime: true}) }

func (s *Surface) emit(d dispatch) {
	if s.page == nil || s.closed {
		return
	}
	_ = dispatched.EmitPage(s.page, d)
}

// Page is the surface's page once shown, for probes that read it.
func (s *Surface) Page() *mygo.Page { return s.page }

// Focus gives the page the keyboard.
func (s *Surface) Focus() {
	if s.view != nil {
		s.view.Focus()
	}
}

// Close destroys the web view.
func (s *Surface) Close() {
	if s.closed {
		return
	}
	s.closed = true
	surfaces.Lock()
	delete(surfaces.byPage, s.page)
	surfaces.Unlock()
	for _, off := range s.offs {
		off()
	}
	if s.view != nil {
		s.view.Destroy()
	}
}

// caller is the Surface whose page called App; other pages may not.
func caller(ctx context.Context) (*Surface, error) {
	page := mygo.CallerPage(ctx)
	surfaces.Lock()
	s := surfaces.byPage[page]
	surfaces.Unlock()
	if s == nil || s.closed {
		return nil, errors.New(i18n.T("chat.toolsUnavailable"))
	}
	return s, nil
}

// onUI runs fn on the UI thread and waits for it, as long as the page
// waits.
func (s *Surface) onUI(ctx context.Context, fn func()) error {
	done := make(chan struct{})
	s.env.Post(func() {
		defer close(done)
		fn()
	})
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ready answers the page's first request: it now hears state changes.
func (s *Surface) ready() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loaded, s.sent = true, s.state
	return s.state
}

// embed answers App.Embed.
func (s *Surface) embed(ctx context.Context, kind, payload string) (string, error) {
	if kind == "ready" {
		return string(s.ready()), nil
	}
	if s.o.Receive == nil {
		return "", fmt.Errorf("unsupported request %q", kind)
	}
	var result any
	var err error
	done := make(chan struct{})
	var once sync.Once
	s.env.Post(func() {
		s.o.Receive(kind, json.RawMessage(params(payload)), func(value any, failure error) {
			once.Do(func() {
				result, err = value, failure
				close(done)
			})
		})
	})
	select {
	case <-done:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	if err != nil || result == nil {
		return "", err
	}
	data, err := json.Marshal(result)
	return string(data), err
}
