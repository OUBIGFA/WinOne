package startupprobe

import (
	"os"
	"testing"
	"winone/internal/catalog"
)

func TestTimedPowerProbeRetainsRollbackCatalogs(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	manifests := map[string]catalog.Manifest{}
	for _, m := range catalog.Builtins() {
		manifests[m.ID] = m
	}
	ids := []string{"capture", "always-on-top", "wintray", "selection", "quick-assistant", "stroke", "timed-power"}
	for count := 3; count <= 7; count++ {
		report := Report{Protocol: 1, PID: os.Getpid(), Nonce: "fixture", Executable: executable}
		for _, id := range ids[:count] {
			report.Modules = append(report.Modules, manifests[id])
		}
		if err := report.Validate(report.PID, executable, report.Nonce); err != nil {
			t.Fatalf("rollback catalog %d: %v", count, err)
		}
		report.Modules[count-1] = report.Modules[0]
		if err := report.Validate(report.PID, executable, report.Nonce); err == nil {
			t.Fatal("duplicate accepted")
		}
	}
}
