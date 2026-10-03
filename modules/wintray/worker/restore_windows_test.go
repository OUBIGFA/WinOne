//go:build windows

package worker

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestHostedRestoreWaitsUntilConfirmed(t *testing.T) {
	calls := 0
	err := waitForHostedRestore(context.Background(), time.Second, func() error {
		calls++
		if calls < 3 {
			return errors.New("pending")
		}
		return nil
	})
	if err != nil || calls != 3 {
		t.Fatalf("restore finished early: calls=%d error=%v", calls, err)
	}
}
func TestHostedRestoreTimeoutAndCancellationRemainFailures(t *testing.T) {
	failure := errors.New("not visible")
	if err := waitForHostedRestore(context.Background(), 10*time.Millisecond, func() error { return failure }); !errors.Is(err, failure) {
		t.Fatal("timeout was reported successful", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := waitForHostedRestore(ctx, time.Second, func() error { cancel(); return failure }); !errors.Is(err, context.Canceled) || !errors.Is(err, failure) {
		t.Fatal("cancellation lost restoration failure", err)
	}
}
