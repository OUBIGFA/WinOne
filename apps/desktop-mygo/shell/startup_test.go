package shell

import (
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/testhost"
	internalhost "winone/internal/host"
)

// Initial rendering waits for the Host snapshot and never loads retired pages.
func TestPageRequestsWaitForInitialHostSnapshot(t *testing.T) {
	var ready atomic.Bool
	var early atomic.Bool
	h := testhost.New(t, func(method string, _ json.RawMessage) (any, error) {
		if !ready.Load() {
			early.Store(true)
		}
		return map[string]any{}, nil
	})
	s := New(h.Env, &feedbackWindow{}, Options{Directory: h.Env.Directory, EnsureHost: func() (internalhost.Snapshot, error) {
		ready.Store(true)
		return internalhost.Snapshot{}, nil
	}})
	t.Cleanup(s.Discarded)
	tt := ui.NewTester(s.View, 1000, 800)
	h.Settle(tt)
	if calls := h.Calls(); early.Load() || len(calls) != 0 {
		t.Fatalf("the root requested the Host before its initial snapshot: %+v", calls)
	}
	s.Start()
	h.Settle(tt)
	if !s.loaded || early.Load() {
		t.Fatal("startup did not complete at the Host snapshot boundary")
	}
}

func TestInitialHostFailureStillOffersReconnect(t *testing.T) {
	defer i18n.SetLocale(i18n.Chinese)
	i18n.SetLocale(i18n.English)
	h := testhost.New(t, func(method string, _ json.RawMessage) (any, error) {
		return map[string]any{}, nil
	})
	s := New(h.Env, &feedbackWindow{}, Options{Directory: h.Env.Directory, EnsureHost: func() (internalhost.Snapshot, error) {
		return internalhost.Snapshot{}, errors.New("Host could not start")
	}})
	t.Cleanup(s.Discarded)
	tt := ui.NewTester(s.View, 1000, 800)
	s.Start()
	h.Settle(tt)
	if !tt.HasText("Host could not start") || !tt.HasText(i18n.T("common.reconnect")) {
		t.Fatal("a real startup failure lost its error or reconnect action")
	}
}
