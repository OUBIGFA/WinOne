//go:build windows

package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCaptureWindowWaitRequiresMatchingTerminalState(t *testing.T) {
	for _, status := range []captureWindowStatus{{ID: "other", State: "completed"}, {ID: "window", State: "unknown"}, {ID: "window", State: "failed", Error: "crashed"}} {
		if err := waitCaptureWindow(context.Background(), "window", func() (captureWindowStatus, error) { return status, nil }); err == nil {
			t.Fatal(status)
		}
	}
	reads := 0
	err := waitCaptureWindow(context.Background(), "window", func() (captureWindowStatus, error) {
		reads++
		state := "running"
		if reads == 2 {
			state = "completed"
		}
		return captureWindowStatus{ID: "window", State: state}, nil
	})
	if err != nil || reads != 2 {
		t.Fatal(reads, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := waitCaptureWindow(ctx, "window", func() (captureWindowStatus, error) { return captureWindowStatus{ID: "window", State: "running"}, nil }); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}
