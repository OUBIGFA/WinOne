//go:build windows

package main

import (
	"fyne.io/systray"
	"sync"
)

// The tray can become ready after a settings request. Retain the latest locale
// before binding native items, so startup cannot overwrite a newer selection.
type trayMenuLanguage struct {
	mu     sync.Mutex
	locale string
	items  [2]*systray.MenuItem
}

func (m *trayMenuLanguage) Update(preference string) {
	locale := applicationLanguage(preference)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.locale = locale
	m.applyLocked()
}
func (m *trayMenuLanguage) Bind(open, quit *systray.MenuItem) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items = [2]*systray.MenuItem{open, quit}
	m.applyLocked()
}
func (m *trayMenuLanguage) applyLocked() {
	labels := [2]string{"打开 WinOne", "退出 WinOne"}
	if m.locale == "en-US" {
		labels = [2]string{"Open WinOne", "Exit WinOne"}
	}
	for index, item := range m.items {
		if item != nil {
			item.SetTitle(labels[index])
		}
	}
}
