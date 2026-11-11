package webui

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

type NativeBrowserOptions struct {
	SessionID, TabID, URL string
	OnTitle               func(title, url string)
	OnClose               func()
	OnNewTab              func(string)
	MarkerNumbers         func() []float64
	OnComment             func(json.RawMessage, func(error))
}
type browserWaiter struct {
	timer *time.Timer
	done  func(map[string]any, error)
}
type NativeBrowserPane struct {
	env                                            *core.Env
	options                                        NativeBrowserOptions
	browser                                        *Browser
	state                                          BrowserState
	address                                        ui.Handle
	addressText                                    string
	visible, closed, ready, commenting, submitting bool
	comment                                        string
	target                                         json.RawMessage
	targetData                                     browserTarget
	styles                                         map[string]string
	editedText                                     string
	error, notice                                  string
	waiters                                        map[string]*browserWaiter
}
type browserTarget struct {
	Kind           string                 `json:"kind"`
	Point          struct{ X, Y float64 } `json:"point"`
	MarkerNumber   float64                `json:"markerNumber"`
	Immediate      bool                   `json:"immediate"`
	DesignBaseline *struct {
		Styles       map[string]string `json:"styles"`
		EditableText *string           `json:"editableText"`
		Provenance   map[string]string `json:"provenance"`
	} `json:"designBaseline"`
}

func NewNativeBrowserPane(env *core.Env, options NativeBrowserOptions) *NativeBrowserPane {
	p := &NativeBrowserPane{env: env, options: options, browser: NewBrowser(env), visible: true, styles: map[string]string{}, waiters: map[string]*browserWaiter{}}
	p.browser.NativeReceive = p.receive
	value, err := p.call("create", map[string]any{"url": first(options.URL, "about:blank")})
	if err != nil {
		p.error = err.Error()
	} else {
		p.accept(value)
		p.ready = true
	}
	return p
}
func (p *NativeBrowserPane) call(method string, values map[string]any) (any, error) {
	if values == nil {
		values = map[string]any{}
	}
	values["sessionId"], values["tabId"] = p.options.SessionID, p.options.TabID
	return p.browser.NativeRequest(method, values)
}
func (p *NativeBrowserPane) accept(value any) {
	if state, ok := value.(BrowserState); ok {
		p.state = state
		p.addressText = state.URL
		if p.options.OnTitle != nil {
			p.options.OnTitle(state.Title, state.URL)
		}
	}
}
func (p *NativeBrowserPane) run(method string, values map[string]any) {
	p.error = ""
	value, err := p.call(method, values)
	if err != nil {
		p.error = err.Error()
		return
	}
	p.accept(value)
}
func (p *NativeBrowserPane) SetVisible(visible bool) {
	if p.closed || p.visible == visible {
		return
	}
	p.visible = visible
	p.browser.SetNativeVisible(p.options.SessionID, p.options.TabID, visible)
}
func (p *NativeBrowserPane) Busy() bool { return p.submitting }
func (p *NativeBrowserPane) Close() {
	if p.closed {
		return
	}
	p.closed = true
	for id, waiter := range p.waiters {
		waiter.timer.Stop()
		delete(p.waiters, id)
		waiter.done(nil, fmt.Errorf("%s", i18n.T("chat.browser.closed")))
	}
	p.browser.Close()
}
func (p *NativeBrowserPane) nextMarker() float64 {
	marker := float64(0)
	if p.options.MarkerNumbers != nil {
		for _, number := range p.options.MarkerNumbers() {
			marker = math.Max(marker, number)
		}
	}
	return marker + 1
}
func (p *NativeBrowserPane) command(name string, payload any, done func(map[string]any, error)) {
	p.request("comment", map[string]any{"channel": "browser-comment:" + name, "payload": payload}, done)
}
func (p *NativeBrowserPane) request(method string, values map[string]any, done func(map[string]any, error)) {
	id := rand.Text()
	if method == "capture" {
		values["operationId"] = id
	} else {
		values["payload"] = map[string]any{"requestId": id, "payload": values["payload"]}
	}
	timer := time.AfterFunc(10*time.Second, func() {
		p.env.Post(func() {
			if waiter := p.waiters[id]; waiter != nil {
				delete(p.waiters, id)
				waiter.done(nil, fmt.Errorf("%s", i18n.T("chat.browserTimeout")))
			}
		})
	})
	p.waiters[id] = &browserWaiter{timer: timer, done: done}
	if _, err := p.call(method, values); err != nil {
		timer.Stop()
		delete(p.waiters, id)
		done(nil, err)
	}
}
func (p *NativeBrowserPane) receive(kind string, state BrowserState, value any) {
	if p.closed {
		return
	}
	switch kind {
	case "state":
		p.accept(state)
	case "newTab":
		if address, ok := value.(string); ok && p.options.OnNewTab != nil {
			p.options.OnNewTab(address)
		}
	case "command":
		command, _ := value.(string)
		switch command {
		case "focusAddress":
			p.address.Focus()
		case "close":
			if p.options.OnClose != nil {
				p.options.OnClose()
			}
		default:
			p.run(command, nil)
		}
	case "operation", "comment":
		message, ok := value.(map[string]any)
		if !ok {
			return
		}
		channel, _ := message["channel"].(string)
		if kind == "operation" || channel == "browser-comment:command-result" {
			payload := message
			if kind == "comment" {
				payload, _ = message["payload"].(map[string]any)
			}
			if payload == nil {
				return
			}
			id, _ := payload["operationId"].(string)
			if id == "" {
				id, _ = payload["requestId"].(string)
			}
			waiter := p.waiters[id]
			if waiter == nil {
				return
			}
			waiter.timer.Stop()
			delete(p.waiters, id)
			if failed, _ := payload["error"].(string); failed != "" || payload["ok"] == false {
				waiter.done(nil, fmt.Errorf("%s", i18n.T("chat.browserCommentFailed")))
				return
			}
			result, _ := payload["result"].(map[string]any)
			if result == nil {
				result = payload
			}
			waiter.done(result, nil)
			return
		}
		if channel == "browser-comment:mode-exited" {
			p.commenting = false
			p.target = nil
			return
		}
		if channel == "browser-comment:element-selected" && p.commenting {
			raw, err := json.Marshal(message["payload"])
			if err != nil {
				return
			}
			var target browserTarget
			if json.Unmarshal(raw, &target) != nil || !validBrowserTarget(target) {
				return
			}
			p.target, p.targetData = raw, target
			p.comment = ""
			p.styles = map[string]string{}
			if target.DesignBaseline != nil {
				for name, value := range target.DesignBaseline.Styles {
					p.styles[name] = value
				}
				if target.DesignBaseline.EditableText != nil {
					p.editedText = *target.DesignBaseline.EditableText
				}
			}
			if target.Immediate {
				p.submit()
			}
		}
	}
}
func validBrowserTarget(target browserTarget) bool {
	if target.Kind != "element" && target.Kind != "region" && target.Kind != "text" {
		return false
	}
	if target.MarkerNumber < 1 || target.MarkerNumber > 9007199254740991 || math.Trunc(target.MarkerNumber) != target.MarkerNumber {
		return false
	}
	for _, n := range []float64{target.Point.X, target.Point.Y} {
		if math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || n > 32768 {
			return false
		}
	}
	return true
}
func (p *NativeBrowserPane) toggleComment() {
	p.commenting = !p.commenting
	entering := p.commenting
	command := "exit-mode"
	var payload any
	if entering {
		command = "enter-mode"
		payload = map[string]any{"markerNumber": p.nextMarker()}
	}
	p.command(command, payload, func(_ map[string]any, err error) {
		if err != nil {
			p.error = err.Error()
			p.commenting = !entering
		} else if !entering {
			p.target = nil
		}
	})
}
func (p *NativeBrowserPane) submit() {
	if p.submitting || len(p.target) == 0 || p.options.OnComment == nil {
		return
	}
	p.submitting = true
	p.error = ""
	target := append(json.RawMessage(nil), p.target...)
	marker := p.targetData.MarkerNumber
	comment := p.comment
	pageURL := p.state.URL
	styleChanges := []map[string]string{}
	if baseline := p.targetData.DesignBaseline; baseline != nil {
		for _, name := range []string{"color", "background-color", "font-size", "font-weight", "padding", "border-radius"} {
			if value := p.styles[name]; value != baseline.Styles[name] {
				styleChanges = append(styleChanges, map[string]string{"property": name, "previousValue": baseline.Styles[name], "value": value})
			}
		}
		if baseline.EditableText != nil && *baseline.EditableText != p.editedText {
			styleChanges = append(styleChanges, map[string]string{"property": "text content", "previousValue": *baseline.EditableText, "value": p.editedText})
		}
	}
	finish := func(err error) {
		p.submitting = false
		if err != nil {
			p.error = err.Error()
		}
	}
	markers := []float64{}
	if p.options.MarkerNumbers != nil {
		markers = p.options.MarkerNumbers()
	}
	p.command("prepare-screenshot", map[string]any{"validMarkerNumbers": markers}, func(_ map[string]any, err error) {
		if err != nil {
			finish(err)
			return
		}
		p.request("capture", map[string]any{"save": true}, func(result map[string]any, err error) {
			if err != nil {
				finish(err)
				return
			}
			path, _ := result["screenshotPath"].(string)
			if path == "" {
				finish(fmt.Errorf("%s", i18n.T("chat.browserCommentFailed")))
				return
			}
			annotation, err := json.Marshal(map[string]any{"id": rand.Text(), "markerNumber": marker, "pageUrl": pageURL, "target": target, "comment": comment, "screenshotPath": path, "styleChanges": styleChanges})
			if err != nil {
				finish(err)
				return
			}
			p.options.OnComment(annotation, func(err error) {
				if p.closed {
					return
				}
				if err != nil {
					finish(err)
					return
				}
				p.command("commit-pending", map[string]any{"nextMarkerNumber": math.Max(p.nextMarker(), marker+1)}, func(_ map[string]any, err error) {
					finish(err)
					if err == nil {
						p.target = nil
						p.comment = ""
						p.notice = i18n.T("chat.browserCommentAdded")
					}
				})
			})
		})
	})
}
func (p *NativeBrowserPane) previewDesign(reset bool) {
	name := "design-preview"
	payload := map[string]any{}
	if reset {
		name = "design-reset"
	} else {
		styles := map[string]string{}
		baseline := p.targetData.DesignBaseline
		if baseline == nil {
			return
		}
		for _, name := range []string{"color", "background-color", "font-size", "font-weight", "padding", "border-radius"} {
			if p.styles[name] != baseline.Styles[name] {
				styles[name] = p.styles[name]
			}
		}
		var text any
		if baseline.EditableText != nil && *baseline.EditableText != p.editedText {
			text = p.editedText
		}
		payload = map[string]any{"styles": styles, "text": text}
	}
	p.run("comment", map[string]any{"channel": "browser-comment:" + name, "payload": payload})
}
func (p *NativeBrowserPane) View(c *ui.Context) {
	if p.closed {
		return
	}
	var address ui.Element
	root := ui.Column(c).Fill().MinWidth(0).Gap(4)
	root.Children(func() {
		ui.Row(c).FillWidth().Gap(4).Padding(6).Children(func() {
			if kit.IconButton(c, icons.ArrowLeft, i18n.T("chat.browser.back")).Disabled(!p.state.CanGoBack).Clicked() {
				p.run("back", nil)
			}
			if kit.IconButton(c, icons.ArrowRight, i18n.T("chat.browser.forward")).Disabled(!p.state.CanGoForward).Clicked() {
				p.run("forward", nil)
			}
			if kit.IconButton(c, icons.RefreshCw, i18n.T("chat.refresh")).Clicked() {
				p.run("reload", nil)
			}
			address = kit.TextInput(c, &p.addressText).Grow(1).MinWidth(0).Bind(&p.address).Label(i18n.T("chat.browser.address"))
			if kit.IconButton(c, icons.ArrowUpRight, i18n.T("chat.browser.go")).Clicked() {
				p.run("navigate", map[string]any{"url": p.addressText})
			}
			more := kit.IconButton(c, icons.Ellipsis, i18n.T("chat.more"))
			more.Menu(func(m *ui.Menu) {
				if p.options.OnComment != nil && m.Item(i18n.T("chat.browserComment")).Chosen() {
					p.toggleComment()
				}
				if m.Item(i18n.T("chat.copyLink")).Chosen() {
					c.Services().WriteClipboard(p.state.URL)
				}
				if m.Item(i18n.T("chat.openSystem")).Chosen() {
					p.run("external", map[string]any{"url": p.state.URL})
				}
				if m.Item(i18n.T("chat.browserScreenshot")).Chosen() {
					p.request("capture", map[string]any{"save": false}, func(_ map[string]any, err error) {
						if err != nil {
							p.error = err.Error()
						} else {
							p.notice = i18n.T("chat.browserScreenshotCopied")
						}
					})
				}
				if m.Item(i18n.T("chat.browserMute")).Chosen() {
					p.run("mute", map[string]any{"muted": !p.state.Muted})
				}
				if m.Item(i18n.T("chat.browserRelease")).Chosen() {
					p.run("terminate", nil)
				}
			})
		})
		kit.InlineError(c, first(p.error, p.state.Error))
		if p.notice != "" {
			kit.Help(c, p.notice)
		}
		// MyGo's native compositor paints the dialog/backdrop over this same
		// WebView and excludes their hit regions from browser input. Keep the
		// page placed: hiding it suspends the guest's animation-frame barrier
		// and makes prepare-screenshot time out while the modal is open.
		p.browser.NativeView(c, p.options.SessionID, p.options.TabID, p.visible).Grow(1).MinHeight(0)
	})
	if root.Shortcut(ui.Ctrl, ui.KeyL) {
		p.address.Focus()
	}
	if address.Shortcut(0, ui.KeyEnter) {
		p.run("navigate", map[string]any{"url": p.addressText})
	}
	if len(p.target) > 0 {
		open := true
		if kit.Dialog(c, &open, kit.DialogOptions{Title: i18n.T("chat.browserComment"), Confirm: i18n.T("chat.browserCommentAdd"), Cancel: i18n.T("chat.cancel"), Busy: p.submitting, Error: p.error, Width: 620, Body: func() {
			ui.TextArea(c, &p.comment).FillWidth().Height(120).Label(i18n.T("chat.browserComment"))
			if baseline := p.targetData.DesignBaseline; baseline != nil {
				for _, name := range []string{"color", "background-color", "font-size", "font-weight", "padding", "border-radius"} {
					value := p.styles[name]
					ui.Field(c, name, func() {
						if kit.TextInput(c, &value).FillWidth().Changed() {
							p.styles[name] = value
							p.previewDesign(false)
						}
					})
				}
				if baseline.EditableText != nil {
					ui.Field(c, "text content", func() {
						if ui.TextArea(c, &p.editedText).FillWidth().Height(80).Changed() {
							p.previewDesign(false)
						}
					})
				}
				if kit.Button(c, i18n.T("chat.reset"), icons.RotateCcw, false).Clicked() {
					for name, value := range baseline.Styles {
						p.styles[name] = value
					}
					if baseline.EditableText != nil {
						p.editedText = *baseline.EditableText
					}
					p.previewDesign(true)
				}
			}
		}}) {
			p.submit()
		}
		if !open {
			p.command("cancel-pending", nil, func(_ map[string]any, err error) {
				if err != nil {
					p.error = err.Error()
				} else {
					p.target = nil
				}
			})
		}
	}
}
