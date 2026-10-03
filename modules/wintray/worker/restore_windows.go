//go:build windows

package worker

import (
	"context"
	"errors"
	"time"
)

// Each attempt runs briefly on the UI thread. Waiting outside that thread lets
// target windows and process watchers finish their own restoration messages.
func (r *Runtime) restoreHostedWindows() error {
	return waitForHostedRestore(r.ctx, 2*time.Second, func() error { return r.onUI(r.hosted.RestoreAll) })
}

func waitForHostedRestore(ctx context.Context, timeout time.Duration, attempt func() error) error {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := attempt()
		if err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return errors.Join(err, ctx.Err())
		case <-deadline.C:
			return err
		case <-time.After(50 * time.Millisecond):
		}
	}
}
