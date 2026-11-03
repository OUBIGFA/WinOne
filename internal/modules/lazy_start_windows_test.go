//go:build windows

package modules

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"winone/internal/host"
)

// A worker that never answers: this test binary, started again as the
// module's executable, sleeps and exits.
func init() {
	if os.Getenv("WINONE_LAZY_SLOW_WORKER") == "1" {
		time.Sleep(3 * time.Second)
		os.Exit(0)
	}
}

// The Host's snapshot reads every module's status. A worker launch waits up
// to eight seconds for the worker; reading the status must not wait with it,
// or the settings window's first snapshot times out ("i/o timeout").
func TestStatusAnswersWhileAWorkerStarts(t *testing.T) {
	t.Setenv("WINONE_LAZY_SLOW_WORKER", "1")
	pipe := fmt.Sprintf(`\\.\pipe\WinOne-lazy-test-%d-%d`, os.Getpid(), time.Now().UnixNano())
	lazy := NewLazy(context.Background(), "example.tool", "Example", os.Args[0], pipe, t.TempDir())
	handled := make(chan host.Response, 1)
	go func() { handled <- lazy.Handle(host.Request{Version: 1, ID: "first", Method: "snapshot"}) }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		begin := time.Now()
		status := lazy.Status()
		if took := time.Since(begin); took > 200*time.Millisecond {
			t.Fatalf("status waited %v for the launch", took)
		}
		if status.State == "starting" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("status never reported the launch: %+v", status)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !lazy.Running() {
		t.Fatal("a starting worker was reported as not running")
	}
	select {
	case <-handled:
	case <-time.After(15 * time.Second):
		t.Fatal("the request did not end with the launch")
	}
	if err := lazy.Close(); err != nil {
		t.Fatal(err)
	}
}
