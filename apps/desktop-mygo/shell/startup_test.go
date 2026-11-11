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

// The root's plugin layout also sends Host requests. It must share the first
// snapshot boundary with ordinary pages, rather than report a missing pipe
// while the shell is still starting the real Host.
func TestPluginRequestsWaitForInitialHostSnapshot(t *testing.T) {
	var ready atomic.Bool
	var early atomic.Bool
	h := testhost.New(t, func(method string, _ json.RawMessage) (any, error) {
		if !ready.Load() {
			early.Store(true)
		}
		if method == "chat.ghosts.pages" {
			return []any{}, nil
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
	if len(h.Called("chat.ghosts.pages")) == 0 {
		t.Fatal("plugin declarations were not loaded after Host startup")
	}
}

func TestInitialHostFailureStillOffersReconnect(t *testing.T) {
	defer i18n.SetLocale(i18n.Chinese)
	i18n.SetLocale(i18n.English)
	h := testhost.New(t, func(method string, _ json.RawMessage) (any, error) {
		if method == "chat.ghosts.pages" {
			return []any{}, nil
		}
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
