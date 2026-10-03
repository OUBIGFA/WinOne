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
	items  [3]*systray.MenuItem
}

func (m *trayMenuLanguage) Update(preference string) {
	locale := applicationLanguage(preference)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.locale = locale
	m.applyLocked()
}
func (m *trayMenuLanguage) Bind(open, collected, quit *systray.MenuItem) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items = [3]*systray.MenuItem{open, collected, quit}
	m.applyLocked()
}
func (m *trayMenuLanguage) applyLocked() {
	labels := [3]string{"打开 WinOne", "收纳的程序", "退出 WinOne"}
	if m.locale == "en-US" {
		labels = [3]string{"Open WinOne", "Collected programs", "Exit WinOne"}
	}
	for index, item := range m.items {
		if item != nil {
			item.SetTitle(labels[index])
		}
	}
}
