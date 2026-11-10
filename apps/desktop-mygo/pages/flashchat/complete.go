package flashchat

import (
	"encoding/json"
	"io/fs"
	"net/url"
	"strings"
	"sync/atomic"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
)

// completePage presents MyGo controls over the existing communication engine.
// The engine surface is covered by the opaque native view and cannot receive
// pointer/keyboard input. Its document is inert and contains no accessible UI.
type completePage struct {
	nativePageState
	env                *core.Env
	app                *localApp
	view               *mygo.WebView
	engine             atomic.Pointer[mygo.Page]
	err                string
	state              string
	outbox             chan string
	services           ui.Services
	closed             bool
	offs               []func()
	preferencesReady   bool
	preferencesLoading bool
	nickname           string
}

func newCompletePage(env *core.Env) *completePage {
	p := &completePage{env: env}
	p.initNative()
	return p
}

func (p *completePage) View(c *ui.Context) {
	if p.closed {
		return
	}
	p.services = c.Services()
	p.prepare(c)
	p.presentNative(!c.WindowHidden(), c.WindowActive())
	ui.Column(c).Fill().Children(func() {
		// Chromium defers file media loading in a never-visible document,
		// even with autoplay and background timers enabled. Keep a 1-DIP
		// engine surface under the opaque native page, excluded from input.
		// The inert document exposes no controls. All visible UI remains MyGo.
		if p.view != nil {
			ui.WebView(c, p.view).Absolute().Left(0).Top(0).Size(1, 1).IgnoreInput(true).AccessHidden(true)
		}
		ui.Column(c).Fill().Background(c.Theme().Background).Children(func() { p.nativeView(c) })
	})
}

func (p *completePage) prepare(c *ui.Context) {
	if !p.preferencesReady && p.err == "" {
		if p.env.Host == nil {
			p.preferencesReady = true
		} else if !p.preferencesLoading {
			p.preferencesLoading = true
			core.Go(p.env, func() (preferences, error) {
				var prefs preferences
				err := p.env.Host.CallCompletingInto("interconnect.chat.preferences.get", nil, &prefs)
				return prefs, err
			}, func(prefs preferences, err error) {
				p.preferencesLoading = false
				if p.closed {
					return
				}
				if err != nil {
					p.err = err.Error()
					return
				}
				p.nickname, p.preferencesReady = prefs.Nickname, true
			})
		}
		if !p.preferencesReady {
			return
		}
	}
	theme := "light"
	if c.Theme().Dark {
		theme = "dark"
	}
	state, _ := json.Marshal(map[string]string{"language": i18n.Locale(), "theme": theme})
	if p.view == nil && p.err == "" {
		p.open(string(state))
	}
	if p.err != "" {
		return
	}
	if p.state != string(state) {
		select {
		case p.outbox <- "window.__winoneState=" + string(state) + ";window.dispatchEvent(new Event('winone-state'))":
			p.state = string(state)
		default:
			// Retry on the next frame; a slow web view must not block native UI.
		}
	}
}

func (p *completePage) open(state string) {
	if p.env.WebViews == nil {
		p.err = i18n.T("web.unavailable")
		return
	}
	assets, _ := fs.Sub(bundled, "assets")
	app, err := startLocalApp(p.env.Directory, assets)
	if err != nil {
		p.err = err.Error()
		return
	}
	p.app = app
	p.attachNative(app)
	nickname, _ := json.Marshal(p.nickname)
	native, _ := json.Marshal(map[string]string{"token": p.bridge.token})
	view, err := p.env.WebViews(mygo.WebViewOptions{
		URL: app.origin + "/", BackgroundColor: "light-dark(#fcfcfc, #191919)",
		// The engine is always hidden. React and peer discovery must keep
		// scheduling work; media frame encoding separately pauses when aside.
		Page: mygo.PageOptions{Untrusted: true, BackgroundThrottling: mygo.ThrottlingOff, Autoplay: mygo.AutoplayAllow, Session: mygo.NewSession("winone-flashchat-" + p.env.Directory),
			PreloadScript: "if(window===window.top){window.__winoneState=" + state + ";window.__winoneLegacyNickname=" + string(nickname) + ";window.__winoneNative=" + string(native) + ";document.addEventListener('DOMContentLoaded',()=>{document.body.inert=true;document.body.setAttribute('aria-hidden','true');document.title=''},{once:true})}"},
	})
	if err != nil {
		app.close()
		p.app = nil
		p.err = err.Error()
		return
	}
	p.view, p.state = view, state
	p.engine.Store(view.Page())
	p.outbox = make(chan string, 8)
	go func() {
		for script := range p.outbox {
			if _, err := view.Page().Eval(script); err != nil {
				p.env.Post(func() {
					if !p.closed {
						p.err = err.Error()
					}
				})
			}
		}
	}()
	page := view.Page()
	page.SetPermissionHandler(func(req mygo.PermissionRequest) bool { return appPermission(app.origin, req) })
	p.offs = append(p.offs,
		page.OnRenderProcessGone(func(reason string) {
			p.env.Post(func() {
				if !p.closed {
					p.err = reason
					p.snapshot = nativeSnapshot{}
					p.presentation = ""
				}
			})
		}),
		page.OnDidFinishLoad(func() {
			p.env.Post(func() {
				if !p.closed {
					p.state = ""
					p.presentation = ""
				}
			})
		}),
		page.OnWillNavigate(func(event *mygo.NavigateEvent) {
			if event.URL == "winone-flashchat:appearance" {
				event.PreventDefault()
				p.env.Post(func() { p.env.Navigate("settings", nil) })
				return
			}
			if localURL(event.URL, app.origin) || strings.HasPrefix(event.URL, "blob:"+app.origin+"/") {
				return
			}
			event.PreventDefault()
			if event.UserInitiated && ui.SafeToOpen(event.URL) {
				p.env.Post(func() { p.services.OpenURL(event.URL) })
			}
		}),
		page.OnWillDownload(func(event *mygo.DownloadEvent) {}),
		page.OnDownloadDone(func(download *mygo.Download) {
			p.env.Post(func() {
				if p.closed || p.env.Toast == nil {
					return
				}
				if download.Err != nil {
					p.env.Toast(i18n.T("web.downloadFailed", "error", download.Err.Error()))
				} else {
					p.env.Toast(i18n.T("web.downloaded", "path", download.Path))
				}
			})
		}),
	)
	page.SetWindowOpenHandler(func(req mygo.WindowOpenRequest) *mygo.WindowOptions {
		if req.UserInitiated && ui.SafeToOpen(req.URL) {
			p.env.Post(func() { p.services.OpenURL(req.URL) })
		}
		return nil
	})
}

func localURL(raw, origin string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.User == nil && u.Scheme+"://"+u.Host == origin
}
func appPermission(origin string, req mygo.PermissionRequest) bool {
	if req.Origin != origin || len(req.Permissions) == 0 {
		return false
	}
	for _, kind := range req.Permissions {
		switch kind {
		case mygo.PermissionCamera, mygo.PermissionMicrophone, mygo.PermissionNotifications:
		default:
			return false
		}
	}
	return true
}

// Hidden web views keep their page and WebRTC session; only Close tears down.
func (p *completePage) SetAside() {
	p.presentNative(false, false)
	if p.pushTalking {
		p.pushTalking = false
		p.backgroundCommand("media", "microphone", map[string]any{"enabled": false})
	}
}
func (p *completePage) Close() {
	if p.closed {
		return
	}
	p.closed = true
	p.engine.Store(nil)
	for _, off := range p.offs {
		off()
	}
	if p.view != nil {
		p.view.Destroy()
		p.view = nil
	}
	if p.outbox != nil {
		close(p.outbox)
	}
	p.app.close()
}
