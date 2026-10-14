//go:build windows

package main

import (
	"encoding/json"
	"testing"
	"winone/internal/host"
	"winone/internal/selection"
)

func TestResultSelectionReplyIsBoundToWindowProcessAndGesture(t *testing.T) {
	read := &resultSelectionRead{session: "source", pid: 123, reply: make(chan selection.Selection, 1)}
	results := &resultWindows{sessions: map[string]*resultSession{"source": {}}, selectionReads: map[string]*resultSelectionRead{"gesture": read}}
	reply := func(pid uint32, id, request string) host.Response {
		data, _ := json.Marshal(map[string]string{"id": id, "request": request, "text": "selected content"})
		return results.acceptSelection(host.Request{ID: "test", PeerPID: pid, Params: data})
	}
	for _, sample := range []struct {
		pid         uint32
		id, gesture string
	}{{124, "source", "gesture"}, {123, "other", "gesture"}, {123, "source", "old"}} {
		if reply(sample.pid, sample.id, sample.gesture).Error == nil {
			t.Fatal("unrelated reply accepted")
		}
	}
	if reply(123, "source", "gesture").Error != nil {
		t.Fatal("owned reply rejected")
	}
	if reply(123, "source", "gesture").Error == nil {
		t.Fatal("duplicate reply accepted")
	}
	if got := <-read.reply; got.Status != "selected" || got.Text != "selected content" {
		t.Fatal(got)
	}
	delete(results.selectionReads, "gesture")
	if reply(123, "source", "gesture").Error == nil {
		t.Fatal("expired reply accepted")
	}
}
