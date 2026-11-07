// Package mcpapp is the native content host of a standard MCP App. The original
// Cindy backend/official SDK owns its protocol and policy, never a Go binding.
package mcpapp

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
)

var viewID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var nativeKey = regexp.MustCompile(`^[a-f0-9]{64}$`)

type Session struct {
	ViewID        string `json:"viewId"`
	Transport     string `json:"transport"`
	PrefersBorder bool   `json:"prefersBorder"`
	Policy        struct {
		FrameDomains []string `json:"frameDomains"`
	} `json:"policy"`
	Bridge struct {
		BaseURL string `json:"baseUrl"`
		Key     string `json:"key"`
	} `json:"bridge"`
}

// UIState is public UI state only, produced by the trusted proxy. It carries no
// tool result, HTML, credential or model-context data.
type UIState struct {
	Height      float32 `json:"height"`
	DisplayMode string  `json:"displayMode"`
	Initialized bool    `json:"initialized"`
	Failed      bool    `json:"failed"`
}

type framePolicy interface{ close() }

type Page struct {
	env                                        *core.Env
	caller, sessionID, clientID, openID, title string
	ctx                                        context.Context
	cancel                                     context.CancelFunc
	ready, done                                chan struct{}
	closing, disposed, proxyServed             atomic.Bool
	mu                                         sync.Mutex
	session                                    Session
	view                                       *mygo.WebView
	offNavigate, offPolicy                     func()
	policy                                     framePolicy
	err                                        error
	state                                      UIState
	lastContext                                string
	// OnState runs on the native UI thread. The container can use the original
	// bounded inline height and actual Host display mode without trusting App RPC.
	OnState func(UIState)
}

func validateSession(session Session) error {
	address, err := url.Parse(session.Bridge.BaseURL)
	if err != nil || !viewID.MatchString(session.ViewID) || session.Transport != "sandbox" || !nativeKey.MatchString(session.Bridge.Key) ||
		address.Scheme != "http" || address.Hostname() != "127.0.0.1" || address.Port() == "" || address.User != nil ||
		address.Path != "/"+session.ViewID || address.RawQuery != "" || address.Fragment != "" {
		return errors.New("MCP_APP_VIEW_UNAVAILABLE")
	}
	return nil
}

// New opens asynchronously: creating a card never blocks the native UI thread.
// Failed/cancelled opens keep their exact receipt; they are not retried or replayed.
func New(env *core.Env, sessionID, clientID, title string) (*Page, error) {
	if env == nil || env.Host == nil || env.WebViews == nil || env.Post == nil || sessionID == "" || clientID == "" || len(sessionID) > 256 || len(clientID) > 256 {
		return nil, errors.New("MCP_APP_SESSION_UNAVAILABLE")
	}
	if protocolErr != nil {
		return nil, protocolErr
	}
	ctx, cancel := context.WithCancel(context.Background())
	p := &Page{env: env, sessionID: sessionID, clientID: clientID, title: title, caller: "native:" + strconv.Itoa(os.Getpid()) + ":" + rand.Text(), openID: rand.Text(),
		ctx: ctx, cancel: cancel, ready: make(chan struct{}), done: make(chan struct{}), state: UIState{Height: 320, DisplayMode: "inline"}}
	core.Go(env, func() (Session, error) {
		var reply Session
		err := p.call(map[string]any{"op": "open", "clientId": clientID, "openId": p.openID, "context": map[string]any{"platform": "desktop", "locale": i18n.Locale()}}, &reply)
		if err == nil {
			err = validateSession(reply)
		}
		p.mu.Lock()
		p.session, p.err = reply, err
		p.mu.Unlock()
		close(p.ready)
		return reply, err
	}, func(session Session, err error) {
		if p.closing.Load() {
			return
		}
		if err == nil {
			err = p.attach(session)
		}
		if err != nil {
			p.fail(err)
		}
	})
	return p, nil
}

func (p *Page) call(input map[string]any, out any) error {
	return p.env.Host.CallInto("chat.ghosts.mcp-app-native", map[string]any{"caller": p.caller, "ownerPid": os.Getpid(), "sessionId": p.sessionID, "input": input}, out)
}
func (p *Page) attach(session Session) error {
	// Create blank first, install native identity/frame policy, then load exactly
	// the Host-issued proxy. There is no window in which an App has a Go bridge.
	view, err := p.env.WebViews(mygo.WebViewOptions{URL: "about:blank", BackgroundColor: "light-dark(#fcfcfc, #191919)",
		Page: mygo.PageOptions{Untrusted: true, Session: mygo.NewSession("winone-mcp-app-" + rand.Text())}})
	if err != nil {
		return err
	}
	p.mu.Lock()
	p.view = view
	p.mu.Unlock()
	// WebView2 creation is asynchronous. Wait on a real native roundtrip while
	// the page is still blank, then attach COM policy before any App HTML runs.
	core.Go(p.env, func() (bool, error) { _, err := view.Page().Eval("return true"); return true, err }, func(_ bool, err error) {
		if p.closing.Load() {
			view.Destroy()
			return
		}
		if err == nil {
			err = p.load(view, session)
		}
		if err != nil {
			p.fail(err)
		}
	})
	return nil
}
func (p *Page) load(view *mygo.WebView, session Session) error {
	policy, err := installFramePolicy(view, session, p.fail)
	if err != nil {
		view.Destroy()
		return err
	}
	p.policy = policy
	p.offPolicy = policy.close
	p.offNavigate = view.Page().OnWillNavigate(func(event *mygo.NavigateEvent) {
		next, err := url.Parse(event.URL)
		if err != nil || !sameProxyDocument(session.ViewID, next) {
			event.PreventDefault()
		}
	})
	view.Page().SetWindowOpenHandler(func(mygo.WindowOpenRequest) *mygo.WindowOptions { return nil })
	view.Page().OnWillDownload(func(event *mygo.DownloadEvent) { event.PreventDefault() })
	view.Page().SetPermissionHandler(func(mygo.PermissionRequest) bool { return false })
	view.Page().SetAuthHandler(func(mygo.AuthRequest) mygo.AuthResponse { return mygo.AuthResponse{} })
	mounted.Lock()
	mounted.pages[session.ViewID] = p
	mounted.Unlock()
	view.Page().LoadURL("cindy-mcp-app://" + session.ViewID + "/sandbox")
	return nil
}
func sameProxyDocument(id string, address *url.URL) bool {
	return address.Scheme == "cindy-mcp-app" && address.Host == id && address.User == nil && address.Path == "/sandbox" && address.RawQuery == ""
}
func (p *Page) fail(err error) {
	p.env.Post(func() {
		if p.closing.Load() {
			return
		}
		p.mu.Lock()
		p.err = err
		p.mu.Unlock()
		if p.env.Toast != nil {
			p.env.Toast(i18n.T("web.unavailable"))
		}
		p.Close()
	})
}
func (p *Page) State() UIState        { p.mu.Lock(); defer p.mu.Unlock(); return p.state }
func (p *Page) Err() error            { p.mu.Lock(); defer p.mu.Unlock(); return p.err }
func (p *Page) Done() <-chan struct{} { return p.done }

func (p *Page) receiveState(state UIState) {
	p.env.Post(func() {
		if p.closing.Load() {
			return
		}
		p.mu.Lock()
		p.state = state
		p.mu.Unlock()
		if p.OnState != nil {
			p.OnState(state)
		}
		if state.Failed {
			p.fail(errors.New("MCP_APP_SANDBOX_FAILED"))
		}
	})
}

// Context updates are Host-validated data. Tool identity and capabilities cannot
// be overridden; dimensions change only when the native allocation changes.
func (p *Page) Context(patch map[string]any) {
	if p.closing.Load() {
		return
	}
	bytes, err := json.Marshal(patch)
	if err != nil {
		return
	}
	serialized := string(bytes)
	p.mu.Lock()
	if p.lastContext == serialized {
		p.mu.Unlock()
		return
	}
	p.lastContext = serialized
	session := p.session
	p.mu.Unlock()
	if session.ViewID == "" {
		return
	}
	var captured map[string]any
	if json.Unmarshal(bytes, &captured) != nil {
		return
	}
	core.Do(p.env, func() error {
		return p.call(map[string]any{"op": "context", "viewId": session.ViewID, "context": captured}, nil)
	}, func(err error) {
		if err != nil {
			p.fail(err)
		}
	})
}
func (p *Page) DisplayMode(mode string) {
	if p.closing.Load() || mode != "inline" && mode != "fullscreen" {
		return
	}
	p.mu.Lock()
	id := p.session.ViewID
	p.mu.Unlock()
	if id == "" {
		return
	}
	core.Do(p.env, func() error { return p.call(map[string]any{"op": "display-mode", "viewId": id, "mode": mode}, nil) }, func(err error) {
		if err != nil {
			p.fail(err)
		}
	})
}
func (p *Page) View(c *ui.Context) {
	if p.closing.Load() {
		return
	}
	p.mu.Lock()
	view, err := p.view, p.err
	p.mu.Unlock()
	if err != nil {
		kit.Help(c, i18n.T("web.unavailable"))
		return
	}
	if view == nil {
		kit.Help(c, i18n.T("chat.loading"))
		return
	}
	element := ui.WebView(c, view).Fill().MinWidth(0).MinHeight(0).Label(p.title)
	width := element.Bounds().W
	p.Context(nativeContext(c, width))
}

// The original App style variables are bound to WinOne's existing native kit,
// not an independently drawn theme. IANA zone/device facts come from the
// trusted proxy's Intl/navigator environment, never the App frame.
func nativeContext(c *ui.Context, width float32) map[string]any {
	t, palette := c.Theme(), kit.Colors(c)
	color := func(value ui.Color) string {
		return fmt.Sprintf("#%02x%02x%02x%02x", value.R, value.G, value.B, value.A)
	}
	theme := "light"
	if t.Dark {
		theme = "dark"
	}
	return map[string]any{"theme": theme, "locale": i18n.Locale(), "containerDimensions": map[string]any{"width": width, "maxHeight": 5000},
		"styles": map[string]any{"variables": map[string]string{
			"--color-background-primary": color(t.Background), "--color-background-secondary": color(t.Surface), "--color-background-tertiary": color(t.SurfaceHover),
			"--color-text-primary": color(t.Text), "--color-text-secondary": color(t.TextMuted), "--color-text-tertiary": color(t.TextMuted),
			"--color-border-primary": color(t.Border), "--color-border-secondary": color(palette.Hairline),
			"--color-text-danger": color(t.Danger), "--color-text-warning": color(t.Warning),
			"--font-sans": t.Font, "--font-mono": kit.MonoFont, "--border-radius-md": fmt.Sprintf("%gpx", t.Radius),
		}},
	}
}

// Keep the actual page/transport mounted until the official bounded teardown
// finishes. A dropped UI callback must not leak the WebView or its file streams.
func (p *Page) Close() {
	if !p.closing.CompareAndSwap(false, true) {
		return
	}
	go func() {
		select {
		case <-p.ready:
		default:
			_ = p.call(map[string]any{"op": "cancel-open", "openId": p.openID}, nil)
			<-p.ready
		}
		p.mu.Lock()
		session, view := p.session, p.view
		p.mu.Unlock()
		if session.ViewID != "" {
			_ = p.call(map[string]any{"op": "teardown", "viewId": session.ViewID}, nil)
		}
		p.disposed.Store(true)
		p.cancel()
		mounted.Lock()
		if mounted.pages[session.ViewID] == p {
			delete(mounted.pages, session.ViewID)
		}
		mounted.Unlock()
		mygo.RunOnMain(func() {
			if p.offNavigate != nil {
				p.offNavigate()
			}
			if p.offPolicy != nil {
				p.offPolicy()
			}
			if view != nil {
				view.Destroy()
			}
		})
		close(p.done)
	}()
}

// Scheme sources are private to this exact native page. Parse data before it
// reaches the Handler; encoded separators and arbitrary hostnames stay rejected.
func requestView(address *url.URL) (string, bool) {
	if address.Scheme != "cindy-mcp-app" || address.User != nil || address.RawQuery != "" || address.Fragment != "" || strings.ContainsAny(address.Path, "\\\x00") {
		return "", false
	}
	id := strings.TrimSuffix(address.Host, "-view")
	return id, viewID.MatchString(id) && (address.Host == id || address.Host == id+"-view")
}
