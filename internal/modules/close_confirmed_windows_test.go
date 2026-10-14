//go:build windows

package modules

import "testing"

func TestConfirmedCloseDoesNotTreatAnEarlierExitAsRestoration(t *testing.T) {
	ended := make(chan struct{})
	close(ended)
	process := &Process{exited: ended}
	if err := process.CloseConfirmed(); err == nil {
		t.Fatal("an earlier process exit counted as restoration acknowledgement")
	}
	if err := process.Close(); err != nil {
		t.Fatal("ordinary idempotent close changed", err)
	}
}
