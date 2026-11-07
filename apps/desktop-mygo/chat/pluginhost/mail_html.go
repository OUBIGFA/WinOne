package pluginhost

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/chat/workspace"
	"winone/desktopmygo/core"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

// The original sanitizer/serializer are supplied by the checked mail-ui bundle.
// Go only owns the native context and binding. No mail-authored script, HTML
// editing attribute, Host bearer, filesystem path or Go bridge is exposed.
type mailHTMLDocument struct {
	h                                                            *Host
	item                                                         *card
	field, id                                                    string
	generation                                                   uint64
	ctx                                                          context.Context
	cancel                                                       context.CancelFunc
	view                                                         *mygo.WebView
	offReady, offNavigate                                        func()
	loopCancel                                                   context.CancelFunc
	closed                                                       atomic.Bool
	mu                                                           sync.Mutex
	resources                                                    map[string]string
	source, emitted, signature, script, html, theme, lastRefresh string
	images                                                       []mailInlineImage
	sequence                                                     uint64
	version                                                      uint64
	height                                                       float32
	editable, reading, allowExternal, externalImages             bool
	visible, loading, ready, refreshing                          bool
	err                                                          string
}
type mailDocumentResult struct {
	HTML              string `json:"html"`
	Script            string `json:"script"`
	HasExternalImages bool   `json:"hasExternalImages"`
}
type mailDocumentEvent struct {
	Kind     string  `json:"kind"`
	HTML     string  `json:"html"`
	Text     string  `json:"text"`
	URL      string  `json:"url"`
	Sequence uint64  `json:"sequence"`
	Height   float32 `json:"height"`
}

var mailDocuments = struct {
	sync.Mutex
	byID map[string]*mailHTMLDocument
}{byID: map[string]*mailHTMLDocument{}}
var mailProtocolError = mygo.Protocol.HandleFunc("winone-mail", serveMailResource)
var mailResourceHTTP = &http.Client{Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

func nativeMailOrigin(id string) string {
	if runtime.GOOS == "windows" {
		return "http://" + id + ".winone-mail.localhost"
	}
	return "winone-mail://" + id
}

// Resources are an immutable projection of this document's Host-validated CID
// list. CallerPage is the OS-provided MyGo page, not an Origin supplied by mail.
// Streaming through the existing media-preview authority avoids a second blob
// store and avoids copying large preview images into draft/JSON/base64 memory.
func serveMailResource(w http.ResponseWriter, r *http.Request) {
	mailDocuments.Lock()
	document := mailDocuments.byID[r.URL.Host]
	mailDocuments.Unlock()
	if document == nil || document.closed.Load() || document.view == nil || mygo.CallerPage(r.Context()) != document.view.Page() || r.Method != "GET" {
		http.NotFound(w, r)
		return
	}
	if r.URL.Path == "/document" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_, _ = io.WriteString(w, document.documentHTML())
		return
	}
	document.mu.Lock()
	source := document.resources[r.URL.Path]
	document.mu.Unlock()
	if source == "" {
		http.NotFound(w, r)
		return
	}
	var preview workspace.FilePreview
	if err := document.h.env.Host.CallInto("chat.ghosts.media-preview", map[string]string{"url": source}, &preview); err != nil {
		http.Error(w, "MAIL_IMAGE_UNAVAILABLE", http.StatusNotFound)
		return
	}
	if !strings.HasPrefix(preview.MIME, "image/") {
		http.Error(w, "MAIL_IMAGE_UNAVAILABLE", http.StatusNotFound)
		return
	}
	request, err := http.NewRequestWithContext(r.Context(), "GET", preview.URL, nil)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	response, err := mailResourceHTTP.Do(request)
	if err != nil {
		http.Error(w, "MAIL_IMAGE_UNAVAILABLE", http.StatusBadGateway)
		return
	}
	defer response.Body.Close()
	w.Header().Set("Content-Type", preview.MIME)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(response.StatusCode)
	_, _ = io.Copy(w, response.Body)
}

func (h *Host) mailHTML(c *ui.Context, item *card, field cardFormField, expanded, editable bool) {
	location := "inline"
	if expanded {
		location = "expanded"
	}
	key := item.CallID + ":" + field.Name + ":" + location
	s := h.cards.mail
	document := s.documents[key]
	if document != nil && document.generation != item.formGeneration {
		document.Close()
		delete(s.documents, key)
		document = nil
	}
	if document == nil {
		ctx, cancel := context.WithCancel(context.Background())
		document = &mailHTMLDocument{h: h, item: item, field: field.Name, id: strings.ToLower(rand.Text()), generation: item.formGeneration, ctx: ctx, cancel: cancel, height: 120, resources: map[string]string{}}
		s.documents[key] = document
	}
	images := []mailInlineImage{}
	if item.Mail != nil {
		images = item.Mail.Images
	}
	if model := s.models[item.CallID]; model != nil && !model.loading && model.err == "" {
		images = model.Images
	}
	document.visible = true
	document.editable = editable && !item.busy
	reading := expanded && !editable
	theme := mailTheme(c)
	document.update(item.values[field.Name], images, reading, theme)
	ui.Column(c.Key(key)).FillWidth().MinWidth(0).Gap(4).Children(func() {
		if !expanded && item.Mail != nil && item.SessionID != "" {
			ui.Row(c).FillWidth().Justify(ui.End).Children(func() {
				if kit.IconButton(c, icons.Maximize, mailText("enlarge")).Clicked() {
					h.enlargeMail(item)
				}
			})
		}
		if reading && document.externalImages && !document.allowExternal && kit.Button(c, mailText("externalImages"), nil, false).Clicked() {
			document.allowExternal = true
			document.signature = ""
			document.update(document.source, document.images, reading, theme)
		}
		if document.err != "" {
			kit.InlineError(c, document.err)
			if kit.Button(c, text("retry"), nil, false).Clicked() {
				document.err = ""
				document.signature = ""
				document.update(document.source, document.images, reading, theme)
			}
		}
		if document.view == nil {
			if document.loading {
				kit.Loader(c)
			}
			return
		}
		height := max(float32(80), document.height)
		if expanded {
			height = min(640, height)
		} else {
			height = min(440, height)
		}
		ui.WebView(c, document.view).FillWidth().Height(height).MinWidth(0).Label(mailText("preview"))
	})
}
func mailTheme(c *ui.Context) string {
	t := c.Theme()
	color := func(value ui.Color) string {
		return fmt.Sprintf("rgba(%d,%d,%d,%.3f)", value.R, value.G, value.B, float64(value.A)/255)
	}
	value, _ := json.Marshal(map[string]any{"dark": t.Dark, "surface": color(t.Surface), "text": color(t.Text), "border": color(t.Border)})
	return string(value)
}
func (p *mailHTMLDocument) resourceImages(images []mailInlineImage) []mailInlineImage {
	p.mu.Lock()
	defer p.mu.Unlock()
	clear(p.resources)
	result := make([]mailInlineImage, 0, len(images))
	for i, image := range images {
		resource := "/image/" + strconv.Itoa(i)
		p.resources[resource] = image.URL
		result = append(result, mailInlineImage{CID: image.CID, URL: nativeMailOrigin(p.id) + resource})
	}
	return result
}
func (p *mailHTMLDocument) update(source string, images []mailInlineImage, reading bool, theme string) {
	if p.closed.Load() {
		return
	}
	modeChanged := p.reading != reading
	p.reading = reading
	p.images = images
	p.theme = theme
	p.source = source
	signature, _ := json.Marshal([]any{source, images, reading, p.allowExternal})
	if p.signature == string(signature) {
		if p.ready {
			p.refresh(false)
		}
		return
	}
	p.signature = string(signature)
	p.version++
	version := p.version
	if p.h.env.WebViews == nil || mailProtocolError != nil {
		p.err = mailText("editorUnavailable")
		return
	}
	p.loading = true
	p.h.call("mail-ui", map[string]any{"op": "document", "source": source, "images": images, "reading": reading, "allowExternalImages": p.allowExternal}, func(raw json.RawMessage, err error) {
		if p.closed.Load() || p.h.closed || version != p.version {
			return
		}
		p.loading = false
		if err != nil {
			p.err = err.Error()
			return
		}
		var result mailDocumentResult
		if json.Unmarshal(raw, &result) != nil || result.HTML == "" || result.Script == "" {
			p.err = mailText("editorUnavailable")
			return
		}
		p.err = ""
		p.script = result.Script
		p.mu.Lock()
		p.html = result.HTML
		p.mu.Unlock()
		p.externalImages = result.HasExternalImages
		if p.view == nil {
			view, err := p.h.env.WebViews(mygo.WebViewOptions{URL: "winone-mail://" + p.id + "/document", ScrollToContainer: false, Transparent: true, Page: mygo.PageOptions{Untrusted: true, Session: mygo.NewSession("winone-mail-" + p.id)}})
			if err != nil {
				p.err = err.Error()
				return
			}
			p.view = view
			mailDocuments.Lock()
			mailDocuments.byID[p.id] = p
			mailDocuments.Unlock()
			p.offNavigate = view.Page().OnWillNavigate(func(event *mygo.NavigateEvent) {
				if strings.Split(event.URL, "#")[0] != "winone-mail://"+p.id+"/document" {
					event.PreventDefault()
				}
			})
			p.offReady = view.Page().OnDOMReady(p.initialize)
		} else if modeChanged || p.allowExternal && p.reading {
			// A fresh browsing context is needed to relax external-image CSP.
			if p.loopCancel != nil {
				p.loopCancel()
			}
			p.ready = false
			p.view.Page().Reload()
		} else if p.ready {
			p.refresh(source != p.emitted)
		}
	})
}
func (p *mailHTMLDocument) documentHTML() string {
	// Only our per-document native origin is added to the original img policy.
	// The original script/form/connect/default restrictions remain unchanged.
	p.mu.Lock()
	source := p.html
	p.mu.Unlock()
	html := strings.Replace(source, "img-src data: cindy-media:", "img-src data: cindy-media: "+nativeMailOrigin(p.id), 1)
	prefix := `<meta name="winone-mail-document" content="` + p.id + `"><style id="__winone_mail_pending">body{visibility:hidden}</style>`
	// Keep the upstream doctype first: prepending native markers would put
	// Chromium in quirks mode and make body.scrollHeight track the viewport.
	if strings.HasPrefix(strings.ToLower(html), "<!doctype html>") {
		return html[:15] + prefix + html[15:]
	}
	return "<!doctype html>" + prefix + html
}
func (p *mailHTMLDocument) initialize() {
	if p.closed.Load() || p.view == nil {
		return
	}
	body := p.script + "\n" + mailNativeBinding
	config, _ := json.Marshal(map[string]any{"id": p.id, "source": p.source, "images": p.images, "displayImages": p.resourceImages(p.images), "editable": p.editable, "reading": p.reading, "external": p.allowExternal, "theme": json.RawMessage(p.theme)})
	body += "\nreturn globalThis.__winoneMailBind(" + string(config) + ");"
	core.Go(p.h.env, func() (any, error) { return p.view.Page().EvalContext(p.ctx, body) }, func(value any, err error) {
		if p.closed.Load() || p.h.closed {
			return
		}
		if err != nil {
			p.err = err.Error()
			return
		}
		if value != true {
			return
		} // Initial about:blank is not the mail document.
		p.ready = true
		p.refresh(false)
		ctx, cancel := context.WithCancel(p.ctx)
		if p.loopCancel != nil {
			p.loopCancel()
		}
		p.loopCancel = cancel
		go p.events(ctx)
	})
}
func (p *mailHTMLDocument) refresh(replace bool) {
	if p.closed.Load() || !p.ready || p.refreshing {
		return
	}
	config, _ := json.Marshal(map[string]any{"source": p.source, "images": p.images, "displayImages": p.resourceImages(p.images), "editable": p.editable, "reading": p.reading, "external": p.allowExternal, "theme": json.RawMessage(p.theme), "html": p.documentHTML(), "replace": replace})
	if string(config) == p.lastRefresh {
		return
	}
	p.lastRefresh = string(config)
	p.refreshing = true
	core.Go(p.h.env, func() (any, error) {
		return p.view.Page().EvalContext(p.ctx, "return globalThis.__winoneMailApply("+string(config)+");")
	}, func(_ any, err error) {
		p.refreshing = false
		if !p.closed.Load() && err != nil {
			p.err = err.Error()
		}
	})
}
func (p *mailHTMLDocument) events(ctx context.Context) {
	for ctx.Err() == nil {
		value, err := p.view.Page().EvalContext(ctx, "return await globalThis.__winoneMailNext();")
		if err != nil {
			return
		}
		raw, err := json.Marshal(value)
		if err != nil {
			return
		}
		var event mailDocumentEvent
		if json.Unmarshal(raw, &event) != nil {
			return
		}
		p.h.env.Post(func() {
			if ctx.Err() == nil && !p.closed.Load() && !p.h.closed {
				p.receive(event)
			}
		})
	}
}
func (p *mailHTMLDocument) receive(event mailDocumentEvent) {
	switch event.Kind {
	case "edit":
		if event.Sequence <= p.sequence || p.generation != p.item.formGeneration || !mailCanEdit(p.item) {
			return
		}
		if cardValueLength(event.HTML) > 64000 || cardValueLength(event.Text) > 16000 || strings.ContainsRune(event.Text, 0) {
			p.item.err = text("formTooLong")
			return
		}
		p.sequence = event.Sequence
		p.emitted = event.HTML
		plain := "body"
		if p.field == "quoteHtml" {
			plain = "quote"
		}
		p.item.values[p.field] = event.HTML
		if _, ok := p.item.values[plain]; ok {
			p.item.values[plain] = event.Text
		}
		p.item.formDirty = true
		if !p.item.formSending && p.item.err == "" {
			p.h.flushCardForm(p.item)
		}
	case "height":
		if event.Height > 0 {
			p.height = event.Height
		}
	case "invalid":
		p.item.err = text("formTooLong")
	case "image":
		if !p.editable && strings.HasPrefix(event.URL, "cindy-media://") {
			p.h.openMailImage(event.URL)
		}
	case "link":
		if p.reading && !p.editable {
			p.h.openMailDocumentLink(p.item.SessionID, event.URL)
		}
	}
}
func (p *mailHTMLDocument) flush(done func(bool)) {
	if p.closed.Load() || !p.ready {
		done(!p.loading && p.err == "")
		return
	}
	core.Go(p.h.env, func() (any, error) {
		return p.view.Page().EvalContext(p.ctx, "return await globalThis.__winoneMailSnapshot();")
	}, func(value any, err error) {
		if p.closed.Load() || p.h.closed {
			done(false)
			return
		}
		if err != nil {
			p.err = err.Error()
			done(false)
			return
		}
		if value != nil {
			raw, _ := json.Marshal(value)
			var event mailDocumentEvent
			if json.Unmarshal(raw, &event) != nil {
				done(false)
				return
			}
			p.receive(event)
		}
		done(p.item.err == "")
	})
}
func (h *Host) captureMailEditors(item *card, done func(bool)) {
	var documents []*mailHTMLDocument
	for _, document := range h.cards.mail.documents {
		if document.item == item && document.generation == item.formGeneration && !document.closed.Load() {
			documents = append(documents, document)
		}
	}
	if len(documents) == 0 {
		done(true)
		return
	}
	remaining, ok := len(documents), true
	for _, document := range documents {
		document.flush(func(success bool) {
			ok = ok && success
			remaining--
			if remaining == 0 {
				done(ok)
			}
		})
	}
}
func (p *mailHTMLDocument) Close() {
	if !p.closed.CompareAndSwap(false, true) {
		return
	}
	p.cancel()
	if p.loopCancel != nil {
		p.loopCancel()
	}
	if p.offReady != nil {
		p.offReady()
	}
	if p.offNavigate != nil {
		p.offNavigate()
	}
	mailDocuments.Lock()
	delete(mailDocuments.byID, p.id)
	mailDocuments.Unlock()
	if p.view != nil {
		p.view.Destroy()
	}
}

// This is lifecycle glue over the original mailBodySerializer and preview paint.
// Input/IME/paste/Enter/undo stay in the real browser editing engine. The single
// native Eval promise transports sanitized edits, never DOM-supplied actions.
const mailNativeBinding = `
globalThis.__winoneMailBind=function(config){
 if(document.querySelector('meta[name="winone-mail-document"]')?.content!==config.id)return false;
 const api=globalThis.WinOneMail;
 if(!api?.mailBodySerializer||!api.followMailPreviewTheme)throw Error('Original mail editor helpers unavailable');
 if(globalThis.__winoneMailState)return true;
 const state=globalThis.__winoneMailState={...config,sequence:0,emitted:null,queue:[],waiter:null,composing:false,stopTheme:null,serializer:null};
 const push=event=>{if(state.waiter){const resolve=state.waiter;state.waiter=null;resolve(event)}else state.queue.push(event)};
 const size=()=>{const body=document.body;if(body)push({kind:'height',height:Math.ceil(body.getBoundingClientRect().height)+12})};
 const serialize=()=>{
  const next=state.serializer();if(!next){push({kind:'invalid'});return null}
  if(next.html===state.emitted)return null;
  state.emitted=next.html;state.sequence++;
  state.lastEdit={kind:'edit',html:next.html,text:next.text,sequence:state.sequence};
  return state.lastEdit;
 };
 const paint=()=>{
  state.stopTheme?.();state.stopTheme=null;
  const root=document.documentElement;root.classList.toggle('dark',Boolean(state.theme.dark));
  for(const [name,key] of [['--surface','surface'],['--text-primary','text'],['--border-default','border']])root.style.setProperty(name,state.theme[key]);
  state.stopTheme=api.followMailPreviewTheme(document,document,state.reading&&!state.editable);
  let adaptation=document.getElementById('__winone_mail_layout');if(!adaptation){adaptation=document.createElement('style');adaptation.id='__winone_mail_layout';document.head.append(adaptation)}
  adaptation.textContent='html{width:100%!important;max-width:100%!important}body{width:auto!important;max-width:100%!important;box-sizing:border-box;overflow-wrap:anywhere}table{max-width:100%!important;table-layout:fixed}td,th{overflow-wrap:anywhere;word-break:break-word}pre{white-space:pre-wrap}img{max-width:100%!important;height:auto}';
  document.body.contentEditable=state.editable?'true':'false';document.body.spellcheck=true;
  for(const el of document.querySelectorAll('[data-cindy-mail-run]'))el.removeAttribute('contenteditable');
  for(const el of document.querySelectorAll('img[data-cindy-mail-cid]')){const image=state.displayImages.find(i=>i.cid===el.getAttribute('data-cindy-mail-cid'));if(image)el.src=image.url;else el.removeAttribute('src')}
  if(state.external)for(const el of document.querySelectorAll('img[data-cindy-mail-external-src]')){const url=api.externalMailImageUrl(el.getAttribute('data-cindy-mail-external-src')||'');if(url)el.src=url}
  document.getElementById('__winone_mail_pending')?.remove();
 };
 globalThis.__winoneMailApply=function(next){
  const scroll=[document.documentElement.scrollTop,document.body.scrollTop];
  if(next.replace){
   state.stopTheme?.();
   const parsed=new DOMParser().parseFromString(next.html,'text/html');
   document.head.replaceChildren(...Array.from(parsed.head.childNodes).map(n=>document.importNode(n,true)));
   for(const attr of Array.from(document.body.attributes))document.body.removeAttribute(attr.name);
   for(const attr of parsed.body.attributes)document.body.setAttribute(attr.name,attr.value);
   document.body.replaceChildren(...Array.from(parsed.body.childNodes).map(n=>document.importNode(n,true)));
   state.emitted=null;state.lastEdit=null;
  }
  Object.assign(state,next);state.serializer=api.mailBodySerializer(document,state.source,()=>state.images);
  paint();size();document.documentElement.scrollTop=scroll[0];document.body.scrollTop=scroll[1];return true;
 };
 globalThis.__winoneMailNext=()=>state.queue.length?Promise.resolve(state.queue.shift()):new Promise(resolve=>state.waiter=resolve);
 globalThis.__winoneMailSnapshot=async()=>{
  if(state.composing)await new Promise(resolve=>document.addEventListener('compositionend',resolve,{once:true}));
  return state.lastEdit||null;
 };
 document.addEventListener('compositionstart',()=>state.composing=true);
 const changed=()=>{if(state.composing||!state.editable)return;const event=serialize();if(event)push(event);size()};
 document.addEventListener('compositionend',()=>{state.composing=false;changed()});document.addEventListener('input',changed);
 const link=event=>{
  const target=event.target instanceof Element?event.target:event.target?.parentElement;
  const a=target?.closest('a');if(a){event.preventDefault();if(state.reading&&!state.editable){const url=api.mailPreviewLinkUrl?.(a.getAttribute('href'));if(url)push({kind:'link',url})}return}
  const image=target?.closest('img[data-cindy-mail-cid]');if(image&&!state.editable){const original=state.images.find(i=>i.cid===image.getAttribute('data-cindy-mail-cid'));if(original)push({kind:'image',url:original.url})}
 };
 document.addEventListener('click',link);document.addEventListener('auxclick',event=>{if(event.button===1)link(event)});
 document.addEventListener('drop',event=>event.preventDefault());
 document.addEventListener('paste',event=>{if(!state.editable)return;event.preventDefault();document.execCommand('insertText',false,event.clipboardData?.getData('text/plain')||'')});
 new ResizeObserver(size).observe(document.body);document.addEventListener('load',size,true);
 state.serializer=api.mailBodySerializer(document,state.source,()=>state.images);paint();size();return true;
};`

// The native reader may hand an ordinary, checked link to Chat's existing
// browser navigation. It is not a plugin request or an authorization gesture.
func (h *Host) openMailDocumentLink(session, address string) {
	if safeMailLink(address) == "" || session == "" {
		return
	}
	if h.cards.mail.browserOpener != nil {
		h.cards.mail.browserOpener(session, address)
		return
	}
	h.QueueMailBrowser(session, address)
}
