//go:build windows

package app

import (
	"sync"
	"testing"
	"time"

	"winone/desktopmygo/core"
)

func TestStartupTraceIsOptInAndWaitsForNativeCallbacks(t *testing.T) {
	t.Setenv("WINONE_DESKTOP_STARTUP_TRACE", "")
	if newStartupTrace() != nil {
		t.Fatal("normal launch acquired benchmark instrumentation")
	}
	t.Setenv("WINONE_DESKTOP_STARTUP_TRACE", "1")
	trace := newStartupTrace()
	var mu sync.Mutex
	var callbacks []func()
	env := &core.Env{Post: func(fn func()) { mu.Lock(); callbacks = append(callbacks, fn); mu.Unlock() }}
	trace.instrument(env)
	blocked, release := make(chan struct{}), make(chan struct{})
	env.Spawn(func() {
		close(blocked)
		<-release
		env.Post(func() {})
	})
	<-blocked
	if trace.work.Load() != 1 || trace.snapshot.Load() || trace.reported {
		t.Fatal("worker acceptance or window creation was reported as ready")
	}
	close(release)
	deadline := time.Now().Add(time.Second)
	for trace.work.Load() != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if trace.work.Load() != 0 || trace.posted.Load() != 1 || trace.reported {
		t.Fatal("worker completion skipped its pending UI callback")
	}
	mu.Lock()
	queued := append([]func(){}, callbacks...)
	mu.Unlock()
	for _, fn := range queued {
		fn()
	}
	if trace.posted.Load() != 0 || trace.reported {
		t.Fatal("callback completion alone claimed a native paint receipt")
	}
}
