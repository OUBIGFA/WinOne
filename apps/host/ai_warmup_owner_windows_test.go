//go:build windows

package main

import (
	"encoding/json"
	"math"
	"path/filepath"
	"strings"
	"testing"

	"winone/internal/ai"
	"winone/internal/host"
)

// A departed caller must not start an ownerless WarmAll. Pinning fails
// before an account worker runs, and the provider gate is released.
func TestWarmAllRefusesExitedCallerBeforeStartingWorker(t *testing.T) {
	s := newAIService(t.Context(), t.TempDir(), "unused.exe")
	store, err := ai.Open(filepath.Join(t.TempDir(), "ai.json"))
	if err != nil {
		t.Fatal(err)
	}
	s.store = store
	c := store.Configuration()
	c.Providers = []ai.Provider{{ID: "private-warm", Name: "Private", AuthMode: "account", Agent: "codex", Models: []string{"model"}}}
	if _, err := store.SaveConfiguration(c, nil); err != nil {
		t.Fatal(err)
	}
	params, _ := json.Marshal(map[string]string{"provider": "private-warm", "agent": "codex"})
	r := s.handleAccounts(host.Request{ID: "warm", Method: "accounts.warm-all", PeerPID: math.MaxUint32, Params: params})
	if r.Error == nil || strings.Contains(r.Error.Message, "预热未能完成") {
		t.Fatalf("departed owner reached a worker: %+v", r)
	}
	if len(s.accountGateFor("private-warm")) != 0 {
		t.Fatal("departed owner's request retained the account gate")
	}
}
