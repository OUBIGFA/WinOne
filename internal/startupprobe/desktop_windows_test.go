//go:build windows

package startupprobe

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"winone/internal/maintenance"
)

func TestNativeDesktopProbeUnderMaintenance(t *testing.T) {
	source := os.Getenv("WINONE_DESKTOP_PROBE_EXE")
	if source == "" {
		t.Skip("set WINONE_DESKTOP_PROBE_EXE for real WebView2 probe")
	}
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	installation, profile := t.TempDir(), t.TempDir()
	executable := filepath.Join(installation, "WinOne.exe")
	if err = os.WriteFile(executable, data, 0700); err != nil {
		t.Fatal(err)
	}
	lease, err := maintenance.AcquireMaintenanceForInstallation(profile, installation)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	pending := filepath.Join(installation, ".winone-pending")
	if err = os.WriteFile(pending, []byte("pending"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = CheckDesktop(context.Background(), executable, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	value, err := os.ReadFile(pending)
	if err != nil || string(value) != "pending" {
		t.Fatal("maintenance marker changed", err)
	}
}
