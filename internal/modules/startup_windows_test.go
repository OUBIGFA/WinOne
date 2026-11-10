//go:build windows

package modules

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"winone/internal/host"
	"winone/internal/transport"
)

// Re-execute this test binary as an isolated protocol worker. A real pipe and
// serialized snapshot reproduce expensive status reads without touching windows.
func init() {
	mode := os.Getenv("WINONE_STARTUP_FIXTURE")
	if mode != "slow-snapshot" && mode != "reject-snapshot" && mode != "exit-snapshot" {
		return
	}
	var pipe, directory string
	for i := 1; i+1 < len(os.Args); i++ {
		if os.Args[i] == "--pipe" {
			pipe = os.Args[i+1]
		}
		if os.Args[i] == "--data-dir" {
			directory = os.Args[i+1]
		}
	}
	listener, err := transport.Listen(pipe)
	if err != nil {
		os.Exit(2)
	}
	ctx, cancel := context.WithCancel(context.Background())
	worker := &startupFixture{cancel: cancel, directory: directory, mode: mode}
	if transport.Serve(ctx, listener, worker) != nil {
		os.Exit(3)
	}
	os.Exit(0)
}

type startupFixture struct {
	mu        sync.Mutex
	cancel    context.CancelFunc
	snapshots int
	directory string
	mode      string
}

func (f *startupFixture) Handle(request host.Request) host.Response {
	f.mu.Lock()
	defer f.mu.Unlock()
	if request.Method == "snapshot" {
		if f.mode == "reject-snapshot" {
			return host.Response{ID: request.ID, Error: &host.RPCError{Code: "fixture", Message: "configuration rejected"}}
		}
		if f.mode == "exit-snapshot" {
			os.Exit(7)
		}
		f.snapshots++
		if err := os.WriteFile(filepath.Join(f.directory, "entered"), nil, 0600); err != nil {
			os.Exit(4)
		}
		time.Sleep(350 * time.Millisecond)
	}
	if request.Method == "shutdown" {
		f.cancel()
	}
	data, _ := json.Marshal(map[string]any{"processId": os.Getpid(), "snapshots": f.snapshots})
	return host.Response{ID: request.ID, Result: data}
}

func TestStartupPreservesWorkerFailureWithoutRepeatedProbes(t *testing.T) {
	for _, mode := range []string{"reject-snapshot", "exit-snapshot"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("WINONE_STARTUP_FIXTURE", mode)
			pipe := fmt.Sprintf(`\\.\pipe\WinOne-startup-failure-%d-%d`, os.Getpid(), time.Now().UnixNano())
			begin := time.Now()
			p := Start(context.Background(), "example.tool", "Example", os.Args[0], pipe, t.TempDir(), false)
			t.Cleanup(func() {
				if p.cmd != nil {
					_ = p.cmd.Process.Kill()
					<-p.exited
				}
			})
			status := p.Status()
			if status.State != "faulted" || status.Detail == "模块启动超时" {
				t.Fatalf("worker failure was lost: %+v", status)
			}
			if mode == "reject-snapshot" && !strings.Contains(status.Detail, "configuration rejected") {
				t.Fatalf("worker reason was lost: %+v", status)
			}
			if elapsed := time.Since(begin); elapsed > 2*time.Second {
				t.Fatalf("worker failure retried for %v", elapsed)
			}
		})
	}
}

func TestStartupWaitsForOneSnapshotInsteadOfQueuingAbandonedProbes(t *testing.T) {
	t.Setenv("WINONE_STARTUP_FIXTURE", "slow-snapshot")
	pipe := fmt.Sprintf(`\\.\pipe\WinOne-startup-test-%d-%d`, os.Getpid(), time.Now().UnixNano())
	begin := time.Now()
	p := Start(context.Background(), "example.tool", "Example", os.Args[0], pipe, t.TempDir(), false)
	t.Cleanup(func() {
		if p.cmd != nil {
			_ = p.cmd.Process.Kill()
			<-p.exited
		}
	})
	if status := p.Status(); status.State != "ready" {
		t.Fatalf("a 350ms snapshot was reported as a startup failure after %v: %+v", time.Since(begin), status)
	}
	response := p.Handle(host.Request{Version: 1, ID: "action", Method: "action"})
	var got struct {
		Snapshots int `json:"snapshots"`
	}
	if response.Error != nil || json.Unmarshal(response.Result, &got) != nil || got.Snapshots != 1 {
		t.Fatalf("action waited behind abandoned readiness requests: %+v, %s", response.Error, response.Result)
	}
	if elapsed := time.Since(begin); elapsed > 2*time.Second {
		t.Fatalf("startup and first action took %v", elapsed)
	}
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestFailedStartupDoesNotDispatchOriginalAction(t *testing.T) {
	t.Setenv("WINONE_STARTUP_FIXTURE", "slow-snapshot")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pipe := fmt.Sprintf(`\\.\pipe\WinOne-startup-cancel-%d-%d`, os.Getpid(), time.Now().UnixNano())
	directory := t.TempDir()
	lazy := NewLazy(ctx, "example.tool", "Example", os.Args[0], pipe, directory)
	done := make(chan host.Response, 1)
	go func() { done <- lazy.Handle(host.Request{Version: 1, ID: "action", Method: "action"}) }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(directory, "entered")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			<-done
			t.Fatal("worker did not receive readiness request")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	response := <-done
	t.Cleanup(func() {
		if p := lazy.process; p != nil && p.cmd != nil {
			_ = p.cmd.Process.Kill()
			<-p.exited
		}
	})
	if response.Error == nil || response.Error.Message != context.Canceled.Error() {
		t.Fatalf("cancelled startup still dispatched its original action: %+v, %s", response.Error, response.Result)
	}
}
