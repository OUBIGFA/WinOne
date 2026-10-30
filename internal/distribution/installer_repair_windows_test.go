//go:build windows

package distribution

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func repairFixture(t *testing.T) (UpgradeExecution, InstallerUpdate) {
	t.Helper()
	if os.Getenv("WINONE_REGISTRY_LIVE_TEST") != "1" {
		t.Skip("explicit isolated registry test")
	}
	r := upgradeExecutionFixture(t, true)
	if err := os.WriteFile(filepath.Join(r.Staged, "WinOne.lnk"), r.NextShortcut.Data, 0600); err != nil {
		t.Fatal(err)
	}
	return r, InstallerUpdate{Installation: r.Installation, Profile: r.Profile, Staged: r.Staged, RegistryKey: r.RegistryKey, ShortcutPath: r.ShortcutPath, CheckStartup: func() error { return nil }}
}

// Prepared journals caused the reported failure without having changed any
// live file. Exercise that exact old rejection AND already-written states with
// a different package. None may demand the previous installer or config.
func TestCurrentInstallerSupersedesOldTransactions(t *testing.T) {
	for _, phase := range []string{"prepared", "applying", "restoring", "damaged-history", "foreign-history"} {
		t.Run(phase, func(t *testing.T) {
			r, update := repairFixture(t)
			work := r.Installation + ".update-27"
			prepared, err := prepareInstallerUpdate(update, work)
			if err != nil {
				t.Fatal(err)
			}
			if phase == "applying" || phase == "restoring" {
				root, err := openUpgradeRoot(r.Installation)
				if err != nil {
					t.Fatal(err)
				}
				if err = ensureExecutionMarker(root, prepared.Journal.Snapshot().ID, true); err != nil {
					t.Fatal(err)
				}
				root.Close()
				if err = prepared.Journal.Transition(0, "applying"); err != nil {
					t.Fatal(err)
				}
				for i, file := range prepared.Journal.Snapshot().Plan.Files {
					if file.Action != "unchanged" {
						if err = ApplyUpgradeFile(r.Installation, prepared.Staged, prepared.Journal, i); err != nil {
							t.Fatal(err)
						}
						break
					}
				}
				if phase == "restoring" {
					if err = prepared.Journal.Transition(prepared.Journal.Snapshot().Revision, "restoring"); err != nil {
						t.Fatal(err)
					}
				}
			}
			if phase == "damaged-history" {
				if err = os.WriteFile(filepath.Join(work, "journal.json"), []byte("partial json"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if phase == "foreign-history" {
				if err = os.WriteFile(filepath.Join(work, "owner.json"), []byte(`{"Installation":"another-copy"}`), 0600); err != nil {
					t.Fatal(err)
				}
			}
			before, err := os.ReadFile(filepath.Join(work, "journal.json"))
			if err != nil {
				t.Fatal(err)
			}
			// A genuinely different same-version payload supersedes the old
			// journal, rather than merely resuming the old requested installation.
			bundle, license := fixture(t)
			if err = os.WriteFile(filepath.Join(bundle, "WinOne.exe"), []byte("current installer after old failed attempt"), 0600); err != nil {
				t.Fatal(err)
			}
			if err = RecordBuild(bundle, license, "0.1.0-alpha.2"); err != nil {
				t.Fatal(err)
			}
			currentPackage := filepath.Join(t.TempDir(), "current.zip")
			if err = Build(bundle, license, currentPackage, "0.1.0-alpha.2"); err != nil {
				t.Fatal(err)
			}
			update.Staged = extractTestPackage(t, currentPackage)
			uninstallerFixture(t, update.Staged, "current uninstall")
			if err = os.WriteFile(filepath.Join(update.Staged, "WinOne.lnk"), r.NextShortcut.Data, 0600); err != nil {
				t.Fatal(err)
			}
			if phase == "damaged-history" {
				// Even partially removed recovery data must not become a new
				// dependency of the self-contained current setup.
				if err = os.Rename(filepath.Join(work, "next.zip"), filepath.Join(work, "retained-old.zip")); err != nil {
					t.Fatal(err)
				}
			}
			if phase == "prepared" {
				if _, err = prepareInstallerUpdate(update, work); err == nil || !strings.Contains(err.Error(), "请使用上次") {
					t.Fatalf("did not reproduce old package binding: %v", err)
				}
			}
			if err = InstallCurrentPackage(update); err != nil {
				t.Fatal(err)
			}
			if _, err = VerifyInstalled(r.Installation, currentPackage); err != nil {
				t.Fatal(err)
			}
			after, err := os.ReadFile(filepath.Join(work, "journal.json"))
			if err != nil || string(before) != string(after) {
				t.Fatal("old recovery evidence changed", err)
			}
		})
	}
}

func TestCurrentInstallerRepairsMissingAndDamagedState(t *testing.T) {
	for _, state := range []string{"registration", "version", "manifest", "program", "uninstaller", "shortcut", "partial-first-install", "partial-uninstall", "missing-profile", "legacy-profile", "corrupt-profile"} {
		t.Run(state, func(t *testing.T) {
			r, update := repairFixture(t)
			profileFile := ""
			switch state {
			case "registration":
				if err := registry.DeleteKey(registry.CURRENT_USER, r.RegistryKey); err != nil {
					t.Fatal(err)
				}
			case "version":
				key, err := registry.OpenKey(registry.CURRENT_USER, r.RegistryKey, registry.SET_VALUE|registry.WOW64_64KEY)
				if err != nil {
					t.Fatal(err)
				}
				err = key.DeleteValue("DisplayVersion")
				key.Close()
				if err != nil {
					t.Fatal(err)
				}
			case "manifest", "program", "uninstaller":
				name := map[string]string{"manifest": "manifest.json", "program": "WinOne.exe", "uninstaller": "Uninstall.exe"}[state]
				if err := os.Rename(filepath.Join(r.Installation, name), filepath.Join(t.TempDir(), name)); err != nil {
					t.Fatal(err)
				}
			case "shortcut":
				if err := os.WriteFile(r.ShortcutPath, []byte("damaged shortcut"), 0600); err != nil {
					t.Fatal(err)
				}
			case "partial-first-install", "partial-uninstall":
				if err := os.WriteFile(filepath.Join(r.Installation, ".winone-pending"), nil, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(r.Installation, ".winone-recovery.ini"), []byte("old package recovery"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(r.Installation, "WinOne.exe"), []byte("partial bytes"), 0600); err != nil {
					t.Fatal(err)
				}
			case "missing-profile":
				update.Profile = filepath.Join(t.TempDir(), "missing", "profile")
			case "legacy-profile", "corrupt-profile":
				name := "configuration.json"
				if state == "legacy-profile" {
					name = "settings.json"
				}
				profileFile = filepath.Join(r.Profile, name)
				if err := os.WriteFile(profileFile, []byte("retained user configuration"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := InstallCurrentPackage(update); err != nil {
				t.Fatal(err)
			}
			if _, err := VerifyInstalled(r.Installation, r.NextPackage); err != nil {
				t.Fatal(err)
			}
			registration, err := CaptureRegistration(r.Installation, r.RegistryKey)
			if err != nil || !registration.Exists {
				t.Fatal("registration not repaired", err)
			}
			link, err := CaptureShortcut(r.ShortcutPath)
			if err != nil || !link.Exists {
				t.Fatal("shortcut not repaired", err)
			}
			if profileFile != "" {
				data, err := os.ReadFile(profileFile)
				if err != nil || string(data) != "retained user configuration" {
					t.Fatal("configuration changed", err)
				}
			}
			data, err := os.ReadFile(filepath.Join(r.Installation, "notes.txt"))
			if err != nil || string(data) != "user data" {
				t.Fatal("extra file changed", err)
			}
		})
	}
}

func TestCurrentInstallerRollsBackAndAcceptsAnotherAttempt(t *testing.T) {
	r, update := repairFixture(t)
	update.CheckStartup = func() error { return errors.New("bad new startup") }
	if err := InstallCurrentPackage(update); err == nil {
		t.Fatal("failed startup accepted")
	}
	if _, err := VerifyInstalled(r.Installation, r.PreviousPackage); err != nil {
		t.Fatal("rollback", err)
	}
	update.CheckStartup = func() error { return nil }
	if err := InstallCurrentPackage(update); err != nil {
		t.Fatal("retry", err)
	}
	if _, err := VerifyInstalled(r.Installation, r.NextPackage); err != nil {
		t.Fatal(err)
	}
}

func TestCurrentInstallerPreservesLockedFile(t *testing.T) {
	r, update := repairFixture(t)
	name, _ := windows.UTF16PtrFromString(filepath.Join(r.Installation, "WinOne.exe"))
	handle, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	err = InstallCurrentPackage(update)
	windows.CloseHandle(handle)
	if err == nil {
		t.Fatal("locked executable reported installed")
	}
	if _, err = VerifyInstalled(r.Installation, r.PreviousPackage); err != nil {
		t.Fatal("rollback after lock", err)
	}
	if err = InstallCurrentPackage(update); err != nil {
		t.Fatal("retry after release", err)
	}
}
