//go:build windows

package main

import (
	"errors"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// SetSettingsActivity mirrors only the active page, never persists drafts or
// starts a module. A changed draft invalidates an earlier close approval.
func (a *App) SetSettingsActivity(dirty, busy bool) {
	a.ctxMu.Lock()
	defer a.ctxMu.Unlock()
	if a.settingsDirty != dirty || a.settingsBusy != busy {
		a.settingsCloseApproved = false
	}
	a.settingsDirty, a.settingsBusy = dirty, busy
}
func (a *App) SettingsClosePending() bool {
	a.ctxMu.RLock()
	defer a.ctxMu.RUnlock()
	return a.settingsClosePending
}
func (a *App) CancelSettingsClose() {
	a.ctxMu.Lock()
	defer a.ctxMu.Unlock()
	a.settingsClosePending = false
	a.settingsCloseApproved = false
}

// Approval is consumed by one close attempt. A newly opened capture editor
// still gets its own decision, and a running save can never be discarded.
func (a *App) ApproveSettingsClose() error {
	a.ctxMu.Lock()
	if a.settingsBusy || !a.settingsClosePending {
		a.ctxMu.Unlock()
		return errors.New("窗口尚不能关闭")
	}
	a.settingsCloseApproved = true
	ctx := a.ctx
	a.ctxMu.Unlock()
	if ctx != nil {
		go runtime.Quit(ctx)
	}
	return nil
}
