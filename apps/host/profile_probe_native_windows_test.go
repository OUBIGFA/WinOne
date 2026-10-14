//go:build windows

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"winone/internal/maintenance"
	"winone/internal/modulepackage"
	"winone/internal/startupprobe"
)

func TestNativeProfileProbeChecksPackagesDuringMaintenance(t *testing.T) {
	goExe := os.Getenv("WINONE_PROFILE_PROBE_GO")
	if goExe == "" {
		t.Skip("explicit built Host profile probe")
	}
	profile, input, publisher := profileProbeFixture(t)
	workspace, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	output := t.TempDir()
	if keep := os.Getenv("WINONE_PROFILE_PROBE_OUTPUT"); keep != "" {
		output = keep
		if err = os.Mkdir(output, 0700); err != nil {
			t.Fatal(err)
		}
	}
	trustFile := filepath.Join(output, "publishers.json")
	raw, _ := json.Marshal(map[string]any{"publishers": []modulepackage.Publisher{publisher}})
	if err = os.WriteFile(trustFile, raw, 0600); err != nil {
		t.Fatal(err)
	}
	bundle := filepath.Join(output, "bundle")
	build := exec.Command("powershell.exe", "-NoProfile", "-File", filepath.Join(workspace, "build", "build-host.ps1"), "-GoExe", goExe, "-OutputDirectory", bundle, "-ModuleTrustFile", trustFile)
	build.Dir = workspace
	build.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build profile probe: %v: %s", err, out)
	}
	if err = os.CopyFS(filepath.Join(output, "valid-profile"), os.DirFS(profile)); err != nil {
		t.Fatal(err)
	}
	lease, err := maintenance.AcquireMaintenanceForInstallation(profile, bundle)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	marker := filepath.Join(bundle, ".winone-pending")
	if err = os.WriteFile(marker, []byte("owned-probe"), 0600); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(bundle, "WinOne.Host.exe")
	report, err := startupprobe.CheckProfileConfiguration(context.Background(), executable, profile, input.Configuration)
	if err != nil || report.ProfileSHA256 != input.Digest() || report.ProfileIssueCount != 0 {
		t.Fatal("target rejected valid signed configuration", report, err)
	}
	registry, err := modulepackage.OpenRegistry(filepath.Join(profile, "packages"))
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := registry.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(profile, "packages", snapshot.Records["example.tool"].StageID, "worker.exe"), []byte("damaged"), 0600); err != nil {
		t.Fatal(err)
	}
	report, err = startupprobe.CheckProfileConfiguration(context.Background(), executable, profile, input.Configuration)
	if err != nil || report.ProfileIssueCount != 1 {
		t.Fatal("pre-existing damaged module was not preserved as quarantined", report, err)
	}
	for _, args := range [][]string{{"--check-profile", "nonce", "--profile-root", profile, "--headless"}, {"--profile-root", ""}} {
		cmd := exec.Command(executable, args...)
		cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
		if err = cmd.Run(); err == nil {
			t.Fatal("maintenance-only arguments accepted a runtime combination")
		}
	}
	after, err := startupprobe.ReadProfileSnapshot(profile)
	if err != nil || after.Digest() != input.Digest() {
		t.Fatal("profile controls changed", err)
	}
	raw, err = os.ReadFile(marker)
	if err != nil || !bytes.Equal(raw, []byte("owned-probe")) {
		t.Fatal("maintenance marker changed", err)
	}
	if _, err = os.Stat(filepath.Join(profile, "modules", "example.tool")); !os.IsNotExist(err) {
		t.Fatal("profile probe ran the module or wrote a projection", err)
	}
	reportBytes, _ := json.MarshalIndent(report, "", "  ")
	if err = os.WriteFile(filepath.Join(output, "profile-report.json"), reportBytes, 0600); err != nil {
		t.Fatal(err)
	}
	// A failed first install can have only preparation bookkeeping, with no
	// module registry yet. Check it through the actual target's process entry.
	preparation := filepath.Join(output, "preparation-only")
	if err = os.MkdirAll(filepath.Join(preparation, "packages"), 0700); err != nil {
		t.Fatal(err)
	}
	configuration := probeConfiguration(t)
	if err = os.WriteFile(filepath.Join(preparation, "configuration.json"), configuration, 0600); err != nil {
		t.Fatal(err)
	}
	record := snapshot.Records["example.tool"]
	ledger := modulepackage.ReclamationSnapshot{SchemaVersion: 3, Items: map[string]modulepackage.ReclamationItem{
		"0000000000000000-" + record.StageID: {Kind: "preparation", Package: record, ChangeID: "install-interrupted", Phase: "queued"},
	}}
	ledgerBytes, _ := json.Marshal(ledger)
	ledgerPath := filepath.Join(preparation, "packages", ".winone-module-reclaim.json")
	if err = os.WriteFile(ledgerPath, ledgerBytes, 0600); err != nil {
		t.Fatal(err)
	}
	beforePreparation, err := startupprobe.ReadProfileSnapshot(preparation)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = startupprobe.CheckProfileConfiguration(context.Background(), executable, preparation, configuration); err != nil {
		t.Fatal("target rejected valid preparation-only profile", err)
	}
	afterPreparation, err := startupprobe.ReadProfileSnapshot(preparation)
	if err != nil || afterPreparation.Digest() != beforePreparation.Digest() {
		t.Fatal("target changed preparation data", err)
	}
	if err = os.WriteFile(ledgerPath, []byte(`{"schemaVersion":999,"items":{}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = startupprobe.CheckProfileConfiguration(context.Background(), executable, preparation, configuration); err == nil {
		t.Fatal("target ignored invalid preparation-only ledger")
	}
	if err = os.WriteFile(ledgerPath, ledgerBytes, 0600); err != nil {
		t.Fatal(err)
	}
}
