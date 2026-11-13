//go:build windows

package main

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestCaptureReadinessMaintainsOnlyEnabledIdleService(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var enabled atomic.Bool
	calls := make(chan struct{}, 8)
	r := newCaptureReadiness(ctx, 15*time.Millisecond, enabled.Load, func(context.Context) { calls <- struct{}{} })
	r.Refresh()
	select {
	case <-calls:
		t.Fatal("disabled capture scheduled preparation")
	case <-time.After(50 * time.Millisecond):
	}
	enabled.Store(true)
	r.Refresh()
	for i := 0; i < 2; i++ {
		select {
		case <-calls:
		case <-time.After(time.Second):
			t.Fatal("enabled capture was not maintained without a user request")
		}
	}
	enabled.Store(false)
	r.Refresh()
	select {
	case <-calls:
		t.Fatal("disabled capture kept preparing")
	case <-time.After(50 * time.Millisecond):
	}
	enabled.Store(true)
	r.Refresh()
	select {
	case <-calls:
	case <-time.After(time.Second):
		t.Fatal("reenabled capture did not resume maintenance")
	}
	cancel()
	select {
	case <-r.done:
	case <-time.After(time.Second):
		t.Fatal("Host exit retained readiness maintenance")
	}
}

func TestCaptureReadinessDoesNotOverlapOrQueueMissedTicks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered := make(chan struct{}, 4)
	release := make(chan struct{})
	r := newCaptureReadiness(ctx, 20*time.Millisecond, func() bool { return true }, func(ctx context.Context) {
		entered <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
		}
	})
	r.Refresh()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("preparation did not start")
	}
	select {
	case <-entered:
		t.Fatal("slow preparation overlapped another tick")
	case <-time.After(70 * time.Millisecond):
	}
	close(release)
	select {
	case <-entered:
		t.Fatal("missed ticks ran immediately after preparation")
	case <-time.After(5 * time.Millisecond):
	}
	cancel()
	<-r.done
}
