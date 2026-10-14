//go:build windows

package traybox

import (
	"context"
	"errors"
	"time"
)

// Observation never emits input. A deadline is inconclusive, not evidence
// that a slow application requires a different click gesture.
func observeOpen(ctx context.Context, before []openWindowState, pid uint32, read func() ([]openWindowState, error), interval time.Duration) error {
	if pid == 0 || interval <= 0 {
		return errors.New("invalid open observation")
	}
	timer := time.NewTicker(interval)
	defer timer.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		after, err := read()
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if observedOpen(before, after, pid) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
}
