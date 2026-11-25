//go:build windows

package modules

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"winone/internal/host"
	"winone/internal/transport"
)

// Re-executed as a worker whose UI thread is reported hung: identity probes
// succeed, every feature command answers "unresponsive", and the process
// never exits by itself.
func init() {
	if os.Getenv("WINONE_UNRESPONSIVE_FIXTURE") != "1" {
		return
	}
	var pipe string
	for i := 1; i+1 < len(os.Args); i++ {
		if os.Args[i] == "--pipe" {
			pipe = os.Args[i+1]
		}
	}
	listener, err := transport.Listen(pipe)
	if err != nil {
		os.Exit(2)
	}
	_ = transport.Serve(context.Background(), listener, unresponsiveFixture{})
	os.Exit(0)
}

type unresponsiveFixture struct{}

func (unresponsiveFixture) Handle(request host.Request) host.Response {
	if request.Method == "snapshot" {
		data, _ := json.Marshal(map[string]any{"processId": os.Getpid()})
		return host.Response{ID: request.ID, Result: data}
	}
	return host.Response{ID: request.ID, Error: &host.RPCError{Code: "unresponsive", Message: "fixture UI thread hung"}}
}

// A hung capture worker used to stay alive forever and fail every shortcut
// with a pipe timeout. The Host must end it and start a fresh one.
func TestUnresponsiveWorkerIsReplaced(t *testing.T) {
	t.Setenv("WINONE_UNRESPONSIVE_FIXTURE", "1")
	pipe := fmt.Sprintf(`\\.\pipe\WinOne-unresponsive-test-%d-%d`, os.Getpid(), time.Now().UnixNano())
	lazy := NewLazy(context.Background(), "example.tool", "Example", os.Args[0], pipe, t.TempDir())
	defer func() {
		if p := lazy.process; p != nil && p.cmd != nil {
			_ = p.cmd.Process.Kill()
		}
	}()
	reply := lazy.Handle(host.Request{Version: 1, ID: "first", Method: "capture.unified"})
	if reply.Error == nil || reply.Error.Code != "module_unavailable" || reply.Error.Message != "fixture UI thread hung" {
		t.Fatalf("unexpected reply: %+v", reply.Error)
	}
	first := lazy.process
	select {
	case <-first.exited:
	case <-time.After(5 * time.Second):
		t.Fatal("unresponsive worker was not terminated")
	}
	if lazy.Running() {
		t.Fatal("terminated worker still reported running")
	}
	reply = lazy.Handle(host.Request{Version: 1, ID: "second", Method: "capture.unified"})
	if reply.Error == nil || reply.Error.Code != "module_unavailable" {
		t.Fatalf("unexpected second reply: %+v", reply.Error)
	}
	if lazy.process == first || lazy.process.pid == 0 || lazy.process.pid == first.pid {
		t.Fatal("next command did not start a fresh worker")
	}
}
