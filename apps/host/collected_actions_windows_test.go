//go:build windows

package main

import (
	"encoding/json"
	"testing"
	"winone/internal/host"
)

type collectedActionFixture struct {
	response host.Response
	request  host.Request
}

func (m *collectedActionFixture) Status() host.Module { return host.Module{State: "ready"} }
func (m *collectedActionFixture) Enabled() bool       { return true }
func (m *collectedActionFixture) Handle(r host.Request) host.Response {
	m.request = r
	return m.response
}

func TestCollectedActionRequiresDeliveryConfirmation(t *testing.T) {
	for _, body := range []string{`{}`, `null`, `{"delivered":false}`} {
		worker := &collectedActionFixture{response: host.Response{Result: json.RawMessage(body)}}
		if err := performCollectedAction(worker, "item", true, collectedAction{ID: "open"}); err == nil {
			t.Errorf("unconfirmed action accepted: %s", body)
		}
	}
	worker := &collectedActionFixture{response: host.Response{Result: json.RawMessage(`{"delivered":true}`)}}
	if err := performCollectedAction(worker, "item", true, collectedAction{ID: "hide"}); err != nil {
		t.Fatal(err)
	}
	var params struct {
		ID, Action string
		Right      bool
	}
	if err := json.Unmarshal(worker.request.Params, &params); err != nil {
		t.Fatal(err)
	}
	if params.ID != "item" || params.Action != "hide" || params.Right {
		t.Fatal(params)
	}
}

func TestHostedMenuActionsRemainOwnedByWinOne(t *testing.T) {
	for _, locale := range []string{"zh-CN", "en-US"} {
		hosted := collectedActions(true, locale)
		if len(hosted) != 4 {
			t.Fatal(hosted)
		}
		for i, id := range []string{"open", "hide", "release", "quit"} {
			if hosted[i].ID != id || hosted[i].Right || hosted[i].Title == "" {
				t.Fatal(hosted)
			}
		}
		native := collectedActions(false, locale)
		if len(native) != 2 || native[0].ID != "open" || native[1].ID != "menu" || !native[1].Right {
			t.Fatal(native)
		}
	}
}

func TestCollectedDoubleClickReachesWorker(t *testing.T) {
	worker := &collectedActionFixture{response: host.Response{Result: json.RawMessage(`{"delivered":true}`)}}
	if err := performCollectedAction(worker, "native-icon", false, collectedAction{ID: "double"}); err != nil {
		t.Fatal(err)
	}
	var params struct {
		Action string
		Right  bool
	}
	if err := json.Unmarshal(worker.request.Params, &params); err != nil {
		t.Fatal(err)
	}
	if params.Action != "double" || params.Right {
		t.Fatal("Double-click was reduced to a single click", params)
	}
}

func TestCollectedGesturePreservesSingleHostedAndRightActions(t *testing.T) {
	for _, tc := range []struct {
		id            string
		hosted, shift bool
		want          string
	}{
		{"open", false, false, "open"}, {"open", false, true, "open"},
		{"open", true, true, "open"}, {"menu", false, true, "menu"},
	} {
		if got := collectedGesture(collectedAction{ID: tc.id}, tc.hosted, tc.shift); got.ID != tc.want {
			t.Fatal(tc, got)
		}
	}
}
