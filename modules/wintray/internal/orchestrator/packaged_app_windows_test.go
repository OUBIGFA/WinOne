//go:build windows

package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"winone/modules/wintray/internal/config"
)

func TestPackagedAppWindowKeepsFileAndPackageIdentity(t *testing.T) {
	path := "shell:AppsFolder\\Example.App_123456789abcd!Reader"
	window := ManagedWindowInfo{ProcessPath: "C:/Apps/Reader.exe", ApplicationID: "Example.App_123456789abcd!Reader"}
	if !matchesExecutable(window, path, "Reader") || !hasTrustedWindowIdentity(window, path, nil) {
		t.Fatal("window did not match its registered application ID")
	}
	if !matchesExecutable(window, normalizePath("C:/Apps/Reader.exe"), "Reader") {
		t.Fatal("package identity hid the executable identity")
	}
	if matchesExecutable(window, "shell:AppsFolder\\Example.App_123456789abcd!Editor", "Reader") {
		t.Fatal("another application from the same package matched")
	}
}

func TestPackagedAppWindowHidesWithoutClosing(t *testing.T) {
	window := ManagedWindowInfo{Handle: 0x501, ProcessID: 1, ApplicationID: "Example.App_123456789abcd!Reader", Title: "Reader"}
	manager := &testManager{}
	service := NewService(&testEnumerator{}, manager, &testLogger{})
	managed, applied := service.tryManageAndVerify(context.Background(), window, closeAllowedScoreThreshold, "close")
	if !applied || !managed.Hidden || len(manager.hideCalls) != 1 || len(manager.closeCalls) != 0 || len(manager.escapeCalls) != 0 {
		t.Fatalf("store app was closed instead of hosted: managed=%+v applied=%t manager=%+v", managed, applied, manager)
	}
}

func TestPackagedAppIdentityStaysVirtual(t *testing.T) {
	path := "shell:AppsFolder\\Example.App_123456789abcd!Reader"
	if got := normalizePath(path); got != path {
		t.Fatalf("application ID became a filesystem path: %q", got)
	}
}

func TestPackagedAppCannotMatchAnotherAppByName(t *testing.T) {
	target := "shell:AppsFolder\\Example.App_123456789abcd!Reader"
	other := ManagedWindowInfo{ProcessPath: "shell:AppsFolder\\Example.App_123456789abcd!Editor", ProcessName: "Reader", Title: "Reader"}
	if matchesExecutableWithIdentityFallback(other, target, "Reader") {
		t.Fatal("packaged app matched a different app through its shared process name or title")
	}
}

func TestPackagedAppLaunchUsesActivation(t *testing.T) {
	entry := config.ManagedAppEntry{ExePath: "shell:AppsFolder\\WinTray.MissingTestApp_123456789abcd!App", Args: "--name \"two words\"", WorkingDirectory: "Z:/missing-directory"}
	process, err := startConfiguredProcess(entry, launchVisible)
	if process != nil || err == nil || !strings.Contains(err.Error(), "activate packaged application") {
		t.Fatalf("missing app must reach Windows activation, not file launch: process=%v err=%v", process, err)
	}
}

// Activation supplies a PID rather than an exec.Cmd; the integration must
// still wait for its actual process and release its owned handle normally.
func TestPackagedAppActivationKeepsProcessWait(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	release := filepath.Join(t.TempDir(), "release-helper")
	child, err := startConfiguredProcess(config.ManagedAppEntry{ExePath: executable, Args: runnerHelperArgs("wait", release)}, launchNoWindow)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = child.Process.Kill(); _ = child.Wait() })
	previous := activatePackagedApplication
	entry := config.ManagedAppEntry{ExePath: `shell:AppsFolder\Example.Reader_123456789abcd!App`, Args: `--profile "two words"`}
	activatePackagedApplication = func(path, arguments string) (uint32, error) {
		if path != entry.ExePath || arguments != entry.Args {
			t.Fatalf("activation changed its identity or arguments: %q %q", path, arguments)
		}
		return uint32(child.Process.Pid), nil
	}
	t.Cleanup(func() { activatePackagedApplication = previous })
	activated, err := startConfiguredProcess(entry, launchVisible)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(release, nil, 0600); err != nil {
		t.Fatal(err)
	}
	waitRunnerCommand(t, activated)
}
