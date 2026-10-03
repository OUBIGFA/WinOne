//go:build windows

package distribution

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Test-only versions distinguish two local fixture packages. Original build
// directories and their records are never relabelled or published.
func nativeUpgradePackage(t *testing.T, source, license, version string, broken bool) string {
	t.Helper()
	directory := t.TempDir()
	if err := os.CopyFS(directory, os.DirFS(source)); err != nil {
		t.Fatal(err)
	}
	if broken {
		if err := os.WriteFile(filepath.Join(directory, "WinOne.exe"), []byte("not a Windows executable"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := RecordBuild(directory, license, version); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "fixture.zip")
	if err := Build(directory, license, archive, version); err != nil {
		t.Fatal(err)
	}
	return archive
}

func TestNativeUpgradeExecutesVerifiedBinaries(t *testing.T) {
	previous, next, license := os.Getenv("WINONE_UPGRADE_OLD_BUNDLE"), os.Getenv("WINONE_UPGRADE_NEW_BUNDLE"), os.Getenv("WINONE_UPGRADE_LICENSE")
	if previous == "" || next == "" || license == "" {
		t.Skip("set old/new bundle and license paths for full native upgrade fixtures")
	}
	oldPackage := nativeUpgradePackage(t, previous, license, "0.0.0-upgrade-test.1", false)
	for _, broken := range []bool{false, true} {
		t.Run(fmt.Sprint(broken), func(t *testing.T) {
			nextPackage := nativeUpgradePackage(t, next, license, "0.0.0-upgrade-test.2", broken)
			installation, staged := extractTestPackage(t, oldPackage), extractTestPackage(t, nextPackage)
			backup := filepath.Join(t.TempDir(), "backup")
			if _, err := BackupInstalled(installation, oldPackage, backup); err != nil {
				t.Fatal(err)
			}
			key := fmt.Sprintf(`Software\WinOneUpgradeNativeTest-%d`, time.Now().UnixNano())
			shortcut := filepath.Join(t.TempDir(), "WinOne.lnk")
			if err := BackupRegistration(installation, oldPackage, backup, key); err != nil {
				t.Fatal(err)
			}
			if err := BackupShortcut(oldPackage, backup, shortcut); err != nil {
				t.Fatal(err)
			}
			registration, err := CaptureRegistration(installation, key)
			if err != nil {
				t.Fatal(err)
			}
			link, err := CaptureShortcut(shortcut)
			if err != nil {
				t.Fatal(err)
			}
			plan, err := PlanUpgrade(oldPackage, nextPackage)
			if err != nil {
				t.Fatal(err)
			}
			journal, err := CreateUpgradeJournal(filepath.Join(t.TempDir(), "upgrade.json"), plan)
			if err != nil {
				t.Fatal(err)
			}
			profile := t.TempDir()
			if err := os.WriteFile(filepath.Join(profile, "keep.txt"), []byte("keep"), 0600); err != nil {
				t.Fatal(err)
			}
			request := UpgradeExecution{Installation: installation, Profile: profile, Staged: staged, Backup: backup, PreviousPackage: oldPackage, NextPackage: nextPackage, RegistryKey: key, ShortcutPath: shortcut, NextRegistration: registration, NextShortcut: link, Journal: journal}
			err = ExecuteUpgrade(request)
			if (err != nil) != broken {
				t.Fatalf("native execution: %v", err)
			}
			reference, phase := nextPackage, "committed"
			if broken {
				reference, phase = oldPackage, "rolled-back"
			}
			if journal.Snapshot().Phase != phase {
				t.Fatal(journal.Snapshot().Phase)
			}
			if _, err := VerifyInstalled(installation, reference); err != nil {
				t.Fatal(err)
			}
			if data, err := os.ReadFile(filepath.Join(profile, "keep.txt")); err != nil || string(data) != "keep" {
				t.Fatal("profile changed", err)
			}
		})
	}
}
