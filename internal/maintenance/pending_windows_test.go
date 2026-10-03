//go:build windows

package maintenance

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIncompleteInstallationRejectsRuntimeButAllowsRepair(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	installation, profile := t.TempDir(), t.TempDir()
	marker := filepath.Join(installation, ".winone-pending")
	if err := os.WriteFile(marker, []byte("interrupted"), 0600); err != nil {
		t.Fatal(err)
	}
	if lease, err := AcquireRuntimeForInstallation(profile, installation); err == nil {
		lease.Close()
		t.Fatal("incomplete installation started")
	}
	lease, err := AcquireMaintenanceForInstallation(profile, installation)
	if err != nil {
		t.Fatal("repair was blocked", err)
	}
	if err = os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	lease.Close()
	runtime, err := AcquireRuntimeForInstallation(profile, installation)
	if err != nil {
		t.Fatal("completed installation still blocked", err)
	}
	runtime.Close()
}
