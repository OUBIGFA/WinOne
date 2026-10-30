//go:build windows

package distribution

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"winone/internal/modulestate"
	"winone/internal/startupprobe"
)

// Explicit setup repairs program bytes without making existing profile schema
// or unfinished module state a prerequisite. Preserve every profile byte; the
// automatic upgrade preflight below keeps its stricter compatibility contract.
func checkInstalledProfilePreservation(t *testing.T, profile string) func() {
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
	return func() {
		t.Helper()
		after, err := startupprobe.ReadProfileSnapshot(profile)
		if err != nil || after.Digest() != before.Digest() {
			t.Fatal("manual installation changed user configuration", err)
		}
		if _, err = os.Stat(filepath.Join(profile, "modules", "example.tool")); !os.IsNotExist(err) {
			t.Fatal("installer executed or projected the optional module", err)
		}
		// Release only the fixture's artificial pending-stop for later uninstall.
		if _, err = state.FinishStop("example.tool"); err != nil {
			t.Fatal(err)
		}
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
