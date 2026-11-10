package flashchat

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image"
	"image/jpeg"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
)

type nativeBridge struct {
	token, origin string
	mu            sync.Mutex
	files         map[string]string
	state         func(nativeSnapshot)
	frame         func(string, image.Image)
}

func newNativeBridge(origin string) *nativeBridge {
	return &nativeBridge{token: randomToken(), origin: origin, files: map[string]string{}}
}
func randomToken() string {
	var token [32]byte
	if _, err := rand.Read(token[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(token[:])
}

// The token is injected only into the top-level bundled engine. Untrusted
// messages, external pages and other local processes receive no file authority.
func (b *nativeBridge) serve(w http.ResponseWriter, r *http.Request) {
	if "http://"+r.Host != b.origin || r.Header.Get("X-WinOne-Token") != b.token || (r.Header.Get("Origin") != "" && r.Header.Get("Origin") != b.origin) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	name := strings.TrimPrefix(r.URL.Path, "/native/")
	switch {
	case name == "state" && r.Method == http.MethodPost:
		var state nativeSnapshot
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<20)).Decode(&state); err != nil {
			http.Error(w, "Invalid state", 400)
			return
		}
		if b.state != nil {
			b.state(state)
		}
	case strings.HasPrefix(name, "frame/") && r.Method == http.MethodPost:
		// Only raster media frames are decoded. No HTML, SVG or URL is rendered.
		data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4<<20))
		if err != nil {
			http.Error(w, "Invalid frame", 400)
			return
		}
		config, err := jpeg.DecodeConfig(strings.NewReader(string(data)))
		if err != nil || config.Width > 4096 || config.Height > 4096 {
			http.Error(w, "Invalid frame", 400)
			return
		}
		frame, err := jpeg.Decode(strings.NewReader(string(data)))
		if err != nil {
			http.Error(w, "Invalid frame", 400)
			return
		}
		if b.frame != nil {
			b.frame(strings.TrimPrefix(name, "frame/"), frame)
		}
	case strings.HasPrefix(name, "file/") && (r.Method == http.MethodGet || r.Method == http.MethodHead):
		b.mu.Lock()
		path := b.files[strings.TrimPrefix(name, "file/")]
		b.mu.Unlock()
		if path == "" {
			http.NotFound(w, r)
			return
		}
		file, err := os.Open(path)
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
		http.ServeContent(w, r, info.Name(), info.ModTime(), file)
	default:
		http.NotFound(w, r)
	}
}

func (p *completePage) attachNative(app *localApp) {
	b := newNativeBridge(app.origin)
	b.state = func(state nativeSnapshot) {
		p.env.Post(func() {
			if p.closed || state.Revision <= p.snapshot.Revision {
				return
			}
			first := !p.snapshot.Ready
			if p.snapshot.RoomID != state.RoomID {
				p.frames = map[string]*ui.Bitmap{}
				p.previewRequested, p.previewErrors = map[string]bool{}, map[string]string{}
				p.drafts, p.lists = map[string]string{}, map[string]*ui.ListState{}
			}
			p.snapshot = state
			if first {
				p.formNickname = state.Settings.Nickname
				p.command("shell", "generate", nil, func(raw json.RawMessage) { _ = json.Unmarshal(raw, &p.formRoom) })
			}
			if state.RoomID == "" {
				p.selected = ""
			}
			live := map[string]bool{}
			for _, stream := range state.Media.Streams {
				live[stream.ID] = true
			}
			for _, items := range state.Previews {
				for _, item := range items {
					live[item.ID] = true
				}
			}
			for id := range p.frames {
				if !live[id] {
					delete(p.frames, id)
				}
			}
		})
	}
	b.frame = func(id string, frame image.Image) {
		p.env.Post(func() {
			if p.closed {
				return
			}
			if bitmap := p.frames[id]; bitmap != nil {
				bitmap.Update(frame)
			} else {
				p.frames[id] = ui.NewBitmap(frame)
			}
		})
	}
	p.bridge = b
	app.native.Store(b)
}

func (p *completePage) invoke(scope, action string, args map[string]any) (json.RawMessage, error) {
	engine := p.engine.Load()
	if engine == nil {
		return nil, errors.New(nt("starting"))
	}
	if args == nil {
		args = map[string]any{}
	}
	args["scope"], args["action"] = scope, action
	data, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	response, err := mygo.EvalAs[struct {
		Value json.RawMessage `json:"value"`
		Error string          `json:"error"`
	}](engine, "window.__flashNativeInvoke("+string(data)+")")
	if err != nil {
		return nil, err
	}
	if response.Error != "" {
		return nil, errors.New(response.Error)
	}
	return response.Value, nil
}

func (p *completePage) command(scope, action string, args map[string]any, done func(json.RawMessage)) {
	download := action == "download"
	if p.busy || (download && p.downloading) {
		return
	}
	// A file download may take minutes. Keep the composer and call controls
	// usable while preventing duplicate download requests.
	p.actionError = ""
	if download {
		p.downloading = true
	} else {
		p.busy = true
	}
	core.Go(p.env, func() (json.RawMessage, error) { return p.invoke(scope, action, args) }, func(value json.RawMessage, err error) {
		if p.closed {
			return
		}
		if download {
			p.downloading = false
		} else {
			p.busy = false
		}
		if err != nil {
			p.actionError = err.Error()
			return
		}
		if done != nil {
			done(value)
		}
	})
}

// Typing, volume and held-talk updates must not discard the release of a key
// just because a longer operation is pending. Failures remain visible.
func (p *completePage) backgroundCommand(scope, action string, args map[string]any) {
	p.background = append(p.background, nativeAction{scope, action, args})
	if len(p.background) == 1 {
		p.drainNative()
	}
}

type nativeAction struct {
	scope, action string
	args          map[string]any
}

func (p *completePage) drainNative() {
	if p.closed || len(p.background) == 0 {
		return
	}
	a := p.background[0]
	core.Do(p.env, func() error { _, err := p.invoke(a.scope, a.action, a.args); return err }, func(err error) {
		if p.closed {
			return
		}
		p.background = p.background[1:]
		if err != nil {
			p.actionError = err.Error()
		}
		p.drainNative()
	})
}

func (p *completePage) presentNative(visible, focused bool) {
	if p.outbox == nil || !p.snapshot.Ready {
		return
	}
	data, _ := json.Marshal(map[string]any{"visible": visible, "focused": focused, "selected": p.selected})
	state := string(data)
	if state == p.presentation {
		return
	}
	select {
	case p.outbox <- "Object.assign(window.__winoneNative," + state + ");window.dispatchEvent(new Event('native-presentation'))":
		p.presentation = state
	default:
	}
}

func (p *completePage) shareNative(paths []string) {
	if p.busy || p.bridge == nil {
		return
	}
	p.busy, p.actionError = true, ""
	scope := "room:" + p.selected
	b := p.bridge
	core.Do(p.env, func() error {
		if paths == nil {
			var err error
			paths, err = p.env.Dialogs.PickFiles(tr("shareInChat"), core.Filter{})
			if err != nil {
				return err
			}
		}
		if len(paths) == 0 {
			return nil
		}
		files := []map[string]string{}
		defer func() {
			b.mu.Lock()
			defer b.mu.Unlock()
			for _, file := range files {
				delete(b.files, file["token"])
			}
		}()
		for _, path := range paths {
			info, err := os.Stat(path)
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return errors.New(nt("filesOnly"))
			}
			token := randomToken()
			b.mu.Lock()
			b.files[token] = path
			b.mu.Unlock()
			files = append(files, map[string]string{"token": token, "name": info.Name(), "type": mime.TypeByExtension(filepath.Ext(path))})
		}
		_, err := p.invoke(scope, "share", map[string]any{"files": files})
		return err
	}, func(err error) {
		if !p.closed {
			p.busy = false
			if err != nil {
				p.actionError = err.Error()
			}
		}
	})
}
