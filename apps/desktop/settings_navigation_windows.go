//go:build windows

package main

import "github.com/wailsapp/wails/v2/pkg/runtime"

func (a *App) KeepSettingsWindow() { a.ctxMu.Lock(); a.captureOnly = false; a.ctxMu.Unlock() }

// Keep the request until React has received it, including during startup.
func (a *App) requestSettingsNavigation() {
	a.ctxMu.Lock()
	a.captureOnly = false
	a.settingsNavigation++
	ctx := a.ctx
	a.ctxMu.Unlock()
	if ctx != nil {
		runtime.EventsEmit(ctx, "winone:open-settings")
	}
}

func (a *App) SettingsNavigation() uint64 {
	a.ctxMu.RLock()
	defer a.ctxMu.RUnlock()
	return a.settingsNavigation
}
