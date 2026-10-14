//go:build windows

package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"winone/internal/configuration"
	"winone/internal/modulepackage"
	"winone/internal/modulestate"
	"winone/internal/startupprobe"
)

func profileProbeFixture(t *testing.T) (string, startupprobe.ProfileSnapshot, modulepackage.Publisher) {
	t.Helper()
	root := t.TempDir()
	state, err := modulestate.Open(filepath.Join(root, "module-state.json"))
	if err != nil {
		t.Fatal(err)
	}
	publisher := installHostPackageFixture(t, root, state, []byte("inert payload; probe must not execute it"))
	var data struct {
		Committed configuration.Document `json:"committed"`
	}
	if err = json.Unmarshal(probeConfiguration(t), &data); err != nil {
		t.Fatal(err)
	}
	data.Committed.Values["example.tool"] = json.RawMessage(`{"size":7}`)
	data.Committed.ModuleEnabled["example.tool"] = false
	raw, _ := json.Marshal(data)
	if err = os.WriteFile(filepath.Join(root, "configuration.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := startupprobe.ReadProfileSnapshot(root)
	if err != nil {
		t.Fatal(err)
	}
	return root, snapshot, publisher
}

func TestProfileProbeValidatesSignedSettingsWithoutMutationOrExecution(t *testing.T) {
	root, snapshot, publisher := profileProbeFixture(t)
	trust := func(string) (modulepackage.Publisher, error) { return publisher, nil }
	issues, err := checkProfileUpgrade(context.Background(), root, snapshot, packageTestEnvironment, trust)
	if err != nil || len(issues) != 0 {
		t.Fatal("valid optional settings were rejected", issues, err)
	}
	after, err := startupprobe.ReadProfileSnapshot(root)
	if err != nil || after.Digest() != snapshot.Digest() {
		t.Fatal("readonly probe changed profile state", err)
	}
	if _, err = os.Stat(filepath.Join(root, "modules", "example.tool")); !os.IsNotExist(err) {
		t.Fatal("profile probe projected settings or started a worker", err)
	}
	newer := packageTestEnvironment
	newer.HostVersion = "2.0.0"
	if _, err = checkProfileUpgrade(context.Background(), root, snapshot, newer, trust); err == nil || !strings.Contains(err.Error(), "example.tool") {
		t.Fatal("incompatible target Host was not identified", err)
	}
}

func TestProfileProbeKeepsDamagedExistingModulesIsolated(t *testing.T) {
	root, snapshot, publisher := profileProbeFixture(t)
	registry, err := modulepackage.OpenRegistry(filepath.Join(root, "packages"))
	if err != nil {
		t.Fatal(err)
	}
	records, err := registry.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "packages", records.Records["example.tool"].StageID, "worker.exe")
	if err = os.WriteFile(file, []byte("damaged"), 0600); err != nil {
		t.Fatal(err)
	}
	issues, err := checkProfileUpgrade(context.Background(), root, snapshot, packageTestEnvironment, func(string) (modulepackage.Publisher, error) { return publisher, nil })
	if err != nil || len(issues) != 1 || issues[0].ModuleID != "example.tool" {
		t.Fatal("existing damage blocked all configuration or was hidden", issues, err)
	}
	after, _ := startupprobe.ReadProfileSnapshot(root)
	if after.Digest() != snapshot.Digest() {
		t.Fatal("isolation check changed profile")
	}
}

func TestProfileProbeRejectsPendingPackageWork(t *testing.T) {
	for _, kind := range []string{"stop", "package"} {
		t.Run(kind, func(t *testing.T) {
			root, _, publisher := profileProbeFixture(t)
			state, err := modulestate.Open(filepath.Join(root, "module-state.json"))
			if err != nil {
				t.Fatal(err)
			}
			if kind == "stop" {
				if _, err = state.SetIntent(state.Snapshot().Revision, "example.tool", false); err != nil {
					t.Fatal(err)
				}
			} else {
				registry, err := modulepackage.OpenRegistry(filepath.Join(root, "packages"))
				if err != nil {
					t.Fatal(err)
				}
				before, err := registry.Snapshot()
				if err != nil {
					t.Fatal(err)
				}
				if _, err = registry.BeginChange(context.Background(), modulepackage.ChangeRequest{ID: "pending-test", RegistryRevision: before.Revision, StateRevision: state.Snapshot().Revision, Current: before.Records["example.tool"]}, state); err != nil {
					t.Fatal(err)
				}
			}
			snapshot, err := startupprobe.ReadProfileSnapshot(root)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = checkProfileUpgrade(context.Background(), root, snapshot, packageTestEnvironment, func(string) (modulepackage.Publisher, error) { return publisher, nil }); err == nil {
				t.Fatal("unfinished module work was accepted")
			}
			after, _ := startupprobe.ReadProfileSnapshot(root)
			if after.Digest() != snapshot.Digest() {
				t.Fatal("probe repaired instead of checking")
			}
		})
	}
}

func TestProfileProbeChecksPreparationLedgerWithoutInventory(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "configuration.json"), probeConfiguration(t), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "packages"), 0700); err != nil {
		t.Fatal(err)
	}
	ledger := filepath.Join(root, "packages", ".winone-module-reclaim.json")
	if err := os.WriteFile(ledger, []byte(`{"schemaVersion":999,"items":{}}`), 0600); err != nil {
		t.Fatal(err)
	}
	input, err := startupprobe.ReadProfileSnapshot(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = checkProfileUpgrade(context.Background(), root, input, packageTestEnvironment, nil); err == nil {
		t.Fatal("preparation ledger was ignored when inventory is absent")
	}
}
