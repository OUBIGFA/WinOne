//go:build windows

package main

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
	"winone/internal/ai"
)

func TestModelRefreshOnlyStartsForEnabledSavedProviders(t *testing.T) {
	var calls atomic.Int32
	m := &aiModelRefresh{ctx: context.Background(),
		snapshot: func() ([]ai.Provider, error) {
			return []ai.Provider{{ID: "off", AuthMode: "api-key", ProviderPreferences: ai.ProviderPreferences{Off: true, RefreshEvery: "1h"}}, {ID: "manual", AuthMode: "api-key"}}, nil
		}, run: func(context.Context, string) error { calls.Add(1); return nil }}
	m.Update()
	m.Close()
	if calls.Load() != 0 || m.done != nil {
		t.Fatal("disabled/manual providers started a background refresh")
	}
}

func TestModelRefreshSaveCancelsAndJoinsPreviousRequest(t *testing.T) {
	started, stopped := make(chan string, 2), make(chan string, 2)
	var active, maximum atomic.Int32
	provider := ai.Provider{ID: "first", AuthMode: "api-key", ProviderPreferences: ai.ProviderPreferences{RefreshEvery: "1h"}}
	m := &aiModelRefresh{ctx: context.Background(),
		snapshot: func() ([]ai.Provider, error) { return []ai.Provider{provider}, nil },
		run: func(ctx context.Context, id string) error {
			current := active.Add(1)
			if current > maximum.Load() {
				maximum.Store(current)
			}
			started <- id
			<-ctx.Done()
			active.Add(-1)
			stopped <- id
			return ctx.Err()
		}}
	t.Cleanup(m.Close)
	wait := func(ch <-chan string, want string) {
		t.Helper()
		select {
		case got := <-ch:
			if got != want {
				t.Fatalf("got %q, want %q", got, want)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("refresh %q did not finish its lifecycle", want)
		}
	}
	m.Update()
	wait(started, "first")
	provider.ID = "second"
	m.Update()
	wait(stopped, "first")
	wait(started, "second")
	m.Close()
	wait(stopped, "second")
	if active.Load() != 0 || maximum.Load() != 1 {
		t.Fatal("saved refreshes overlapped or survived close")
	}
}
