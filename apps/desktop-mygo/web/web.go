// Package web shows pages the Host serves inside a page of the window:
// Magpie's routing editor, the plugin manager, privacy records and the
// panels plugins add. The React window showed them in iframes and talked
// to them with postMessage. Here each is a native web view (MyGo's
// ui.WebView) whose page is the top document, so the page's
// parent.postMessage reaches the page itself: a script injected before the
// page's own carries those messages to Go, and Go answers by posting to the
// page. The pages keep the protocol they had with the React window.
package web

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
)

// scheme carries the pages' messages to Go. WebView2 serves custom schemes
// at http://<scheme>.localhost/, where the injected script posts them.
const scheme = "winone-web"

// Register installs the scheme that carries the pages' messages. Call it
// before the window is made: a window handles the schemes registered when
// it opens.
func Register() error { return mygo.Protocol.HandleFunc(scheme, serve) }

// entry is a surface by the secret only the script in its page knows, and
// the page allowed to post with it. Frames inside the page (a plugin's own
// page) cannot read the secret, so they cannot post as the page.
type entry struct {
	surface *Surface
	page    *mygo.Page
}

var surfaces = struct {
	sync.Mutex
	bySecret map[string]entry
}{bySecret: map[string]entry{}}

// maxMessage bounds a message: the pages post heights, results and
// short requests, never documents.
const maxMessage = 64 << 10

func serve(w http.ResponseWriter, r *http.Request) {
	secret, ok := strings.CutPrefix(r.URL.Path, "/post/")
	surfaces.Lock()
	e, found := surfaces.bySecret[secret]
	surfaces.Unlock()
	if !ok || !found || r.Method != http.MethodPost || mygo.CallerPage(r.Context()) != e.page {
		http.NotFound(w, r)
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxMessage))
	if err != nil {
		http.Error(w, "message too large", http.StatusRequestEntityTooLarge)
		return
	}
	var message Message
	if json.Unmarshal(data, &message) != nil || !slices.Contains(e.surface.o.Accept, message.String("type")) {
		http.Error(w, "unexpected message", http.StatusBadRequest)
		return
	}
	s := e.surface
	s.env.Post(func() { s.receive(message) })
	w.WriteHeader(http.StatusNoContent)
}

// Message is a message a page posted, as decoded from JSON.
type Message map[string]any

// String returns a text field, "" when it is missing or not text.
func (m Message) String(key string) string {
	text, _ := m[key].(string)
	return text
}

// Number returns a number field and whether it is a finite number.
func (m Message) Number(key string) (float64, bool) {
	n, ok := m[key].(float64)
	return n, ok
}

// Options describe a surface.
type Options struct {
	// URL is the page. The surface stays on its origin: links elsewhere
	// open in the browser.
	URL string
	// Label names the page for assistive technology.
	Label string
	// Theme is the type of the message that tells the page the window's
	// language and theme ("winone-plugin-theme" or "winone-routing-theme");
	// the page takes the origin it answers to from it.
	Theme string
	// Accept lists the types of message the page may post to Go.
	Accept []string
	// HeightType is the type of the message reporting the page's height,
	// which the surface takes; Receive does not get it.
	HeightType string
	// Height is the height until the page reports its own.
	Height float32
	// Receive handles the other messages, on the UI thread.
	Receive func(Message)
	// Downloads lets the page save files to the Downloads folder, as a
	// share's page does; the window tells where each went.
	Downloads bool
}

// Surface is a page the Host serves, shown inside a page of the window,
// as React's iframes were. Make it when the page first shows it, and Close
// it when the page closes.
type Surface struct {
	env    *core.Env
	o      Options
	origin string
	secret string
	view   *mygo.WebView
	err    string
	height float32
	loaded bool
	// sent is the language and theme last told to the page.
	sent     string
	services ui.Services
	outbox   chan string
	offs     []func()
	closed   bool
}

// New prepares a surface; its web view is made the first time it shows.
func New(env *core.Env, o Options) *Surface {
	return &Surface{env: env, o: o, height: o.Height}
}

// View shows the page, as high as it reports itself, and returns its
// element for the caller to round or border. disabled keeps the pointer
// from the page while the window works, as React's inert did.
func (s *Surface) View(c *ui.Context, disabled bool) ui.Element {
	s.services = c.Services()
	s.open()
	if s.err != "" {
		return kit.Notice(c, kit.Failure, "", s.err, nil)
	}
	theme := "light"
	if c.Theme().Dark {
		theme = "dark"
	}
	if state := i18n.Locale() + "/" + theme; s.o.Theme != "" && s.loaded && state != s.sent {
		s.sent = state
		s.Post(map[string]any{"type": s.o.Theme, "language": i18n.Locale(), "theme": theme})
	}
	return ui.WebView(c, s.view).FillWidth().Height(s.height).Label(s.o.Label).Children(func() {
		if disabled {
			// Painted over the page, it takes the pointer from it.
			ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Background(c.Theme().Background.Alpha(0.4))
		}
	})
}

// open makes the web view, once.
func (s *Surface) open() {
	if s.view != nil || s.err != "" || s.closed {
		return
	}
	if s.env.WebViews == nil {
		s.err = i18n.T("web.unavailable")
		return
	}
	u, err := url.Parse(s.o.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		s.err = i18n.T("web.invalidAddress")
		return
	}
	s.origin = u.Scheme + "://" + u.Host
	secret := make([]byte, 16)
	_, _ = rand.Read(secret)
	s.secret = hex.EncodeToString(secret)
	// A page that posts nothing gets no script, so no secret.
	script := ""
	if len(s.o.Accept) > 0 {
		script = s.script()
	}
	view, err := s.env.WebViews(mygo.WebViewOptions{
		URL:             s.o.URL,
		BackgroundColor: "light-dark(#fcfcfc, #191919)",
		Page: mygo.PageOptions{
			PreloadScript: script,
			// The pages are not the window's: they call no Go method, and
			// what they may do beyond their origin the handlers decide.
			Untrusted: true,
		},
	})
	if err != nil {
		s.err = err.Error()
		return
	}
	s.view = view
	page := view.Page()
	surfaces.Lock()
	surfaces.bySecret[s.secret] = entry{surface: s, page: page}
	surfaces.Unlock()
	s.outbox = make(chan string, 64)
	go deliver(page, s.outbox)
	s.offs = append(s.offs,
		page.OnDidFinishLoad(func() {
			s.env.Post(func() {
				// A new document listens afresh: tell it again.
				s.loaded, s.sent = true, ""
			})
		}),
		page.OnWillNavigate(func(e *mygo.NavigateEvent) {
			if sameOrigin(e.URL, s.origin) {
				return
			}
			e.PreventDefault()
			if e.UserInitiated && ui.SafeToOpen(e.URL) {
				s.env.Post(func() { s.services.OpenURL(e.URL) })
			}
		}),
	)
	if s.o.Downloads {
		// Listening lets an untrusted page download, to the default path.
		s.offs = append(s.offs,
			page.OnWillDownload(func(*mygo.DownloadEvent) {}),
			page.OnDownloadDone(func(d *mygo.Download) {
				s.env.Post(func() {
					if d.Err != nil {
						s.env.Toast(i18n.T("web.downloadFailed", "error", d.Err.Error()))
					} else {
						s.env.Toast(i18n.T("web.downloaded", "path", d.Path))
					}
				})
			}),
		)
	}
	// Links and window.open go to the browser, as from the React window.
	page.SetWindowOpenHandler(func(req mygo.WindowOpenRequest) *mygo.WindowOptions {
		if req.UserInitiated && ui.SafeToOpen(req.URL) {
			s.env.Post(func() { s.services.OpenURL(req.URL) })
		}
		return nil
	})
}

// deliver runs the posts of a page in order, off the UI thread: Eval waits
// for the page.
func deliver(page *mygo.Page, outbox <-chan string) {
	for code := range outbox {
		_, _ = page.Eval(code)
	}
}

func sameOrigin(raw, origin string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme+"://"+u.Host == origin
}

// Post sends message to the page as its parent did, from the page's own
// origin, which the page accepts once the theme message named it.
func (s *Surface) Post(message any) {
	if s.outbox == nil || s.closed {
		return
	}
	data, err := json.Marshal(message)
	if err != nil {
		return
	}
	select {
	case s.outbox <- "window.postMessage(" + string(data) + ", location.origin)":
	default:
		// The page stopped answering; a later message tells it again.
	}
}

// Load shows another page of the surface's origin, as a new src of
// React's iframe did.
func (s *Surface) Load(raw string) {
	if !sameOrigin(raw, s.origin) {
		return
	}
	s.o.URL = raw
	if s.view != nil {
		s.loaded = false
		_ = s.view.Page().LoadURL(raw)
	}
}

// URL returns the page the surface shows.
func (s *Surface) URL() string { return s.o.URL }

func (s *Surface) receive(message Message) {
	if s.closed {
		return
	}
	if message.String("type") == s.o.HeightType && s.o.HeightType != "" {
		if height, ok := message.Number("height"); ok && height > 0 && height < 1e6 {
			s.height = float32(height)
		}
		return
	}
	if s.o.Receive != nil {
		s.o.Receive(message)
	}
}

// Close destroys the web view and forgets the page's secret.
func (s *Surface) Close() {
	if s.closed {
		return
	}
	s.closed = true
	surfaces.Lock()
	delete(surfaces.bySecret, s.secret)
	surfaces.Unlock()
	for _, off := range s.offs {
		off()
	}
	if s.outbox != nil {
		close(s.outbox)
	}
	if s.view != nil {
		s.view.Destroy()
	}
}

// script carries the page's posts to its parent to Go. A top-level page's
// parent is itself, so its posts arrive as messages from its own window;
// frames inside it are other windows and are ignored, as React ignored
// what did not come from the iframe's document.
func (s *Surface) script() string {
	accept, _ := json.Marshal(s.o.Accept)
	endpoint := "http://" + scheme + ".localhost/post/" + s.secret
	return fmt.Sprintf(`(() => {
  const origin = %q, endpoint = %q, accept = new Set(%s);
  if (location.origin !== origin) return;
  const send = window.fetch.bind(window);
  let queue = Promise.resolve();
  addEventListener("message", event => {
    if (event.source !== window || event.origin !== origin) return;
    const data = event.data;
    if (!data || typeof data !== "object" || !accept.has(data.type)) return;
    let body;
    try { body = JSON.stringify(data); } catch { return; }
    queue = queue.then(() => send(endpoint, {method: "POST", mode: "no-cors", body})).catch(() => {});
  });
})();`, s.origin, endpoint, accept)
}
