//go:build windows

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"testing"
	"winone/internal/host"
	"winone/internal/learning"
)

func TestResultLearningStateRequiresOwnedWindowAndSurvivesHostRestart(t *testing.T) {
	dir := t.TempDir()
	newHost := func() *resultWindows {
		return &resultWindows{logDirectory: dir, sessions: map[string]*resultSession{"study": {attached: true, process: &resultProcess{cmd: &exec.Cmd{Process: &os.Process{Pid: 123}}}}}}
	}
	s := newHost()
	call := func(pid uint32, method, term, state string) host.Response {
		data, _ := json.Marshal(map[string]string{"id": "study", "term": term, "learningState": state})
		return s.handle(host.Request{ID: "learning", Method: method, PeerPID: pid, Params: data})
	}
	if call(124, "learning.set", "world", "ignored").Error == nil {
		t.Fatal("unrelated process changed learning state")
	}
	if r := call(123, "learning.set", "world", "ignored"); r.Error != nil {
		t.Fatal(r.Error)
	}
	s = newHost()
	r := call(123, "learning.get", "", "")
	if r.Error != nil {
		t.Fatal(r.Error)
	}
	var states learning.TermStates
	if err := json.Unmarshal(r.Result, &states); err != nil || states.State("World") != learning.Ignored {
		t.Fatal(states, err)
	}
	if r = call(123, "learning.set", "world", ""); r.Error != nil {
		t.Fatal(r.Error)
	}
	delete(s.sessions, "study")
	if call(123, "learning.get", "", "").Error == nil {
		t.Fatal("closed window read learning state")
	}
}
