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
	"winone/internal/resultwindow"
)

// A collected OCR result is independent of the selection assistant and never
// starts another recognition; its supplied text reaches the actual renderer pipe.
func TestCollectedTextOpensSharedResultWithoutSelectionModule(t *testing.T) {
	writer, reader := net.Pipe()
	defer writer.Close()
	defer reader.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	process := &resultProcess{input: writer, done: make(chan struct{}), cmd: &exec.Cmd{Process: &os.Process{Pid: 42}}}
	s := &resultWindows{ctx: ctx, process: process, sessions: map[string]*resultSession{}, ai: newAIService(ctx, t.TempDir(), "WinOne.Host.exe"), appearance: func() (string, string) { return "light", "zh-CN" }}
	done := make(chan host.Response, 1)
	go func() {
		done <- s.handle(host.Request{ID: "request", Method: "openText", PeerPID: 7, Params: json.RawMessage(`{"id":"0123456789abcdef0123456789abcdef","text":"第一段\n第二段"}`)})
	}()
	_ = reader.SetReadDeadline(time.Now().Add(3 * time.Second))
	var command resultwindow.Command
	data, err := protocol.ReadFrame(reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &command); err != nil {
		t.Fatal(err)
	}
	i := command.Initial
	if i == nil || i.Source != "capture-collection" || i.Mode != "ocr" || i.Text != "第一段\n第二段" || i.Toolbar {
		t.Fatalf("invalid collection handoff: %+v", i)
	}
	params, _ := json.Marshal(map[string]string{"id": i.ID, "token": i.Token, "window": "123"})
	if got := s.handle(host.Request{ID: "ready", Method: "ready", PeerPID: 42, Params: params}); got.Error != nil {
		t.Fatal(got.Error)
	}
	select {
	case got := <-done:
		if got.Error != nil {
			t.Fatal(got.Error)
		}
	case <-time.After(time.Second):
		t.Fatal("collection window did not finish opening")
	}
	s.remove(i.ID)
	if s.active() {
		t.Fatal("collection result retained session")
	}
}
