//go:build windows

package maintenance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallationLockBlocksOtherProfilesAndPreservesOtherCopies(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	install := t.TempDir()
	maintenance, err := AcquireMaintenanceForInstallation(t.TempDir(), install)
	if err != nil {
		t.Fatal(err)
	}
	defer maintenance.Close()
	for _, alias := range []string{install, strings.ToUpper(install), filepath.Join(install, ".")} {
		if runtime, err := AcquireRuntimeForInstallation(t.TempDir(), alias); err == nil {
			runtime.Close()
			t.Fatal("profile bypass", alias)
		}
	}
	other, err := AcquireRuntimeForInstallation(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal("unrelated installation blocked", err)
	}
	other.Close()
	maintenance.Close()
	runtime, err := AcquireRuntimeForInstallation(t.TempDir(), install)
	if err != nil {
		t.Fatal(err)
	}
	runtime.Close()
}

func TestProfileFailureReleasesInstallationLease(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	install, profile := t.TempDir(), t.TempDir()
	busy, err := AcquireMaintenance(profile)
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	if runtime, err := AcquireRuntimeForInstallation(profile, install); err == nil {
		runtime.Close()
		t.Fatal("profile lock bypassed")
	}
	lease, err := AcquireMaintenanceForInstallation(t.TempDir(), install)
	if err != nil {
		t.Fatal("failed acquisition leaked installation lease", err)
	}
	lease.Close()
}

func TestInstallationIdentityResolvesLinks(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	install := t.TempDir()
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(install, alias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	lease, err := AcquireMaintenanceForInstallation(t.TempDir(), install)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	if runtime, err := AcquireRuntimeForInstallation(t.TempDir(), alias); err == nil {
		runtime.Close()
		t.Fatal("symlink bypass")
	}
}
