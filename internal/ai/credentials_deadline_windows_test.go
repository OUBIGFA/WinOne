//go:build windows

package ai

import (
	"context"
	"testing"
	"time"
)

// Inspect the callback's real context rather than waiting for a 60-minute
// drawing. The outer deadline already bounds the entire key failover chain.
func TestKeyFailoverPreservesExplicitRequestDeadline(t *testing.T) {
	store, _ := multiKeyStore(t)
	_, plan, err := store.ForTaskKeys("chat", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, budget := range []time.Duration{30 * time.Second, 61 * time.Minute} {
		deadline := time.Now().Add(budget)
		ctx, cancel := context.WithDeadline(context.Background(), deadline)
		err := store.WithKeys(ctx, plan, func(call context.Context, _ string) (bool, error) {
			actual, ok := call.Deadline()
			if !ok || !actual.Equal(deadline) {
				t.Errorf("request deadline changed: %v, want %v", actual, deadline)
			}
			return false, nil
		})
		cancel()
		if err != nil {
			t.Fatal(err)
		}
	}
	err = store.WithKeys(context.Background(), plan, func(call context.Context, _ string) (bool, error) {
		deadline, ok := call.Deadline()
		if left := time.Until(deadline); !ok || left > 90*time.Second || left < 89*time.Second {
			t.Errorf("unbounded caller lost the default: %v", left)
		}
		return false, nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
