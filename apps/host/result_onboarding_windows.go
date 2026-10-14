//go:build windows

package main

import "winone/internal/onboarding"

// Resolve lazily so read-only startup, settings probes and tests do not mark a
// real machine as onboarded. Only an authenticated composer claim reaches here.
func claimResultComposerHint() (bool, error) {
	directory, err := onboarding.MachineDirectory()
	if err != nil {
		return false, err
	}
	if err := onboarding.PrepareDirectory(directory); err != nil {
		return false, err
	}
	return onboarding.ClaimSendHint(directory)
}
