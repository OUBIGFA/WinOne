//go:build windows

package resultwindow

import "testing"

// Validation must reject stale/absent HWNDs without restoring or focusing any
// user window. Real activation is covered by the isolated desktop probe.
func TestActivationRejectsInvalidOrStaleWindow(t *testing.T) {
	for _, target := range []WindowTarget{{}, {Window: "0", ProcessID: 1}, {Window: "-1", ProcessID: 1}, {Window: "18446744073709551616", ProcessID: 1}, {Window: "123", ProcessID: ^uint32(0)}} {
		if err := target.Activate(); err == nil {
			t.Fatalf("accepted invalid activation target: %+v", target)
		}
	}
}
