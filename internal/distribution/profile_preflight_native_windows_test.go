//go:build windows

package distribution

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"winone/internal/modulestate"
	"winone/internal/startupprobe"
)

// Opt-in extension of the full NSIS overwrite test. The production installer
// must preserve the old installation on preflight failure, then run its actual
// worker/UI checks once the injected unfinished state is released.
func checkInstalledProfilePreflight(t *testing.T, installer, installation, profile, key, shortcut, previous string) func() {
	t.Helper()
	fixture := os.Getenv("WINONE_INSTALLER_PROFILE_FIXTURE")
	if fixture == "" {
		return func() {}
	}
	if err := os.CopyFS(profile, os.DirFS(fixture)); err != nil {
		t.Fatal(err)
	}
	state, err := modulestate.Open(filepath.Join(profile, "module-state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = state.SetIntent(state.Snapshot().Revision, "example.tool", false); err != nil {
		t.Fatal(err)
	}
	before, err := startupprobe.ReadProfileSnapshot(profile)
	if err != nil {
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
	runNativeSetupBounded(t, 1, 5*time.Minute, installer, "/S", "/D="+installation)
	if _, err = VerifyInstalled(installation, previous); err != nil {
		t.Fatal("preflight changed installed payload", err)
	}
	journal, err := OpenUpgradeJournal(filepath.Join(installation+".update", "journal.json"))
	if err != nil || journal.Snapshot().Phase != "prepared" {
		t.Fatal("preflight did not retain the prepared transaction", err)
	}
	if _, err = os.Stat(filepath.Join(installation, ".winone-pending")); !os.IsNotExist(err) {
		t.Fatal("preflight left old program blocked", err)
	}
	after, err := startupprobe.ReadProfileSnapshot(profile)
	if err != nil || after.Digest() != before.Digest() {
		t.Fatal("preflight changed profile", err)
	}
	actualRegistration, err := CaptureRegistration(installation, key)
	if err != nil || !sameRegistration(actualRegistration, registration) {
		t.Fatal("preflight changed registration", err)
	}
	actualLink, err := CaptureShortcut(shortcut)
	if err != nil || actualLink.Exists != link.Exists || actualLink.SHA256 != link.SHA256 {
		t.Fatal("preflight changed shortcut", err)
	}
	// This test never executed the optional payload. Only remove its injected
	// pending-stop marker; production recovery remains the Host's responsibility.
	if _, err = state.FinishStop("example.tool"); err != nil {
		t.Fatal(err)
	}
	stable, err := startupprobe.ReadProfileSnapshot(profile)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("NSIS preflight rejected unfinished module recovery and preserved payload, registration, shortcut and profile")
	return func() {
		t.Helper()
		after, err := startupprobe.ReadProfileSnapshot(profile)
		if err != nil || after.Digest() != stable.Digest() {
			t.Fatal("successful overwrite changed profile controls", err)
		}
		raw, err := os.ReadFile(filepath.Join(installation+".update", "profile-check.json"))
		if err != nil {
			t.Fatal("full installer did not retain the profile check", err)
		}
		var report startupprobe.Report
		if err = json.Unmarshal(raw, &report); err != nil {
			t.Fatal(err)
		}
		if report.ProfileSHA256 != stable.Digest() || report.ProfileIssueCount != 1 || len(report.ProfileIssues) != 1 || report.ProfileIssues[0].ModuleID != "example.tool" {
			t.Fatal("untrusted fixture module was not explicitly quarantined", report)
		}
		if _, err = os.Stat(filepath.Join(profile, "modules", "example.tool")); !os.IsNotExist(err) {
			t.Fatal("installer ran or projected the quarantined module", err)
		}
		t.Log("NSIS retry and repeated overwrite preserved quarantined module data; actual worker and UI startup checks passed")
	}
}

// Uses the real target Host checker inside the installer transaction. Payload
// and UI bodies are inert fixtures; normal worker/UI startup is tested elsewhere.
func TestInstallerProfilePreflightStopsBeforeReplacementAndCanRetry(t *testing.T) {
	executable, profile := os.Getenv("WINONE_PROFILE_TARGET_EXE"), os.Getenv("WINONE_PROFILE_FIXTURE")
	if executable == "" || profile == "" {
		t.Skip("explicit target Host and signed-package profile")
	}
	request := upgradeExecutionFixtureUsingHost(t, executable, true)
	if err := os.CopyFS(request.Profile, os.DirFS(profile)); err != nil {
		t.Fatal(err)
	}
	state, err := modulestate.Open(filepath.Join(request.Profile, "module-state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = state.SetIntent(state.Snapshot().Revision, "example.tool", false); err != nil {
		t.Fatal(err)
	}
	before, err := startupprobe.ReadProfileSnapshot(request.Profile)
	if err != nil {
		t.Fatal(err)
	}
	prepared, started := false, false
	update := InstallerUpdate{Installation: request.Installation, Profile: request.Profile, Staged: request.Staged, RegistryKey: request.RegistryKey, ShortcutPath: request.ShortcutPath, Prepare: func() error { prepared = true; return nil }, CheckStartup: func() error { started = true; return nil }}
	err = RunInstallerUpdate(update)
	if err == nil || !strings.Contains(err.Error(), "example.tool") || prepared || started {
		t.Fatal("installer did not fail in profile preflight", err, prepared, started)
	}
	if _, err = VerifyInstalled(request.Installation, request.PreviousPackage); err != nil {
		t.Fatal("preflight changed old payload", err)
	}
	journal, err := OpenUpgradeJournal(filepath.Join(request.Installation+".update", "journal.json"))
	if err != nil || journal.Snapshot().Phase != "prepared" {
		t.Fatal("preflight changed transaction phase", err)
	}
	if _, err = os.Stat(filepath.Join(request.Installation, ".winone-pending")); !os.IsNotExist(err) {
		t.Fatal("old program remained blocked", err)
	}
	after, _ := startupprobe.ReadProfileSnapshot(request.Profile)
	if after.Digest() != before.Digest() {
		t.Fatal("profile preflight modified data")
	}
	// No worker was ever started in this fixture. Release only its injected marker.
	if _, err = state.FinishStop("example.tool"); err != nil {
		t.Fatal(err)
	}
	if err = RunInstallerUpdate(update); err != nil || !prepared || !started {
		t.Fatal("installer did not retry after state became valid", err)
	}
	if _, err = VerifyInstalled(request.Installation, request.NextPackage); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(request.Profile, "modules", "example.tool")); !os.IsNotExist(err) {
		t.Fatal("preflight executed or projected the optional module", err)
	}
}
