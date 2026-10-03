//go:build windows

package main

import "testing"

func TestDesktopProbeRequiresSnapshotAndMatchingNonce(t *testing.T) {
	state := &desktopProbeState{nonce: "nonce"}
	if err := state.complete("nonce"); err == nil {
		t.Fatal("completed before snapshot")
	}
	state.snapshot = true
	if err := state.complete("nonce"); err == nil {
		t.Fatal("homepage modules were not read")
	}
	state.modulesRead = true
	state.frameReady = true
	state.surfaceReady = true
	if err := state.complete("other"); err == nil {
		t.Fatal("wrong nonce accepted")
	}
	if err := state.complete("nonce"); err != nil {
		t.Fatal(err)
	}
	if !state.completed {
		t.Fatal("completion missing")
	}
}
