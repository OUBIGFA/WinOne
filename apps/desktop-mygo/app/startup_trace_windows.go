//go:build windows

package app

import (
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/core"
)

var desktopLaunchTime = time.Now()

// startupTrace is opt-in measurement of the production window, not a second
// readiness implementation. It waits for actual Host data, UI callbacks and
// native painting. It neither prewarms resources nor changes window lifetime.
// The external benchmark also measures process creation to this receipt;
// neither timestamp claims physical input-to-photon latency.
type startupTrace struct {
	work, posted atomic.Int64
	snapshot     atomic.Bool
	reported     bool
}

func newStartupTrace() *startupTrace {
	if os.Getenv("WINONE_DESKTOP_STARTUP_TRACE") != "1" {
		return nil
	}
	return &startupTrace{}
}

func (t *startupTrace) stage(name string) {
	if t != nil {
		fmt.Fprintf(os.Stderr, "startup %.3fms %s\n", float64(time.Since(desktopLaunchTime).Microseconds())/1000, name)
	}
}

func (t *startupTrace) instrument(env *core.Env) {
	if t == nil {
		return
	}
	post := env.Post
	env.Post = func(fn func()) {
		t.posted.Add(1)
		post(func() { defer t.posted.Add(-1); fn() })
	}
	env.Spawn = func(fn func()) {
		t.work.Add(1)
		go func() { defer t.work.Add(-1); fn() }()
	}
}

func (t *startupTrace) content(w *window) func(*ui.Context) {
	if t == nil {
		return w.shell.View
	}
	return func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() { w.shell.View(c) }).Draw(func(_ *ui.Painter, r ui.Rect) {
			if t.reported || r.W <= 0 || r.H <= 0 || !t.snapshot.Load() || t.work.Load() != 0 || t.posted.Load() != 0 {
				return
			}
			t.reported = true
			// Queue after this completed native draw, not when the shell has
			// merely accepted a snapshot or created a window handle.
			w.win.Update(func() {
				fmt.Fprintf(os.Stderr, "startup %.3fms ui-ready\n", float64(time.Since(desktopLaunchTime).Microseconds())/1000)
			})
		})
		if !t.reported {
			c.After(10 * time.Millisecond)
		}
	}
}
