//go:build windows

package worker

import (
	"errors"
	"time"

	"winone/modules/wintray/internal/config"
	"winone/modules/wintray/internal/traybox"
)

// trayRetryDelay is how often the collector retries while Explorer has not
// created its taskbar yet.
const trayRetryDelay = time.Second

// collectTrayIcons applies the tray selection. A taskbar that is not ready
// yet is a temporary condition at sign-in, not a broken component: it is
// retried instead of failing the whole reconciliation.
func (r *Runtime) collectTrayIcons(settings config.Settings) error {
	return collectTrayIconsWith(r.applyTrayPaths, r.waitTrayRetry, settings, r.logger)
}

// collectTrayIconsWith is the retry loop without the runtime, so its
// behavior can be checked directly.
func collectTrayIconsWith(apply func([]string) error, wait func() error, settings config.Settings, logger trayLogger) error {
	paths := config.CollectedTrayIconPaths(settings)
	err := apply(paths)
	if err == nil || !errors.Is(err, traybox.ErrTaskbarUnavailable) {
		return err
	}
	if logger != nil {
		logger.Warn("taskbar is not ready; tray collection will retry")
	}
	for {
		if err := wait(); err != nil {
			return err
		}
		err := apply(paths)
		if err == nil {
			return nil
		}
		if !errors.Is(err, traybox.ErrTaskbarUnavailable) {
			return err
		}
	}
}

// trayLogger is the part of the module logger the retry needs.
type trayLogger interface {
	Warn(string)
}

// applyTrayPaths changes the selection on the UI thread, which owns the box.
func (r *Runtime) applyTrayPaths(paths []string) error {
	return r.onUI(func() error { return r.box.SetPaths(paths) })
}

// waitTrayRetry sleeps until the next attempt or the worker stops.
func (r *Runtime) waitTrayRetry() error {
	timer := time.NewTimer(trayRetryDelay)
	defer timer.Stop()
	select {
	case <-r.ctx.Done():
		return r.ctx.Err()
	case <-timer.C:
		return nil
	}
}
