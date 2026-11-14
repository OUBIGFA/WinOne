//go:build windows

package ai

import (
	"context"
	"testing"
	"time"
)

// Inspect the callback's real context rather than waiting for a 60-minute
// drawing. Credential transport must preserve the caller's deadline; the
// upstream gateway owns the default wait and request scheduling.
func TestKeySnapshotPreservesCallerDeadline(t *testing.T) {
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
		if deadline, ok := call.Deadline(); ok {
			t.Errorf("credential transport added its own deadline: %v", deadline)
		}
		return false, nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
