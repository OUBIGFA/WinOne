//go:build windows

package traybox

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestObserveOpenWaitsForDelayedTarget(t *testing.T) {
	calls := 0
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := observeOpen(ctx, nil, 7, func() ([]openWindowState, error) {
		calls++
		if calls < 3 {
			return nil, nil
		}
		return []openWindowState{{handle: 1, pid: 7, visible: true}}, nil
	}, time.Millisecond)
	if err != nil || calls != 3 {
		t.Fatal(calls, err)
	}
}
func TestObserveOpenPreservesFailureAndCancellation(t *testing.T) {
	failure := errors.New("identity changed")
	if err := observeOpen(context.Background(), nil, 7, func() ([]openWindowState, error) { return nil, failure }, time.Millisecond); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	err := observeOpen(ctx, nil, 7, func() ([]openWindowState, error) { called = true; return nil, nil }, time.Millisecond)
	if !errors.Is(err, context.Canceled) || called {
		t.Fatal(called, err)
	}
}
func TestObserveOpenTimeoutDoesNotClaimSuccess(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	err := observeOpen(ctx, nil, 7, func() ([]openWindowState, error) { return []openWindowState{{handle: 1, pid: 9, visible: true}}, nil }, time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}
