//go:build windows

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"testing"
	"winone/internal/host"
)

func TestComposerHintClaimRequiresAttachedResultOwner(t *testing.T) {
	calls := 0
	s := &resultWindows{sessions: map[string]*resultSession{"result": {attached: true, process: &resultProcess{cmd: &exec.Cmd{Process: &os.Process{Pid: 42}}}}}, claimComposerHint: func() (bool, error) { calls++; return calls == 1, nil }}
	request := func(pid uint32) host.Response {
		return s.handle(host.Request{ID: "hint", Method: "composerHint.claim", PeerPID: pid, Params: json.RawMessage(`{"id":"result"}`)})
	}
	if response := request(43); response.Error == nil || calls != 0 {
		t.Fatal("unowned window consumed machine hint")
	}
	for i, want := range []bool{true, false} {
		response := request(42)
		if response.Error != nil {
			t.Fatal(response.Error)
		}
		var result struct{ Show bool }
		if json.Unmarshal(response.Result, &result) != nil || result.Show != want {
			t.Fatalf("claim %d: %s", i, response.Result)
		}
	}
	s.sessions["result"].attached = false
	if request(42).Error == nil || calls != 2 {
		t.Fatal("unattached window consumed hint")
	}
}
