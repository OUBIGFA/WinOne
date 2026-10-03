//go:build windows

package distribution

import (
	"os"
	"path/filepath"
	"testing"
	"winone/internal/maintenance"
)

func TestNativeUpgradePreparationReleasesActualWorker(t *testing.T) {
	executable := os.Getenv("WINONE_WINTRAY_EXE")
	if executable == "" {
		t.Skip("set WINONE_WINTRAY_EXE for native preparation")
	}
	installation, profile := filepath.Dir(executable), t.TempDir()
	marker := filepath.Join(profile, "keep.txt")
	if err := os.WriteFile(marker, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	lease, err := maintenance.AcquireMaintenanceForInstallation(profile, installation)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	for i := 0; i < 2; i++ {
		if err := PrepareNativeUpgrade(installation, profile); err != nil {
			t.Fatal(err)
		}
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "keep" {
		t.Fatal("profile content changed", err)
	}
	if err := maintenance.RequireStopped(); err != nil {
		t.Fatal("worker remained active", err)
	}
}
