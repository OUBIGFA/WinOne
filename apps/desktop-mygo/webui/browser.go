package webui

import (
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
)

// Browser is the side panel's web pages, as the Rust window's
// BrowserManager kept them (apps/desktop-rust/src/component_browser.rs):
// the React panel draws the address bar and reports where a tab's page
// shows; Go shows an untrusted web view there, over the panel's page, and
// reports the page's state back as winone:chat-browser events.
//
// Pages of a conversation share a session (their cookies and storage),
// which other conversations don't see. A page talks back only through the
// browser comment script injected into it (browser_comment_guest.js, the
// Rust window's), and the keys the panel handles: its messages wait in the
// page until Go collects them, while the tab shows or a comment is under
// way, and are taken as untrusted data.
type Browser struct {
	env     *core.Env
	surface *Surface
	tabs    map[string]*browserTab
	order   int
}

// BrowserState is a tab's state as the panel reads it (ChatBrowserState).
type BrowserState struct {
	SessionID    string `json:"sessionId"`
	TabID        string `json:"tabId"`
	URL          string `json:"url"`
	Title        string `json:"title"`
	Loading      bool   `json:"loading"`
	CanGoBack    bool   `json:"canGoBack"`
	CanGoForward bool   `json:"canGoForward"`
	Error        string `json:"error"`
	Visible      bool   `json:"visible"`
	Favicon      string `json:"favicon"`
	IsAudible    bool   `json:"isAudible"`
	Muted        bool   `json:"muted"`
	Crashed      bool   `json:"crashed"`
}

// rect is a box in the panel page's CSS pixels.
type rect struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

type browserTab struct {
	key   string
	state BrowserState
	view  *mygo.WebView
	offs  []func()
	// generation tells a page's late events from those of the page that
	// replaced it after a crash or a terminate.
	generation int
	bounds     rect
	occlusion  *rect
	// commenting keeps collecting the page's messages while a comment is
	// under way, even hidden; pumping is the collector running.
	commenting, pumping bool
}

// NewBrowser makes the pages of a Chat page's side panel; Close it with
// the page.
func NewBrowser(env *core.Env) *Browser {
	return &Browser{env: env, tabs: map[string]*browserTab{}}
}

func failure(key string, args ...any) error { return errors.New(i18n.T("chat.browser."+key, args...)) }

// navigationURL keeps the web addresses a tab may show: http and https
// with a host, not WinOne's own pages, no credentials; and about:blank.
func navigationURL(value string) (string, error) {
	if value == "about:blank" {
		return value, nil
	}
	if len(value) > 32768 || strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return "", failure("invalidAddress")
	}
	u, err := url.Parse(value)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", failure("fullAddress")
	}
	// WebView2 serves the window's own pages and schemes at
	// http://<scheme>.localhost; a tab never shows them.
	host := strings.ToLower(u.Hostname())
	if strings.HasSuffix(host, ".localhost") || u.User != nil {
		return "", failure("refused")
	}
	return u.String(), nil
}

// tabKey identifies a tab by its conversation and the panel's tab id.
func tabKey(p browserPayload) (string, error) {
	valid := func(value string, colon bool) bool {
		if value == "" || len(value) > 128 {
			return false
		}
		for _, c := range value {
			if !(c < 128 && (unicode.IsLetter(c) || unicode.IsDigit(c)) || c == '_' || c == '-' || colon && c == ':') {
				return false
			}
		}
		return true
	}
	if !valid(p.SessionID, false) || !valid(p.TabID, true) {
		return "", failure("invalidTab")
	}
	return p.SessionID + "/" + p.TabID, nil
}

type browserPayload struct {
	SessionID   string          `json:"sessionId"`
	TabID       string          `json:"tabId"`
	URL         string          `json:"url"`
	Visible     *bool           `json:"visible"`
	Bounds      *rect           `json:"bounds"`
	Occlusion   *rect           `json:"occlusion"`
	OperationID string          `json:"operationId"`
	Save        bool            `json:"save"`
	Channel     string          `json:"channel"`
	Payload     json.RawMessage `json:"payload"`
	Muted       *bool           `json:"muted"`
}

// commentChannels are the comment script's commands the panel may send.
var commentChannels = map[string]bool{
	"browser-comment:enter-mode": true, "browser-comment:exit-mode": true, "browser-comment:cancel-pending": true,
	"browser-comment:commit-pending": true, "browser-comment:prepare-screenshot": true,
	"browser-comment:design-preview": true, "browser-comment:design-reset": true,
}

// request runs a ChatBrowser call of the panel, on the UI thread.
func (b *Browser) request(s *Surface, method string, raw json.RawMessage) (any, error) {
	b.surface = s
	var p browserPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	if method == "closeSession" {
		for key, tab := range b.tabs {
			if tab.state.SessionID == p.SessionID {
				b.drop(key)
			}
		}
		return nil, nil
	}
	key, err := tabKey(p)
	if err != nil {
		return nil, err
	}
	if method == "close" {
		b.drop(key)
		return nil, nil
	}
	if method == "create" && b.tabs[key] == nil {
		if len(b.tabs) >= 32 {
			return nil, failure("tooMany")
		}
		address, err := navigationURL(first(p.URL, "about:blank"))
		if err != nil {
			return nil, err
		}
		tab := &browserTab{key: key, state: BrowserState{SessionID: p.SessionID, TabID: p.TabID, URL: address}}
		b.tabs[key] = tab
		if err := b.materialize(tab); err != nil {
			delete(b.tabs, key)
			return nil, err
		}
	}
	tab := b.tabs[key]
	if tab == nil {
		return nil, failure("closed")
	}
	if (method == "reload" || method == "sync" && p.Visible != nil && *p.Visible) && tab.view == nil {
		if err := b.materialize(tab); err != nil {
			return nil, err
		}
	}
	switch method {
	case "capture":
		return b.capture(tab, p)
	case "comment":
		if !commentChannels[p.Channel] {
			return nil, failure("invalidComment")
		}
		if tab.view == nil {
			return nil, failure("notReady")
		}
		switch p.Channel {
		case "browser-comment:enter-mode":
			tab.commenting = true
		case "browser-comment:exit-mode":
			tab.commenting = false
		}
		channel, _ := json.Marshal(p.Channel)
		payload := p.Payload
		if len(payload) == 0 {
			payload = json.RawMessage("null")
		}
		page := tab.view.Page()
		go page.Eval(fmt.Sprintf("window.__winoneBrowserComment?.(%s,%s)", channel, payload))
		b.pump(tab)
		return nil, nil
	case "external":
		address, err := navigationURL(p.URL)
		if err != nil {
			return nil, err
		}
		if address == "about:blank" {
			return nil, failure("blankExternal")
		}
		openURL(address)
		return nil, nil
	case "sync":
		if p.Visible == nil {
			return nil, failure("noVisibility")
		}
		if *p.Visible {
			if p.Bounds == nil || !p.Bounds.valid() {
				return nil, failure("invalidBounds")
			}
			tab.bounds = *p.Bounds
			tab.occlusion = nil
			if p.Occlusion != nil && p.Occlusion.valid() {
				tab.occlusion = p.Occlusion
			}
		}
		tab.state.Visible = *p.Visible
		if *p.Visible {
			b.pump(tab)
		}
	case "create", "state":
	default:
		if tab.view == nil {
			return nil, failure("stopped")
		}
		page := tab.view.Page()
		switch method {
		case "navigate":
			address, err := navigationURL(p.URL)
			if err != nil {
				return nil, err
			}
			if err := page.LoadURL(address); err != nil {
				return nil, err
			}
			tab.state.URL, tab.state.Loading, tab.state.Error = address, true, ""
		case "back":
			page.GoBack()
		case "forward":
			page.GoForward()
		case "reload":
			page.Reload()
			tab.state.Error, tab.state.Loading = "", true
		case "stop":
			page.Stop()
			tab.state.Loading = false
		case "mute":
			page.SetAudioMuted(p.Muted == nil || *p.Muted)
		case "terminate":
			b.release(tab)
			tab.state.Crashed, tab.state.Loading, tab.state.Error = true, false, i18n.T("chat.browser.terminated")
		default:
			return nil, failure("unsupported")
		}
	}
	b.refresh(tab)
	return tab.state, nil
}

func (r rect) valid() bool {
	for _, v := range []float64{r.X, r.Y, r.Width, r.Height} {
		if math.IsNaN(v) || math.IsInf(v, 0) || v > 32768 {
			return false
		}
	}
	return r.X >= 0 && r.Y >= 0 && r.Width > 0 && r.Height > 0
}

// guestScript is the comment script with its way back to the host: a
// queue in the page that Go collects, in place of WebView2's message
// channel, which MyGo keeps for its own pages. The page's keys that the
// panel handles go the same way, as the Rust window's accelerators did.
var guestScript = `(() => {
  if (window !== window.top) return;
  const outbox = [];
  const post = text => { if (typeof text === "string" && text.length <= 131072 && outbox.length < 256) outbox.push(text); };
  Object.defineProperty(window, "__winoneBrowserTake", {value: () => outbox.splice(0)});
  Object.defineProperty(window, "__winoneBrowserPost", {value: post});
  addEventListener("keydown", event => {
    const ctrl = event.ctrlKey && !event.altKey, alt = event.altKey && !event.ctrlKey;
    const key = event.key.toLowerCase();
    const command = ctrl && key === "l" ? "focusAddress" : ctrl && key === "w" ? "close" :
      ctrl && key === "r" || !event.ctrlKey && !event.altKey && event.key === "F5" ? "reload" :
      alt && event.key === "ArrowLeft" ? "back" : alt && event.key === "ArrowRight" ? "forward" : "";
    if (!command) return;
    event.preventDefault();
    event.stopPropagation();
    post(JSON.stringify({kind: "command", value: command}));
  }, true);
})();
` + strings.Replace(commentGuest, "window.chrome?.webview?.postMessage(", "window.__winoneBrowserPost(", 1)

//go:embed browser_comment_guest.js
var commentGuest string

// materialize makes a tab's web view, again after a crash or terminate.
func (b *Browser) materialize(tab *browserTab) error {
	if b.env.WebViews == nil {
		return errors.New(i18n.T("web.unavailable"))
	}
	view, err := b.env.WebViews(mygo.WebViewOptions{
		URL:             tab.state.URL,
		BackgroundColor: "#ffffff",
		Page: mygo.PageOptions{
			PreloadScript: guestScript,
			Untrusted:     true,
			Session:       mygo.NewSession("winone-chat-browser-" + tab.state.SessionID),
		},
	})
	if err != nil {
		return err
	}
	b.order++
	tab.generation = b.order
	generation := tab.generation
	tab.view = view
	page := view.Page()
	// Page events may come from other threads; each applies on the UI
	// thread if the page is still the tab's.
	on := func(fn func()) {
		b.env.Post(func() {
			if b.tabs[tab.key] == tab && tab.generation == generation && tab.view != nil {
				fn()
				b.refresh(tab)
				b.tell(tab, "state", nil)
			}
		})
	}
	tab.offs = []func(){
		page.OnPageTitleUpdated(func(e *mygo.TitleEvent) { title := e.Title; on(func() { tab.state.Title = title }) }),
		page.OnDidNavigate(func(string) {
			on(func() { tab.state.Loading, tab.state.Error, tab.state.Crashed = true, "", false })
		}),
		page.OnDidFinishLoad(func() { on(func() { tab.state.Loading = false }) }),
		page.OnDidFailLoad(func(e *mygo.LoadError) {
			message := i18n.T("chat.browser.loadFailed", "code", e.Code)
			on(func() { tab.state.Error, tab.state.Loading = message, false })
		}),
		page.OnRenderProcessGone(func(string) {
			on(func() {
				tab.state.Crashed, tab.state.Loading, tab.state.Error = true, false, i18n.T("chat.browser.crashed")
				b.release(tab)
			})
		}),
		page.OnAudibleChanged(func(bool) { on(func() {}) }),
		page.OnWillNavigate(func(e *mygo.NavigateEvent) {
			if _, err := navigationURL(e.URL); err != nil {
				e.PreventDefault()
				message := err.Error()
				on(func() { tab.state.Error = message })
			}
		}),
	}
	// A new window opens as a tab of the panel.
	page.SetWindowOpenHandler(func(req mygo.WindowOpenRequest) *mygo.WindowOptions {
		if address, err := navigationURL(req.URL); err == nil {
			b.env.Post(func() {
				if b.tabs[tab.key] == tab {
					b.tell(tab, "newTab", address)
				}
			})
		}
		return nil
	})
	tab.state.Crashed, tab.state.Error, tab.state.Loading = false, "", true
	return nil
}

// release destroys a tab's web view, keeping the tab.
func (b *Browser) release(tab *browserTab) {
	for _, off := range tab.offs {
		off()
	}
	tab.offs = nil
	if tab.view != nil {
		tab.view.Destroy()
		tab.view = nil
	}
	tab.commenting = false
}

func (b *Browser) drop(key string) {
	if tab := b.tabs[key]; tab != nil {
		b.release(tab)
		delete(b.tabs, key)
	}
}

// Close destroys every page.
func (b *Browser) Close() {
	for key := range b.tabs {
		b.drop(key)
	}
}

// refresh reads what the page knows of itself.
func (b *Browser) refresh(tab *browserTab) {
	if tab.view == nil {
		return
	}
	page := tab.view.Page()
	if current := page.URL(); current != "" && current != "about:blank" {
		tab.state.URL = current
	}
	tab.state.CanGoBack, tab.state.CanGoForward = page.CanGoBack(), page.CanGoForward()
	tab.state.IsAudible, tab.state.Muted = page.IsAudible(), page.IsAudioMuted()
}

// tell sends the panel an event of tab: its state, or kind with value.
func (b *Browser) tell(tab *browserTab, kind string, value any) {
	if b.surface == nil {
		return
	}
	if kind == "state" {
		b.surface.Dispatch("winone:chat-browser", map[string]any{"kind": "state", "state": tab.state})
		return
	}
	b.surface.Dispatch("winone:chat-browser", map[string]any{
		"kind": kind, "sessionId": tab.state.SessionID, "tabId": tab.state.TabID, "value": value,
	})
}

// capture takes a screenshot of the page: saved for a comment, else
// copied, and reported as an operation event, as the Rust window did.
func (b *Browser) capture(tab *browserTab, p browserPayload) (any, error) {
	if p.OperationID == "" || len(p.OperationID) > 128 {
		return nil, failure("invalidOperation")
	}
	if tab.view == nil {
		return nil, failure("notReady")
	}
	path := ""
	if p.Save {
		id := make([]byte, 16)
		_, _ = rand.Read(id)
		path = filepath.Join(b.env.Directory, "chat-browser-comments", tab.state.SessionID, hex.EncodeToString(id)+".png")
	}
	view, generation, operation := tab.view, tab.generation, p.OperationID
	go func() {
		result, err := savedCapture(view, path)
		b.env.Post(func() {
			if b.tabs[tab.key] != tab || tab.generation != generation {
				return
			}
			value := map[string]any{"operationId": operation}
			if err != nil {
				value["error"] = err.Error()
			} else {
				value["result"] = result
			}
			b.tell(tab, "operation", value)
		})
	}()
	return map[string]any{"operationId": operation}, nil
}

func savedCapture(view *mygo.WebView, path string) (map[string]any, error) {
	data, err := view.CapturePage()
	if err != nil {
		return nil, err
	}
	if len(data) == 0 || len(data) > 64<<20 {
		return nil, failure("captureSize")
	}
	if path == "" {
		if err := copyPNG(data); err != nil {
			return nil, err
		}
		return map[string]any{"copied": true}, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return nil, err
	}
	return map[string]any{"screenshotPath": path, "bytes": len(data)}, nil
}

// pump collects the page's messages while the tab shows or a comment is
// under way, five times a second, and hands them to the panel.
func (b *Browser) pump(tab *browserTab) {
	if tab.pumping || tab.view == nil {
		return
	}
	tab.pumping = true
	page, generation := tab.view.Page(), tab.generation
	go func() {
		for {
			value, err := page.Eval("return window.__winoneBrowserTake ? window.__winoneBrowserTake() : []")
			var texts []string
			if list, ok := value.([]any); ok && err == nil {
				for _, item := range list {
					if text, ok := item.(string); ok {
						texts = append(texts, text)
					}
				}
			}
			stop := make(chan bool, 1)
			b.env.Post(func() {
				live := b.tabs[tab.key] == tab && tab.generation == generation && tab.view != nil
				if live {
					for _, text := range texts {
						b.receive(tab, text)
					}
				}
				if !live || !tab.state.Visible && !tab.commenting {
					tab.pumping = false
					stop <- true
					return
				}
				stop <- false
			})
			select {
			case done := <-stop:
				if done {
					return
				}
			case <-time.After(5 * time.Second):
				// The window closed under the collector.
				return
			}
			time.Sleep(200 * time.Millisecond)
		}
	}()
}

// pageCommands are the keys of a page the panel handles.
var pageCommands = map[string]bool{"focusAddress": true, "close": true, "reload": true, "back": true, "forward": true}

// receive takes a message of the page: a key command, or a comment
// script's message on its three channels; anything else is dropped.
func (b *Browser) receive(tab *browserTab, text string) {
	var m struct {
		Kind    string          `json:"kind"`
		Channel string          `json:"channel"`
		Value   string          `json:"value"`
		Payload json.RawMessage `json:"payload"`
	}
	if json.Unmarshal([]byte(text), &m) != nil {
		return
	}
	switch {
	case m.Kind == "command" && pageCommands[m.Value]:
		b.tell(tab, "command", m.Value)
	case m.Kind == "comment" && (m.Channel == "browser-comment:element-selected" || m.Channel == "browser-comment:mode-exited" || m.Channel == "browser-comment:command-result"):
		if m.Channel == "browser-comment:mode-exited" {
			tab.commenting = false
		}
		var value any
		_ = json.Unmarshal([]byte(text), &value)
		b.tell(tab, "comment", value)
	}
}

// Overlay places the visible tab's page where the panel reported its
// slot, inside the panel's web view element. A popover of the panel over
// the slot (the comment editor) is left uncovered: the page shows in the
// largest part of the slot beside it, keeping its size and place.
func (b *Browser) Overlay(c *ui.Context) {
	for _, tab := range b.tabs {
		if !tab.state.Visible || tab.view == nil {
			continue
		}
		r := tab.bounds
		clip := r
		if tab.occlusion != nil {
			clip = beside(r, *tab.occlusion)
		}
		ui.Box(c.Key(tab.key)).Absolute().Left(float32(clip.X)).Top(float32(clip.Y)).
			Width(float32(clip.Width)).Height(float32(clip.Height)).Clip().Children(func() {
			ui.WebView(c, tab.view).Absolute().Left(float32(r.X - clip.X)).Top(float32(r.Y - clip.Y)).
				Width(float32(r.Width)).Height(float32(r.Height)).Label(first(tab.state.Title, tab.state.URL))
		})
	}
}

// beside is the largest part of r that o, a box in the same coordinates
// relative to r's top-left corner, does not cover.
func beside(r, o rect) rect {
	ox, oy := r.X+o.X, r.Y+o.Y
	parts := []rect{
		{r.X, r.Y, r.Width, oy - r.Y},
		{r.X, oy + o.Height, r.Width, r.Y + r.Height - (oy + o.Height)},
		{r.X, r.Y, ox - r.X, r.Height},
		{ox + o.Width, r.Y, r.X + r.Width - (ox + o.Width), r.Height},
	}
	best := rect{r.X, r.Y, 0, 0}
	for _, p := range parts {
		if p.Width > 0 && p.Height > 0 && p.Width*p.Height > best.Width*best.Height {
			best = p
		}
	}
	return best
}
