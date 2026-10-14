//go:build windows

package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
	"winone/internal/ai"
	"winone/internal/host"
)

func queuedAccountService(t *testing.T, ctx context.Context) *aiService {
	t.Helper()
	s := newAIService(ctx, t.TempDir(), "missing-worker.exe")
	store, err := ai.Open(s.storePath)
	if err != nil {
		t.Fatal(err)
	}
	s.store = store
	config := store.Configuration()
	config.Providers = []ai.Provider{{ID: "account", Name: "Fixture account", AuthMode: "account", Agent: "codex", Models: []string{}}}
	if _, err = store.SaveConfiguration(config, nil); err != nil {
		t.Fatal(err)
	}
	return s
}

func quotaRequest() host.Request {
	return host.Request{ID: "quota", Method: "accounts.quota", Params: json.RawMessage(`{"provider":"account","agent":"codex"}`)}
}

// Token refresh and quota history can write the private account profile. A
// short read must wait for its current owner, never run a second writer or
// report "not signed in" merely because another read won the race.
func TestAccountQuotaWaitsForTheProfileOwner(t *testing.T) {
	s := queuedAccountService(t, context.Background())
	gate := s.accountGateFor("account")
	gate <- struct{}{}
	done := make(chan host.Response, 1)
	go func() { done <- s.handleAccounts(quotaRequest()) }()
	select {
	case response := <-done:
		<-gate
		t.Fatalf("quota did not wait for the in-flight account operation: %+v", response.Error)
	case <-time.After(50 * time.Millisecond):
	}
	<-gate
	select {
	case response := <-done:
		// After acquiring the gate the real worker startup is attempted. A
		// missing test executable is an actual failure, never a fabricated quota.
		if response.Error == nil || !strings.Contains(response.Error.Message, "账号服务无法启动") || len(response.Result) > 0 {
			t.Fatalf("worker failure was hidden: %+v", response)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("quota did not continue after the profile became available")
	}
	if len(gate) != 0 {
		t.Fatal("finished quota kept the account gate")
	}
}

func TestAccountQuotaCancelsItsWaitWithoutTakingTheProfile(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s := queuedAccountService(t, ctx)
	gate := s.accountGateFor("account")
	gate <- struct{}{}
	done := make(chan host.Response, 1)
	go func() { done <- s.handleAccounts(quotaRequest()) }()
	cancel()
	select {
	case response := <-done:
		if response.Error == nil || !strings.Contains(response.Error.Message, "已取消") {
			t.Fatalf("cancellation was misreported: %+v", response)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled quota kept waiting")
	}
	if len(gate) != 1 {
		t.Fatal("cancelled read changed the active profile owner")
	}
	<-gate
}
