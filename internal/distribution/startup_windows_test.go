//go:build windows

package distribution

import (
	"os"
	"path/filepath"
	"testing"
	"winone/internal/maintenance"
)

func TestUpgradeProfileDistinguishesEmptyFromLegacyAndDamaged(t *testing.T) {
	profile := t.TempDir()
	data, err := readUpgradeProfile(profile)
	if err != nil || data != nil {
		t.Fatal(data, err)
	}
	legacy := filepath.Join(profile, "settings.json")
	if err = os.WriteFile(legacy, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = readUpgradeProfile(profile); err == nil {
		t.Fatal("legacy profile accepted as empty")
	}
	canonical := filepath.Join(profile, "configuration.json")
	if err = os.WriteFile(canonical, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	data, err = readUpgradeProfile(profile)
	if err != nil || string(data) != `{}` {
		t.Fatal(data, err)
	}
	// Decoding is performed by the target version, not by the old updater.
}

func TestNativeUpgradeStartupChecksCompleteBundle(t *testing.T) {
	installation := os.Getenv("WINONE_UPGRADE_PROBE_BUNDLE")
	if installation == "" {
		t.Skip("set WINONE_UPGRADE_PROBE_BUNDLE for combined native startup checks")
	}
	profile, scratch := t.TempDir(), t.TempDir()
	lease, err := maintenance.AcquireMaintenanceForInstallation(profile, installation)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	if err = CheckNativeUpgrade(installation, profile, scratch); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(profile, "configuration.json")); !os.IsNotExist(err) {
		t.Fatal("empty profile changed", err)
	}
}
