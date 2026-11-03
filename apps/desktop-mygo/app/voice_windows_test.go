package app

import "testing"

// Suspending and locking stop OneBot's voice input; resuming and
// unlocking never start a microphone (component_ui.rs's test).
func TestSuspendAndLockStopVoiceInput(t *testing.T) {
	if voicePowerEvent(0x218, 4) != "winone:voice-system-suspend" || voicePowerEvent(0x2b1, 7) != "winone:voice-screen-locked" {
		t.Fatal("suspend or lock not reported")
	}
	if voicePowerEvent(0x218, 18) != "" || voicePowerEvent(0x2b1, 8) != "" {
		t.Fatal("resume or unlock reported")
	}
}
