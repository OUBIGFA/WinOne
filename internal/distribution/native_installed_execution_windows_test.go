//go:build windows

package distribution

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"golang.org/x/sys/windows/registry"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
)

func compileNativeUpgradeInstaller(t *testing.T, compiler, archive, key, profile, shortcut string) (string, string, string) {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	payload := extractTestPackage(t, archive)
	manifest, err := Verify(archive)
	if err != nil {
		t.Fatal(err)
	}
	macros, err := NSISFileMacros(manifest)
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	macroFile := filepath.Join(work, "files.nsh")
	if err = os.WriteFile(macroFile, []byte(macros), 0600); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(payload, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	setup, uninstall := filepath.Join(work, "setup.exe"), filepath.Join(work, "exported-uninstall.exe")
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, compiler, "/V2", "/INPUTCHARSET", "UTF8", "/DWINONE_TEST_BUILD", "/DWINONE_EXPORT_UNINSTALLER",
		"/DWINONE_PAYLOAD_DIR="+payload, "/DWINONE_FILES_NSH="+macroFile, "/DWINONE_INSTALLER_OUT="+setup,
		"/DWINONE_VERSION="+manifest.Version, "/DWINONE_MANIFEST_SHA256="+hex.EncodeToString(sum[:]),
		"/DWINONE_ICON="+filepath.Join(root, "internal", "branding", "icon.ico"),
		"/DWINONE_REGISTRY_KEY="+key, "/DWINONE_PROFILE_DIR="+profile, "/DWINONE_SHORTCUT="+shortcut,
		filepath.Join(root, "build", "windows", "installer.nsi"))
	cmd.Env = append(os.Environ(), "WINONE_UNINSTALLER_EXPORT="+uninstall)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("compile actual installer: %v: %s", err, output)
	}
	return setup, uninstall, payload
}

func runNativeSetup(t *testing.T, executable string, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	if output, err := exec.CommandContext(ctx, executable, args...).CombinedOutput(); err != nil {
		t.Fatalf("installer operation: %v: %s", err, output)
	}
}

// Both paths use production installation, upgrade and uninstall code, with
// isolated destinations. Version labels belong only to copied test fixtures.
func TestNativeInstalledUpgradeThenUninstall(t *testing.T) {
	compiler, previous, next, license := os.Getenv("WINONE_NSIS_EXE"), os.Getenv("WINONE_UPGRADE_OLD_BUNDLE"), os.Getenv("WINONE_UPGRADE_NEW_BUNDLE"), os.Getenv("WINONE_UPGRADE_LICENSE")
	if compiler == "" || previous == "" || next == "" || license == "" {
		t.Skip("set NSIS and native upgrade bundle/license paths")
	}
	oldPackage := nativeUpgradePackage(t, previous, license, "0.0.0-installed-test.1", false)
	for _, broken := range []bool{false, true} {
		t.Run(fmt.Sprint(broken), func(t *testing.T) {
			nextPackage := nativeUpgradePackage(t, next, license, "0.0.0-installed-test.2", broken)
			base := t.TempDir()
			installation, profile, shortcut := filepath.Join(base, "installed"), filepath.Join(base, "profile"), filepath.Join(base, "WinOne.lnk")
			key := fmt.Sprintf(`Software\WinOneInstalledUpgradeTests\case-%d`, time.Now().UnixNano())
			t.Cleanup(func() { registry.DeleteKey(registry.CURRENT_USER, key) })
			oldSetup, _, _ := compileNativeUpgradeInstaller(t, compiler, oldPackage, key, profile, shortcut)
			_, nextUninstaller, staged := compileNativeUpgradeInstaller(t, compiler, nextPackage, key, profile, shortcut)
			runNativeSetup(t, oldSetup, "/S", "/D="+installation)
			if _, err := VerifyInstalled(installation, oldPackage); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(profile, 0700); err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{filepath.Join(profile, "keep.txt"), filepath.Join(installation, "keep.txt")} {
				if err := os.WriteFile(path, []byte("keep"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			backup := filepath.Join(t.TempDir(), "backup")
			if _, err := BackupInstalled(installation, oldPackage, backup); err != nil {
				t.Fatal(err)
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
			if !registration.Exists {
				t.Fatal("old installer did not register")
			}
			for i, value := range registration.Values {
				if strings.EqualFold(value.Name, "DisplayVersion") {
					encoded := utf16.Encode([]rune("0.0.0-installed-test.2\x00"))
					data := make([]byte, len(encoded)*2)
					for j, v := range encoded {
						binary.LittleEndian.PutUint16(data[j*2:], v)
					}
					registration.Values[i].Data = data
				}
			}
			link, err := CaptureShortcut(shortcut)
			if err != nil || !link.Exists {
				t.Fatal("old shortcut missing", err)
			}
			raw, err := os.ReadFile(nextUninstaller)
			if err != nil {
				t.Fatal(err)
			}
			uninstaller := uninstallerFixture(t, staged, string(raw))
			plan, err := PlanUpgrade(oldPackage, nextPackage)
			if err != nil {
				t.Fatal(err)
			}
			journal, err := CreateUpgradeJournal(filepath.Join(t.TempDir(), "upgrade.json"), plan)
			if err != nil {
				t.Fatal(err)
			}
			err = ExecuteUpgrade(UpgradeExecution{Installation: installation, Profile: profile, Staged: staged, Backup: backup, PreviousPackage: oldPackage, NextPackage: nextPackage, RegistryKey: key, ShortcutPath: shortcut, NextRegistration: registration, NextShortcut: link, NextUninstaller: uninstaller, Journal: journal})
			if (err != nil) != broken {
				t.Fatalf("installed upgrade: %v", err)
			}
			reference, phase, version := nextPackage, "committed", "0.0.0-installed-test.2"
			if broken {
				reference, phase, version = oldPackage, "rolled-back", "0.0.0-installed-test.1"
			}
			if journal.Snapshot().Phase != phase {
				t.Fatal(journal.Snapshot().Phase)
			}
			if _, err := VerifyInstalled(installation, reference); err != nil {
				t.Fatal(err)
			}
			entry, err := registry.OpenKey(registry.CURRENT_USER, key, registry.QUERY_VALUE|registry.WOW64_64KEY)
			if err != nil {
				t.Fatal(err)
			}
			actual, _, err := entry.GetStringValue("DisplayVersion")
			entry.Close()
			if err != nil || actual != version {
				t.Fatal("registration version", actual, err)
			}
			// Execute the file actually left by upgrade/rollback, outside its target.
			raw, err = os.ReadFile(filepath.Join(installation, "Uninstall.exe"))
			if err != nil {
				t.Fatal(err)
			}
			runner := filepath.Join(t.TempDir(), "run-uninstall.exe")
			if err = os.WriteFile(runner, raw, 0700); err != nil {
				t.Fatal(err)
			}
			runNativeSetup(t, runner, "/S", "_?="+installation)
			remaining, err := os.ReadDir(installation)
			if err != nil || len(remaining) != 1 || remaining[0].Name() != "keep.txt" {
				t.Fatal("uninstall file cleanup incomplete", remaining, err)
			}
			if _, err = os.Stat(shortcut); !os.IsNotExist(err) {
				t.Fatal("shortcut remained", err)
			}
			entry, err = registry.OpenKey(registry.CURRENT_USER, key, registry.READ|registry.WOW64_64KEY)
			if err == nil {
				entry.Close()
				t.Fatal("registration remained")
			}
			if err != registry.ErrNotExist {
				t.Fatal(err)
			}
			for _, path := range []string{filepath.Join(profile, "keep.txt"), filepath.Join(installation, "keep.txt")} {
				data, err := os.ReadFile(path)
				if err != nil || string(data) != "keep" {
					t.Fatal("retained file changed", err)
				}
			}
		})
	}
}
