package mathview

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui/markdown"
)

// fakeWorker answers like the real one: "bad" fails alone, "die" ends the worker.
func fakeWorker(started *int) func() (*Worker, error) {
	return func() (*Worker, error) {
		*started++
		inRead, inWrite := io.Pipe()
		outRead, outWrite := io.Pipe()
		go func() {
			defer outWrite.Close()
			scanner := bufio.NewScanner(inRead)
			for scanner.Scan() {
				var q request
				_ = json.Unmarshal(scanner.Bytes(), &q)
				if q.TeX == "die" {
					return
				}
				answer := response{ID: q.ID, OK: q.TeX != "bad", SVG: `<svg viewBox="0 -700 500 900"><rect width="500" height="100"/></svg>`, Box: [4]float32{0, -700, 500, 900}, Width: .5, Ascent: .7, Descent: .2}
				data, _ := json.Marshal(answer)
				_, _ = outWrite.Write(append(data, '\n'))
			}
		}()
		return &Worker{In: inWrite, Out: outRead, Close: func() { inRead.Close(); outRead.Close() }}, nil
	}
}

type loop struct {
	mu    sync.Mutex
	posts []func()
}

func (l *loop) post(fn func()) {
	l.mu.Lock()
	l.posts = append(l.posts, fn)
	l.mu.Unlock()
}

// until runs posted work, as the window's loop would, until the condition holds.
func (l *loop) until(t *testing.T, done func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		l.mu.Lock()
		posts := l.posts
		l.posts = nil
		l.mu.Unlock()
		for _, fn := range posts {
			fn()
		}
		if done() {
			return
		}
	}
	t.Fatal("timed out")
}

func TestAFormulaIsAskedForOnceAndThenDrawnOnItsBaseline(t *testing.T) {
	l, started := &loop{}, 0
	r := New(l.post, fakeWorker(&started))
	r.idle = 10 * time.Millisecond
	o := r.Apply(&markdown.Options{})
	if _, ok := o.InlineMath("x", 20); ok {
		t.Fatal("a formula not laid out yet must show as TeX")
	}
	var object markdown.InlineObject
	l.until(t, func() bool {
		var ok bool
		object, ok = o.InlineMath("x", 20)
		return ok
	})
	if object.Width != 10 || object.Height != 14 || object.Depth != 4 {
		t.Fatalf("object = %+v", object)
	}
	if started != 1 {
		t.Fatalf("started %d workers", started)
	}
}

func TestAMalformedFormulaStaysAsItsSourceAndOthersContinue(t *testing.T) {
	l, started := &loop{}, 0
	r := New(l.post, fakeWorker(&started))
	r.idle = 50 * time.Millisecond
	r.lookup("bad", false)
	r.lookup("good", false)
	l.until(t, func() bool { return r.lookup("bad", false) != nil && r.lookup("good", false) != nil })
	if !r.lookup("bad", false).failed || r.lookup("good", false).formula == nil {
		t.Fatalf("bad=%+v good=%+v", r.lookup("bad", false), r.lookup("good", false))
	}
}

func TestAWorkerThatEndsFailsItsFormulasAndTheNextStartsAgain(t *testing.T) {
	l, started := &loop{}, 0
	r := New(l.post, fakeWorker(&started))
	r.idle = 10 * time.Millisecond
	r.lookup("die", false)
	l.until(t, func() bool { return r.lookup("die", false) != nil })
	if !r.lookup("die", false).failed {
		t.Fatal("the formula that ended the worker must fail")
	}
	l.until(t, func() bool { return r.lookup("next", false) != nil })
	if started != 2 {
		t.Fatalf("started %d workers", started)
	}
}

func TestAMissingRuntimeLeavesTheSource(t *testing.T) {
	l := &loop{}
	r := New(l.post, func() (*Worker, error) { return nil, errors.New("missing") })
	l.until(t, func() bool { return r.lookup("x", true) != nil })
	if !r.lookup("x", true).failed {
		t.Fatal("expected failure")
	}
}

func TestStreamingKeepsOnlyTheLatestSources(t *testing.T) {
	l := &loop{}
	block := make(chan struct{})
	r := New(l.post, func() (*Worker, error) { <-block; return nil, errors.New("closed") })
	for i := 0; i < maxWaiting*3; i++ {
		r.lookup(string(rune('a'+i)), false)
	}
	r.mu.Lock()
	count := len(r.queue)
	r.mu.Unlock()
	close(block)
	if count > maxWaiting {
		t.Fatalf("queue holds %d", count)
	}
}

// TestTheBundledWorkerDrawsAFormula runs the real bundle when a build gives it.
func TestTheBundledWorkerDrawsAFormula(t *testing.T) {
	script, runtime := os.Getenv("WINONE_MATH_WORKER"), os.Getenv("WINONE_MATH_RUNTIME")
	if script == "" || runtime == "" {
		t.Skip("set WINONE_MATH_WORKER and WINONE_MATH_RUNTIME")
	}
	script, _ = filepath.Abs(script)
	l := &loop{}
	r := New(l.post, func() (*Worker, error) { return StartWorker(runtime, script) })
	r.idle = 50 * time.Millisecond
	const tex = `\frac{a}{\mathcal{L}}+\text{速度}`
	l.until(t, func() bool { return r.lookup(tex, true) != nil })
	f := r.lookup(tex, true).formula
	if f == nil || f.width <= 0 || len(f.texts) != 1 || f.texts[0].Text != "速度" {
		t.Fatalf("formula = %+v", f)
	}
}
