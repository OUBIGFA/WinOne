package distribution

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyBackupRejectsInvalidCompletionRecord(t *testing.T) {
	for _, scenario := range []string{"missing", "wrong-inventory", "old-format", "unsafe-file"} {
		t.Run(scenario, func(t *testing.T) {
			dir, archive := installedFixture(t)
			destination := filepath.Join(t.TempDir(), "backup")
			record, err := BackupInstalled(dir, archive, destination)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(destination, "backup-complete.json")
			if scenario == "missing" {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			} else {
				switch scenario {
				case "wrong-inventory":
					record.ManifestSHA256 = "mismatch"
				case "old-format":
					record.InstallerFilesCaptured = false
				case "unsafe-file":
					record.InstallerFiles = []File{{Path: "../outside"}}
				}
				data, err := json.Marshal(record)
				if err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := VerifyBackup(destination, archive); err == nil {
				t.Fatal("invalid backup record accepted", scenario)
			}
		})
	}
}

func TestBackupInstalledCopiesVerifiedPayloadOnly(t *testing.T) {
	dir, archive := installedFixture(t)
	extra := filepath.Join(dir, "user.txt")
	if err := os.WriteFile(extra, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "backup")
	record, err := BackupInstalled(dir, archive, destination)
	if err != nil {
		t.Fatal(err)
	}
	if record.Version != "0.1.0-alpha.1" || record.ManifestSHA256 == "" {
		t.Fatal(record)
	}
	if !record.InstallerFilesCaptured || len(record.InstallerFiles) != 0 {
		t.Fatal("portable installer-file state not recorded", record)
	}
	if _, err := VerifyInstalled(destination, archive); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(destination, "backup-complete.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(destination, "user.txt")); !os.IsNotExist(err) {
		t.Fatal("extra copied", err)
	}
	if data, err := os.ReadFile(extra); err != nil || string(data) != "keep" {
		t.Fatal("extra changed", err)
	}
	if _, err := BackupInstalled(dir, archive, destination); err == nil {
		t.Fatal("existing backup overwritten")
	}
}

func TestBackupPreservesInstallerOwnedUninstaller(t *testing.T) {
	dir, archive := installedFixture(t)
	uninstaller := []byte("test uninstaller bytes")
	if err := os.WriteFile(filepath.Join(dir, "Uninstall.exe"), uninstaller, 0600); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "backup")
	record, err := BackupInstalled(dir, archive, destination)
	if err != nil {
		t.Fatal(err)
	}
	if !record.InstallerFilesCaptured || len(record.InstallerFiles) != 1 || record.InstallerFiles[0].Path != "Uninstall.exe" || record.InstallerFiles[0].Size != int64(len(uninstaller)) || len(record.InstallerFiles[0].SHA256) != 64 {
		t.Fatal(record)
	}
	if data, err := os.ReadFile(filepath.Join(destination, "Uninstall.exe")); err != nil || string(data) != string(uninstaller) {
		t.Fatal("uninstaller not preserved", err)
	}
	if _, err := VerifyBackup(destination, archive); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destination, "Uninstall.exe"), []byte("changed backup"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyBackup(destination, archive); err == nil {
		t.Fatal("modified uninstaller backup accepted")
	}
}

func TestBackupRejectsNonFileUninstaller(t *testing.T) {
	dir, archive := installedFixture(t)
	if err := os.Mkdir(filepath.Join(dir, "Uninstall.exe"), 0700); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "backup")
	if _, err := BackupInstalled(dir, archive, destination); err == nil {
		t.Fatal("directory uninstaller accepted")
	}
	if _, err := os.Stat(filepath.Join(destination, "backup-complete.json")); !os.IsNotExist(err) {
		t.Fatal("invalid backup marked ready")
	}
}

func TestBackupRejectsUnsafeDestinationAndChangedSource(t *testing.T) {
	dir, archive := installedFixture(t)
	if _, err := BackupInstalled(dir, archive, filepath.Join(dir, "backup")); err == nil {
		t.Fatal("backup inside installation accepted")
	}
	if err := os.WriteFile(filepath.Join(dir, "WinOne.exe"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "backup")
	if _, err := BackupInstalled(dir, archive, destination); err == nil {
		t.Fatal("changed source accepted")
	}
	if _, err := os.Stat(filepath.Join(destination, "backup-complete.json")); !os.IsNotExist(err) {
		t.Fatal("failed backup marked complete", err)
	}
}
