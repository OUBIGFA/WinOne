//go:build windows

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"winone/internal/configuration"
	"winone/internal/modulepackage"
	"winone/internal/modulestate"
)

func hostMigrationArchiveOptions(version string) hostPackageArchiveOptions {
	return hostPackageArchiveOptions{Version: version, SchemaVersion: 2, Schema: `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","properties":{"size":{"type":"integer","minimum":1,"maximum":20},"label":{"type":"string"}},"required":["size","label"],"additionalProperties":false}`, Defaults: `{"size":3,"label":"default"}`, Migrations: []modulepackage.SettingsMigration{{From: 1, To: 2, Patch: "migration.json"}}, Patches: map[string][]byte{"migration.json": []byte(`[{"op":"add","path":"/label","value":"migrated"}]`)}}
}

// Materialize the durable crash point after the new configuration was committed
// but before package FinishChange. The preceding real Host has already restored
// and exited; the next Host must roll back this state, keeping unrelated changes.
func migratedConfigurationCrashFixture(t *testing.T, profile, archive string, publisher modulepackage.Publisher, registry *modulepackage.Registry) (string, []byte) {
	t.Helper()
	ctx := context.Background()
	configFile := filepath.Join(profile, "configuration.json")
	raw, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatal(err)
	}
	var disk struct {
		Committed configuration.Document  `json:"committed"`
		Pending   *configuration.Document `json:"pending,omitempty"`
	}
	if err = json.Unmarshal(raw, &disk); err != nil || disk.Pending != nil {
		t.Fatal("fixture configuration is not committed", err)
	}
	state, err := modulestate.Open(filepath.Join(profile, "module-state.json"))
	if err != nil {
		t.Fatal(err)
	}
	inventory, err := registry.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	checked, err := modulepackage.VerifyArchive(ctx, archive, publisher)
	if err != nil {
		t.Fatal(err)
	}
	approval := modulepackage.StageOptions{Directory: filepath.Join(profile, "packages"), Publisher: publisher, Environment: packageTestEnvironment, ExpectedManifestSHA256: checked.Manifest().Digest()}
	stage, err := modulepackage.StageArchive(ctx, archive, approval)
	if err != nil {
		t.Fatal(err)
	}
	definition, err := stage.LoadSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	values, err := definition.Migrate(1, disk.Committed.Values["example.tool"])
	if err != nil {
		t.Fatal(err)
	}
	before := inventory.Records["example.tool"]
	migration := configuration.PackageMigrationSnapshot{SchemaVersion: 1, OperationID: "interrupted-migration", ModuleID: "example.tool", Revision: disk.Committed.Revision, BeforeSchema: 1, AfterSchema: 2, BeforePackageSHA256: before.ManifestSHA256, AfterPackageSHA256: stage.Manifest().Digest(), Before: disk.Committed.Values["example.tool"], After: values}
	backup, err := json.Marshal(migration)
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(profile, "package-migrations")
	if err = os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(directory, "migration-"+migration.OperationID+".json")
	if err = os.WriteFile(file, backup, 0600); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(backup)
	change := modulepackage.ChangeRequest{ID: migration.OperationID, RegistryRevision: inventory.Revision, StateRevision: state.Snapshot().Revision, Current: before, Replacement: &modulepackage.ReplacementStage{ID: filepath.Base(stage.Directory()), Options: approval}, MigrationSHA256: hex.EncodeToString(hash[:])}
	if _, err = registry.BeginChange(ctx, change, state); err != nil {
		t.Fatal(err)
	}
	if _, err = state.SetIntent(state.Snapshot().Revision, "example.tool", false); err != nil {
		t.Fatal(err)
	}
	if _, err = state.FinishStop("example.tool"); err != nil {
		t.Fatal(err)
	}
	if _, err = registry.ApplyChange(ctx, change.ID, state, &approval); err != nil {
		t.Fatal(err)
	}
	disk.Committed.Values["example.tool"] = values
	disk.Committed.Revision++
	raw, err = json.Marshal(disk)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(configFile, raw, 0600); err != nil {
		t.Fatal(err)
	}
	return file, backup
}
