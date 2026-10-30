//go:build windows

package timedpower

import "testing"

// Inspect flags only: no automated test is permitted to execute a power API.
func TestNativePowerFlags(t *testing.T) {
	for action, base := range map[string]uintptr{"shutdown": 8, "restart": 2, "logoff": 0} {
		for _, force := range []bool{false, true} {
			p := Defaults()
			p.Action = action
			p.Force = force
			got, err := shutdownFlags(p)
			want := base
			if force {
				want |= 4
			}
			if err != nil || got != want {
				t.Fatal(action, force, got, err)
			}
		}
	}
	p := Defaults()
	p.DryRun = true
	if err := ExecuteNative(p); err == nil {
		t.Fatal("dry-run must be rejected before any native call")
	}
	p.Action = "invalid"
	if err := ExecuteNative(p); err == nil {
		t.Fatal("invalid action accepted")
	}
}
