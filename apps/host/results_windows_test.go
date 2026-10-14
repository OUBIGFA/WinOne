//go:build windows

package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"testing"
	"winone/internal/host"
)

func TestResultSessionOwnershipAndCloseCancellation(t *testing.T) {
	cancelled := false
	engine := newAIService(context.Background(), t.TempDir(), "WinOne.Host.exe")
	engine.jobs["job"] = &aiJob{owner: 42, session: "owned", cancel: func() { cancelled = true }}
	process := &resultProcess{cmd: &exec.Cmd{Process: &os.Process{Pid: 42}}}
	entry := &resultSession{id: "owned", token: "token", creator: 7, image: []byte("snapshot"), process: process, ready: make(chan struct{}), cancelled: make(chan struct{})}
	s := &resultWindows{sessions: map[string]*resultSession{"owned": entry}, ai: engine}
	call := func(method string, pid uint32, token string) host.Response {
		data, _ := json.Marshal(map[string]string{"id": "owned", "token": token})
		return s.handle(host.Request{ID: "test", Method: method, PeerPID: pid, Params: data})
	}
	if call("ready", 41, "token").Error == nil || call("ready", 42, "wrong").Error == nil {
		t.Fatal("foreign renderer attached")
	}
	if call("ready", 42, "token").Error != nil {
		t.Fatal("owned renderer could not attach")
	}
	if _, err := s.image("owned", 41); err == nil {
		t.Fatal("foreign renderer read image")
	}
	if image, err := s.image("owned", 42); err != nil || string(image) != "snapshot" {
		t.Fatal("owned image unavailable")
	}
	if call("closed", 41, "").Error == nil || cancelled {
		t.Fatal("foreign close affected request")
	}
	if call("closed", 42, "").Error != nil || !cancelled || s.active() {
		t.Fatal("closing window did not release its image and AI request")
	}
}

func TestCancellingUnstartedResultDoesNotLaunchOrPanic(t *testing.T) {
	entry := &resultSession{id: "pending", creator: 7, cancelled: make(chan struct{})}
	s := &resultWindows{sessions: map[string]*resultSession{"pending": entry}}
	response := s.handle(host.Request{ID: "cancel", Method: "cancel", PeerPID: 7, Params: json.RawMessage(`{"id":"pending"}`)})
	if response.Error != nil || s.active() {
		t.Fatal("pending result not cancelled")
	}
	select {
	case <-entry.cancelled:
	default:
		t.Fatal("open waiter not notified")
	}
}
