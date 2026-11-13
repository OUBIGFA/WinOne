//go:build windows

package main

import (
	"context"
	"time"
)

// Keep the enabled capture/OCR path ready during ordinary long idle periods,
// not only at login or unlock. One timer covers both existing workers. It is
// disarmed while capture is disabled and never queues missed ticks after sleep.
type captureReadiness struct {
	changed chan struct{}
	done    chan struct{}
}

func newCaptureReadiness(ctx context.Context, interval time.Duration, enabled func() bool, prepare func(context.Context)) *captureReadiness {
	r := &captureReadiness{changed: make(chan struct{}, 1), done: make(chan struct{})}
	go func() {
		defer close(r.done)
		timer := time.NewTimer(interval)
		timer.Stop()
		defer timer.Stop()
		var tick <-chan time.Time
		for {
			select {
			case <-ctx.Done():
				return
			case <-r.changed:
				if !enabled() {
					timer.Stop()
					tick = nil
				} else if tick == nil {
					timer.Reset(interval)
					tick = timer.C
				}
			case <-tick:
				if ctx.Err() != nil {
					return
				}
				if !enabled() {
					tick = nil
					continue
				}
				prepare(ctx)
				timer.Reset(interval)
			}
		}
	}()
	return r
}

func (r *captureReadiness) Refresh() {
	select {
	case r.changed <- struct{}{}:
	default:
	}
}
