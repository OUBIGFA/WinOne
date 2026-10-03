//go:build windows

package distribution

import (
	"errors"
	"fmt"
	"golang.org/x/sys/windows/registry"
	"os"
	"path/filepath"
	"testing"
	"time"
	"winone/internal/maintenance"
)

func TestUpgradeExecutionCommitsOrRollsBack(t *testing.T) {
	for _, failProbe := range []bool{false, true} {
		t.Run(fmt.Sprint(failProbe), func(t *testing.T) {
			request := upgradeExecutionFixture(t)
			installation, oldPackage, nextPackage, journal := request.Installation, request.PreviousPackage, request.NextPackage, request.Journal
			prepared, probed := false, false
			request.Prepare = func() error { prepared = true; return nil }
			request.CheckStartup = func() error {
				probed = true
				if _, err := VerifyUpgradePayload(installation, nextPackage); err != nil {
					return err
				}
				if failProbe {
					return errors.New("startup probe failed")
				}
				return nil
			}
			err := ExecuteUpgrade(request)
			if (err != nil) != failProbe || !prepared || !probed {
				t.Fatal(err, prepared, probed)
			}
			want, phase := nextPackage, "committed"
			if failProbe {
				want, phase = oldPackage, "rolled-back"
			}
			if _, err := VerifyInstalled(installation, want); err != nil {
				t.Fatal(err)
			}
			if journal.Snapshot().Phase != phase {
				t.Fatal(journal.Snapshot().Phase)
			}
			if data, err := os.ReadFile(filepath.Join(installation, "notes.txt")); err != nil || string(data) != "user data" {
				t.Fatal("extra file changed", err)
			}
		})
	}
}

func TestDefaultStartupCheckRejectsLegacyProfileAndRollsBack(t *testing.T) {
	request := upgradeExecutionFixture(t)
	request.CheckStartup = nil
	legacy := filepath.Join(request.Profile, "settings.json")
	if err := os.WriteFile(legacy, []byte(`{"keep":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	err := ExecuteUpgrade(request)
	if err == nil || request.Journal.Snapshot().Phase != "rolled-back" {
		t.Fatal("default native check was bypassed", err)
	}
	if _, err := VerifyInstalled(request.Installation, request.PreviousPackage); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(legacy)
	if err != nil || string(data) != `{"keep":true}` {
		t.Fatal("profile changed", err)
	}
}

func TestUpgradeExecutionResumesJournal(t *testing.T) {
	for _, restore := range []bool{false, true} {
		t.Run(fmt.Sprint(restore), func(t *testing.T) {
			request := upgradeExecutionFixture(t)
			lease, err := maintenance.AcquireMaintenanceForInstallation(request.Profile, request.Installation)
			if err != nil {
				t.Fatal(err)
			}
			root, err := openUpgradeRoot(request.Installation)
			if err != nil {
				lease.Close()
				t.Fatal(err)
			}
			err = ensureExecutionMarker(root, request.Journal.Snapshot().ID, true)
			root.Close()
			if err != nil {
				lease.Close()
				t.Fatal(err)
			}
			if err = request.Journal.Transition(0, "applying"); err != nil {
				lease.Close()
				t.Fatal(err)
			}
			for i, file := range request.Journal.Snapshot().Plan.Files {
				if file.Action != "unchanged" {
					if err = ApplyUpgradeFile(request.Installation, request.Staged, request.Journal, i); err != nil {
						lease.Close()
						t.Fatal(err)
					}
					break
				}
			}
			if restore {
				if err = request.Journal.Transition(request.Journal.Snapshot().Revision, "restoring"); err != nil {
					lease.Close()
					t.Fatal(err)
				}
			}
			lease.Close()
			request.Journal, err = OpenUpgradeJournal(request.Journal.path)
			if err != nil {
				t.Fatal(err)
			}
			request.Prepare = func() error { return errors.New("preparation must not rerun") }
			err = ExecuteUpgrade(request)
			if (err != nil) != restore {
				t.Fatal(err)
			}
			reference := request.NextPackage
			if restore {
				reference = request.PreviousPackage
			}
			if _, err = VerifyInstalled(request.Installation, reference); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestUpgradeExecutionRejectsForeignMarkerAndActiveRuntime(t *testing.T) {
	request := upgradeExecutionFixture(t)
	prepared := false
	request.Prepare = func() error { prepared = true; return nil }
	lease, err := maintenance.AcquireRuntimeForInstallation(request.Profile, request.Installation)
	if err != nil {
		t.Fatal(err)
	}
	if err = ExecuteUpgrade(request); err == nil {
		lease.Close()
		t.Fatal("active runtime ignored")
	}
	lease.Close()
	marker := filepath.Join(request.Installation, ".winone-pending")
	if err = os.WriteFile(marker, []byte("other operation"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = ExecuteUpgrade(request); err == nil {
		t.Fatal("foreign marker adopted")
	}
	if prepared {
		t.Fatal("resources touched before ownership checked")
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "other operation" {
		t.Fatal("foreign marker changed", err)
	}
}

func upgradeExecutionFixture(t *testing.T, installed ...bool) UpgradeExecution {
	t.Helper()
	withInstaller := len(installed) > 0 && installed[0]
	if withInstaller && os.Getenv("WINONE_REGISTRY_LIVE_TEST") != "1" {
		t.Skip("set WINONE_REGISTRY_LIVE_TEST=1")
	}
	installation, oldPackage := installedFixture(t)
	if err := os.WriteFile(filepath.Join(installation, "notes.txt"), []byte("user data"), 0600); err != nil {
		t.Fatal(err)
	}
	root, license := fixture(t)
	if err := os.WriteFile(filepath.Join(root, "WinOne.exe"), []byte("next program"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := RecordBuild(root, license, "0.2.0"); err != nil {
		t.Fatal(err)
	}
	nextPackage := filepath.Join(t.TempDir(), "next.zip")
	if err := Build(root, license, nextPackage, "0.2.0"); err != nil {
		t.Fatal(err)
	}
	staged := extractTestPackage(t, nextPackage)
	var nextUninstaller *File
	if withInstaller {
		uninstallerFixture(t, installation, "old uninstall")
		nextUninstaller = uninstallerFixture(t, staged, "new uninstall")
	}
	backup := filepath.Join(t.TempDir(), "backup")
	if _, err := BackupInstalled(installation, oldPackage, backup); err != nil {
		t.Fatal(err)
	}
	key := fmt.Sprintf(`Software\WinOneInstallerTests\execution-%d`, time.Now().UnixNano())
	shortcut := filepath.Join(t.TempDir(), "WinOne.lnk")
	if withInstaller {
		entry, _, err := registry.CreateKey(registry.CURRENT_USER, key, registry.ALL_ACCESS|registry.WOW64_64KEY)
		if err != nil {
			t.Fatal(err)
		}
		if err = entry.SetStringValue("InstallLocation", installation); err != nil {
			entry.Close()
			t.Fatal(err)
		}
		if err = entry.SetStringValue("DisplayVersion", "0.1.0-alpha.1"); err != nil {
			entry.Close()
			t.Fatal(err)
		}
		entry.Close()
		t.Cleanup(func() { registry.DeleteKey(registry.CURRENT_USER, key) })
		shortcutState(t, shortcut, 1)
	}
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
	if withInstaller {
		entry, err := registry.OpenKey(registry.CURRENT_USER, key, registry.SET_VALUE|registry.WOW64_64KEY)
		if err != nil {
			t.Fatal(err)
		}
		if err = entry.SetStringValue("DisplayVersion", "0.2.0"); err != nil {
			entry.Close()
			t.Fatal(err)
		}
		entry.Close()
		next, err := CaptureRegistration(installation, key)
		if err != nil {
			t.Fatal(err)
		}
		if err = RestoreRegistration(installation, key, registration, next); err != nil {
			t.Fatal(err)
		}
		registration = next
	}
	link, err := CaptureShortcut(shortcut)
	if err != nil {
		t.Fatal(err)
	}
	if withInstaller {
		next := shortcutState(t, shortcut, 2)
		if err = RestoreShortcut(shortcut, link, next); err != nil {
			t.Fatal(err)
		}
		link = next
	}
	plan, err := PlanUpgrade(oldPackage, nextPackage)
	if err != nil {
		t.Fatal(err)
	}
	journal, err := CreateUpgradeJournal(filepath.Join(t.TempDir(), "upgrade.json"), plan)
	if err != nil {
		t.Fatal(err)
	}
	return UpgradeExecution{Installation: installation, Profile: t.TempDir(), Staged: staged, Backup: backup, PreviousPackage: oldPackage, NextPackage: nextPackage, RegistryKey: key, ShortcutPath: shortcut, NextRegistration: registration, NextShortcut: link, NextUninstaller: nextUninstaller, Journal: journal, Prepare: func() error { return nil }, CheckStartup: func() error { _, err := VerifyUpgradePayload(installation, nextPackage); return err }}
}

func TestInstalledUpgradeExecutionMetadataRoundTrip(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			request := upgradeExecutionFixture(t, true)
			if fail {
				request.CheckStartup = func() error { return errors.New("probe failed") }
			}
			err := ExecuteUpgrade(request)
			if (err != nil) != fail {
				t.Fatal(err)
			}
			entry, err := registry.OpenKey(registry.CURRENT_USER, request.RegistryKey, registry.READ|registry.WOW64_64KEY)
			if err != nil {
				t.Fatal(err)
			}
			defer entry.Close()
			version, _, err := entry.GetStringValue("DisplayVersion")
			if err != nil {
				t.Fatal(err)
			}
			want, uninstaller := "0.2.0", "new uninstall"
			if fail {
				want, uninstaller = "0.1.0-alpha.1", "old uninstall"
			}
			if version != want {
				t.Fatal(version, want)
			}
			if data, err := os.ReadFile(filepath.Join(request.Installation, "Uninstall.exe")); err != nil || string(data) != uninstaller {
				t.Fatal(string(data), err)
			}
		})
	}
}
