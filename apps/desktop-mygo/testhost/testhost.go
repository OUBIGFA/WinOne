// Package testhost runs page views against a fake Host for tests: a real
// named pipe served in the test process, reached through the window's real
// client, so pages are tested with their actual requests and replies.
package testhost

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"winone/desktopmygo/core"
	"winone/desktopmygo/host"
	internalhost "winone/internal/host"
	"winone/internal/transport"
)

// Handler answers one request: a result to encode, or an error the client
// reports as the Host's.
type Handler func(method string, params json.RawMessage) (any, error)

// Error is a Host error a Handler returns.
type Error struct{ Code, Message string }

func (e Error) Error() string { return e.Message }

// Harness is a page environment connected to a fake Host.
type Harness struct {
	Env *core.Env
	t   testing.TB

	mu      sync.Mutex
	calls   []Call
	work    sync.WaitGroup
	posted  []func()
	handler Handler
	// Picked is what the fake dialogs return, by title.
	Picked map[string]any
}

// Call is a request the page sent.
type Call struct {
	Method string
	Params json.RawMessage
}

type service struct{ h *Harness }

func (s service) Handle(request internalhost.Request) internalhost.Response {
	s.h.mu.Lock()
	s.h.calls = append(s.h.calls, Call{Method: request.Method, Params: request.Params})
	handler := s.h.handler
	s.h.mu.Unlock()
	result, err := handler(request.Method, request.Params)
	if err != nil {
		code := "failed"
		if e, ok := err.(Error); ok {
			code = e.Code
		}
		return internalhost.Response{ID: request.ID, Error: &internalhost.RPCError{Code: code, Message: err.Error()}}
	}
	data, err := json.Marshal(result)
	if err != nil {
		return internalhost.Response{ID: request.ID, Error: &internalhost.RPCError{Code: "test", Message: err.Error()}}
	}
	return internalhost.Response{ID: request.ID, Result: data}
}

// New serves handler on a fresh pipe for the test's duration.
func New(t testing.TB, handler Handler) *Harness {
	t.Helper()
	pipe := fmt.Sprintf(`\\.\pipe\WinOne-test-%d`, time.Now().UnixNano())
	listener, err := transport.Listen(pipe)
	if err != nil {
		t.Fatal(err)
	}
	h := &Harness{t: t, handler: handler, Picked: map[string]any{}}
	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan struct{})
	go func() { _ = transport.Serve(ctx, listener, service{h}); close(served) }()
	client, err := host.NewClient(pipe)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Cancel(); cancel(); <-served })
	h.Env = &core.Env{
		Host:          client,
		Post:          func(fn func()) { h.mu.Lock(); h.posted = append(h.posted, fn); h.mu.Unlock() },
		Spawn:         func(fn func()) { h.work.Add(1); go func() { defer h.work.Done(); fn() }() },
		Snapshot:      func() internalhost.Snapshot { return internalhost.Snapshot{} },
		Refresh:       func() {},
		Navigate:      func(string, func()) {},
		RequestChange: func(apply func()) { apply() },
		Activity:      &core.Activity{},
		Rail:          &core.Rail{},
		Dialogs:       dialogs{h},
		Directory:     t.TempDir(),
	}
	sidebar := "collapsed"
	h.Env.SidebarStartup = func() string { return sidebar }
	h.Env.SetSidebarStartup = func(value string) error { sidebar = value; return nil }
	return h
}

// SetHandler replaces how the fake Host answers.
func (h *Harness) SetHandler(handler Handler) { h.mu.Lock(); h.handler = handler; h.mu.Unlock() }

// Settle waits for the page's requests, applies their results on the test
// goroutine (the UI thread of a Tester) and draws, until nothing is left.
func (h *Harness) Settle(tt *ui.Tester) {
	for range 100 {
		h.work.Wait()
		h.mu.Lock()
		posted := h.posted
		h.posted = nil
		h.mu.Unlock()
		if len(posted) == 0 {
			tt.Frame()
			return
		}
		for _, fn := range posted {
			fn()
		}
		tt.Frame()
	}
	h.t.Fatal("the page keeps starting requests")
}

// Calls returns the requests sent so far.
func (h *Harness) Calls() []Call {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]Call(nil), h.calls...)
}

// Called returns the requests to method.
func (h *Harness) Called(method string) []Call {
	var calls []Call
	for _, call := range h.Calls() {
		if call.Method == method {
			calls = append(calls, call)
		}
	}
	return calls
}

type dialogs struct{ h *Harness }

func (d dialogs) picked(title string) any {
	d.h.mu.Lock()
	defer d.h.mu.Unlock()
	return d.h.Picked[title]
}

func (d dialogs) PickFile(title string, _ core.Filter) (string, error) {
	path, _ := d.picked(title).(string)
	return path, nil
}

func (d dialogs) PickFiles(title string, _ core.Filter) ([]string, error) {
	paths, _ := d.picked(title).([]string)
	return paths, nil
}

func (d dialogs) PickFolder(title string) (string, error) {
	path, _ := d.picked(title).(string)
	return path, nil
}

func (d dialogs) PickFolders(title string) ([]string, error) {
	paths, _ := d.picked(title).([]string)
	return paths, nil
}

func (d dialogs) SaveFile(title string, _ core.Filter, _ string) (string, error) {
	path, _ := d.picked(title).(string)
	return path, nil
}
