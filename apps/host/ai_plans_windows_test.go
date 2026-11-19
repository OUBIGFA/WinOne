//go:build windows

package main

import (
	"strings"
	"testing"
)

func TestPlatformSessionInputPreservesUpstreamBound(t *testing.T) {
	state := aiSignInState{State: "input", Prompt: &aiSignInPrompt{Type: "password", Key: "plan-session"}}
	if !validSignInInput(state, strings.Repeat("x", 64<<10)) || validSignInInput(state, strings.Repeat("x", (64<<10)+1)) {
		t.Fatal("platform paste bound differs from upstream")
	}
	state.Prompt.Key = "account-secret"
	if validSignInInput(state, strings.Repeat("x", 16385)) {
		t.Fatal("ordinary account input bound changed")
	}
}
