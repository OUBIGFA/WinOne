package main

import (
	"errors"
	"github.com/yetone/magpie/internal/provider"
	"testing"
)

func TestSignInStartErrorKeepsActionableClientRequirement(t *testing.T) {
	for _, client := range []string{"Grok Build", "Cursor CLI", "Devin CLI"} {
		message := client + " is required; install the official client first"
		if got := pluginSignInStartError(errors.New(message)).Error(); got != message {
			t.Fatalf("missing client hint: %s", got)
		}
	}
	if got := pluginSignInStartError(errors.New("request failed: private-fixture-key")).Error(); got != "account sign-in could not start; check the connection and try again" {
		t.Fatal("unexpected plugin details escaped the boundary")
	}
}

func TestSignInDeviceCodesAcrossProviderInstructions(t *testing.T) {
	for _, test := range []struct{ name, instructions, url, want string }{
		{"cline", "Approve the sign-in in the browser — the code is HFPM-MCLY.", "", "HFPM-MCLY"},
		{"factory", "Confirm the code ABCD-EFGH on Factory's page", "", "ABCD-EFGH"},
		{"minimax", "Sign in to MiniMax in the browser and confirm the code 123456; this finishes by itself.", "", "123456"},
		{"colon", "Device code: AB12-CD34", "", "AB12-CD34"},
		{"short", "Confirm the code A1B2 on the page", "", "A1B2"},
		{"structured URL", "Approve in your browser", "https://example.invalid/activate?user_code=a1b2-c3d4", "a1b2-c3d4"},
		{"ordinary words", "Sign in to Command Code Studio.", "", ""},
		{"no code value", "Paste the code shown in your browser.", "", ""},
		{"oauth result is not a device code", "Sign in", "https://example.invalid/callback?code=private-authorization-code", ""},
		{"polling credential is private", "Sign in", "https://example.invalid/activate?device_code=private-polling-credential", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := normalizeSignInState(provider.SignInState{State: "waiting", Instructions: test.instructions, URL: test.url})
			if got.Code != test.want {
				t.Fatalf("device code %q, want %q", got.Code, test.want)
			}
		})
	}
}

func TestSignInNormalizationPreservesAuthorityAndInputDirection(t *testing.T) {
	state := provider.SignInState{Code: "exact vendor code", Instructions: "Device code ABCD-EFGH", State: "waiting"}
	if got := normalizeSignInState(state); got.Code != state.Code {
		t.Fatal("vendor code was replaced")
	}
	for _, state := range []provider.SignInState{{PasteCode: true}, {PasteCallback: true}, {PasteKey: true}} {
		state.State = "waiting"
		state.Instructions = "Paste the code ABCD-EFGH shown in the browser"
		if got := normalizeSignInState(state); got.Code != "" {
			t.Fatal("inbound authorization became an outbound device code")
		}
	}
}
