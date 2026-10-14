//go:build windows

package main

import "testing"

func TestChatTitleActionsReachTheHostBridge(t *testing.T) {
	for _, method := range []string{"title.generate", "title.regenerate"} {
		app := &App{startupProbe: &desktopProbeState{}}
		_, err := app.Chat(method, `{}`)
		if err == nil || err.Error() != "startup probe permits snapshot only" {
			t.Errorf("Chat(%q) did not forward to the Host request gate: %v", method, err)
		}
	}
}
