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

func TestUpgradeDoesNotTreatOrphanModulePreferencesAsANewProfile(t *testing.T) {
	for _, module := range []string{"capture", "always-on-top", "wintray", "selection"} {
		for _, name := range []string{"settings.json", "managed-settings.json"} {
			t.Run(module+"/"+name, func(t *testing.T) {
				profile := t.TempDir()
				directory := filepath.Join(profile, "modules", module)
				if err := os.MkdirAll(directory, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(directory, name), []byte(`{"preserve":true}`), 0600); err != nil {
					t.Fatal(err)
				}
				if _, err := readUpgradeProfile(profile); err == nil {
					t.Fatal("existing module preferences were treated as an empty profile")
				}
			})
		}
	}
}

func TestUpgradeDoesNotTreatAnExistingPackageRegistryAsAnEmptyProfile(t *testing.T) {
	directory := t.TempDir()
	packages := filepath.Join(directory, "packages")
	if err := os.Mkdir(packages, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packages, ".winone-module-registry.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readUpgradeProfile(directory); err == nil {
		t.Fatal("registered packages were treated as a new profile")
	}
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
