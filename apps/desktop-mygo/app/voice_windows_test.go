package app

import "testing"

// Suspending and locking stop OneBot's voice input; resuming and
// unlocking never start a microphone (component_ui.rs's test).
func TestSuspendAndLockStopVoiceInput(t *testing.T) {
	if !stopsVoiceInput(0x218, 4) || !stopsVoiceInput(0x2b1, 7) {
		t.Fatal("suspend or lock not reported")
	}
	if stopsVoiceInput(0x218, 18) || stopsVoiceInput(0x2b1, 8) {
		t.Fatal("resume or unlock reported")
	}
}
