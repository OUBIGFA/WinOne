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
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
	"unicode/utf16"
	"winone/internal/maintenance"
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
	// Avoid the generic setup.exe name, which activates Windows compatibility
	// shims and is flagged by NSIS. Product installers already use versioned names.
	setup, uninstall := filepath.Join(work, "WinOne-fixture.exe"), filepath.Join(work, "exported-uninstall.exe")
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	updateHelper := filepath.Join(work, "WinOne.Update.exe")
	build := exec.CommandContext(ctx, filepath.Join(runtime.GOROOT(), "bin", "go.exe"), "build", "-trimpath", "-ldflags=-H windowsgui", "-o", updateHelper, "./build/installer-update")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("update helper: %v: %s", err, output)
	}
	onboardingHelper := filepath.Join(work, "WinOne.Onboarding.exe")
	// All installer versions in this scenario share one simulated machine.
	onboardingDirectory := filepath.Join(filepath.Dir(profile), "machine", "WinOne", "onboarding")
	build = exec.CommandContext(ctx, filepath.Join(runtime.GOROOT(), "bin", "go.exe"), "build", "-trimpath", "-ldflags=-H windowsgui -X \"winone/internal/onboarding.machineDirectoryOverride="+filepath.ToSlash(onboardingDirectory)+"\"", "-o", onboardingHelper, "./build/installer-onboarding")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("isolated onboarding helper: %v: %s", err, output)
	}
	// Match build-installer.ps1: compiler File sources need the Unicode path
	// namespace even though the generated installer is itself long-path aware.
	source := payload
	if !strings.HasPrefix(source, `\\?\`) {
		if strings.HasPrefix(source, `\\`) {
			source = `\\?\UNC\` + source[2:]
		} else {
			source = `\\?\` + source
		}
	}
	cmd := exec.CommandContext(ctx, compiler, "/V2", "/INPUTCHARSET", "UTF8", "/DWINONE_TEST_BUILD", "/DWINONE_EXPORT_UNINSTALLER",
		"/DWINONE_UPDATE_HELPER="+updateHelper,
		"/DWINONE_ONBOARDING_HELPER="+onboardingHelper,
		"/DWINONE_PAYLOAD_DIR="+source, "/DWINONE_FILES_NSH="+macroFile, "/DWINONE_INSTALLER_OUT="+setup,
		"/DWINONE_VERSION="+manifest.Version, "/DWINONE_MANIFEST_SHA256="+hex.EncodeToString(sum[:]),
		"/DWINONE_ICON="+filepath.Join(root, "internal", "branding", "icon.ico"),
		"/DWINONE_REGISTRY_KEY="+key, "/DWINONE_PROFILE_DIR="+profile, "/DWINONE_SHORTCUT="+shortcut,
		filepath.Join(root, "build", "windows", "installer.nsi"))
	cmd.Env = append(os.Environ(), "WINONE_UNINSTALLER_EXPORT="+uninstall)
	started := time.Now()
	t.Logf("compiling installer %s", manifest.Version)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("compile actual installer: %v; context=%v: %s", err, ctx.Err(), output)
	}
	t.Logf("compiled installer %s in %.2fs", manifest.Version, time.Since(started).Seconds())
	return setup, uninstall, payload
}

func runNativeSetup(t *testing.T, executable string, args ...string) {
	runNativeSetupResult(t, 0, executable, args...)
}

func runNativeSetupResult(t *testing.T, want int, executable string, args ...string) {
	runNativeSetupBounded(t, want, 120*time.Second, executable, args...)
}

func runNativeSetupBounded(t *testing.T, want int, limit time.Duration, executable string, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()
	// NSIS consumes the final /D= or _?= directory as raw command-line text.
	// Standard Go argument quoting changes its target when paths contain spaces.
	// https://nsis.sourceforge.io/Docs/Chapter3.html#installerusage
	if len(args) == 0 {
		t.Fatal("installer directory argument missing")
	}
	directory := args[len(args)-1]
	if !(strings.HasPrefix(directory, "/D=") || strings.HasPrefix(directory, "_?=")) || strings.ContainsAny(directory, "\"\r\n\x00") || !filepath.IsAbs(directory[3:]) {
		t.Fatal("invalid trailing installer directory argument")
	}
	commandLine := syscall.EscapeArg(executable)
	for _, arg := range args[:len(args)-1] {
		commandLine += " " + syscall.EscapeArg(arg)
	}
	cmd := exec.CommandContext(ctx, executable)
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: commandLine + " " + directory, HideWindow: true}
	output, runErr := cmd.CombinedOutput()
	code := -1
	if cmd.ProcessState != nil {
		code = cmd.ProcessState.ExitCode()
	}
	if code != want || (runErr != nil && code == 0) {
		diagnostic, readErr := os.ReadFile(filepath.Join(filepath.Dir(directory[3:]), "profile.installer-failure.txt"))
		if raw, err := os.ReadFile(filepath.Join(filepath.Dir(directory[3:]), "profile", "installer-error.txt")); err == nil {
			units := make([]uint16, len(raw)/2)
			for i := range units {
				units[i] = binary.LittleEndian.Uint16(raw[i*2:])
			}
			diagnostic = []byte(string(utf16.Decode(units)))
			readErr = nil
		}
		if raw, err := os.ReadFile(filepath.Join(filepath.Dir(directory[3:]), "profile.onboarding-failure.txt")); err == nil {
			diagnostic = append(diagnostic, raw...)
			readErr = nil
		}
		t.Fatalf("installer operation: %v (context=%v exit=%d want=%d): %s; failure detail=%s (read=%v)", runErr, ctx.Err(), code, want, output, diagnostic, readErr)
	}
}

// Exercises the installer users launch, not just the Go transaction underneath.
// Both packages deliberately use the same version while their Host bytes differ.
func TestNativeInstallerSameVersionOverwrite(t *testing.T) {
	compiler, previous, next, license := os.Getenv("WINONE_NSIS_EXE"), os.Getenv("WINONE_UPGRADE_OLD_BUNDLE"), os.Getenv("WINONE_UPGRADE_NEW_BUNDLE"), os.Getenv("WINONE_UPGRADE_LICENSE")
	if compiler == "" || previous == "" || next == "" || license == "" {
		t.Skip("explicit full installer integration")
	}
	// Installation locks also derive from LOCALAPPDATA. Keep them alongside
	// the isolated profile rather than touching the user's real WinOne folder.
	t.Setenv("LOCALAPPDATA", t.TempDir())
	if err := maintenance.RequireStopped(); err != nil {
		t.Fatal(err)
	}
	for _, broken := range []bool{false, true} {
		t.Run(map[bool]string{false: "overwrite-and-repeat", true: "startup-failure-rollback"}[broken], func(t *testing.T) {
			root := t.TempDir()
			if evidence := os.Getenv("WINONE_INSTALLER_EVIDENCE_DIR"); evidence != "" {
				root = filepath.Join(evidence, map[bool]string{false: "overwrite", true: "rollback"}[broken])
				if err := os.MkdirAll(root, 0700); err != nil {
					t.Fatal(err)
				}
			}
			installation, profile, shortcut := filepath.Join(root, "installed"), filepath.Join(root, "profile"), filepath.Join(root, "WinOne.lnk")
			if err := os.Mkdir(profile, 0700); err != nil {
				t.Fatal(err)
			}
			key := fmt.Sprintf(`Software\WinOneInstallerTests\entry-%d`, time.Now().UnixNano())
			t.Cleanup(func() {
				if !t.Failed() || os.Getenv("WINONE_INSTALLER_EVIDENCE_DIR") == "" {
					registry.DeleteKey(registry.CURRENT_USER, key)
				}
			})
			oldPackage := os.Getenv("WINONE_INSTALLER_OLD_PACKAGE")
			if oldPackage == "" {
				oldPackage = nativeUpgradePackage(t, previous, license, "0.0.1", false)
			}
			nextPackage := os.Getenv("WINONE_INSTALLER_NEW_PACKAGE")
			if nextPackage == "" || broken {
				nextPackage = nativeUpgradePackage(t, next, license, "0.0.1", broken)
			}
			oldInstaller, _, _ := compileNativeUpgradeInstaller(t, compiler, oldPackage, key, profile, shortcut)
			nextInstaller, _, _ := compileNativeUpgradeInstaller(t, compiler, nextPackage, key, profile, shortcut)
			if os.Getenv("WINONE_INSTALLER_EVIDENCE_DIR") != "" {
				for name, source := range map[string]string{"old-setup.exe": oldInstaller, "next-setup.exe": nextInstaller} {
					raw, err := os.ReadFile(source)
					if err != nil {
						t.Fatal(err)
					}
					if err = os.WriteFile(filepath.Join(root, name), raw, 0700); err != nil {
						t.Fatal(err)
					}
				}
			}
			runNativeSetup(t, oldInstaller, "/S", "/D="+installation)
			hintMarker := filepath.Join(root, "machine", "WinOne", "onboarding", "result-send-hint-v1.once")
			for _, path := range []string{filepath.Join(profile, "keep.txt"), filepath.Join(installation, "keep.txt"), hintMarker} {
				if err := os.WriteFile(path, []byte("keep"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			verifyProfile := func() {}
			if !broken {
				verifyProfile = checkInstalledProfilePreflight(t, nextInstaller, installation, profile, key, shortcut, oldPackage)
			}
			want := 0
			if broken {
				want = 1
			}
			// This entry includes reconstruction, durable full-package backup,
			// staging and transaction checks. The old 120s covered only NSIS file
			// extraction; the native startup probe retains its own 120s deadline.
			runNativeSetupBounded(t, want, 5*time.Minute, nextInstaller, "/S", "/D="+installation)
			if raw, err := os.ReadFile(hintMarker); err != nil || string(raw) != "keep" {
				t.Fatal("upgrade/rollback reset the machine hint", err)
			}
			journal, err := OpenUpgradeJournal(installation + ".update" + string(filepath.Separator) + "journal.json")
			if err != nil {
				t.Fatal("installer transaction missing", err)
			}
			phase := "committed"
			if broken {
				phase = "rolled-back"
			}
			if journal.Snapshot().Phase != phase {
				t.Fatal("installer did not finish the expected transaction", journal.Snapshot().Phase)
			}
			reference := nextPackage
			if broken {
				reference = oldPackage
			}
			if _, err := VerifyInstalled(installation, reference); err != nil {
				t.Fatal(err)
			}
			if !broken {
				runNativeSetupBounded(t, 0, 5*time.Minute, nextInstaller, "/S", "/D="+installation)
				verifyProfile()
			}
			raw, err := os.ReadFile(filepath.Join(installation, "Uninstall.exe"))
			if err != nil {
				t.Fatal(err)
			}
			runner := filepath.Join(root, "run-uninstall.exe")
			if err = os.WriteFile(runner, raw, 0700); err != nil {
				t.Fatal(err)
			}
			runNativeSetup(t, runner, "/S", "_?="+installation)
			remaining, err := os.ReadDir(installation)
			if err != nil || len(remaining) != 1 || remaining[0].Name() != "keep.txt" {
				t.Fatal("uninstall retained payload", remaining, err)
			}
			for _, path := range []string{filepath.Join(profile, "keep.txt"), filepath.Join(installation, "keep.txt"), hintMarker} {
				raw, err := os.ReadFile(path)
				if err != nil || string(raw) != "keep" {
					t.Fatal("settings changed", err)
				}
			}
			if _, err := os.Stat(shortcut); !os.IsNotExist(err) {
				t.Fatal("shortcut retained", err)
			}
			entry, err := registry.OpenKey(registry.CURRENT_USER, key, registry.READ|registry.WOW64_64KEY)
			if err == nil {
				entry.Close()
				t.Fatal("registration retained")
			}
			if err != registry.ErrNotExist {
				t.Fatal(err)
			}
		})
	}
}

// Both paths use production installation, upgrade and uninstall code, with
// isolated destinations. Version labels belong only to copied test fixtures.
func TestNativeInstalledUpgradeThenUninstall(t *testing.T) {
	compiler, previous, next, license := os.Getenv("WINONE_NSIS_EXE"), os.Getenv("WINONE_UPGRADE_OLD_BUNDLE"), os.Getenv("WINONE_UPGRADE_NEW_BUNDLE"), os.Getenv("WINONE_UPGRADE_LICENSE")
	if compiler == "" || previous == "" || next == "" || license == "" {
		t.Skip("set NSIS and native upgrade bundle/license paths")
	}
	t.Setenv("LOCALAPPDATA", t.TempDir())
	// The installer takes the product-wide maintenance pipe, even for isolated
	// destinations. Fail before hashing/copying/compiling if a real session is up.
	if err := maintenance.RequireStopped(); err != nil {
		t.Fatalf("installer lifecycle requires WinOne to be closed before testing: %v", err)
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
			runNativeSetup(t, oldSetup, "/S", "/D="+installation)
			if _, err := VerifyInstalled(installation, oldPackage); err != nil {
				t.Fatal(err)
			}
			_, nextUninstaller, staged := compileNativeUpgradeInstaller(t, compiler, nextPackage, key, profile, shortcut)
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
			if broken {
				requireBrokenDesktopFailure(t, err, installation)
			}
			if (err != nil) != broken {
				_, restored := VerifyInstalled(installation, oldPackage)
				t.Fatalf("installed upgrade: %v; phase=%s; previous payload verification=%v", err, journal.Snapshot().Phase, restored)
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
