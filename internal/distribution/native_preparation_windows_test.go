//go:build windows

package distribution

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"winone/internal/maintenance"
)

// Preparation must release the worker executable before the first replacement.
// Run only with explicit real bundles, using copied workers and empty profiles.
func TestNativePreparationReleasesWorkerExecutable(t *testing.T) {
	oldBundle, newBundle := os.Getenv("WINONE_UPGRADE_OLD_BUNDLE"), os.Getenv("WINONE_UPGRADE_NEW_BUNDLE")
	if oldBundle == "" || newBundle == "" {
		t.Skip("set old/new native bundles for worker replacement checks")
	}
	oldData, err := os.ReadFile(filepath.Join(oldBundle, "WinOne.WinTray.exe"))
	if err != nil {
		t.Fatal(err)
	}
	newData, err := os.ReadFile(filepath.Join(newBundle, "WinOne.WinTray.exe"))
	if err != nil {
		t.Fatal(err)
	}
	oldHash, newHash := sha256.Sum256(oldData), sha256.Sum256(newData)
	for _, scenario := range []struct {
		name          string
		prepare, full bool
	}{{"never-started", false, false}, {"after-preparation", true, false}, {"full-bundle", false, true}, {"full-bundle-after-preparation", true, true}} {
		t.Run(scenario.name, func(t *testing.T) {
			installation, staged := t.TempDir(), t.TempDir()
			if scenario.full {
				if err := os.CopyFS(installation, os.DirFS(oldBundle)); err != nil {
					t.Fatal(err)
				}
			}
			for directory, data := range map[string][]byte{installation: oldData, staged: newData} {
				if err := os.WriteFile(filepath.Join(directory, "WinOne.WinTray.exe"), data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			target, err := openUpgradeRoot(installation)
			if err != nil {
				t.Fatal(err)
			}
			defer target.Close()
			if scenario.prepare {
				profile := t.TempDir()
				lease, err := maintenance.AcquireMaintenanceForInstallation(profile, installation)
				if err != nil {
					t.Fatal(err)
				}
				defer lease.Close()
				if err := PrepareNativeUpgrade(installation, profile); err != nil {
					t.Fatal(err)
				}
			}
			source, err := openUpgradeRoot(staged)
			if err != nil {
				t.Fatal(err)
			}
			defer source.Close()
			if err := replaceUpgradeFile(target, source, "WinOne.WinTray.exe", "WinOne.WinTray.exe", hex.EncodeToString(newHash[:]), hex.EncodeToString(oldHash[:])); err != nil {
				t.Fatal(err)
			}
			if actual, err := upgradeFileHash(target, "WinOne.WinTray.exe"); err != nil || actual != hex.EncodeToString(newHash[:]) {
				t.Fatal("replacement content differs", actual, err)
			}
		})
	}
}
