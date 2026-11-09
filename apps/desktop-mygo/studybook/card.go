package studybook

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"winone/desktopmygo/core"
)

// A private MyGo protocol streams card media without a network listener.
// Capabilities disappear on close; OpenRoot confines reads to the media folder.
var cardDocuments sync.Map
var cardProtocolError = mygo.Protocol.HandleFunc("winone-anki", serveCard)

type cardView struct {
	link                           func(string)
	frontend                       func(string, []byte) ([]byte, error)
	dark                           bool
	command                        func(string)
	view                           *mygo.WebView
	off                            func()
	token, document, media, assets string
}

func serveCard(w http.ResponseWriter, r *http.Request) {
	entry, ok := cardDocuments.Load(r.URL.Host)
	if !ok {
		http.NotFound(w, r)
		return
	}
	p := entry.(*cardView)
	if p.view == nil || mygo.CallerPage(r.Context()) != p.view.Page() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	name := strings.TrimPrefix(r.URL.Path, "/")
	if (name == "_anki/getSchedulingStatesWithContext" || name == "_anki/setSchedulingStates") && r.Method == http.MethodPost {
		data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil {
			http.Error(w, "Request too large", 413)
			return
		}
		if p.frontend == nil {
			http.Error(w, "Review unavailable", 409)
			return
		}
		result, err := p.frontend(strings.TrimPrefix(name, "_anki/"), data)
		if err != nil {
			http.Error(w, err.Error(), 409)
			return
		}
		w.Header().Set("Content-Type", "application/binary")
		_, _ = w.Write(result)
		return
	}
	if name == "command" && r.Method == http.MethodPost {
		data, err := io.ReadAll(io.LimitReader(r.Body, 128))
		if err != nil {
			http.Error(w, "Invalid command", 400)
			return
		}
		command := string(data)
		if command != "ans" && command != "ease1" && command != "ease2" && command != "ease3" && command != "ease4" && command != "statesMutated" {
			http.Error(w, "Unknown card command", 400)
			return
		}
		if p.command != nil {
			p.command(command)
		}
		w.WriteHeader(204)
		return
	}
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	if name == "index.html" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(p.document))
		return
	}
	directory := p.media
	asset := strings.HasPrefix(name, "_anki/")
	if asset {
		directory = p.assets
		name = strings.TrimPrefix(name, "_anki/")
	}
	if name == "" || strings.ContainsAny(name, "\\\x00") || !asset && strings.Contains(name, "/") || !filepath.IsLocal(name) {
		http.NotFound(w, r)
		return
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer root.Close()
	file, err := root.Open(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	http.ServeContent(w, r, name, info.ModTime(), file)
}

func newCardView(env *core.Env, body string, state *review, command func(string), link func(string), dark, allowExternal bool) (*cardView, error) {
	css, media, autoplay, side, replayQuestion, mutationKey, customScheduling := state.Rendered.CSS, state.MediaDirectory, state.AutoPlay, state.Side, state.ReplayQuestion, state.MutationKey, state.CustomScheduling
	if state.CustomApplied {
		customScheduling = ""
	}
	if cardProtocolError != nil {
		return nil, cardProtocolError
	}
	if env.WebViews == nil {
		return nil, errors.New(text("rendererUnavailable"))
	}
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	assets := filepath.Join(filepath.Dir(executable), "shared", "anki", "renderer")
	if value := os.Getenv("WINONE_ANKI_RENDERER"); value != "" {
		assets = value
	}
	if _, err := os.Stat(filepath.Join(assets, "js", "reviewer.js")); err != nil {
		return nil, err
	}
	p := &cardView{token: strings.ToLower(rand.Text()), media: media, assets: assets, command: func(value string) { env.Post(func() { command(value) }) }}
	p.link = func(value string) { env.Post(func() { link(value) }) }
	p.frontend = func(method string, data []byte) ([]byte, error) {
		var result string
		err := env.Host.CallInto("learning.call", map[string]any{"method": "winone.frontend", "params": map[string]string{"method": method, "key": mutationKey, "data": base64.StdEncoding.EncodeToString(data)}}, &result)
		if err != nil {
			return nil, err
		}
		return base64.StdEncoding.DecodeString(result)
	}
	p.dark = dark
	p.document = cardDocument(body, css, dark)
	if allowExternal {
		for _, directive := range []string{"connect-src 'self'", "img-src 'self' data:", "media-src 'self' data:", "font-src 'self' data:", "style-src 'self' 'unsafe-inline'", "script-src 'self' 'unsafe-inline'"} {
			p.document = strings.Replace(p.document, directive, directive+" https: http:", 1)
		}
	}
	if customScheduling != "" {
		key, _ := json.Marshal(mutationKey)
		script := `<script>anki.mutateNextCardStates(` + string(key) + `,async(states,customData,ctx)=>{` + "\n" + customScheduling + "\n" + `}).then(()=>bridgeCommand('statesMutated')).catch(error=>{document.getElementById('qa').insertAdjacentText('beforebegin',String(error));});</script>`
		p.document = strings.Replace(p.document, "</body>", script+"</body>", 1)
	}
	if autoplay {
		filter := ""
		if side == "answer" && !replayQuestion {
			filter = `.filter(item=>item.dataset.side!=='q')`
		}
		script := `<script>(()=>{const qa=document.getElementById('qa');const observer=new MutationObserver(play);function play(){if(qa.style.opacity!=='1')return;observer.disconnect();const items=[...qa.querySelectorAll('audio,video')]` + filter + `;let at=0;function next(){const item=items[at++];if(!item)return;item.addEventListener('ended',next,{once:true});item.addEventListener('error',()=>{const message=document.createElement('p');message.textContent='Audio unavailable. / 音频不可用。';item.after(message);},{once:true});item.play().catch(error=>{const message=document.createElement('p');message.textContent=String(error.message||error);item.after(message);});}next();}observer.observe(qa,{attributes:true,attributeFilter:['style']});play();})();</script>`
		p.document = strings.Replace(p.document, "</body>", script+"</body>", 1)
	}
	address := "winone-anki://" + p.token + "/index.html"
	view, err := env.WebViews(mygo.WebViewOptions{URL: "about:blank", Page: mygo.PageOptions{Untrusted: true, Session: mygo.NewEphemeralSession()}})
	if err != nil {
		cardDocuments.Delete(p.token)
		return nil, err
	}
	p.view = view
	cardDocuments.Store(p.token, p)
	p.off = view.Page().OnWillNavigate(func(event *mygo.NavigateEvent) {
		if event.URL != address && !strings.HasPrefix(event.URL, address+"#") {
			event.PreventDefault()
			if parsed, err := url.Parse(event.URL); err == nil && event.UserInitiated && (parsed.Scheme == "https" || parsed.Scheme == "http") {
				p.link(event.URL)
			}
		}
	})
	if err = view.Page().LoadURL(address); err != nil {
		p.Close()
		return nil, err
	}
	return p, nil
}

// cardDocument keeps user HTML inside the original, unprivileged Anki renderer.
// JSON escaping prevents a card string from ending the bootstrap script.
func cardDocument(body, css string, dark ...bool) string {
	content, _ := json.Marshal(body)
	class := "card"
	if len(dark) > 0 && dark[0] {
		class = "card nightMode"
		css += "\n:root{--canvas:#202020;--fg:#eeeeee}.card.nightMode{background-color:var(--canvas);color:var(--fg)}"
	}
	classJSON, _ := json.Marshal(class)
	return `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; connect-src 'self'; img-src 'self' data:; media-src 'self' data:; font-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline'"><link rel="stylesheet" href="/_anki/css/reviewer.css"><style>html{color-scheme:light dark}body{margin:16px;overflow-wrap:anywhere}img,video{max-width:100%}*{box-sizing:border-box}` + css + `</style><script>window.bridgeCommand=window.pycmd=function(command){if(/^(ans|ease[1-4]|statesMutated)$/.test(command))return fetch('/command',{method:'POST',body:command});};</script><script src="/_anki/js/reviewer.js"></script></head><body class="` + class + `"><div id="qa"></div><div id="_flag" hidden></div><div id="_mark" hidden></div><script>_showQuestion(` + string(content) + `,` + string(content) + `,` + string(classJSON) + `);</script></body></html>`
}
func (p *cardView) View(c *ui.Context) {
	ui.WebView(c, p.view).FillWidth().Height(360).MinWidth(0).Label(text("card"))
}
func (p *cardView) Close() {
	cardDocuments.Delete(p.token)
	if p.off != nil {
		p.off()
	}
	p.view.Destroy()
}

func (p *cardView) Replay() {
	go func() {
		_, _ = p.view.Page().Eval(`const items=[...document.querySelectorAll('audio,video')];for(const item of items){item.currentTime=0;await item.play();await new Promise(resolve=>{item.addEventListener('ended',resolve,{once:true});item.addEventListener('error',resolve,{once:true});});}`)
	}()
}
