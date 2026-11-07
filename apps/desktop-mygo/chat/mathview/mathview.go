// Package mathview draws TeX in Markdown as vector formulas. MyGo leaves math to
// the application; the layout runs in a small worker of the bundled runtime
// (modules/chat-pi/math), so a formula never holds up the window and a window
// without formulas never starts it. Until a formula is ready, and when it cannot
// be drawn, its TeX stays readable and copyable.
package mathview

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/egoist/mygo/ui/markdown/parser"

	"winone/desktopmygo/core"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
	"winone/desktopmygo/kit/icons"
)

const (
	maxCached  = 1024
	maxWaiting = 64
	// idleTime keeps the worker for the formulas of one reply, which arrive a
	// few at a time, and releases its memory soon after.
	idleTime = 20 * time.Second
	// displayScale is how much larger a block of math is than its text, as
	// KaTeX draws display math.
	displayScale = 1.21
)

// run is text the math fonts have no glyphs for, in viewBox units.
type run struct {
	Text string  `json:"text"`
	X    float32 `json:"x"`
	Y    float32 `json:"y"`
	Size float32 `json:"size"`
}

type formula struct {
	svg                    *ui.SVG
	box                    [4]float32
	width, ascent, descent float32 // in em
	texts                  []run
}

type entry struct {
	formula *formula
	failed  bool
}

type request struct {
	ID      string `json:"id"`
	TeX     string `json:"tex"`
	Display bool   `json:"display"`
}

type response struct {
	ID      string     `json:"id"`
	OK      bool       `json:"ok"`
	SVG     string     `json:"svg"`
	Box     [4]float32 `json:"box"`
	Width   float32    `json:"width"`
	Ascent  float32    `json:"ascent"`
	Descent float32    `json:"descent"`
	Texts   []run      `json:"texts"`
}

// Worker is a started math worker: lines of request go in, lines of response
// come out. Close ends it.
type Worker struct {
	In    io.WriteCloser
	Out   io.Reader
	Close func()
}

// Renderer lays out the formulas of one window. Formula is called on the UI
// thread, and results come back there through the window's Post.
type Renderer struct {
	post     func(func())
	start    func() (*Worker, error)
	idle     time.Duration
	cache    map[string]*entry
	order    []string
	waiting  []request
	revision uint64 // Shaping changes only when an asynchronous formula completes.
	asked    map[string]bool

	mu      sync.Mutex // guards queue and running, shared with the worker goroutine
	queue   []request
	running bool
}

// New makes a renderer. start launches the worker when the first formula needs it.
func New(post func(func()), start func() (*Worker, error)) *Renderer {
	return &Renderer{post: post, start: start, idle: idleTime, cache: map[string]*entry{}, asked: map[string]bool{}}
}

var (
	sharedMu sync.Mutex
	shared   = map[*core.Env]*Renderer{}
)

// For is the renderer of a window, which all its pages share.
func For(env *core.Env) *Renderer {
	sharedMu.Lock()
	defer sharedMu.Unlock()
	r := shared[env]
	if r == nil {
		r = New(env.Post, startBundledWorker)
		shared[env] = r
	}
	return r
}

func key(tex string, display bool) string { return strconv.FormatBool(display) + ":" + tex }

// lookup returns what is known of a formula and asks for it once.
func (r *Renderer) lookup(tex string, display bool) *entry {
	k := key(tex, display)
	if e := r.cache[k]; e != nil {
		return e
	}
	if !r.asked[k] {
		r.asked[k] = true
		r.request(request{ID: k, TeX: tex, Display: display})
	}
	return nil
}

func (r *Renderer) request(q request) {
	r.mu.Lock()
	// Each frame of a formula still streaming in is a new source. Only the
	// latest ones matter; a dropped one is asked for again if it comes back.
	if len(r.queue) >= maxWaiting {
		delete(r.asked, r.queue[0].ID)
		r.queue = r.queue[1:]
	}
	r.queue = append(r.queue, q)
	start := !r.running
	r.running = true
	r.mu.Unlock()
	if start {
		go r.serve()
	}
}

func (r *Renderer) next(wait time.Duration) (request, bool) {
	deadline := time.Now().Add(wait)
	for {
		r.mu.Lock()
		if len(r.queue) > 0 {
			q := r.queue[0]
			r.queue = r.queue[1:]
			r.mu.Unlock()
			return q, true
		}
		if !time.Now().Before(deadline) {
			r.running = false
			r.mu.Unlock()
			return request{}, false
		}
		r.mu.Unlock()
		time.Sleep(20 * time.Millisecond)
	}
}

// serve runs the worker while formulas keep coming. One request is out at a
// time, so a formula that fails the worker fails alone.
func (r *Renderer) serve() {
	worker, err := r.start()
	if err != nil {
		r.failAll()
		return
	}
	defer worker.Close()
	reader := bufio.NewReaderSize(worker.Out, 1<<16)
	encoder := json.NewEncoder(worker.In)
	for {
		q, ok := r.next(r.idle)
		if !ok {
			worker.In.Close()
			return
		}
		var answer response
		err := encoder.Encode(q)
		if err == nil {
			var line []byte
			if line, err = reader.ReadBytes('\n'); err == nil {
				err = json.Unmarshal(line, &answer)
			}
		}
		if err != nil {
			r.deliver(q.ID, nil)
			r.failAll()
			return
		}
		r.deliver(q.ID, r.formulaOf(answer))
	}
}

func (r *Renderer) failAll() {
	r.mu.Lock()
	queue := r.queue
	r.queue, r.running = nil, false
	r.mu.Unlock()
	for _, q := range queue {
		r.deliver(q.ID, nil)
	}
}

func (r *Renderer) formulaOf(a response) *formula {
	if !a.OK || a.Width <= 0 || a.Box[2] <= 0 || a.Box[3] <= 0 {
		return nil
	}
	svg, err := ui.ParseSVG([]byte(a.SVG))
	if err != nil {
		return nil
	}
	return &formula{svg: svg, box: a.Box, width: a.Width, ascent: a.Ascent, descent: a.Descent, texts: a.Texts}
}

func (r *Renderer) deliver(id string, f *formula) {
	r.post(func() {
		delete(r.asked, id)
		if len(r.order) >= maxCached {
			delete(r.cache, r.order[0])
			r.order = r.order[1:]
		}
		r.cache[id] = &entry{formula: f, failed: f == nil}
		r.revision++
		r.order = append(r.order, id)
	})
}

// draw paints a formula in r, its box in the window.
func (f *formula) draw(p *ui.Painter, r ui.Rect) {
	color := p.Theme().Text
	p.Icon(f.svg, r, color)
	if len(f.texts) == 0 {
		return
	}
	scale := r.W / f.box[2]
	for _, t := range f.texts {
		size := t.Size * scale
		// A run's place is its baseline; Painter.Text takes the top of the line.
		p.Text(r.X+(t.X-f.box[0])*scale, r.Y+(t.Y-f.box[1])*scale-size*0.86, t.Text, size, color)
	}
}

// Apply makes the Markdown of o draw math. It may be called every frame, for
// options made every frame.
func (r *Renderer) Apply(o *markdown.Options) *markdown.Options {
	if o.InlineMath != nil {
		return o
	}
	if o.Extensions == 0 {
		o.Extensions = parser.Default
	}
	o.Extensions |= parser.ExtDollarMath
	o.InlineMathRevision = func() uint64 { return r.revision }
	o.InlineMath = func(tex string, size float32) (markdown.InlineObject, bool) {
		e := r.lookup(tex, false)
		if e == nil || e.formula == nil {
			return markdown.InlineObject{}, false
		}
		f := e.formula
		return markdown.InlineObject{Width: f.width * size, Height: f.ascent * size, Depth: f.descent * size, Draw: f.draw}, true
	}
	o.Math = r.block
	return o
}

// block shows display math. A formula wider than its column shrinks to fit:
// Chat's columns never scroll sideways.
func (r *Renderer) block(c *ui.Context, tex string) {
	t := c.Theme()
	e := r.lookup(tex, true)
	ui.Column(c.Key(key(tex, true))).FillWidth().Gap(4).Children(func() {
		if e == nil || e.formula == nil {
			ui.Text(c, tex).Font(kit.MonoFont).TextColor(t.TextMuted).Selectable().MinWidth(0)
			return
		}
		f := e.formula
		size := c.Theme().Rem(1) * displayScale
		height := f.ascent + f.descent
		ui.Box(c).FillWidth().MaxWidth(f.width * size).AspectRatio(f.width / max(height, 0.01)).AlignSelf(ui.Center).Label(tex).Draw(f.draw)
		if kit.IconButton(c, icons.Copy, i18n.T("chat.copy")).AlignSelf(ui.End).Clicked() {
			c.WriteClipboard(tex)
		}
	})
}

var errNoRuntime = errors.New("math runtime is unavailable")
