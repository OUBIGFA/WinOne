//go:build windows

package traybox

import (
	"context"
	"errors"
	"time"
)

// One user action gets one single click and, only without an observed opening,
// one double-click fallback. Never retry indefinitely or require a modifier.
func autoOpen(send func(Action) error, observe func() error) error {
	if err := send(ActionClick); err != nil {
		return err
	}
	err := observe()
	if err == nil {
		return nil
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return send(ActionDoubleClick)
}

func OpenAutomatically(ctx context.Context, icon Icon) error {
	return openAutomatically(ctx, icon, nil)
}

func openAutomatically(ctx context.Context, icon Icon, latest func() (Icon, error)) error {
	target, err := openIconTarget(icon)
	if err != nil {
		return err
	}
	defer target.Close()
	before, err := snapshotOpenTarget(target)
	if err != nil {
		return err
	}
	return autoOpen(func(action Action) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := snapshotOpenTarget(target); err != nil {
			return err
		}
		if action == ActionDoubleClick && latest != nil {
			current, err := latest()
			if err != nil {
				return err
			}
			if !icon.SameIcon(current) {
				return ErrIconGone
			}
			icon = current
		}
		return Activate(icon, action)
	}, func() error {
		wait, cancel := context.WithTimeout(ctx, 600*time.Millisecond)
		defer cancel()
		return observeOpen(wait, before, target.PID, func() ([]openWindowState, error) { return snapshotOpenTarget(target) }, 50*time.Millisecond)
	})
}
