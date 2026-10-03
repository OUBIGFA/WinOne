//go:build windows

package main

import "os"

// The browser cannot select another process as the recording owner.
func (a *App) BeginShortcutRecording() (string, error) {
	var response struct {
		Token string `json:"token"`
	}
	err := a.request("shortcuts.record.start", map[string]any{"processId": os.Getpid()}, &response)
	return response.Token, err
}
func (a *App) EndShortcutRecording(token string) error {
	var response struct {
		Stopped bool `json:"stopped"`
	}
	return a.request("shortcuts.record.stop", map[string]string{"token": token}, &response)
}
