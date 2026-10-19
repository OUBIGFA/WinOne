//go:build windows

package main

import (
	"os"
	"os/exec"
	"testing"
)

func TestQuickActivationTargetRequiresLiveAttachedOwnedResult(t *testing.T) {
	process := &resultProcess{cmd: &exec.Cmd{Process: &os.Process{Pid: 42}}, done: make(chan struct{})}
	entry := &resultSession{source: "quick-assistant", attached: true, process: process, window: "123"}
	s := &resultWindows{quickAssistantAllowed: true, sessions: map[string]*resultSession{"quick": entry}}
	target, err := s.quickAssistantTarget()
	if err != nil || target.Window != "123" || target.ProcessID != 42 {
		t.Fatalf("wrong activation target: %+v, %v", target, err)
	}
	for _, change := range []func(){
		func() { entry.attached = false },
		func() { entry.attached = true; entry.closing = true },
		func() { entry.closing = false; entry.window = "" },
		func() { entry.window = "123"; s.quickAssistantAllowed = false },
		func() { s.quickAssistantAllowed = true; close(process.done) },
	} {
		change()
		if _, err := s.quickAssistantTarget(); err == nil {
			t.Fatal("inactive/closing result was offered for activation")
		}
	}
}
