//go:build windows

package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"winone/internal/resultwindow"
)

func TestSelectionHandoffRequiresItsOwnedReadySession(t *testing.T) {
	process := &resultProcess{cmd: &exec.Cmd{Process: &os.Process{Pid: 42}}}
	windows := &resultWindows{sessions: map[string]*resultSession{"selection": {source: "selection", process: process, attached: true}, "quick": {source: "quick-assistant", process: process, attached: true}}}
	valid := resultwindow.SelectionHandoff{Session: "selection", Text: "selected content"}
	if err := windows.validateSelectionHandoff(valid, 42); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		input resultwindow.SelectionHandoff
		pid   uint32
	}{{valid, 43}, {resultwindow.SelectionHandoff{Session: "quick", Text: "text"}, 42}, {resultwindow.SelectionHandoff{Session: "missing", Text: "text"}, 42}, {resultwindow.SelectionHandoff{Session: "selection", Text: " "}, 42}, {resultwindow.SelectionHandoff{Session: "selection", Text: strings.Repeat("x", 65537)}, 42}} {
		if windows.validateSelectionHandoff(test.input, test.pid) == nil {
			t.Fatal("invalid handoff accepted", test.pid)
		}
	}
	windows.sessions["selection"].attached = false
	if windows.validateSelectionHandoff(valid, 42) == nil {
		t.Fatal("unready selection accepted")
	}
}
