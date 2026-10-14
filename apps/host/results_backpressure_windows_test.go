//go:build windows

package main

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"
	"winone/internal/host"
	"winone/internal/resultwindow"
)

type observedResultConnection struct {
	net.Conn
	started chan struct{}
	once    sync.Once
}

func (c *observedResultConnection) Write(data []byte) (int, error) {
	c.once.Do(func() { close(c.started) })
	return c.Conn.Write(data)
}

// A renderer that stops reading must not hold the Host's session lock. This
// reproduces the live chain: stalled write -> open -> selection stop -> Quit.
func TestBlockedResultOpenDoesNotBlockSessionStatus(t *testing.T) {
	writer, reader := net.Pipe()
	defer reader.Close()
	defer writer.Close()
	connection := &observedResultConnection{Conn: writer, started: make(chan struct{})}
	process := &resultProcess{input: connection, done: make(chan struct{}), cmd: &exec.Cmd{Process: &os.Process{Pid: 42}}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := &resultWindows{ctx: ctx, process: process, quickAssistantAllowed: true,
		sessions: map[string]*resultSession{}, ai: newAIService(ctx, t.TempDir(), "WinOne.Host.exe"),
		appearance: func() (string, string) { return "system", "zh-CN" }}
	opened := make(chan error, 1)
	go func() {
		opened <- s.openSource("0123456789abcdef0123456789abcdef", "chat", nil, "", [2]int32{}, 42, nil)
	}()
	select {
	case <-connection.started:
	case <-time.After(time.Second):
		t.Fatal("open did not reach the renderer")
	}
	status := make(chan host.Response, 1)
	go func() {
		status <- s.handle(host.Request{ID: "status", Method: "status", Params: json.RawMessage(`{"id":"missing"}`)})
	}()
	select {
	case response := <-status:
		if response.Error == nil {
			t.Error("missing session unexpectedly accepted")
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("a stalled renderer blocked the Host's independent status request")
	}
	// Release the real pipe even on failure; never leave a blocked test goroutine.
	reader.Close()
	select {
	case err := <-opened:
		if err == nil {
			t.Error("open succeeded without a renderer acknowledgement")
		}
	case <-time.After(time.Second):
		t.Fatal("open did not return after the pipe closed")
	}
}

func TestResultWriteTimesOutAndRejectsReuseOfTruncatedStream(t *testing.T) {
	writer, reader := net.Pipe()
	defer writer.Close()
	defer reader.Close()
	p := &resultProcess{input: writer, done: make(chan struct{})}
	done := make(chan error, 1)
	go func() { done <- p.send(resultwindow.Command{Kind: "quit"}) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("an unread command was acknowledged")
		}
	case <-time.After(3 * time.Second):
		reader.Close()
		<-done
		t.Fatal("renderer write had no deadline")
	}
	started := time.Now()
	if err := p.send(resultwindow.Command{Kind: "activate"}); err == nil {
		t.Fatal("failed stream was reused")
	}
	if time.Since(started) > 250*time.Millisecond {
		t.Fatal("a known failed renderer made the next action wait again")
	}
}
