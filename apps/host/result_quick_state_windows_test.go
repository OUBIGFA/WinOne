//go:build windows

package main

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"testing"
	"time"
	"winone/internal/host"
	"winone/internal/protocol"
)

func TestQuickCloseAcknowledgesWithoutRetainingDraft(t *testing.T) {
	writer, reader := net.Pipe()
	defer writer.Close()
	defer reader.Close()
	process := &resultProcess{input: writer, cmd: &exec.Cmd{Process: &os.Process{Pid: 42}}, done: make(chan struct{})}
	entry := &resultSession{id: "quick", source: "quick-assistant", process: process, window: "123", cancelled: make(chan struct{})}
	s := &resultWindows{ctx: context.Background(), sessions: map[string]*resultSession{"quick": entry}, ai: newAIService(context.Background(), t.TempDir(), "WinOne.Host.exe")}
	quickCancelled := false
	s.ai.jobs["quick-job"] = &aiJob{session: "quick", owner: 42, Text: "private quick reply", cancel: func() { quickCancelled = true }}
	s.ai.jobs["other-chat"] = &aiJob{session: "other", owner: 42, Text: "unrelated chat", cancel: func() { t.Error("unrelated conversation was cancelled") }}
	done := make(chan error, 1)
	go func() { done <- s.closeSource("quick-assistant") }()
	if _, err := protocol.ReadFrame(reader); err != nil {
		t.Fatal(err)
	}
	call := func(method string, pid uint32) host.Response {
		return s.handle(host.Request{ID: "test", Method: method, PeerPID: pid, Params: json.RawMessage(`{"id":"quick","state":{"Version":1,"Prompt":"unfinished draft"}}`)})
	}
	if call("remember", 43).Error == nil {
		t.Fatal("another process retained a draft")
	}
	if response := call("remember", 42); response.Error == nil {
		t.Fatal("closed Quick content was accepted for restoration")
	}
	select {
	case <-done:
		t.Fatal("close completed before the window acknowledged it")
	default:
	}
	if call("closed", 42).Error != nil {
		t.Fatal("owned close failed")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("acknowledged close did not return")
	}
	if !quickCancelled || s.ai.jobs["quick-job"] != nil || s.ai.jobs["other-chat"] == nil {
		t.Fatal("Quick close failed to isolate job cleanup")
	}
}
