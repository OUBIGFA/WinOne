//go:build windows

package main

import (
	"errors"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"os"
	"winone/internal/capturehandoff"
)

const captureLaunchEvent = "winone:capture-launch"

func (a *App) queueCaptureDraft(id string) error {
	if err := a.captureLaunches.Push(id); err != nil {
		return err
	}
	a.ctxMu.Lock()
	a.captureRecoveryChecked = true
	a.ctxMu.Unlock()
	a.notifyCaptureLaunch()
	return nil
}
func (a *App) notifyCaptureLaunch() {
	if ctx := a.windowContext(); ctx != nil && a.captureLaunches.Peek() != nil {
		runtime.EventsEmit(ctx, captureLaunchEvent)
	}
}

// Frontend startup reads the queue after subscribing; events alone would lose
// requests arriving before React has mounted. Confirmation consumes only head.
func (a *App) CaptureLaunch() (*capturehandoff.Request, error) {
	if !a.dedicatedCapture {
		return nil, nil
	}
	if pending := a.captureLaunches.Peek(); pending != nil {
		return pending, nil
	}
	a.ctxMu.Lock()
	check := !a.captureRecoveryChecked && a.captureDirectory != ""
	a.captureRecoveryChecked = true
	directory := a.captureDirectory
	a.ctxMu.Unlock()
	if check {
		var saved struct {
			ID string `json:"id"`
		}
		err := readCaptureMetadata(directory, "capture-draft.json", &saved)
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		if err == nil {
			err = a.captureLaunches.Push(saved.ID)
		}
		if err != nil {
			a.ctxMu.Lock()
			a.captureRecoveryChecked = false
			a.ctxMu.Unlock()
			return nil, err
		}
	}
	return a.captureLaunches.Peek(), nil
}
func (a *App) AcknowledgeCaptureLaunch(id string) bool {
	acknowledged := a.captureLaunches.Acknowledge(id)
	if acknowledged {
		a.notifyCaptureLaunch()
	}
	return acknowledged
}

func (a *App) SetCaptureEditing(active bool) {
	a.ctxMu.Lock()
	a.captureEditing = active
	quit := !active && (a.captureClosePending || a.captureOnly)
	ctx := a.ctx
	if quit {
		a.captureClosePending = false
	}
	a.ctxMu.Unlock()
	if quit && ctx != nil {
		runtime.Quit(ctx)
	}
}

// Opening failed before an editor could own the close guard. Keep the error
// window available, while still honoring an explicit pending close request.
func (a *App) AbortCaptureOpening() {
	a.ctxMu.Lock()
	a.captureOnly = false
	a.ctxMu.Unlock()
	a.SetCaptureEditing(false)
}
func (a *App) CaptureClosePending() bool {
	a.ctxMu.RLock()
	defer a.ctxMu.RUnlock()
	return a.captureClosePending
}
func (a *App) CancelCaptureClose() { a.ctxMu.Lock(); a.captureClosePending = false; a.ctxMu.Unlock() }
func (a *App) blockCaptureClose() bool {
	a.ctxMu.Lock()
	if !a.captureEditing {
		if a.settingsCloseApproved {
			a.settingsCloseApproved = false
			a.settingsClosePending = false
			a.ctxMu.Unlock()
			return false
		}
		if a.settingsDirty || a.settingsBusy {
			a.settingsClosePending = true
			ctx := a.ctx
			a.ctxMu.Unlock()
			if ctx != nil {
				runtime.EventsEmit(ctx, "winone:settings-close")
			}
			return true
		}
		a.ctxMu.Unlock()
		return false
	}
	a.settingsCloseApproved = false
	a.captureClosePending = true
	ctx := a.ctx
	a.ctxMu.Unlock()
	if ctx != nil {
		runtime.EventsEmit(ctx, "winone:capture-close")
	}
	return true
}
