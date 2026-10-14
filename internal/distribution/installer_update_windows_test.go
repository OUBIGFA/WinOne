//go:build windows

package distribution

import (
	"errors"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallerEntryReinstallPreservesDataAndRollsBackFailure(t *testing.T) {
	if os.Getenv("WINONE_REGISTRY_LIVE_TEST") != "1" {
		t.Skip("explicit isolated registry test")
	}
	for _, broken := range []bool{false, true} {
		t.Run(map[bool]string{false: "replace", true: "rollback"}[broken], func(t *testing.T) {
			req := upgradeExecutionFixture(t, true)
			// Rebuild a different payload at the installed version, as local 0.0.1
			// development installers do. The old archive need not accompany setup.
			root, license := fixture(t)
			if err := os.WriteFile(filepath.Join(root, "WinOne.exe"), []byte("local fixed program"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := RecordBuild(root, license, "0.1.0-alpha.1"); err != nil {
				t.Fatal(err)
			}
			next := filepath.Join(t.TempDir(), "same.zip")
			if err := Build(root, license, next, "0.1.0-alpha.1"); err != nil {
				t.Fatal(err)
			}
			staged := extractTestPackage(t, next)
			uninstallerFixture(t, staged, "replacement uninstall")
			keep := filepath.Join(req.Profile, "configuration.json")
			if err := os.WriteFile(keep, []byte(`{"keep":true}`), 0600); err != nil {
				t.Fatal(err)
			}
			// This is a file-transaction fixture; native profile validation is
			// exercised separately with a real Host, not these placeholder bytes.
			update := InstallerUpdate{Installation: req.Installation, Profile: req.Profile, Staged: staged, RegistryKey: req.RegistryKey, ShortcutPath: req.ShortcutPath, Prepare: func() error { return nil }, CheckConfiguration: func() error { return nil }, CheckStartup: func() error {
				if broken {
					return errors.New("startup failed")
				}
				return nil
			}}
			err := RunInstallerUpdate(update)
			if (err != nil) != broken {
				t.Fatalf("update error: %v", err)
			}
			want := next
			if broken {
				want = req.PreviousPackage
			}
			if _, err := VerifyInstalled(req.Installation, want); err != nil {
				t.Fatal(err)
			}
			for path, want := range map[string]string{keep: `{"keep":true}`, filepath.Join(req.Installation, "notes.txt"): "user data"} {
				raw, err := os.ReadFile(path)
				if err != nil || string(raw) != want {
					t.Fatal("retained data changed", path, err)
				}
			}
			if !broken {
				// WebView2 caches or a diagnostic reader can retain a completed
				// transaction directory. Reinstall must not depend on renaming it.
				workName, _ := windows.UTF16PtrFromString(req.Installation + ".update")
				retained, err := windows.CreateFile(workName, windows.GENERIC_READ,
					windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
				if err != nil {
					t.Fatal(err)
				}
				defer windows.CloseHandle(retained)
				if err := RunInstallerUpdate(update); err != nil {
					t.Fatal("identical reinstall", err)
				}
				raw, err := os.ReadFile(filepath.Join(req.Installation, "Uninstall.exe"))
				if err != nil || string(raw) != "replacement uninstall" {
					t.Fatal("uninstaller not replaced", err)
				}
			}
			entry, err := registry.OpenKey(registry.CURRENT_USER, req.RegistryKey, registry.QUERY_VALUE|registry.WOW64_64KEY)
			if err != nil {
				t.Fatal(err)
			}
			defer entry.Close()
			version, _, err := entry.GetStringValue("DisplayVersion")
			if err != nil || version != "0.1.0-alpha.1" {
				t.Fatal("version changed", version, err)
			}
		})
	}
}

func TestInstallerPreparationFailureCanRetryWithoutChangingInstallation(t *testing.T) {
	if os.Getenv("WINONE_REGISTRY_LIVE_TEST") != "1" {
		t.Skip("explicit isolated registry test")
	}
	req := upgradeExecutionFixture(t, true)
	staged := extractTestPackage(t, req.NextPackage)
	update := InstallerUpdate{Installation: req.Installation, Profile: req.Profile, Staged: staged, RegistryKey: req.RegistryKey, ShortcutPath: req.ShortcutPath, Prepare: func() error { return nil }, CheckStartup: func() error { return nil }}
	if err := RunInstallerUpdate(update); err == nil {
		t.Fatal("missing candidate uninstaller accepted")
	}
	if _, err := VerifyInstalled(req.Installation, req.PreviousPackage); err != nil {
		t.Fatal(err)
	}
	uninstallerFixture(t, staged, "fixed uninstall")
	if err := RunInstallerUpdate(update); err != nil {
		t.Fatalf("preparation retry failed: %v", err)
	}
	if _, err := VerifyInstalled(req.Installation, req.NextPackage); err != nil {
		t.Fatal(err)
	}
}

func TestInstallerEntryDoesNotOverwriteModifiedOrUnregisteredFiles(t *testing.T) {
	if os.Getenv("WINONE_REGISTRY_LIVE_TEST") != "1" {
		t.Skip("explicit isolated registry test")
	}
	for _, missingRegistration := range []bool{false, true} {
		t.Run(map[bool]string{false: "changed-file", true: "missing-registration"}[missingRegistration], func(t *testing.T) {
			req := upgradeExecutionFixture(t, true)
			program := filepath.Join(req.Installation, "WinOne.exe")
			before, err := os.ReadFile(program)
			if err != nil {
				t.Fatal(err)
			}
			if missingRegistration {
				if err = registry.DeleteKey(registry.CURRENT_USER, req.RegistryKey); err != nil {
					t.Fatal(err)
				}
			} else {
				before = []byte("user modified executable")
				if err = os.WriteFile(program, before, 0600); err != nil {
					t.Fatal(err)
				}
			}
			called := false
			err = RunInstallerUpdate(InstallerUpdate{Installation: req.Installation, Profile: req.Profile, Staged: req.Staged, RegistryKey: req.RegistryKey, ShortcutPath: req.ShortcutPath, Prepare: func() error { called = true; return nil }, CheckStartup: func() error { return nil }})
			if err == nil || called {
				t.Fatal("invalid source entered update", err)
			}
			after, err := os.ReadFile(program)
			if err != nil || string(after) != string(before) {
				t.Fatal("original file overwritten", err)
			}
		})
	}
}

func TestInstallerWorkFindsUnfinishedTransactionAfterHistoryGap(t *testing.T) {
	if os.Getenv("WINONE_REGISTRY_LIVE_TEST") != "1" {
		t.Skip("explicit isolated registry test")
	}
	r := upgradeExecutionFixture(t, true)
	update := InstallerUpdate{Installation: r.Installation, Profile: r.Profile, Staged: r.Staged, RegistryKey: r.RegistryKey, ShortcutPath: r.ShortcutPath}
	base := r.Installation + ".update"
	if _, err := prepareInstallerUpdate(update, base+"-2"); err != nil {
		t.Fatal(err)
	}
	selected, err := installerWorkDirectory(update, base)
	if err != nil || selected != base+"-2" {
		t.Fatal("missing old history hid the active transaction", selected, err)
	}
}

func TestInstallerEntryResumesInterruptedPayloadChange(t *testing.T) {
	if os.Getenv("WINONE_REGISTRY_LIVE_TEST") != "1" {
		t.Skip("explicit isolated registry test")
	}
	req := upgradeExecutionFixture(t, true)
	update := InstallerUpdate{Installation: req.Installation, Profile: req.Profile, Staged: req.Staged, RegistryKey: req.RegistryKey, ShortcutPath: req.ShortcutPath, Prepare: func() error { return nil }, CheckStartup: func() error { return nil }}
	prepared, err := prepareInstallerUpdate(update, req.Installation+".update")
	if err != nil {
		t.Fatal(err)
	}
	root, err := openUpgradeRoot(req.Installation)
	if err != nil {
		t.Fatal(err)
	}
	if err = ensureExecutionMarker(root, prepared.Journal.Snapshot().ID, true); err != nil {
		t.Fatal(err)
	}
	root.Close()
	if err = prepared.Journal.Transition(prepared.Journal.Snapshot().Revision, "applying"); err != nil {
		t.Fatal(err)
	}
	for i, file := range prepared.Journal.Snapshot().Plan.Files {
		if file.Action != "unchanged" {
			if err = ApplyUpgradeFile(req.Installation, prepared.Staged, prepared.Journal, i); err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	// No destructor rolls this transaction back: this is the durable state left
	// when an installer is killed after one file has been replaced.
	if err = RunInstallerUpdate(update); err != nil {
		t.Fatal("resume", err)
	}
	if _, err = VerifyInstalled(req.Installation, req.NextPackage); err != nil {
		t.Fatal(err)
	}
}
