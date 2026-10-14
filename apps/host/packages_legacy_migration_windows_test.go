//go:build windows

package main

import (
	"aead.dev/minisign"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"winone/internal/configuration"
	"winone/internal/host"
	"winone/internal/modulepackage"
	"winone/internal/modules"
)

func TestUnmatchedLegacyMigrationIsVisibleWithoutPackageDirectory(t *testing.T) {
	p, store, _ := packagePreviewFixture(t)
	backup := configuration.PackageMigrationSnapshot{SchemaVersion: 1, OperationID: "orphan-backup", ModuleID: "example.tool", BeforeSchema: 1, AfterSchema: 2, BeforePackageSHA256: strings.Repeat("a", 64), AfterPackageSHA256: strings.Repeat("b", 64), Before: json.RawMessage(`{}`), After: json.RawMessage(`{"value":1}`)}
	digest := writeLegacyMigrationFixture(t, p.data, backup)
	if status := p.packageCleanupStatus(); !status.Pending {
		t.Fatal("orphaned backup is hidden", status)
	}
	if reply := p.cleanupPackages(host.Request{Method: "cleanupPackages"}, store); reply.Error == nil {
		t.Fatal("unverifiable backup reported as cleaned")
	}
	if _, err := store.LoadPackageMigration(backup.OperationID, digest); err != nil {
		t.Fatal("unverifiable backup changed", err)
	}
}

func writeLegacyMigrationFixture(t *testing.T, profile string, value configuration.PackageMigrationSnapshot) string {
	t.Helper()
	directory := filepath.Join(profile, "package-migrations")
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(directory, "migration-"+value.OperationID+".json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func TestLegacyMigrationBackupRequiresCurrentSettingsAndSignedCandidate(t *testing.T) {
	for _, changed := range []bool{false, true} {
		t.Run(map[bool]string{false: "matches", true: "changed-settings"}[changed], func(t *testing.T) {
			public, private, err := minisign.GenerateKey(rand.Reader)
			if err != nil {
				t.Fatal(err)
			}
			old, publisher := hostPackageArchiveFixtureSigned(t, []byte("old worker"), public, private, "1.0.0")
			next, _ := hostPackageArchiveFixtureDefined(t, []byte("new worker"), public, private, hostMigrationArchiveOptions("1.1.0"))
			p, store, _ := packagePreviewFixtureFromArchive(t, old, publisher)
			service := host.New(store)
			control := modules.NewControl(p.state, nil, service.FeatureCatalog)
			if err = p.bind(service, control, store); err != nil {
				t.Fatal(err)
			}
			preview := readyPackagePreview(t, p, store, old)
			raw, _ := json.Marshal(map[string]string{"id": preview.ID})
			if reply := p.installPreview(host.Request{PeerPID: 11, Params: raw}, store, service, control); reply.Error != nil {
				t.Fatal(reply.Error)
			}
			checked, err := modulepackage.VerifyArchive(context.Background(), next, publisher)
			if err != nil {
				t.Fatal(err)
			}
			stage, err := modulepackage.StageArchive(context.Background(), next, modulepackage.StageOptions{Directory: filepath.Join(p.data, "packages"), Publisher: publisher, Environment: p.environment, ExpectedManifestSHA256: checked.Manifest().Digest()})
			if err != nil {
				t.Fatal(err)
			}
			registry, err := modulepackage.OpenRegistry(filepath.Join(p.data, "packages"))
			if err != nil {
				t.Fatal(err)
			}
			current, err := registry.Snapshot()
			if err != nil {
				t.Fatal(err)
			}
			document := store.CommittedDocument()
			before := document.Values["example.tool"]
			if changed {
				before = json.RawMessage(`{"size":4}`)
			}
			definition, err := stage.LoadSettings(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			after, err := definition.Migrate(1, before)
			if err != nil {
				t.Fatal(err)
			}
			backup := configuration.PackageMigrationSnapshot{SchemaVersion: 1, OperationID: "old-preparation", ModuleID: "example.tool", Revision: document.Revision, BeforeSchema: 1, AfterSchema: 2, BeforePackageSHA256: current.Records["example.tool"].ManifestSHA256, AfterPackageSHA256: stage.Manifest().Digest(), Before: before, After: after}
			// Materialize a historical backup without its now-missing preparation
			// journal; changed-settings covers a record that no longer matches live data.
			digest := writeLegacyMigrationFixture(t, p.data, backup)
			if _, err = registry.RecoverPreparations(context.Background(), p.trust); err != nil {
				t.Fatal(err)
			}
			result := p.recoverLegacyMigrationReferences(registry, store)
			ledger, err := registry.Reclamation()
			if err != nil {
				t.Fatal(err)
			}
			stageID := filepath.Base(stage.Directory())
			for _, item := range ledger.Items {
				if item.Package.StageID != stageID {
					continue
				}
				if changed {
					if len(result.failures) == 0 || !result.blockedStages[stageID] || item.MigrationSHA256 != "" {
						t.Fatal("changed backup gained cleanup ownership", result, item)
					}
				} else if len(result.failures) != 0 || item.MigrationSHA256 != digest || item.ChangeID != backup.OperationID {
					t.Fatal("valid backup was not linked", result, item)
				}
			}
			if _, err = store.LoadPackageMigration(backup.OperationID, digest); err != nil {
				t.Fatal("inspection removed the backup", err)
			}
			if changed {
				if reply := p.cleanupPackages(host.Request{Method: "cleanupPackages"}, store); reply.Error == nil {
					t.Fatal("mismatched migration cleanup reported success")
				}
				if _, err = os.Stat(stage.Directory()); err != nil {
					t.Fatal("cleanup removed the migration's remaining stage", err)
				}
				if _, err = store.LoadPackageMigration(backup.OperationID, digest); err != nil {
					t.Fatal("cleanup removed mismatched backup", err)
				}
			}
		})
	}
}
